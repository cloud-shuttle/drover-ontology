# Path to Production Plan

**Project**: drover-ontology  
**Current Production Readiness**: **4.8 / 10** (Late Prototype / Early Development)  
**Target**: 8.5+ / 10 (Suitable for production use in agent/tooling environments)  
**Date**: May 2026  
**Status**: Living Document

---

## Executive Summary

The drover-ontology project has made excellent architectural progress through recent consolidation, but it is **not yet safe for production use**.

The two biggest risks are:
1. **Durability & Silent Failure** — Many database writes silently ignore errors.
2. **State Consistency** — The in-memory view (`applyToMemory`) is significantly behind the source-of-truth log replay (`LoadAll`).

This document defines a **prioritized, phased plan** to reach production readiness.

**Estimated Effort to Production**: 4–7 weeks of focused engineering (1–2 people).

---

## Current State Snapshot (May 2026)

| Dimension                        | Score   | Key Issues |
|----------------------------------|---------|------------|
| Core Durability & Correctness    | 3.5/10  | Silent DB write failures; incomplete state materialization |
| Testing & Verification           | 3.0/10  | ~27% coverage; critical invariants barely tested |
| Error Handling & Robustness      | 2.5/10  | Widespread `_, _ = db.Exec(...)` pattern |
| CLI Completeness                 | 5.0/10  | Several important commands still stubs |
| Documentation & Operations       | 5.5/10  | Missing recovery, backup, and failure mode guidance |
| Architecture & Design            | 7.5/10  | Strong (best area) |

See the full assessment in the conversation history (search for "Production Readiness Rating").

---

## Guiding Principles

1. **Safety First** — Data durability and consistency before new features.
2. **Verification Over Features** — Generative testing (fuzz + property-based) is non-negotiable for this domain.
3. **Make the Hard Things Easy** — `NewTestOntology` + rapid generators should make correctness work fast.
4. **Incremental Deliverable Value** — Every phase should leave the system meaningfully safer.

---

## Prioritized Workstreams

### Workstream A: Durability & Error Handling (Critical Path)
**Goal**: Eliminate silent data loss.

**Key Tasks**:
- Audit and fix all `_, _ = db.Exec(...)` calls in `storage.go` and `graph.go`.
- Add proper error handling + context in `hybridStorage.Apply`.
- **Decide on and document failure policy** (done — see below).
- Add basic WAL + integrity checks on startup.
- Implement `ontology fsck` or `ontology repair` command (light version).

**Priority**: **P0**

#### Decided SQLite Failure Policy (as of 2026-05-30)

**JSONL Log (source of truth)**: Hard failure. If `log.Append()` fails, the entire `Apply()` operation fails and the error is returned to the caller. This is already the current behavior and is correct.

**SQLite (query acceleration layer)**: Best effort.
- Failures during SQLite writes (after a successful log append) are logged to stderr as warnings.
- The `Apply()` call still succeeds from the caller's perspective.
- The system remains fully operational because it can always recover by replaying the durable JSONL log on the next restart.
- This prevents a temporary SQLite issue (disk full, lock contention, corruption, etc.) from taking down mutation paths.

This policy is now implemented in `hybridStorage.Apply()` and documented in the source code comments.

### Workstream B: State Consistency (Critical Path)
**Goal**: Make the live view and log replay always agree.

**Key Tasks**:
- Complete `applyToMemory` to match `Log.LoadAll` (handle `relate`, `unrelate`, `invalidate`, `tombstone`).
- Add strong property test: **Log Replay Equivalence** (already started in Phase 2).
- Add roundtrip tests between `Ontology.Apply` + restart vs pure `LoadAll`.
- Remove or clearly mark the legacy `Graph` type as deprecated.

**Priority**: **P0**

### Workstream C: Verification & Generative Testing
**Goal**: Make correctness testable and continuously validated.

**Key Tasks** (builds on existing `TESTING_STRATEGY.md`):
- Finish Phase 1 fuzz targets (already done) and run them in CI.
- Mature Phase 2 PBT:
  - Log Replay Equivalence (in progress)
  - Acyclicity soundness (`createsCycle`)
  - Drover policy enforcement (approved Plan → Review)
  - Temporal validity invariants
- Reach >65% statement coverage on `internal/ontology` with meaningful tests.
- Add mutation testing or property-based contract tests for `Storage` and `Embedder` seams.

**Priority**: **P1**

### Workstream D: Domain Policy Completeness
**Goal**: Make Drover-specific rules reliable and observable.

**Key Tasks**:
- Fully implement and test all policies from `validate.go` + `references/contracts.md`.
- Expose `ontology validate --strict` with clear error codes.
- Add policy violation metrics / events.
- Document all enforced policies in a machine-readable + human-readable form.

**Priority**: **P1**

### Workstream E: CLI Completeness
**Goal**: Eliminate "advanced implementation pending" stubs.

**Key Tasks**:
- Implement `delete` (with soft-delete via `invalidate` + tombstone semantics).
- Implement `history` (replay log for an entity or relation).
- Implement `validate` command (currently a stub).
- Improve `migrate` command with progress, dry-run, and verification.
- Add `fsck` / integrity check command.

**Priority**: **P2**

### Workstream F: Operational Readiness
**Goal**: Make the system operable in real environments.

**Key Tasks**:
- Backup & restore story (document + tooling for hybrid store).
- Graceful degradation when SQLite is unavailable.
- Structured logging + basic metrics (especially on Apply failures).
- Clear migration and upgrade guidance.
- Health check endpoint or command (`ontology health`).

**Priority**: **P2**

### Workstream G: Documentation & Developer Experience
**Goal**: Reduce time-to-confidence for users and contributors.

**Key Tasks**:
- Production operations guide (backup, recovery, corruption handling).
- Decision records for major trade-offs (hybrid storage, dummy embeddings fallback, etc.).
- Improve godoc on public types (`Ontology`, `Operation`, `Config`).
- Example-driven documentation for common agent workflows.

**Priority**: **P3**

---

## Phased Roadmap

### Phase 0: Stop the Bleeding (1–2 weeks)
**Focus**: P0 workstreams (Durability + Consistency)

- Fix all silent DB write errors
- Complete `applyToMemory` parity with `LoadAll`
- Land and harden the **Log Replay Equivalence** property test
- Add basic startup integrity checks

**Exit Criteria**:
- Zero silent write failures in normal paths
- `TestLogReplayEquivalence` runs 10,000+ cases reliably
- `go test -race ./internal/ontology` passes cleanly

### Phase 1: Make It Verifiable (2–3 weeks)
**Focus**: P1 workstreams (Verification + Policy)

- Expand generative testing (fuzz + rapid) across critical invariants
- Implement full Drover policy enforcement + tests
- Reach 60%+ coverage on core package with meaningful tests
- Add CI jobs for fuzzing and property tests (short runs)

**Exit Criteria**:
- All high-value properties from `TESTING_STRATEGY.md` have initial implementations
- `make test-generative` is part of the regular development loop

### Phase 2: Make It Complete (1–2 weeks)
**Focus**: P2 workstreams (CLI + Operations)

- Finish remaining stub commands
- Deliver basic `fsck` and recovery tooling
- Produce first version of "Production Operations Guide"

**Exit Criteria**:
- All major commands are functional (no "pending" messages)
- A new user can follow the operations guide to safely back up and recover data

### Phase 3: Production Polish (1–2 weeks)
**Focus**: P3 + hardening

- Documentation pass
- Performance & scale validation (large graphs, many relations)
- Security review (especially around untrusted input in search and JSONL loading)
- Release candidate tagging + changelog

**Exit Criteria**:
- Overall production readiness score ≥ 8.5/10
- At least one real internal team using it in production-like workloads for 2+ weeks

---

## Estimated Total Effort

| Phase     | Effort     | Parallelizable? | Dependencies |
|-----------|------------|------------------|--------------|
| Phase 0   | 1–2 weeks  | Low              | None |
| Phase 1   | 2–3 weeks  | Medium           | Phase 0 |
| Phase 2   | 1–2 weeks  | High             | Phase 0–1 |
| Phase 3   | 1–2 weeks  | Medium           | Phase 0–2 |

**Total**: **5–9 weeks** for a small focused team.

Many tasks in Phase 1+ can be parallelized once Phase 0 stabilizes the core.

---

## Success Criteria (Definition of "Production Ready")

A system is considered production-ready for agent/tooling use when:

- **No silent data loss** is possible under normal failure modes.
- **Log replay equivalence** holds under property-based testing with high confidence.
- All critical Drover policies are enforced and tested.
- A new engineer can safely operate the system using documented procedures.
- Generative tests (fuzz + PBT) are part of the regular CI and development workflow.
- Overall readiness score reaches **8.5+ / 10**.

---

## Quick Wins (Can be done in < 3 days)

1. Replace all `_, _ = db.Exec(...)` with proper error checking + logging (even if just for now).
2. Make `applyToMemory` handle `relate` and `invalidate` (big consistency win).
3. Run the existing fuzz targets for 30+ minutes each and triage findings.
4. Add a simple `ontology fsck` command that validates log replay matches current state.
5. Write the first version of the "Production Operations Guide" (even if incomplete).

---

## Risks & Open Questions

- **Performance at scale** — How does the hybrid store behave with 50k+ entities and dense relations?
- **Embedding model drift** — Long-term strategy for embedding model changes.
- **Multi-writer safety** — Current design assumes single-writer.
- **Migration from Python version** — How much real-world data exists in the old format?

---

## How to Track Progress

- Update this document after each phase.
- Link completed items back to PRs or commits.
- Track all work as small, actionable beads in [beads.json](/beads.json) (with acceptance criteria).
- Maintain a living "Production Readiness Scorecard" → [docs/PRODUCTION_READINESS_SCORECARD.md](docs/PRODUCTION_READINESS_SCORECARD.md)

---

**Next Step Recommendation**

Start with **Phase 0** immediately. The combination of fixing silent writes + completing state materialization + hardening the Log Replay property will move the needle from **4.8 → ~6.5** in a very short time and dramatically reduce the biggest risks.

---

## Phase 0 Execution Log (Live)

**Started:** 2026-05-30

### Completed in this session
- Fixed all silent DB write errors (`_, _ =`) in `hybridStorage.Apply` and `StoreEmbedding` (new primary path). Writes now return proper wrapped errors.
- Fixed the same pattern in legacy `Graph.Apply` (for consistency during transition).
- Significantly improved `applyToMemory` in `Ontology` to handle `relate`, `unrelate`, `invalidate`, and `tombstone` (now much closer to `Log.LoadAll`).
- Improved in-memory update logic in legacy `Graph.Apply` for the same operations.
- **Invalidate handling made robust**: Added full "invalidate" support to `log.LoadAll()` (critical for log replay equivalence). Also made create paths in both storage and legacy Graph respect an initial `ValidUntil` on the Entity for complete temporal semantics from creation time.
- All changes pass existing tests including the `LogReplayEquivalence` property test (which now exercises invalidate through full replay).

**Current impact**: Major reduction in silent data loss risk. In-memory view + pure log replay are now much more consistent for temporal validity (invalidate). The critical "Log Replay Equivalence" property now properly validates invalidate behavior.

- **Decided & implemented SQLite failure policy**:
  - JSONL appends = hard failure (source of truth).
  - SQLite writes = best effort (logged warnings only). Operation succeeds if log append succeeded.
  - Policy documented in code and in this document.

---

*This plan should be reviewed and updated every 2–3 weeks during active development.*