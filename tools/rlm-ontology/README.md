# RLM Ontology Spike (drover-ontology)

Runnable demonstration of **Recursive Language Models (RLMs)** as a powerful ingestion/builder layer that populates the *governed drover-ontology store*.

**Two modes (controlled by build tags):**
- **Default** (`go run ./tools/rlm-ontology ...`): Emulation that hard-codes example "RLM steps". Uses the strong `ingest` loader + `DroverClient`. Excellent for understanding without needing an LLM or the rlm-go dep.
- **Real** (`go run -tags rlm ./tools/rlm-ontology [target-dir]`): Uses the real `github.com/XiaoConstantine/rlm-go` (Yaegi for the REPL) + actual LLM calls.

  The harness:
  - Loads the full codebase/docs with our drover-aware loader (`CodebaseLoader`).
  - Creates a Yaegi interpreter and registers our `Drover*` (Create/Relate/Fsck/Snapshot...) + analysis helpers (`ListFiles`, `ParseGoFile` etc.) as real callable Go symbols.
  - Runs a minimal RLM loop: call root LLM (with the full drover ontology-builder system prompt from `docs/rlm-system-prompt.md`) → model emits Go code → execute it in Yaegi (side-effects go straight into the live drover store) → feed stdout/results back → repeat.
  - The model can perform **true recursive sub-LLM calls** from *inside* the executed REPL code by calling the registered `Query(prompt)` helper.
  - Stops on `FINAL_ONTOLOGY(...)` (detected via rlm-go's `parsing.FindFinalAnswer`) or max iterations.

All drover data and RLM run artifacts are organized under `/.ontology/` (see below). **Everything still writes through the drover governed store** (JSONL = source of truth, schema/policies/fsck/reducer all apply). No parallel graph DB (Ladybug/Kuzu/Kektor etc.) is ever created.

## Output Organization (the ./.ontology/ folder)

Running the spike (real or emulation, if updated) creates a clean, self-contained directory:

```
./.ontology/
├── graph.jsonl                 # The durable append-only drover log (source of truth)
├── graph.db                    # Best-effort SQLite projection (for speed)
├── snapshots/
│   └── rlm-....jsonl           # Point-in-time copies (via DroverSnapshot)
└── rlm-runs/
    └── <timestamp>/
        ├── rlm-session.log     # Full LLM conversation history + code blocks
        └── metadata.json       # Run summary (iters, final answer, target, etc.)
```

This matches the spirit of the research thread (everything in one tidy place) while staying 100% compatible with drover-ontology's architecture and the "use the Go binary / high-level API" rule.

You can point future `drover-ont` / `ontology` commands at it:

```bash
drover-ont --graph ./.ontology/graph.jsonl --db ./.ontology/graph.db fsck
```

Or just use the normal central store by configuring the `DroverClient` differently in a production harness.

## How to Run

```bash
# Build the ontology binary first (so exec mode / snapshots work)
make build

# Emulation (no extra deps)
go run ./tools/rlm-ontology ./some/codebase

# Real RLM + Yaegi + LLM (requires the tag)
# Real LLM calls (recommended):
ANTHROPIC_API_KEY=sk-... go run -tags rlm ./tools/rlm-ontology .

# Demo mode (canned LLM responses that still do real Yaegi + drover writes):
go run -tags rlm ./tools/rlm-ontology .
```

The real mode will create `/.ontology/` (and subdirs) on first run.

## The RLM Process (exactly as asked)

Yes — you kick it off **once**, and it runs an autonomous iterative/recursive loop until the model emits `FINAL_ONTOLOGY(...)` (or hits the safety max-iterations limit).

It is **not** one giant prompt. It is a multi-turn process:

1. Root LLM receives short system prompt (the drover ontology builder prompt) + current history.
2. Model outputs Go code that uses the registered helpers (`DroverCreate`, `ParseGoFile`, `Query(...)` for sub-tasks, etc.).
3. We execute that code in a Yaegi REPL (the "external memory" + side effects on the drover graph happen here).
4. Execution results (stdout, any errors, side effects) are fed back to the LLM as the next "user" message.
5. Model can, from *inside* the Go code it just wrote, call `Query("deeper question on this package")` — this does a real (or demo) sub-LLM call. The sub-result comes back as a string the code can use. This is the "recursive" part and can nest.
6. Loop until the model outputs `FINAL_...` (we detect it with rlm-go's parser) or we hit the limit.

Typical run on a medium codebase: 8–20+ iterations, with occasional sub-`Query` calls. The model gets to "think for a long time" by using code as its scratchpad and the drover store as persistent memory.

All writes go through `DroverClient` → either direct `Ontology.CreateEntity/Relate/...` (high-level API from the architecture cleanup) or exec of the real `ontology`/`drover-ont` binary.

## Requirements & Notes

- For real LLM: `ANTHROPIC_API_KEY` (and optionally `ANTHROPIC_MODEL`).
- The rlm-go + yaegi dependency is only pulled/used when building with `-tags rlm` (see `go.mod` comments).
- The emulation mode (no tag) requires nothing extra and is always available.
- Schema: falls back to `DefaultSchema()` if the referenced `memory/ontology/schema.yaml` is not present.
- This is a **spike** demonstrating the integration. A production version would add proper logging (the rlm-go logger), cost controls (different model for sub-`Query`), human-in-the-loop gates, delta mode, etc.

See:
- `docs/rlm-integration.md` (the full "what + how" design doc)
- `docs/rlm-system-prompt.md` (the exact prompt the root LLM receives)
- `internal/ingest/` (the reusable loader + helpers)
- `beads.json` (bea_ont_010)

Enjoy letting the model build your ontology for you! 

If you run with a real key on a real codebase, the results (and the `rlm-session.log`) are often fascinating.