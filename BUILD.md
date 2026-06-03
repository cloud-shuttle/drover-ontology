# Building the Ontology CLI (Drover Version)

This is the working implementation based on the original share.

## Build

```bash
cd /Users/peterhanssens/consulting/drover-org/drover-ontology

go mod tidy
go build -o bin/ontology ./cmd/ontology

./bin/ontology --help
```

## Install

```bash
go install ./cmd/ontology
# Now `ontology` is available globally (if $GOPATH/bin is in PATH)
```

## First Run

```bash
mkdir -p memory/ontology

# Migrate any existing data
./bin/ontology migrate

# Pull embedding model (requires Ollama running)
ollama serve
./bin/ontology embed-model pull nomic-embed-text

./bin/ontology validate
```

## Notes on Structure

- The deepened implementation now lives in `internal/ontology/` (the single canonical module after consolidation).
- `internal/core/` has been reduced to a thin compatibility layer.
- Commands in `commands/` continue to work via the compatibility layer.
- No more polluting root-level shims.

The feature set (temporal entities, SQLite backend, Ollama embeddings, hybrid search, all the commands) is fully functional.
