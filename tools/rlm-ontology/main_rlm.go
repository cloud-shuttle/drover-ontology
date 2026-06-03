//go:build rlm

// Real rlm-go + Yaegi integration spike for drover-ontology.
// Build/run with: go run -tags rlm ./tools/rlm-ontology [target-dir]
// 
// This implements the full RLM process:
// - Kick off once.
// - Loop: call root LLM (with drover ontology builder prompt + growing history) 
//   -> model outputs Go code using the registered helpers
//   -> execute the code in Yaegi (mutates live drover Ontology via Drover* funcs)
//   -> feed results back to LLM
//   -> model can do recursive sub-LLM calls via the registered Query() helper inside executed code
//   -> continues until model emits FINAL_ONTOLOGY(...) or max iters.
// 
// All drover data (graph.jsonl + graph.db) and RLM artifacts go under ./.ontology/
// This keeps things organized (no scattered root files), while still writing *through* the drover
// governed store (JSONL is source of truth, no parallel Ladybug/Kuzu DB).
// 
// Requires ANTHROPIC_API_KEY for real LLM calls; otherwise uses a demo LLM
// that returns canned (but useful) responses exercising the drover helpers via Yaegi.
// The sub-Query calls also go to the LLM (demo or real).
//
// See docs/rlm-integration.md for the process explanation and why we target drover (not a side graph).

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/drover-org/drover-ontology/internal/ingest"
	"github.com/drover-org/drover-ontology/internal/ontology"

	"github.com/XiaoConstantine/rlm-go/pkg/parsing"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// captureWriter is a thread-safe io.Writer that duplicates writes to os.Stdout/os.Stderr
// and also buffers them so the RLM harness can feed execution outputs back into the LLM history.
type captureWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	target io.Writer
}

func (cw *captureWriter) Write(p []byte) (n int, err error) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	cw.buf.Write(p)
	return cw.target.Write(p)
}

func (cw *captureWriter) Reset() {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	cw.buf.Reset()
}

func (cw *captureWriter) String() string {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return cw.buf.String()
}

var (
	subCallCount          int32
	maxSubCalls           int32 = 10 // Maximum allowed sub-queries per session to prevent billing runaway
	totalPromptTokens     int64
	totalCompletionTokens int64
	estimatedCostUSD      float64
	costMutex             sync.Mutex
)

// --- Output organization: everything under the target's .ontology/ ---
var outputBase string

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureOutputDirs(targetDir string) (graphPath, dbPath, snapshotsDir, rlmRunDir string) {
	outputBase = filepath.Join(targetDir, ".ontology")
	if err := os.MkdirAll(outputBase, 0o755); err != nil {
		panic(err)
	}
	snapshotsDir = filepath.Join(outputBase, "snapshots")
	if err := os.MkdirAll(snapshotsDir, 0o755); err != nil {
		panic(err)
	}
	ts := time.Now().Format("20060102-150405")
	rlmRunDir = filepath.Join(outputBase, "rlm-runs", ts)
	if err := os.MkdirAll(rlmRunDir, 0o755); err != nil {
		panic(err)
	}
	graphPath = filepath.Join(outputBase, "graph.jsonl")
	dbPath = filepath.Join(outputBase, "graph.db")

	// Auto-propagate references and schema from current workspace directory if they exist
	cwd, err := os.Getwd()
	if err == nil {
		// 1. Copy references/ directory
		srcRefs := filepath.Join(cwd, "references")
		dstRefs := filepath.Join(outputBase, "references")
		if info, err := os.Stat(srcRefs); err == nil && info.IsDir() {
			_ = copyDir(srcRefs, dstRefs)
		}

		// 2. Copy schema.yaml
		srcSchema := filepath.Join(cwd, ".ontology", "schema.yaml")
		if _, err := os.Stat(srcSchema); err != nil {
			// fallback to references/drover-schema.yaml
			srcSchema = filepath.Join(cwd, "references", "drover-schema.yaml")
		}
		dstSchema := filepath.Join(outputBase, "schema.yaml")
		if _, err := os.Stat(srcSchema); err == nil {
			_ = copyFile(srcSchema, dstSchema)
		}
	}

	return
}

// --- Minimal OpenAI LLM caller ---

type openaiRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func callOpenAI(ctx context.Context, apiKey, model, system, user string) (string, error) {
	messages := []openaiMessage{}
	if system != "" {
		messages = append(messages, openaiMessage{Role: "system", Content: system})
	}
	messages = append(messages, openaiMessage{Role: "user", Content: user})

	if os.Getenv("BIFROST_GATEWAY_URL") != "" && !strings.Contains(model, "/") {
		model = "openai/" + model
	}

	reqBody := openaiRequest{
		Model:    model,
		Messages: messages,
	}
	jsonBody, _ := json.Marshal(reqBody)

	baseURL := os.Getenv("BIFROST_GATEWAY_URL")
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_API_BASE")
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	apiURL := baseURL + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer " + apiKey)

	startTime := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	durationMs := time.Since(startTime).Milliseconds()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var apiResp openaiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", err
	}
	if apiResp.Error != nil {
		return "", fmt.Errorf("openai: %s", apiResp.Error.Message)
	}

	// Accumulate token usage and estimated cost
	accumulateCost(model, apiResp.Usage.PromptTokens, apiResp.Usage.CompletionTokens)

	if len(apiResp.Choices) > 0 {
		content := apiResp.Choices[0].Message.Content
		prompt := system + "\n" + user
		go logToLangfuse(model, prompt, content, apiResp.Usage.PromptTokens, apiResp.Usage.CompletionTokens, durationMs)
		return content, nil
	}
	return "", fmt.Errorf("no choice in response")
}

// --- Minimal Anthropic LLM caller (real calls when key present) ---

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func callAnthropic(ctx context.Context, apiKey, model, system, user string) (string, error) {
	if os.Getenv("BIFROST_GATEWAY_URL") != "" && !strings.Contains(model, "/") {
		model = "anthropic/" + model
	}

	reqBody := anthropicRequest{
		Model:     model,
		MaxTokens: 4096,
		Messages:  []anthropicMessage{{Role: "user", Content: user}},
		System:    system,
	}
	jsonBody, _ := json.Marshal(reqBody)

	baseURL := os.Getenv("BIFROST_GATEWAY_URL")
	if baseURL == "" {
		baseURL = os.Getenv("ANTHROPIC_API_BASE")
	}
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	apiURL := baseURL + "/messages"

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	startTime := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	durationMs := time.Since(startTime).Milliseconds()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(body))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", err
	}
	if apiResp.Error != nil {
		return "", fmt.Errorf("anthropic: %s", apiResp.Error.Message)
	}

	// Accumulate token usage and estimated cost
	accumulateCost(model, apiResp.Usage.InputTokens, apiResp.Usage.OutputTokens)

	for _, b := range apiResp.Content {
		if b.Type == "text" {
			prompt := system + "\n" + user
			go logToLangfuse(model, prompt, b.Text, apiResp.Usage.InputTokens, apiResp.Usage.OutputTokens, durationMs)
			return b.Text, nil
		}
	}
	return "", fmt.Errorf("no text in response")
}

// demoLLM returns canned but realistic responses that use our registered helpers
// so the spike always demonstrates the Yaegi + drover integration even without keys.
// It progresses with iter to simulate multi-turn RLM behavior.
func demoLLM(iter int, query string) string {
	switch iter {
	case 0:
		return `I will start by exploring the codebase using the available helpers.
` + "```go" + `
files := ListFiles()
fmt.Println("visible files:", len(files))
parsed := ParseGoFile("internal/ontology/reducer.go")
fmt.Println("parsed reducer for OperationReducer concepts")
DroverCreate("Term", "Term:rlm-integration", map[string]any{
  "label": "RLM integration spike", 
  "status": "proposed", 
  "definition": "Using rlm-go+Yaegi to build ontology via REPL code execution against drover store",
  "domain": "asdlc",
})
` + "```"
	case 1:
		return `Good progress. Now wire governance and extract more structure.
` + "```go" + `
DroverRelate("Term:rlm-integration", "governed_by", "Person:demo-curator", map[string]any{"role": "curator"})
DroverUpdate("Term:rlm-integration", map[string]any{"status": "accepted"})
pkg := "Package:tools/rlm-ontology"
DroverCreate("Package", pkg, map[string]any{"path": "tools/rlm-ontology", "summary": "RLM harness code"})
DroverRelate(pkg, "part_of", "Module:drover-ontology", nil)
sub := Query("Summarize the purpose of the OperationReducer in reducer.go")
fmt.Println("Sub LLM summary:", sub)
` + "```"
	default:
		return `Now validate the work and finish the ontology build.
` + "```go" + `
DroverFsck()
DroverSnapshot(".ontology/snapshots/rlm-real-spike.jsonl")
` + "```" + `
FINAL("Successfully built demo ontology via real rlm-go/Yaegi harness + drover helpers. See .ontology/ for graph.jsonl + artifacts. Governance followed, multiple entities and relations created.")`
	}
}

// callLLM is the root LLM used by the harness loop.
func callLLM(ctx context.Context, system, user string) (string, error) {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		model := "claude-haiku-4-5"
		if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
			model = m
		}
		return callAnthropic(ctx, key, model, system, user)
	}
	// demo mode - the "LLM" returns code that exercises the ontology builder
	// In a real run the model would generate this dynamically from the prompt + history.
	return demoLLM(0, user), nil
}

// callSubLLM is used for the registered Query helper inside REPL code (recursive sub-calls).
func callSubLLM(prompt string) string {
	count := atomic.AddInt32(&subCallCount, 1)
	if count > maxSubCalls {
		fmt.Printf("⚠️ Safety Guard: Total sub-calls exceeded safety limit (%d/%d). Halting sub-LLM execution.\n", count, maxSubCalls)
		return "Safety Limit Reached: Recursive sub-call aborted to prevent API token exhaustion."
	}

	// For sub-calls we use a lighter system prompt focused on analysis.
	subSystem := "You are a helpful code analysis assistant. Answer concisely."
	openaiKey := os.Getenv("OPENAI_API_KEY")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")

	subCtx, subCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer subCancel()

	if openaiKey != "" {
		model := "gpt-4o-mini"
		if m := os.Getenv("OPENAI_SUB_MODEL"); m != "" {
			model = m
		} else if m := os.Getenv("LLM_SUB_MODEL"); m != "" {
			model = m
		} else if m := os.Getenv("OPENAI_MODEL"); m != "" {
			model = m
		}
		resp, err := callOpenAI(subCtx, openaiKey, model, subSystem, prompt)
		if err != nil {
			fmt.Printf("⚠️ Sub-LLM API call failed (%v). Falling back to demo mode gracefully...\n", err)
			return "Demo response: The OperationReducer in reducer.go processes structural events sequentially to construct the ontology."
		}
		return resp
	} else if anthropicKey != "" {
		model := "claude-3-5-haiku-latest" // Default to cheap, fast Haiku for sub-queries
		if m := os.Getenv("ANTHROPIC_SUB_MODEL"); m != "" {
			model = m
		} else if m := os.Getenv("LLM_SUB_MODEL"); m != "" {
			model = m
		} else if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
			model = m
		}
		resp, err := callAnthropic(subCtx, anthropicKey, model, subSystem, prompt)
		if err != nil {
			fmt.Printf("⚠️ Sub-LLM API call failed (%v). Falling back to demo mode gracefully...\n", err)
			return "Demo response: The OperationReducer in reducer.go processes structural events sequentially to construct the ontology."
		}
		return resp
	}
	return "Sub-LLM demo response for: " + prompt[:min(50, len(prompt))]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	deltaFlag := flag.Bool("delta", false, "Use Git-driven incremental ingestion (Delta Mode)")
	flag.Parse()

	target := "."
	args := flag.Args()
	if len(args) > 0 {
		target = args[0]
	}
	absTarget, _ := filepath.Abs(target)
	fmt.Printf("Real RLM+Yaegi drover ontology spike on: %s\n", absTarget)
	fmt.Println("Build tag 'rlm' active. Using rlm-go for harness + Yaegi for REPL.")
	if *deltaFlag {
		fmt.Println("Incremental Delta Mode active. Ingesting git status diff only.")
	}
	fmt.Printf("All outputs (drover graph + RLM artifacts) will be under: %s/\n", filepath.Join(absTarget, ".ontology"))

	graphPath, dbPath, snapshotsDir, rlmRunDir := ensureOutputDirs(absTarget)
	_ = snapshotsDir // used in demo responses

	// 1. Load context with our strong drover-aware loader
	loader := &ingest.CodebaseLoader{
		RootPath:     absTarget,
		MaxFileBytes: 512 * 1024,
		UseGitDelta:  *deltaFlag,
	}
	contextStr, err := loader.LoadGoCodebase()
	if err != nil {
		fmt.Printf("loader error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded context: ~%d bytes\n", len(contextStr))

	analysis := &ingest.CodeAnalysis{Context: contextStr}

	// 2. Setup a governed drover store under ./.ontology/ (clean, organized, still uses real drover JSONL+SQLite)
	cfg := ontology.Config{
		GraphPath:  graphPath,
		DBPath:     dbPath,
		SchemaPath: "", // use DefaultSchema() - always works and includes the governed_by / Plan review policies etc.
		UseSQLite:  true,
	}
	ont, err := ontology.New(cfg)
	if err != nil {
		fmt.Printf("note: could not open .ontology ontology (%v), falling back to exec mode using binary\n", err)
	}
	droverC := ingest.NewDroverClientDirect(ont)
	if ont == nil {
		droverC = ingest.NewDroverClientExec("ontology", graphPath, dbPath, filepath.Join(outputBase, "schema.yaml"))
		droverC.ForceExec = true
	}
	
	// Seed designated curators if they do not exist
	if droverC != nil {
		_ = droverC.Create("Person", "Person:peter-hanssens", map[string]any{
			"name": "Peter Hanssens",
			"role": "Taxonomy Curator",
		})
		_ = droverC.Create("Term", "Term:taxonomy-curator", map[string]any{
			"label":      "Taxonomy Curator",
			"definition": "Designated role responsible for reviewing and accepting domain Terms",
			"status":     "accepted",
		})
		_ = droverC.Relate("Person:peter-hanssens", "tagged_with", "Term:taxonomy-curator", nil)
		_ = droverC.Relate("Term:taxonomy-curator", "governed_by", "Person:peter-hanssens", nil)
	}
	defer func() {
		if ont != nil {
			ont.Close()
		}
	}()

	// 3. Yaegi interpreter with our custom drover + analysis symbols injected
	// This allows the LLM-generated Go code (from the REPL) to directly call real Go funcs
	// that mutate the live drover-governed store.
	stdoutWriter := &captureWriter{target: os.Stdout}
	stderrWriter := &captureWriter{target: os.Stderr}

	i := interp.New(interp.Options{
		Stdout: stdoutWriter,
		Stderr: stderrWriter,
	})
	if err := i.Use(restrictedSymbols()); err != nil {
		panic(err)
	}

	droverSyms := interp.Exports{
		"drover/drover": {
			"DroverCreate": reflect.ValueOf(func(typ, id string, props map[string]any) (string, error) {
				err := droverC.Create(typ, id, props)
				return id, err
			}),
			"DroverRelate":   reflect.ValueOf(droverC.Relate),
			"DroverUpdate":   reflect.ValueOf(droverC.Update),
			"DroverDelete":   reflect.ValueOf(droverC.SoftDelete),
			"DroverValidate": reflect.ValueOf(func() ([]string, error) { e, _ := droverC.Validate(); return e, nil }),
			"DroverFsck":     reflect.ValueOf(droverC.IntegrityCheck),
			"DroverSnapshot": reflect.ValueOf(func(dest string) error {
				// Make relative to our output base if not absolute
				if !filepath.IsAbs(dest) {
					dest = filepath.Join(outputBase, dest)
				}
				return droverC.ExportSnapshot(dest)
			}),
			"ListFiles":                  reflect.ValueOf(analysis.ListFiles),
			"GetFileContent":              reflect.ValueOf(analysis.GetFileContent),
			"ParseGoFile":                 reflect.ValueOf(analysis.ParseGoFile),
			"ParseFileSymbols":            reflect.ValueOf(analysis.ParseFileSymbols),
			"FindReferences":              reflect.ValueOf(analysis.FindReferences),
			"LoadBeadsJSON":              reflect.ValueOf(analysis.LoadBeadsJSON),
			"RecommendedDroverRelations": reflect.ValueOf(analysis.RecommendedDroverRelations),
			"ListDroverEntityTypes":      reflect.ValueOf(analysis.ListDroverEntityTypes),
			// Register Query so the model can do recursive sub-LLM calls *from inside* executed REPL code.
			"Query": reflect.ValueOf(callSubLLM),
		},
	}
	if err := i.Use(droverSyms); err != nil {
		panic(fmt.Errorf("failed to use drover symbols: %w", err))
	}

	// preload context + dot-import the helpers so model code can call DroverCreate etc directly
	safeCtx := strings.ReplaceAll(contextStr, "`", "'")
	setup := fmt.Sprintf(`
package main
import "fmt"
import "strings"
import "time"
import "strconv"
import . "drover/drover"

var context = `+"`"+`%s`+"`"+`
`, safeCtx)

	if _, err := i.Eval(setup); err != nil {
		i.Eval(`package main; var context = "loaded (fallback)"`)
		fmt.Printf("note: context preload warning: %v\n", err)
	} else {
		i.Eval(fmt.Sprintf(`package main; fmt.Println("✅ Yaegi REPL initialized with drover helpers + context (all writes go through drover store under %s)")`, strings.ReplaceAll(outputBase, "\\", "/")))
	}

	// 4. The harness loop (minimal RLM-style, using rlm-go parsing for FINAL/codeblocks)
	system := loadSystemPrompt()
	query := "Analyze this entire codebase and build a useful governed drover ontology. Use ListFiles/GetFileContent/ParseGoFile/FindReferences/Query (for sub-tasks) to explore. Use the Drover* helpers to create Terms, Packages, relations etc following the drover schema and governance rules (governed_by etc). Iterate, validate with DroverFsck, snapshot to .ontology/snapshots/, and terminate with FINAL_ONTOLOGY(...)."

	fmt.Println("Starting real RLM loop (LLM decides + writes Go code executed in Yaegi with our symbols)...")
	fmt.Printf("Outputs will be in %s/ (graph.jsonl + graph.db + rlm-runs/ + snapshots/)\n", outputBase)

	history := []string{system + "\n\n" + query}
	var finalAnswer string
	var iters int

	for it := 0; it < 12; it++ {  // safety max, real rlm-go uses config
		iters = it + 1
		prompt := strings.Join(history, "\n\n---\n\n")
		if len(prompt) > 120000 {
			prompt = prompt[len(prompt)-120000:]
		}

		var resp string
		var err error
		openaiKey := os.Getenv("OPENAI_API_KEY")
		anthropicKey := os.Getenv("ANTHROPIC_API_KEY")

		timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer timeoutCancel()

		if openaiKey != "" {
			model := "gpt-4o"
			if m := os.Getenv("OPENAI_MODEL"); m != "" {
				model = m
			} else if m := os.Getenv("LLM_MODEL"); m != "" {
				model = m
			}
			resp, err = callOpenAI(timeoutCtx, openaiKey, model, system, prompt)
		} else if anthropicKey != "" && !strings.HasPrefix(anthropicKey, "demo-") {
			model := "claude-3-5-sonnet-latest"
			if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
				model = m
			} else if m := os.Getenv("LLM_MODEL"); m != "" {
				model = m
			}
			resp, err = callAnthropic(timeoutCtx, anthropicKey, model, system, prompt)
		} else {
			resp = demoLLM(it, query)
		}
		if err != nil {
			fmt.Printf("LLM error: %v\n", err)
			break
		}

		fmt.Printf("\n[LLM iter %d response snippet]\n%s\n\n", it, truncate(resp, 300))

		// Execute any code the model wrote (this is where the ontology gets built)
		stdoutWriter.Reset()
		stderrWriter.Reset()
		codes := parsing.FindCodeBlocks(preprocessResponse(resp))
		for _, code := range codes {
			fmt.Printf("[Yaegi EXEC]\n%s\n", code)
			evalWithRecover := func() (err error) {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("Yaegi interpreter panic: %v", r)
					}
				}()
				_, err = i.Eval(code)
				return err
			}
			if execErr := evalWithRecover(); execErr != nil {
				fmt.Printf("[Yaegi ERR] %v\n", execErr)
				fmt.Fprintf(stderrWriter, "Yaegi compilation/runtime/panic error: %v\n", execErr)
			}
		}

		// Check for termination
		if fa := parsing.FindFinalAnswer(resp); fa != nil {
			finalAnswer = fa.Content
			fmt.Printf("\n🎉 FINAL from LLM: %s\n", finalAnswer)
			break
		}

		// Feed back for next iteration (results of exec + the response)
		history = append(history, resp)
		
		output := stdoutWriter.String()
		errOutput := stderrWriter.String()
		if len(output) > 0 || len(errOutput) > 0 {
			var feedback strings.Builder
			feedback.WriteString("Execution feedback from the Go REPL:\n")
			if len(output) > 0 {
				feedback.WriteString(fmt.Sprintf("=== STDOUT ===\n%s\n", output))
			}
			if len(errOutput) > 0 {
				feedback.WriteString(fmt.Sprintf("=== STDERR ===\n%s\n", errOutput))
			}
			history = append(history, feedback.String())
		}
	}

	// 5. Save RLM-specific artifacts under ./.ontology/rlm-runs/<ts>/
	rlmLogPath := filepath.Join(rlmRunDir, "rlm-session.log")
	_ = os.WriteFile(rlmLogPath, []byte(strings.Join(history, "\n\n========== ITERATION END ==========\n\n")), 0644)

	meta := map[string]any{
		"timestamp":               time.Now().Format(time.RFC3339),
		"target":                  absTarget,
		"iterations":              iters,
		"final":                   finalAnswer,
		"output_dir":              outputBase,
		"graph":                   graphPath,
		"total_prompt_tokens":     atomic.LoadInt64(&totalPromptTokens),
		"total_completion_tokens": atomic.LoadInt64(&totalCompletionTokens),
		"estimated_cost_usd":      estimatedCostUSD,
		"note":                    "All drover mutations went through the governed store (JSONL source of truth). RLM used Yaegi for code execution + sub-Query for recursion.",
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(filepath.Join(rlmRunDir, "metadata.json"), metaBytes, 0644)

	fmt.Println("\nReal rlm-go/Yaegi spike finished.")
	fmt.Printf("Durable drover store: %s (graph.jsonl + graph.db)\n", outputBase)
	fmt.Printf("RLM run artifacts: %s/\n", rlmRunDir)
	fmt.Println("  • rlm-session.log (full LLM history + code)")
	fmt.Println("  • metadata.json")
	fmt.Printf("  • Snapshots (if model called DroverSnapshot): %s/\n", snapshotsDir)

	fmt.Println("\n=== Token Ingestion & Cost Safety Net ===")
	fmt.Printf("  • Total Prompt Tokens:     %d\n", atomic.LoadInt64(&totalPromptTokens))
	fmt.Printf("  • Total Completion Tokens: %d\n", atomic.LoadInt64(&totalCompletionTokens))
	costMutex.Lock()
	fmt.Printf("  • Estimated Session Cost:  $%.5f USD\n", estimatedCostUSD)
	costMutex.Unlock()
	fmt.Println("=========================================")
}

func loadSystemPrompt() string {
	// Prefer the canonical one we maintain (the battle-tested drover ontology builder prompt)
	if data, err := os.ReadFile("docs/rlm-system-prompt.md"); err == nil {
		return string(data)
	}
	return `You are an expert software architect, domain-driven design practitioner, and enterprise ontologist for the drover project.

Available in the Go REPL (after the setup import . "drover/drover"):

⚠️ CRITICAL REPL RULES:
1. Do NOT write "package main", "import (...)", or wrap your code in "func main()". The Go REPL executes raw statements immediately line-by-line (e.g. files := ListFiles(), DroverCreate(...)). Writing a "package main" program compiles in Yaegi but NEVER executes, resulting in a completely failed run with zero side effects. Always write bare Go statements directly.
2. You MUST use "fmt.Println(...)" or "println(...)" to print variables you want to inspect. Simply writing the variable name or expression on a line by itself (e.g. files or content) evaluates but prints nothing, leaving you completely blind. Always do fmt.Println(files) or println(f).

⚠️ CRITICAL DOMAIN MODELING RULE: Do NOT write Go code to define, implement, or seed the drover store/engine itself (e.g. "type Entity struct", "type Store struct", "package drover"). The Go REPL is already fully initialized with a live drover store and pre-imported helpers like DroverCreate and DroverRelate. Your task is to analyze the target codebase in 'context' (which may be Go, TypeScript, Python, etc.) and model its actual domain concepts (e.g., for a banners project, model entities like 'Speaker', 'BannerLayout', 'MeetupEvent'; for a Go backend, model 'Package', 'Struct'). Create these directly via Drover* helpers.

- ListFiles() []string
- GetFileContent(relPath string) string
- ParseGoFile(relPath string) string
- ParseFileSymbols(relPath string) (string, error) // language-agnostic class/func/key line-span parser (JS, TS, Python, Go, YAML)
- FindReferences(symbol string) string
- Query(prompt string) string   // recursive sub-LLM call from inside code
- DroverCreate(typ, id string, props map[string]any) (id string, error)
- DroverRelate(from, rel, to string, props map[string]any) error
- DroverUpdate(id string, set map[string]any) error
- DroverDelete(id, reason string) error
- DroverValidate() ([]string, error)
- DroverFsck() error
- DroverSnapshot(destPath string) error

Build a high-quality ontology iteratively from the 'context' variable.
Always satisfy required schema properties (e.g. Term requires "label", "definition", and "status"; Plan requires "kind", "title", and "status"; Review requires "kind", "decision", and "reviewer").
Always capture code provenance metadata in your entity property maps when modeling code: "src_path" (relative file path e.g. "src/brand.js"), "line_start" (integer line start), and "line_end" (integer line end).
Follow the recommended entity types and relationships from the drover schemas.
Use Query() for hard sub-problems (this enables true recursion).
Finally call DroverFsck() and DroverSnapshot("snapshots/...").
⚠️ CRITICAL: At the very end of your response, OUTSIDE of any markdown Go code blocks, output your final summary as a plain text block in exactly this format (do NOT write it as a Go function call, it will fail to compile):
FINAL_ONTOLOGY("summary of built entities, relationships, and key insights")
Be precise and obey governance (e.g. accepted Terms need governed_by).`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func accumulateCost(model string, prompt, completion int) {
	// Standard model pricing per 1,000,000 tokens
	var inputRate, outputRate float64
	model = strings.ToLower(model)

	if strings.Contains(model, "gpt-4o-mini") {
		inputRate = 0.15
		outputRate = 0.60
	} else if strings.Contains(model, "gpt-4o") {
		inputRate = 5.00
		outputRate = 15.00
	} else if strings.Contains(model, "claude-3-5-sonnet") {
		inputRate = 3.00
		outputRate = 15.00
	} else if strings.Contains(model, "claude-3-5-haiku") || strings.Contains(model, "claude-haiku") {
		inputRate = 0.25
		outputRate = 1.25
	} else {
		// Conservative fallback pricing (GPT-4 rate)
		inputRate = 5.00
		outputRate = 15.00
	}

	cost := (float64(prompt) * inputRate / 1e6) + (float64(completion) * outputRate / 1e6)

	atomic.AddInt64(&totalPromptTokens, int64(prompt))
	atomic.AddInt64(&totalCompletionTokens, int64(completion))

	costMutex.Lock()
	estimatedCostUSD += cost
	costMutex.Unlock()
}

func restrictedSymbols() interp.Exports {
	exports := make(interp.Exports)
	for pkg, symbols := range stdlib.Symbols {
		// Filter out unsafe packages from developer environment to ensure sandboxed safety
		if pkg == "os" || pkg == "os/exec" || pkg == "syscall" || pkg == "unsafe" ||
			strings.HasPrefix(pkg, "net") {
			continue
		}
		exports[pkg] = symbols
	}
	return exports
}

// preprocessResponse strips leading indentation from markdown code block markers
// so that the rlm-go parsing.FindCodeBlocks library successfully extracts them
// even when the model writes them inside nested markdown lists.
func preprocessResponse(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```go") || strings.HasPrefix(trimmed, "```") {
			lines[i] = trimmed
		}
	}
	return strings.Join(lines, "\n")
}

func logToLangfuse(model, prompt, completion string, promptTokens, completionTokens int, durationMs int64) {
	host := os.Getenv("LANGFUSE_HOST")
	if host == "" {
		host = "http://localhost:4000"
	}
	host = strings.TrimSuffix(host, "/")

	publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	if publicKey == "" {
		publicKey = os.Getenv("NEXT_PUBLIC_LANGFUSE_PUBLIC_KEY")
	}
	if publicKey == "" {
		publicKey = "pk-lf-a163c801-f761-430b-ab9e-dfd85fd838a4"
	}
	secretKey := os.Getenv("LANGFUSE_SECRET_KEY")
	if secretKey == "" {
		secretKey = "sk-lf-539105a8-2bb0-46dd-8236-5db9abb2de55"
	}

	traceID := fmt.Sprintf("trace-%d", time.Now().UnixNano())
	generationID := fmt.Sprintf("gen-%d", time.Now().UnixNano())
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	startTime := time.Now().Add(-time.Duration(durationMs) * time.Millisecond).UTC().Format(time.RFC3339Nano)

	payload := map[string]interface{}{
		"batch": []map[string]interface{}{
			{
				"id":        fmt.Sprintf("evt-trace-%d", time.Now().UnixNano()),
				"timestamp": nowStr,
				"type":      "trace-create",
				"body": map[string]interface{}{
					"id":        traceID,
					"name":      "Drover RLM Run",
					"userId":    "peter-hanssens",
					"projectId": "cmpwm82du0006cp2jbiyopnib",
					"metadata": map[string]interface{}{
						"environment": "local-sandbox",
						"ingestion":   "rlm-go-direct",
					},
				},
			},
			{
				"id":        fmt.Sprintf("evt-gen-%d", time.Now().UnixNano()),
				"timestamp": nowStr,
				"type":      "generation-create",
				"body": map[string]interface{}{
					"id":        generationID,
					"traceId":   traceID,
					"name":      "LLM Completion Call",
					"startTime": startTime,
					"endTime":   nowStr,
					"model":     model,
					"input":     prompt,
					"output":    completion,
					"usage": map[string]interface{}{
						"input":  promptTokens,
						"output": completionTokens,
						"total":  promptTokens + completionTokens,
					},
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", host+"/api/public/ingestion", bytes.NewReader(jsonBytes))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(publicKey, secretKey)
	req.Header.Set("x-langfuse-ingestion-version", "4")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
	}
}
