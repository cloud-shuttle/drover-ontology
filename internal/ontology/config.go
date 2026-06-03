package ontology

// Config holds all configuration for an Ontology instance.
// This is the deepened configuration seam.
type Config struct {
	// GraphPath is the path to the append-only JSONL audit log.
	// This is the source of truth for all mutations.
	GraphPath string

	// DBPath is the path to the SQLite database used for fast queries.
	DBPath string

	// SchemaPath is the path to the YAML schema definition.
	SchemaPath string

	// EmbedModel is the name of the Ollama embedding model to use.
	EmbedModel string

	// UseSQLite controls whether to enable the SQLite query store.
	// When false, the system operates in pure JSONL mode.
	UseSQLite bool

	// CheckIntegrityOnOpen, when true, runs IntegrityCheck() immediately
	// after loading the ontology. This is useful for development, CI, and
	// high-assurance deployments. It is off by default for performance.
	CheckIntegrityOnOpen bool
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		GraphPath:            ".ontology/graph.jsonl",
		DBPath:               ".ontology/graph.db",
		SchemaPath:           ".ontology/schema.yaml",
		EmbedModel:           "nomic-embed-text",
		UseSQLite:            true,
		CheckIntegrityOnOpen: false,
	}
}

// FromCLI returns a Config populated from command-line flag values.
func FromCLI(graphPath, dbPath, schemaPath string) Config {
	cfg := DefaultConfig()
	if graphPath != "" {
		cfg.GraphPath = graphPath
	}
	if dbPath != "" {
		cfg.DBPath = dbPath
	}
	if schemaPath != "" {
		cfg.SchemaPath = schemaPath
	}
	return cfg
}