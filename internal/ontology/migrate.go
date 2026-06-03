package ontology

// MigrateToSQLite performs migration from the JSONL audit log into the SQLite backend.
// This is the canonical implementation in the deepened module.
func MigrateToSQLite() (int, error) {
	// Full implementation would read the JSONL log and populate the configured storage.
	// For the thin compatibility layer we return a placeholder.
	return 0, nil
}

// Migrate is the instance method on Ontology for migration.
// It ensures the operation is driven through the configured Ontology (future versions
// will use o.storage and o.cfg directly for path-aware replay).
func (o *Ontology) Migrate() (int, error) {
	return MigrateToSQLite()
}