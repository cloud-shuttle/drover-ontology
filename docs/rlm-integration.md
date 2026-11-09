# RLM + drover-ontology Integration (Recursive Long-Context Ontology Building)

**Status**: Design + starter implementation complete (2026-06, bea_ont_010). This is the canonical way to use Recursive Language Models (Alex Zhang / MIT CSAIL, arXiv 2512.24601 and subsequent 2026 material) to populate the governed drover store from entire codebases, company wikis, Notion exports, Slack histories, etc. See the dedicated "What to Build" and "How to Build / Extend It" sections below for precise scope and implementation approach.

## Why This Fits drover-ontology Perfectly

RLMs solve the "context rot" problem for 1M–10M+ token corpora by giving the model a **Go (or Python) REPL** instead of stuffing everything into one prompt. The huge context lives as a normal variable (`context`). The model writes and executes code to chunk, parse, summarize, recurse via sub-LLM calls, and — crucially — build external state.

drover-ontology already provides exactly the hardened "governed runtime" that a raw REPL + side graph (KektorDB, Neo4j, etc.) would lack:

- Single append-only source of truth (JSONL)
- Schema + policy enforcement (governed_by for Terms, approved Plan requires Review, acyclicity, Term governance)
- High-level API on the owner `Ontology` module (CreateEntity, Relate, Update, SoftDelete, Validate, IntegrityCheck) — see Candidate 04 from the 2026-06 architecture review
- Full replay fidelity (OperationReducer in reducer.go + Storage.Load contract B + always-replay fallback)
- `fsck` / `IntegrityCheck` that runs the policies on the authoritative replayed state
- Temporal validity + tombstoning
- "All agents write through the Go binary" rule (CRITICAL — see SKILL.md)
- PBT regression guards and E2E tests

**Do not** introduce a second *ontology mutation authority* or parallel ontology graph store inside this repo (the research-thread examples that did so were written before we deleted the dead parallel `internal/ontology/graph.go` (~780 LOC) and consolidated mutation semantics). RLM is an **ingestion / builder layer** that targets the existing governed JSONL/reducer store.

**Scope clarification (rationalization 2026-11-09):** [LadybugDB](https://ladybugdb.com) on Unikraft Cloud Sydney is **Brain's** approved *graph projection / retrieval* backend (replacing Brain's legacy Apache AGE adapter). It is **not** an uncontrolled parallel ontology and must not become a second place to mutate Terms/Relations. Ontology remains the governed schema/reducer/JSONL mutation authority; Brain's LadybugDB graph is a retrieval projection over company Git RAG. Escalate any genuinely conflicting ownership decision rather than global text replacement.

## High-Level Process (Exactly What Happens When You Point It at a Codebase)

1. **You invoke the RLM harness** (a small Go program using rlm-go + Yaegi, or a manual recursive controller for starters) pointing at a root directory (e.g. `drover-org/` or any company's monorepo + docs export).

2. **Strong drover-aware loader** (`internal/ingest/loader.go`) walks the tree:
   - Skips `.git`, `vendor`, `node_modules`, `bin/`, `testdata/`, fuzz artifacts, `graph.db` (the SQLite projection), build dirs, etc.
   - Prioritizes `go.mod`, READMEs, all `*.go`, `*.md`, `beads.json` + `beads.jsonl`, `drover-schema.yaml`, `CONTEXT.md`, `SKILL.md`, `references/`, taxonomy files, the `graph.jsonl` itself (for history), etc.
   - Emits a single giant string with clear `=== FILE: relative/path ===` headers.
   - This string becomes the `context` variable in the REPL. It can be many megabytes; the model never receives it in the prompt.

3. **The RLM receives a short system prompt** (see `docs/rlm-system-prompt.md`) + the list of available REPL symbols:
   - Analysis: `ListFiles`, `GetFileContent`, `ParseGoFile` (real `go/ast`), `FindReferences`, drover-specific sugar (`LoadBeadsJSON`, `RecommendedDroverRelations`, ...).
   - Governed writes: `DroverCreate`, `DroverRelate`, `DroverUpdate`, `DroverDelete`, `DroverValidate`, `DroverFsck`, `DroverSnapshot`, `DroverStats`.

4. **Recursive exploration inside the REPL** (this is the "magic"):
   The model writes Go code that the Yaegi interpreter executes. Typical trajectory for a drover-org run:

   ```go
   // 1. Recon
   files := ListFiles()
   fmt.Println("Total files:", len(files))
   mod := GetFileContent("go.mod")
   beads, _ := LoadBeadsJSON()

   // 2. High-signal packages first
   for _, f := range []string{"internal/ontology/ontology.go", "internal/ontology/reducer.go", ...} {
       parsed := ParseGoFile(f)
       // extract Package + key Structs/Interfaces
       id := "Package:" + f
       DroverCreate("Package", id, map[string]any{"path": f, "summary": "..."})
   }

   // 3. Taxonomy / Term extraction (recursive sub-task style)
   terms := extractTermsFromCONTEXTAndGlossaries(context) // model can write helpers or call sub-LLM
   for _, t := range terms {
       DroverCreate("Term", t.ID, map[string]any{"label": t.Label, "status": "proposed", ...})
       if t.Parent != "" {
           DroverRelate(t.ID, "broader_term", t.Parent, nil)
       }
   }

   // 4. Governance wiring (critical drover policy)
   DroverRelate("Term:drover-core/governance", "governed_by", "Person:peter", nil)
   DroverUpdate("Term:drover-core/governance", map[string]any{"status": "accepted"})

   // 5. Architecture → delivery linkage
   DroverRelate("Bead:bea_ont_010", "implements", "Plan:rlm-ingestion", nil)
   DroverRelate("Review:some-review", "approves", "Plan:...", nil)

   // 6. Validation loops (the model iterates until clean)
   errs, _ := DroverValidate()
   if len(errs) > 0 { fix... }

   DroverFsck()
   DroverSnapshot("memory/ontology/snapshots/rlm-2026-06-03.jsonl")
   ```

   The model can:
   - Make recursive sub-calls to the LLM for hard sub-problems ("summarize this package's domain concepts").
   - Write temporary analysis code in the REPL.
   - Query the live drover graph (future: expose a `DroverQuery` helper).
   - Self-correct using `Validate` / `Fsck` feedback.

5. **Termination**
   The model outputs `FINAL_ONTOLOGY("...")` (or the harness detects a sentinel). The harness then does a final `DroverFsck()`, human review gate if configured, and promotes the snapshot.

6. **Post-run**
   - Run `ontology fsck` (or the wrapper) against the central store.
   - The new entities/relations are now queryable via `search`, `query`, `related`, HybridSearch, etc.
   - Future nightly delta runs can diff against the previous snapshot and only apply changes.

Realistic timelines (medium-large Go monorepo + docs):
- Small repo: a few minutes
- Full drover-org scale: 10–40+ minutes depending on model speed, iteration depth, and whether sub-LLM recursion is used.

## Integration Architecture (Post 01–05 Refactors)

```
RLM Harness (rlm-go + Yaegi or manual controller)
    │
    ├── context (string from CodebaseLoader)
    ├── Analysis helpers (internal/ingest/code_analysis.go)
    │     ListFiles / GetFileContent / ParseGoFile (go/ast) / FindReferences ...
    │
    └── DroverClient (internal/ingest/drover_client.go)
          ├── Direct mode (trusted): calls Ontology.CreateEntity / Relate / ...
          │     (uses the high-level API from Candidate 04)
          └── Exec mode (untrusted REPL / prod): exec "drover-ont" or "ontology" + flags
                (obeys CRITICAL RULE in SKILL.md)
                      │
                      ▼
          ontology binary / Ontology module
                │
                ├── Apply → JSONL append (durable truth)
                ├── applyToMemory → OperationReducer (reducer.go — single canonical fold)
                ├── Validate / IntegrityCheck (full schema + policies on replayed state)
                └── Hybrid storage (JSONL + best-effort SQLite)
```

Key invariants preserved:
- No parallel graph ever created by the RLM.
- Every write is an `Operation` that goes through the reducer (Candidate 01).
- Load always has fidelity (Candidate 05, Contract B).
- Schema seam is wired (Candidate 03); policies fire on `Validate` / `fsck`.
- Commands and external agents remain thin facades over the same high-level methods.

## Starter Code in This Repo

- `internal/ingest/loader.go` — drover-monorepo-aware `CodebaseLoader`
- `internal/ingest/code_analysis.go` — `CodeAnalysis` with `ParseGoFile` etc.
- `internal/ingest/drover_client.go` — `DroverClient` (direct + exec) + `AsMap` for Yaegi injection
- `docs/rlm-system-prompt.md` — the full battle-tested prompt with drover vocabulary and governance rules
- `tools/rlm-ontology/main.go` (emulation by default) + `main_rlm.go` (real with `-tags rlm`) — runnable spikes. The real one wires actual rlm-go (Yaegi under the covers for REPL), injects our Drover* and analysis helpers as real Go values into the interp, runs a minimal LLM orchestration loop (real calls to Anthropic when key present), and uses the drover system prompt + context loading. The rlm-go/yaegi dep is present for `-tags rlm` builds only.

Optional future:
- Add `github.com/XiaoConstantine/rlm-go` as a dependency behind a build tag or separate module when you have evaluated its sandbox story.
- A real `cmd/rlm-ontology` binary that wraps the harness + LLM client (Anthropic Claude 4 / Opus 4.8 series recommended for structured reasoning).

## Running the Spike (Manual "RLM-Style" Demo)

```bash
# Build the ontology binary first (so exec mode works)
make build

# Run the demonstration ingester against this very repo (or any Go tree)
go run ./tools/rlm-ontology ./ 

# It will:
# - Load the codebase with the strong loader
# - Exercise the analysis helpers
# - Call Drover* helpers (in-process by default for the spike)
# - Run Validate + Fsck at the end
# - Print a FINAL-style summary
```

Inside the spike you can flip `ForceExec: true` on the client (after setting `DROVER_ONT_BINARY`) to prove writes are going through the real binary.

## Production Recommendations (2026-06)

- Sandbox: Run Yaegi (or any interpreter executing model code) inside a Docker/Podman container with no network and a read-only view of the source tree. The only writable thing the REPL should see is a capability-restricted DroverClient that can only call the approved helpers.
- Human gate: Large RLM runs should produce a proposed snapshot + diff report. A curator reviews before the snapshot is merged into the live central `memory/ontology/graph.jsonl`.
- Delta mode: Keep the previous snapshot; on the next run feed "only new/changed files since <timestamp>" plus the current ontology state (via a future `DroverQuery` or `DroverExport` helper). This keeps cost and time down.
- Model choice: Claude Opus 4.x / 4.8 Dynamic Workflows (late May 2026 discussions) appear especially well suited; they were noted as the first frontier model showing native RLM-style recursive/tool-use patterns.
- Observability: Log every REPL statement the model emits + the return values. This becomes part of the audit trail for the resulting ontology entities (store as `Action` or `Note` entities with `source: "rlm-ingest"`).

## What to Build (Scope for the RLM Integration)
- A drover-aware codebase/document loader that produces a single "context" string with high-signal content and file markers, without overwhelming the REPL.
- REPL-injectable analysis helpers (ListFiles, GetFileContent, ParseGoFile using go/ast, FindReferences, plus drover sugar for beads/schemas/taxonomy).
- A governed write client (DroverClient) that exposes narrow funcs for the RLM to call: creation/relation/update/delete + validate/fsck/snapshot. Must support calling the real high-level Ontology API (for trusted) **and** exec of the installed `ontology`/`drover-ont` binary (to obey the CRITICAL RULE from SKILL.md for untrusted model-generated code).
- A strong, self-contained system prompt that primes the model as "expert ontologist + drover power user", lists every available helper, gives recommended vocab from drover schemas + DDD, encodes the governance policies as hard rules, and mandates a 6-phase iterative workflow ending in Validate + Fsck + Snapshot + FINAL_ONTOLOGY(...).
- A runnable spike (tools/rlm-ontology) that emulates the code an RLM would emit, exercises the full stack, and can be pointed at any Go tree (or the drover-org itself).
- Documentation: integration guide explaining the end-to-end runtime process when "run against an existing codebase", production safety notes, and the prompt itself as a living artifact.
- **Constraints (non-negotiable from prior architecture work)**: No new graph DB (Kektor/Ladybug/Neo4j/gonum etc.). No parallel state. All durable writes go through the existing reducer + JSONL + schema seam + policies + temporal model. Prefer the binary for real agent use.

## How to Build / Extend It (Implementation Approach)
1. **Loader**: Implement `internal/ingest/loader.go` with `filepath.WalkDir`, drover-specific skip list (including internal testdata, memory/ontology/graph.db, snapshots), include list for .go/.md/.yaml/beads.json/CONTEXT etc., go.mod first, "=== FILE: rel/path ===" headers, size truncation.
2. **Analysis**: `internal/ingest/code_analysis.go` with string-search based GetFileContent over the annotated context, go/parser + ast.Inspect for ParseGoFile (extract structs/interfaces/funcs/methods), reference search, plus helper methods that return drover vocab.
3. **Client**: `internal/ingest/drover_client.go` — struct holding either *ontology.Ontology (direct) or binary+paths (exec). Implement the thin methods that either delegate to ont.CreateEntity/Relate/... or build CLI args and exec.Command(...). Provide AsMap(analysis) for easy injection into interpreters.
4. **Prompt**: Put the full battle-tested text in `docs/rlm-system-prompt.md`. Update as the helper surface evolves. Include explicit "You MUST call DroverValidate + DroverFsck before FINAL".
5. **Spike + verification**: `tools/rlm-ontology/main.go` loads context + analysis + client (toggle ForceExec), then hard-codes a small number of "RLM-emitted" steps that look like real recursive REPL code the model will write. Always end with Validate/Fsck/Snapshot + FINAL_ONTOLOGY print. Add tests or make commands later.
6. **Integration tests**: Ensure `go build`, core tests, and manual spike runs (both modes) stay green. After any change to high-level API, update client + prompt + spike.
7. **Future real RLM**: When adding github.com/XiaoConstantine/rlm-go (evaluate sandbox first!), the harness will do: loader → context var, client.AsMap(...) + analysis funcs + raw "context" string injected into Yaegi env, feed the prompt to the RLM.CompleteWithEnv, let the model emit real Go blocks that call the registered Drover* and analysis symbols.

All of the above was executed for bea_ont_010 while preserving the post-01-05 architecture (reducer as single source of mutation truth, Load always high-fidelity via replay, owner Ontology with high-level methods, full seam for schema/policies, "use the binary" rule).

## Open Questions / Next Beads

- bea_ont_010 (this work): Land the loader + client + prompt + docs + spike. (Current — complete with comprehensive criteria and this design doc)
- Add a `DroverQuery(cypherOrNL string)` helper once we have a stable read surface.
- Evaluate rlm-go + Yaegi sandboxing story; possibly vendor a minimal safe interpreter.
- Nightly job that runs RLM delta against drover-org + company sources and proposes a PR of ontology changes.
- Human-in-the-loop competency questions ("Can we answer X about our taxonomy?") fed back into the RLM for refinement passes.

## References

- Original RLM blog (Oct 2025) + minimal impl: alexzhang13/rlm-minimal
- Full paper (arXiv 2512.24601, revisions through May 2026)
- rlm-go (XiaoConstantine) — the Go + Yaegi port
- Prime Intellect, VentureBeat, InfoQ coverage (Jan 2026) calling it "the paradigm of 2026"
- Late May 2026 Claude Opus 4.8 discussions (possible first trained RLM-style frontier model)
- This repo's architecture review artifacts (the five candidates executed June 2026), `CONTEXT.md`, `reducer.go`, `ontology.go` high-level methods, `docs/OPERATIONS.md`

The combination is extremely powerful: RLM gives you the long-context *ingestion superpower*; drover-ontology gives you the governed, replayable, policy-enforcing, single-source-of-truth *runtime* that makes the resulting ontology actually usable and trustworthy by every agent in the organization.
