---
title: "ADR-0002: EPhemeral SQLite relation loading and physical deletion"
description: "Records the decision to physically delete rows from the SQLite relations table on 'unrelate' operations, and filter out ghost relations pointing to deleted entities during Load()."
product: drover-ontology
audience: developer
doc_type: adr
topics: [storage, relations, consistency]
---

# ADR-0002: Ephemeral SQLite relation loading and physical deletion

| Field | Value |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-06-01 |
| **Deciders** | Ontology developer & Agent |
| **Supersedes** | — |
| **Superseded by** | — |

---

## Context

In `drover-ontology`, entities in the knowledge graph are connected via directed edges called **Relations**.
* In the **durable audit log (`graph.jsonl`)**, every mutation—including `unrelate` operations—is append-only. To determine the set of relations for an entity during log replay, the engine replays the entire history and removes relations when an `unrelate` record is encountered.
* In the **query acceleration layer (SQLite)**, relations are stored in a relational `relations` table.

However, two major gaps were discovered in the SQLite integration:
1. **Relations were never loaded from SQLite**: `hybridStorage.Load()` only queried the `entities` table, leaving all in-memory entity relations empty after a restart. This caused severe consistency drift between live memory and restarted state, violating the `TestLogReplayEquivalence` property.
2. **Relations pointing to deleted entities (Ghost Relations)**: When an entity is soft-deleted (tombstoned or invalidated), its incoming and outgoing relations remain in the database. If these relations are returned during search or validation, they violate referential integrity and cause policy checks to fail.

A clean strategy is needed to ensure:
1. Relations are loaded correctly on startup.
2. `unrelate` operations are reflected in SQLite efficiently.
3. Ghost relations pointing to soft-deleted or invalidated entities are safely filtered out.

---

## Decision

**We adopt physical deletion of SQLite relations on unrelate, combined with strict filter-on-load filtering for soft-deleted/invalidated entities.**

### 1. SQLite Relation Loading & Stitching
During `hybridStorage.Load()`, we perform a second query to retrieve all stored relations:
```sql
SELECT from_id, rel, to_id, properties FROM relations
```
We then stitch these relations onto the `Relations` slice of each loaded `Entity`.

### 2. Ghost Relation Filtering (Filter-on-Load)
If a relation's `from_id` or `to_id` refers to an entity that has been soft-deleted (tombstoned or whose `ValidUntil` has passed), we **exclude** that relation from the in-memory `Relations` slice. This guarantees referential integrity at the application boundary, matching the append-only `Log.LoadAll()` replayed view.

### 3. Physical Deletion on Unrelate
When an `unrelate` operation is executed:
* The **durable log** appends an `unrelate` record.
* The **SQLite database** executes a physical deletion of the matching row:
  ```sql
  DELETE FROM relations WHERE from_id = ? AND rel = ? AND to_id = ?
  ```

*Why physical deletion?*  
Unlike the durable JSONL log, SQLite is an ephemeral query acceleration layer. Physical deletion keeps queries fast, indices clean, and avoids the need for complex temporal queries on join tables, while aligning perfectly with the current valid in-memory state.

---

## Alternatives considered

### Alternative A: Soft-deletion of relations in SQLite
Add a `deleted` or `valid_until` column to the SQLite `relations` table and filter out relations in the query.
* **Rejected because**: It introduces unnecessary overhead and complexity to SQLite query planning and index maintenance for an ephemeral projection layer. Physical deletion is simpler, faster, and perfectly represents the current valid state of the graph.

---

## Consequences

### Positive
- **Complete Log-to-SQLite Equivalence**: Resolves the relation drift issue in `TestLogReplayEquivalence`.
- **Referential Integrity**: Guarantees that query results never return "ghost relations" pointing to deleted entities.
- **Fast Queries**: Physical deletion ensures the SQLite `relations` table remains compact and highly queryable.

### Negative / Risks
- **Requires two queries during Load**: Loading relations requires querying the `relations` table and stitching them in Go. (Mitigated by the fast in-memory stitch using an $O(1)$ map lookup).

---

## Verification
- Generative property-based tests (`TestLogReplayEquivalence`) verify that after writing arbitrary combinations of `create`, `relate`, `unrelate`, `tombstone`, and `invalidate`, the restarted state matches the in-memory state perfectly.
