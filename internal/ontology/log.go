package ontology

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Log manages the append-only JSONL mutation log.
type Log struct {
	path string
}

// NewLog opens (or creates) the graph.jsonl file.
func NewLog(path string) (*Log, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		f.Close()
	}
	return &Log{path: path}, nil
}

// Append writes a single mutation to the append-only JSONL log.
//
// Durability note: After a successful return, the operation has been written
// to the filesystem. For stronger durability guarantees (e.g. against power loss),
// callers may want to ensure the underlying filesystem has been synced.
func (l *Log) Append(m Operation) error {
	if m.Ts.IsZero() {
		m.Ts = time.Now().UTC()
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s for append: %w", l.path, err)
	}
	defer f.Close()

	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal operation for log append: %w", err)
	}

	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("failed to write operation to log %s: %w", l.path, err)
	}

	// Best-effort durability. We do not call f.Sync() by default because it can
	// be expensive on high-throughput workloads. Callers who need stronger
	// guarantees should consider wrapping or using a different storage backend.
	return nil
}

// LoadAll reads the entire log into memory (current state + full history).
func (l *Log) LoadAll() (map[string]*Entity, []Operation, error) {
	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*Entity{}, nil, nil
		}
		return nil, nil, err
	}
	defer f.Close()

	entities := make(map[string]*Entity)
	var mutations []Operation

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var m Operation
		if err := json.Unmarshal(line, &m); err != nil {
			// Skip corrupt lines in production; surface in strict mode later
			continue
		}
		mutations = append(mutations, m)

		// Delegate to the single OperationReducer (applyOperation) so that
		// log replay and live Ontology Apply use identical folding logic.
		// This is the core of Candidate 01 (collapse duplicated Operation folding).
		applyOperation(entities, m)
	}
	return entities, mutations, scanner.Err()
}
