package ontology

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// hybridStorage is the default Storage implementation that combines
// JSONL (append-only durable log) with SQLite (fast queryable store).
// This is the production hybrid backend.
type hybridStorage struct {
	graphPath string
	db        *sql.DB
	log       *Log // append-only JSONL log
}

// newHybridStorage creates the default production storage.
func newHybridStorage(cfg Config) (Storage, error) {
	s := &hybridStorage{
		graphPath: cfg.GraphPath,
	}

	// Ensure directories
	dir := filepath.Dir(cfg.GraphPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Open append-only log
	var err error
	s.log, err = NewLog(cfg.GraphPath)
	if err != nil {
		return nil, err
	}

	// Open SQLite if requested
	if cfg.UseSQLite && cfg.DBPath != "" {
		dbDir := filepath.Dir(cfg.DBPath)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return nil, err
		}
		s.db, err = sql.Open("sqlite3", cfg.DBPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open SQLite: %w", err)
		}

		// Create schema (same as before)
		_, err = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS entities (
				id TEXT PRIMARY KEY,
				type TEXT NOT NULL,
				properties TEXT,
				created TEXT,
				updated TEXT,
				valid_from TEXT NOT NULL DEFAULT (datetime('now')),
				valid_until TEXT
			);
			CREATE TABLE IF NOT EXISTS relations (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				from_id TEXT,
				rel TEXT,
				to_id TEXT,
				properties TEXT,
				timestamp TEXT DEFAULT (datetime('now'))
			);
			CREATE TABLE IF NOT EXISTS embeddings (
				entity_id TEXT PRIMARY KEY,
				model TEXT,
				vector TEXT,
				summary TEXT,
				created TEXT DEFAULT (datetime('now'))
			);
			CREATE TABLE IF NOT EXISTS _meta (
				key TEXT PRIMARY KEY,
				value TEXT
			);
			CREATE INDEX IF NOT EXISTS idx_entities_valid ON entities(valid_until);
		`)
		if err != nil {
			return nil, fmt.Errorf("failed to create schema: %w", err)
		}
	}

	// Run integrity check and auto-recovery on open
	if cfg.UseSQLite && cfg.DBPath != "" {
		if err := s.CheckAndRecover(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to run storage integrity check and recovery: %v\n", err)
		}
	}

	return s, nil
}

func (s *hybridStorage) Load() (map[string]*Entity, error) {
	// Always obtain the entity state (including relations, properties, and
	// temporal ValidUntil/ValidFrom markers) via pure log replay through the
	// single OperationReducer (applyOperation in reducer.go).
	//
	// This guarantees:
	// - Full fidelity and consistency between live Apply (which updates in-mem
	//   via the reducer + best-effort SQL) and reopened state (Contract B from
	//   architecture review).
	// - No subtle drift from SQL projection.
	// - Temporal validity (invalidate/tombstone) applied exactly as in the
	//   reducer on every reload.
	//
	// SQLite remains best-effort acceleration (CheckAndRecover, embeddings).
	// Load uses the log (via the reducer) for the authoritative map.
	ents, _, err := s.log.LoadAll()
	if err != nil {
		return nil, err
	}
	return ents, nil
}

func (s *hybridStorage) Apply(op Operation) error {
	now := time.Now().UTC()

	// Durability: always append to JSONL
	if err := s.log.Append(op); err != nil {
		return err
	}

	// Update SQLite if available.
	// Policy: SQLite is a best-effort query acceleration layer.
	// Failures here are logged as warnings but do **not** cause the overall
	// operation to fail, because the durable JSONL log append has already
	// succeeded. The system can always recover by replaying the log on restart.
	if s.db != nil {
		sqliteErr := s.writeToSQLite(op, now)
		if sqliteErr == nil {
			stat, err := os.Stat(s.graphPath)
			if err == nil {
				_, _ = s.db.Exec(`INSERT OR REPLACE INTO _meta (key, value) VALUES ('log_offset', ?)`, stat.Size())
			}
		} else {
			// Best-effort policy: log warning but do not fail the operation.
			fmt.Fprintf(os.Stderr, "Warning: SQLite write failed (operation still durable in JSONL): %v\n", sqliteErr)
		}
	}
	return nil
}

func (s *hybridStorage) writeToSQLite(op Operation, now time.Time) error {
	if s.db == nil {
		return nil
	}
	var err error

	switch op.Op {
	case "create":
		if op.Entity == nil {
			return nil
		}
		propsJSON, err := json.Marshal(op.Entity.Properties)
		if err != nil {
			return fmt.Errorf("failed to marshal properties for create: %w", err)
		}
		validUntil := ""
		if op.Entity.ValidUntil != nil {
			validUntil = op.Entity.ValidUntil.Format(time.RFC3339)
		}
		_, err = s.db.Exec(`
			INSERT OR REPLACE INTO entities 
			(id, type, properties, created, updated, valid_from, valid_until)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			op.Entity.ID, op.Entity.Type, string(propsJSON),
			op.Entity.Created.Format(time.RFC3339), now.Format(time.RFC3339),
			op.Entity.ValidFrom.Format(time.RFC3339), validUntil,
		)
		if err != nil {
			return fmt.Errorf("failed to persist create to SQLite: %w", err)
		}

	case "update":
		if op.ID == "" || op.Set == nil {
			return nil
		}
		propsJSON, err := json.Marshal(op.Set)
		if err != nil {
			return fmt.Errorf("failed to marshal set for update: %w", err)
		}
		_, err = s.db.Exec(`
			UPDATE entities 
			SET properties = json_patch(properties, ?), updated = ?
			WHERE id = ?`,
			string(propsJSON), now.Format(time.RFC3339), op.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to persist update to SQLite: %w", err)
		}

	case "relate":
		propsJSON, err := json.Marshal(op.Properties)
		if err != nil {
			return fmt.Errorf("failed to marshal properties for relate: %w", err)
		}
		_, err = s.db.Exec(`
			INSERT INTO relations (from_id, rel, to_id, properties, timestamp)
			VALUES (?, ?, ?, ?, ?)`,
			op.From, op.Rel, op.To, string(propsJSON), now.Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("failed to persist relate to SQLite: %w", err)
		}

	case "unrelate":
		_, err = s.db.Exec(`
			DELETE FROM relations WHERE from_id = ? AND rel = ? AND to_id = ?`,
			op.From, op.Rel, op.To,
		)
		if err != nil {
			return fmt.Errorf("failed to delete relation from SQLite (unrelate): %w", err)
		}

	case "invalidate":
		if op.ID == "" {
			return nil
		}
		_, err = s.db.Exec(`
			UPDATE entities SET valid_until = ?, updated = ? WHERE id = ?`,
			op.ValidUntil, now.Format(time.RFC3339), op.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to persist invalidate to SQLite: %w", err)
		}

	case "tombstone":
		if op.ID == "" {
			return nil
		}
		// Always include tombstone_reason (even if empty) to match the unified
		// applyOperation reducer (and prior Log.LoadAll). This ensures that
		// properties after SQL reload match pure replay, reducing drift surface.
		tombProps := map[string]any{
			"status":             "tombstoned",
			"tombstone_reason":   op.Reason,
		}
		propsJSON, err := json.Marshal(tombProps)
		if err != nil {
			return fmt.Errorf("failed to marshal tombstone properties: %w", err)
		}
		_, err = s.db.Exec(`
			UPDATE entities 
			SET properties = json_patch(properties, ?), updated = ?
			WHERE id = ?`,
			string(propsJSON), now.Format(time.RFC3339), op.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to persist tombstone to SQLite: %w", err)
		}
	}

	return nil
}

func (s *hybridStorage) CheckAndRecover() error {
	if s.db == nil {
		return nil
	}

	// 1. Get current log size on disk
	stat, err := os.Stat(s.graphPath)
	logExistsAndNotEmpty := err == nil && stat.Size() > 0
	logSize := int64(0)
	if err == nil {
		logSize = stat.Size()
	}

	// 2. Query log_offset from SQLite
	var storedOffsetStr string
	err = s.db.QueryRow(`SELECT value FROM _meta WHERE key = 'log_offset'`).Scan(&storedOffsetStr)

	mismatch := false
	if err != nil {
		// log_offset missing
		if logExistsAndNotEmpty {
			mismatch = true
		}
	} else {
		var storedOffset int64
		if _, perr := fmt.Sscanf(storedOffsetStr, "%d", &storedOffset); perr != nil || storedOffset != logSize {
			mismatch = true
		}
	}

	if mismatch {
		fmt.Fprintf(os.Stderr, "Warning: SQLite projection out of sync with durable log. Rebuilding SQLite cache...\n")

		// Truncate SQLite tables
		_, _ = s.db.Exec(`DELETE FROM entities`)
		_, _ = s.db.Exec(`DELETE FROM relations`)
		_, _ = s.db.Exec(`DELETE FROM embeddings`)
		_, _ = s.db.Exec(`DELETE FROM _meta`)

		// Replay log
		_, ops, err := s.log.LoadAll()
		if err != nil {
			return fmt.Errorf("auto-recovery failed to load log: %w", err)
		}

		for _, op := range ops {
			_ = s.writeToSQLite(op, op.Ts)
		}

		// Write final offset
		_, _ = s.db.Exec(`INSERT OR REPLACE INTO _meta (key, value) VALUES ('log_offset', ?)`, logSize)
	}

	return nil
}

func (s *hybridStorage) StoreEmbedding(entityID string, vector []float32, summary string) error {
	if s.db == nil {
		return nil
	}
	vecJSON, err := json.Marshal(vector)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding vector: %w", err)
	}
	if _, err := s.db.Exec(`
		INSERT OR REPLACE INTO embeddings (entity_id, model, vector, summary)
		VALUES (?, ?, ?, ?)`,
		entityID, "nomic-embed-text", string(vecJSON), summary,
	); err != nil {
		return fmt.Errorf("failed to persist embedding to SQLite: %w", err)
	}
	return nil
}

func (s *hybridStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Replay implements the Storage seam by returning the state obtained
// from fully replaying the append-only JSONL log (bypassing SQLite).
func (s *hybridStorage) Replay() (map[string]*Entity, error) {
	ents, _, err := s.log.LoadAll()
	if err != nil {
		return nil, err
	}
	return ents, nil
}