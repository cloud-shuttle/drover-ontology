package ontology

// Storage is the seam for all persistence concerns (append-only log + query store).
// This allows swapping in-memory, test, or different backends without changing the Ontology logic.
type Storage interface {
	// Load returns the current entity state (full map including historically
	// invalidated/tombstoned entities, with ValidUntil and Relations populated).
	// Per Contract B (architecture review): must deliver complete, queryable
	// state with relations or fall back to pure log replay rather than return
	// partial data. Callers (Ontology, commands, HybridSearch) apply IsDeleted
	// filtering for default "current views".
	Load() (map[string]*Entity, error)

	// Apply persists an operation (durability) and updates queryable state.
	Apply(op Operation) error

	// StoreEmbedding persists a vector embedding for an entity.
	StoreEmbedding(entityID string, vector []float32, summary string) error

	// Close releases any resources (e.g. DB connections).
	Close() error

	// Replay returns the entity state obtained purely by replaying the append-only
	// JSONL log. This is the authoritative source of truth used for integrity checks.
	// Custom Storage implementations may return an error if they cannot provide replay.
	Replay() (map[string]*Entity, error)
}

// Embedder is the seam for generating vector embeddings used by HybridSearch
// and automatic embedding on create/update.
// This allows real Ollama usage in production and fully deterministic (or mocked)
// embeddings in tests without external dependencies.
type Embedder interface {
	Generate(text string) ([]float32, error)
}




// Default implementations will be provided in storage.go and embed.go (or existing files).
// For now we define the interfaces here as the primary seams.
