package ontology

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
)

// defaultEmbedder is the concrete implementation of the Embedder interface.
// It talks to Ollama when available and falls back to deterministic dummy vectors.
type defaultEmbedder struct {
	client *api.Client
	Model  string
}

// NewEmbedder creates an embedder satisfying the Embedder seam.
// If Ollama is not available, it falls back to deterministic dummy vectors.
func NewEmbedder(model string) Embedder {
	if model == "" {
		model = "nomic-embed-text"
	}
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return &defaultEmbedder{client: nil, Model: model}
	}
	return &defaultEmbedder{client: client, Model: model}
}

func (e *defaultEmbedder) Generate(text string) ([]float32, error) {
	if e.client == nil {
		return generateDummyVector(text), nil
	}

	ctx := context.Background()
	req := &api.EmbeddingRequest{
		Model:  e.Model,
		Prompt: text,
	}

	resp, err := e.client.Embeddings(ctx, req)
	if err != nil {
		return generateDummyVector(text), nil
	}

	vec := make([]float32, len(resp.Embedding))
	for i, v := range resp.Embedding {
		vec[i] = float32(v)
	}
	return vec, nil
}



// --- Public embedding model management (tied to Config) ---

type ListModel struct {
	Name     string
	Size     int64
	Modified string
}

func ListEmbeddingModels() ([]ListModel, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("ollama not available: %w (try: ollama serve)", err)
	}

	ctx := context.Background()
	resp, err := client.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	var models []ListModel
	for _, m := range resp.Models {
		models = append(models, ListModel{
			Name:     m.Name,
			Size:     m.Size,
			Modified: m.ModifiedAt.Format("2006-01-02"),
		})
	}
	return models, nil
}

func PullModel(name string) error {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return fmt.Errorf("ollama not running: %w", err)
	}

	req := &api.PullRequest{Name: name}
	return client.Pull(context.Background(), req, func(pr api.ProgressResponse) error {
		if pr.Status != "" {
			fmt.Printf("%s: %s\n", name, pr.Status)
		}
		return nil
	})
}

// BatchGenerateEmbeddings is exported for compatibility.
func BatchGenerateEmbeddings(limit int, modelOverride string) (int, error) {
	// During full consolidation this will use the configured Ontology.
	// For now, a minimal implementation to keep the thin layer working.
	return 0, nil
}

// BatchEmbed is the instance-bound version for use via GetOntology().
func (o *Ontology) BatchEmbed(limit int, modelOverride string) (int, error) {
	// Future: use o.embedder and o.storage to find missing and generate/store.
	// For now delegate to package impl (which will be updated to be instance-aware).
	if modelOverride == "" {
		modelOverride = o.cfg.EmbedModel
	}
	return BatchGenerateEmbeddings(limit, modelOverride)
}

// Model management (will be per-Graph via Config in full consolidation)
var currentModel = "nomic-embed-text"

func GetCurrentModel() string { return currentModel }

func SetCurrentModel(name string) error {
	currentModel = name
	return nil
}

// GenerateSummary is the canonical way to produce text for embedding an Entity.
func GenerateSummary(e Entity) string {
	title := ""
	if t, ok := e.Properties["title"].(string); ok {
		title = t
	} else if n, ok := e.Properties["name"].(string); ok {
		title = n
	} else if d, ok := e.Properties["description"].(string); ok {
		title = d
	}
	return fmt.Sprintf("%s %s: %s", e.Type, e.ID, title)
}

