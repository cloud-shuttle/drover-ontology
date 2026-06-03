# Production Operations Guide

**For the drover-ontology hybrid store (JSONL audit log + best-effort SQLite).**

**Status**: bea_ont_008 complete (2026-06-03). This fulfills the original acceptance criteria (new guide covering backup/restore/corruption/recovery + commands + accuracy review against storage code). It is a living document — update from real incidents, RLM runs, CI usage, and monorepo scale experience.

**Core invariants (never violate):**
- JSONL is the only durable source of truth.
- All writes go through the `ontology` / `drover-ont` binary (or the high-level `Ontology` methods that feed it).
- The single `OperationReducer` (reducer.go) + replay is the definition of "current state".
- Schema + Drover policies (governed_by for accepted Terms, approved Plan requires Review, acyclicity, etc.) are enforced on `validate` and `fsck`.

## Core Mental Model
- **JSONL (`graph.jsonl`)**: Append-only, immutable audit log. The hard source of truth. Replay it to reconstruct everything.
- **SQLite (`graph.db`)**: Best-effort query acceleration (current entities/relations + embeddings). Can be thrown away and rebuilt.
- **Ontology binary / type**: The single owner. `Apply` always appends to JSONL first; SQLite and in-memory are derived.
- **Snapshots/**: Point-in-time copies of the JSONL (produced by RLM runs, manual backups, or future tooling). Treat as immutable artifacts.

**Failure policy (Phase 0 decision):**
- JSONL append fails → operation fails hard (data loss prevented).
- SQLite write fails after successful JSONL append → warning to stderr; operation succeeds. System recovers on next open via `CheckAndRecover`.

## Starting / Opening (in code or CLI)
```go
ont, err := ontology.New(ontology.Config{
    GraphPath:            "memory/ontology/graph.jsonl",
    DBPath:               "memory/ontology/graph.db",
    SchemaPath:           "memory/ontology/schema.yaml",
    UseSQLite:            true,
    CheckIntegrityOnOpen: true,   // strongly recommended for prod / important jobs
})
```

CLI equivalent (respects the same flags):
```bash
ontology --graph memory/ontology/graph.jsonl \
         --db    memory/ontology/graph.db \
         --schema memory/ontology/schema.yaml \
         fsck
```

Inside the drover-org monorepo, **always prefer the wrapper**:
```bash
drover-ont fsck
drover-ont create Term my-term -p label="..." -p status=proposed ...
```

## fsck / IntegrityCheck (the most important regular operation)
```bash
ontology fsck
ontology fsck --check-invalidate   # currently still runs the full check
```

What it does (see `IntegrityCheck` + `commands/fsck.go`):
- Full replay of the JSONL via the reducer.
- Entity count, type, property, and basic relation parity vs. current state.
- Full `ValidateFull` (schema structural + relation type/cardinality/acyclicity + Drover policies) **on the replayed authoritative state**.

**Interpreting failures (common cases + recovery):**
- `entity count mismatch (current=N, replayed=M)`: Drift. Usually means a previous SQLite-only path or manual edit. Recovery: stop writers → backup JSONL → delete .db → restart (triggers CheckAndRecover) or `ontology fsck` again. If still wrong, the JSONL tail is suspect — inspect last 50 lines.
- `entity X properties differ from log replay`: Most often from an old partial SQL projection. Recovery: same as above (force replay). Rare real corruption — compare the entity in the log vs. what the command reports.
- `validation violations in replayed state: [Term missing ... / approved Plan without Review / ...]`: Policy breach in the data. Use `ontology validate --type Term` (or Plan) to list. Fix with `update` / `relate` (add the missing `governed_by` or `reviewed_by`/`approves` link) or accept the state and document why. Never bypass by editing the log.
- `type mismatch`: Someone created an entity under the wrong type historically. Usually requires a compensating update or a migration script that replays + rewrites.

Run fsck before any RLM promotion, before important agent work, or on a schedule (e.g. nightly in CI).

In code:
```go
if err := ont.IntegrityCheck(); err != nil {
    // surface or alert
}
```

## Auto-Recovery (CheckAndRecover)
On every `New` (when SQLite is enabled), the storage layer compares the on-disk JSONL size against the `_meta.log_offset` stored in SQLite.

If they differ (or the meta row is missing):
- Prints: `Warning: SQLite projection out of sync with durable log. Rebuilding SQLite cache...`
- Truncates entities/relations/embeddings/_meta tables.
- Replays the entire log via `log.LoadAll()` + `writeToSQLite`.
- Writes the new log size back to `_meta`.

This is automatic and safe. It can be slow on very large logs (current design trades speed for fidelity after the 01–05 architecture work).

Manual trigger: delete the .db file (or just the tables) and reopen.

## Backup & Snapshots (Production Critical)
**Golden rule**: The JSONL (plus any snapshots/) is all you need to restore the world.

Recommended backup procedure (run from a job or cron that has the directory locked or writers paused):

```bash
TS=$(date +%Y%m%d-%H%M%S)
cp memory/ontology/graph.jsonl memory/ontology/snapshots/graph.jsonl.$TS
# Optional: also snapshot the current .db for speed, but not required
cp memory/ontology/graph.db     memory/ontology/snapshots/graph.db.$TS 2>/dev/null || true

# Verify the copy is readable and has the expected tail
tail -c 200 memory/ontology/snapshots/graph.jsonl.$TS
head -5 memory/ontology/snapshots/graph.jsonl.$TS

# Optional strong verify (replay + policies on the copy)
ontology --graph memory/ontology/snapshots/graph.jsonl.$TS fsck
```

For offsite / long-term:
```bash
tar -czf ontology-snapshot-$TS.tar.gz -C memory/ontology graph.jsonl snapshots/
# gpg encrypt if it contains sensitive props
```

RLM runs automatically produce snapshots (see `DroverSnapshot` / the spike). Treat RLM snapshots the same as manual ones — they are governed artifacts.

Retention: keep at least daily for 14 days, weekly for 90 days, plus one per major RLM ingest or schema change. Name them clearly.

## Restore / Point-in-Time Recovery
1. Stop all writers (coordinate via locks or process management).
2. Choose the snapshot JSONL you want (e.g. `graph.jsonl.20260602-...`).
3. Option A (safe): point a new process at the snapshot:
   ```bash
   ontology --graph memory/ontology/snapshots/graph.jsonl.20260602-... \
            --db    /tmp/recover.db \
            list
   ```
4. Option B (replace live): `cp memory/ontology/snapshots/graph.jsonl.XXX memory/ontology/graph.jsonl` then delete the .db and restart writers. The next open will rebuild SQLite.
5. After restore, run `ontology fsck` and `validate`.

**Never** truncate the live log from the end unless you have a verified backup and understand you are discarding history. Prefer "start fresh from a known-good snapshot" instead.

For the central drover-org store (via wrapper): coordinate with other users; the wrapper hard-codes the paths, so restores usually mean replacing the files under `~/consulting/drover-org/memory/ontology/`.

## Large-Scale / RLM Ingestion Operations (bea_ont_010)
See `docs/rlm-integration.md`, `docs/rlm-system-prompt.md`, and `tools/rlm-ontology/`.

Operational flow for a big ingest:
1. Run the harness / spike (or your rlm-go controller) in a sandboxed environment. Point the `DroverClient` at either the live central store (careful) or a throwaway store for review.
2. The RLM uses the loader + analysis helpers + `Drover*` funcs (or execs the binary).
3. At termination the harness calls `DroverFsck()` + `DroverSnapshot(".ontology/snapshots/rlm-YYYY...jsonl")` (or equivalent under the configured store). Dedicated RLM/spike runs default to organizing everything (drover graph.jsonl + graph.db + rlm-runs/ with session logs + metadata + snapshots/) under `/.ontology/` for cleanliness (no scattered root files).
4. **Human review gate** (strongly recommended for any non-trivial run):
   - Inspect the snapshot (size, tail, `ontology --graph <snapshot> fsck`).
   - Review the entities/relations it would add (future diff tool or `query` against a temp store).
   - Decide: promote (see below), iterate (give feedback to the model + re-run delta), or discard.
5. Promotion (once approved):
   - For a fresh store: just replace the target `graph.jsonl`.
   - For delta into live central: the safest current method is to replay the new operations (or cat the approved snapshot onto a copy of the live log, then verify with fsck). Avoid blind appends if the snapshot contains a full history.
   - Concrete example (after review):
     ```bash
     cp .ontology/snapshots/rlm-approved-20260603.jsonl .ontology/graph.jsonl   # or the central store path
     rm -f memory/ontology/graph.db   # force clean rebuild
     drover-ont fsck
     drover-ont validate --strict
     # Then create a provenance Note:
     # drover-ont create Note rlm-ingest-20260603 -p source=snapshot -p snapshot=rlm-approved-20260603.jsonl -p summary="..."
     ```
6. After promotion: run `drover-ont fsck`, notify consumers, and record the snapshot id in an `Action` or `Note` entity for provenance.

**Safety rules for RLM runs:**
- Never give the Yaegi REPL raw filesystem or network access. Only the narrow `Drover*` helpers (and analysis funcs) should be registered.
- Prefer `ForceExec` mode + a capability-wrapped binary for untrusted model code.
- Run large RLM jobs against a copy of the store first.
- Always end with `DroverFsck` + snapshot + human gate before touching the live central (or `/.ontology/`) `graph.jsonl`. Dedicated spikes use `/.ontology/` for self-contained runs (graph + rlm artifacts).

Delta RLM runs (future optimization): feed only files changed since last snapshot + the current ontology state. Keeps cost and time bounded.

## CI / Jobs / Automation Patterns
Example (nightly fsck + health):
```yaml
# .github/workflows/ontology-health.yml (sketch)
- run: make build
- run: ./bin/ontology --graph memory/ontology/graph.jsonl fsck
- run: ./bin/ontology validate
# Alert on non-zero exit
```

RLM delta job (sketch):
- Checkout the monorepo + any doc exports.
- `go run ./tools/rlm-ontology --target . --force-exec` (or your full harness) pointed at a temp store or the central (with lock).
- On success + review signal, promote snapshot.
- Archive the RLM transcript (the exact REPL statements the model emitted) as a Document or Note.

E2E tests in CI already do real binary builds + isolated stores (see `e2e/cli_test.go`).

Generative / replay tests: `make test-replay-long` (set `RAPID_CHECKS=10000` for heavier runs). These are the canary for reducer / fidelity regressions.

## Monitoring & Observability
- Stderr warnings of the form `Warning: SQLite write failed...` are normal under load or transient DB pressure. Sustained or increasing volume may indicate disk or lock contention.
- `ontology list | wc -l` or queries for rough entity growth.
- Track JSONL size (it grows forever — that's the point).
- fsck failures should page or block merges.
- After RLM runs or big migrations, compare pre/post entity counts and run fsck.

Simple health check one-liner:
```bash
ontology fsck && echo "OK" || echo "FAIL"
```

## Security & Access Control
- The store is intentionally single-writer by design (append-only log). If multiple agents/processes write, they must coordinate (file locks, advisory `flock`, or a single writer daemon + queue).
- Directory permissions: `chmod 750 memory/ontology` (or tighter). The JSONL contains whatever Properties you store (PII, secrets, customer data, internal architecture). Treat the whole directory as sensitive.
- Snapshots inherit the same sensitivity.
- When using the central drover-org store via `drover-ont`, the wrapper runs as the current user; protect `~/consulting/drover-org/memory/ontology/`.
- Never commit `graph.jsonl` or live `.db` to git. Snapshots in the repo (if any) should be tiny, redacted, or test-only.

## Performance & Scale Notes
Current design replays the full log on every `Load()` / `New` (and thus on every CLI invocation and on RLM harness startup) to guarantee fidelity after the architecture review (Contract B + unified reducer).

This is acceptable for the current expected size (low tens of thousands of entities at most for agent memory graphs across drover-org and similar).

If you see pain:
- The "trusted offset" fast path (trust SQLite when `_meta.log_offset` matches the log size) is the obvious future optimization.
- For very large RLM runs, run the harness against a throwaway store, review the delta, then apply only the net new operations.
- Embeddings are best-effort and non-fatal; they do not affect durability or policy.

Relations are stored both denormalized on the Entity (for fast traversal) and in the relations table (for SQLite queries).

## Migration Playbooks
Schema is forward-compatible by design:
- Unknown types are allowed.
- Required-property checks only apply to types present in the loaded `Schema` (DefaultSchema or the one at `--schema`).
- New required fields on existing types will cause `validate`/`fsck` failures on old data until you backfill.

When you need to evolve:
1. Update `references/drover-schema.yaml` (or the active `memory/ontology/schema.yaml`).
2. Test the change against a copy of the live JSONL (point a temp binary at the copy + new schema).
3. For data backfills: write a small Go program or script that does `ontology.New(oldCfg)`, reads, then `ontology.New(newCfg)` + high-level `Update`/`Relate` for the needed fixes. All changes are new appends.
4. Run full fsck + validate on the result.
5. The `migrate` command (if/when implemented) will be a thin facade over the same pattern.

Never edit the JSONL by hand. Use compensating operations (`update`, `relate`, `unrelate`, `invalidate`, `tombstone` via delete).

## When Things Go Wrong — Concrete Playbooks
- **Corrupt line in JSONL**: Current `LoadAll` skips bad unmarshal lines (with a log). For strict mode in the future we can surface. Recovery: restore from the most recent clean snapshot before the bad line, then re-apply the good operations after it (manual or scripted).
- **SQLite locked**: The JSONL path still works. Kill the locking process or wait. On reopen, recovery will run.
- **Drift after crash or partial write**: Delete .db and reopen (or let CheckAndRecover do it). Then fsck.
- **Policy violations after an RLM run**: The snapshot is still valid history. Fix the live graph with targeted `relate` / `update` to satisfy governance (add the missing Review or `governed_by`), then snapshot the corrected state.
- **Accidental delete of graph.jsonl**: Restore from the most recent snapshot in `snapshots/`. All history before that point is gone unless you have offsite backups.

## Commands Cheat Sheet
See `ontology <cmd> --help` for full flags (all respect `--graph`/`--db`/`--schema`).

Common:
- create / update / delete / invalidate / relate
- get / list / search / query / history
- validate [--strict] [--type Plan]
- fsck [--check-invalidate]
- embed (force embedding work)

Inside drover-org: prefix with `drover-ont`.

## Related Documents
- `beads.json` + `beads.jsonl` (bea_ont_008 complete for this guide; bea_ont_010 for RLM; the .jsonl is a compact line-per-bead export useful for RLM ingestion and tooling)
- `PRODUCTION_READINESS_SCORECARD.md`
- `TESTING_STRATEGY.md` (generative / PBT expectations)
- `docs/rlm-integration.md` + `tools/rlm-ontology/` (large-scale ingestion)
- `CONTEXT.md` (domain language)
- `references/drover-schema.yaml` + `taxonomy-relations.md`
- `INTEGRATION.md` and `SKILL.md` (how agents must use the binary)

---

**bea_ont_008 complete.** This guide was expanded and reviewed for accuracy against `storage.go` (CheckAndRecover, Load fidelity via reducer replay, Apply best-effort policy), `commands/fsck.go`, `commands/validate.go`, `internal/ontology/ontology.go` (IntegrityCheck + ValidateFull), and `reducer.go`. All mutations must still go through the official binary / `Ontology` type.

Update this doc from real usage. When something surprising happens in production or in a big RLM run, add the exact symptoms + the recovery steps that worked.
