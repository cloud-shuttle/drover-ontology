---
title: "ADR-0001: SQLite best-effort query acceleration layer metadata-based startup integrity check"
description: "Records the decision to implement a sub-millisecond, metadata-based startup integrity check for the SQLite projection to detect log divergence without replaying the entire graph.jsonl on every CLI call."
product: drover-ontology
audience: developer
doc_type: adr
topics: [storage, durability, performance, integrity]
---

# ADR-0001: SQLite best-effort query acceleration layer metadata-based startup integrity check

| Field | Value |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-06-01 |
| **Deciders** | Ontology developer & Agent |
| **Supersedes** | — |
| **Superseded by** | — |

---

## Context

`drover-ontology` provides a strongly typed context graph and knowledge base for agent memory. It uses a **hybrid storage system** composed of two parts:
1. **Durable audit log (`graph.jsonl`)**: An append-only, line-delimited stream of mutations. This is the absolute, authoritative source of truth.
2. **Query acceleration layer (SQLite)**: A read-optimized relational projection used to enable high-performance querying, search, and validation.

As the durable audit log grows over time to thousands or millions of lines, replaying the entire file from the beginning on every CLI command invocation to ensure that SQLite matches the log would introduce massive, unacceptable startup latency (multiple seconds).

However, SQLite is run in a local desktop/CLI environment where disk full errors, lock contention, or sudden process termination could lead to a mismatch between SQLite and the log. If the SQLite database goes out of sync with the durable log, agents could read stale or corrupted context, leading to wrong decisions.

A startup check (Task `bea_ont_004`) is needed that satisfies two conflicting requirements:
1. **Performance**: It must execute in under 1 millisecond so that it adds no noticeable overhead to the CLI startup.
2. **Correctness**: It must reliably detect when the SQLite state is stale or diverged from the latest state of the durable log.

---

## Decision

**We adopt a sub-millisecond, metadata-based integrity check stored directly in the SQLite database.**

We introduce a metadata table in the SQLite schema:
```sql
CREATE TABLE IF NOT EXISTS _meta (
    key TEXT PRIMARY KEY,
    value TEXT
);
```

On every successful write to the durable audit log and SQLite in `Apply()`, we write the current **byte offset** (and optionally the total line count) of the durable audit log into the `_meta` table.

On startup:
1. We query the `_meta` table for the last known log byte offset.
2. We check the size of `graph.jsonl` on disk.
3. If the file size is less than the stored offset, or if a seek to the stored offset fails to find the expected record structure, we declare a **mismatch**.

### Mismatch Recovery Behavior:
* **CLI/Interactive Mode**: We print a warning to `stderr` and **automatically recover** the SQLite cache by wiping the SQLite tables and running a background replay (`log.LoadAll()`) to fully reconstruct the database. The user is not blocked, preserving the "best-effort" nature of the SQLite layer.
* **Strict/CI Mode (`--strict` flag)**: We fail-stop and exit with a non-zero code to ensure tests catch any real-time drift immediately.

---

## Alternatives considered

### Alternative A: Full Replay on Startup
Replay the entire `graph.jsonl` log and compare the resulting in-memory map against the SQLite DB.
* **Rejected because**: It is an $O(N)$ operation that grows linearly with the size of the history, violating the sub-millisecond CLI startup requirement.

### Alternative B: File Hash (SHA-256) check
Hash the entire `graph.jsonl` log on startup and compare it against a hash stored in SQLite.
* **Rejected because**: Hashing a large file on disk still requires reading the entire file, which introduces disk I/O bottlenecks and violates the performance target as the log grows.

---

## Consequences

### Positive
- **Sub-millisecond startup overhead**: The check is $O(1)$ and takes `< 1` millisecond.
- **Fail-safe recovery**: Ephemeral SQLite corruption is repaired automatically without manual intervention.
- **Strict verification in CI**: Using `--strict` ensures that any genuine logic bugs that cause SQLite-log drift are caught immediately during test suites.

### Negative / Risks
- **Requires file seeking safety**: Seeking to a specific byte offset relies on the operating system file system accuracy. If the log is manually edited, the byte offset might align with a random line, leading to false positives or false negatives. (Mitigated by validation of the record signature at that offset).
- **SQLite best-effort assumption**: Rebuilding SQLite from scratch could take several seconds for extremely large logs. While this only happens once upon detecting corruption, it may cause a one-off delay.

---

## Verification
- Automated property tests (`TestLogReplayEquivalence`) verify that after writing arbitrary operations, reopening the database triggers a sub-millisecond meta check, and simulated corruption successfully triggers an auto-repair.
