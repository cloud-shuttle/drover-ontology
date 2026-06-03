# Production Readiness Scorecard

**Project**: drover-ontology  
**Purpose**: Lightweight, living table to track production readiness over time.  
**Update Cadence**: Every 1–2 weeks during active development, or after major changes.

---

## How to Update This Scorecard

1. Review the dimensions below.
2. Update the **Current Score** (1–10) based on evidence (tests passing, bugs found/fixed, etc.).
3. Add or update **Notes** with concrete observations.
4. Update the **Overall Score** at the top.
5. Add a new row to the **History** table at the bottom with today's date.
6. Link any relevant work (e.g. Phase 0 fixes, new tests) in the notes.

Keep entries factual and concise.

---

## Current Snapshot

**Overall Score**: **5.8 / 10**  
**Last Updated**: 2026-06-03  
**Trend**: Improving (Phase 0 durability + core PBT hardening + RLM ingestion layer + Operations guide complete)

| Dimension                        | Current | Target | Trend     | Key Gaps / Risks                              | Recent Progress / Notes |
|----------------------------------|---------|--------|-----------|-----------------------------------------------|---------------------------|
| Core Durability & Correctness    | 3.5     | 8.7    | ↑         | Silent DB writes, incomplete in-memory view   | Fixed most `_, _ =` . applyToMemory expanded. 01+05: unified reducer + Load fallback to replay on relations failure (Contract B). |
| Architectural Quality            | 7.8     | 8.5    | ↑         | Schema/validation seam unwired                | 01+02+03 executed (reducer, delete Graph, wired Validator into Ontology + Validate/Integrity). SchemaPath respected. |
| Testing & Verification           | 4.2     | 8.0    | ↑         | Low coverage, weak invariant testing          | 27% coverage. Fuzz + hardened replay PBT + new deterministic regression tests from actual rapid failing seeds (TestRegression_ReplayMismatch_*). Drift has executable guard. |
| Error Handling & Robustness      | 2.5     | 8.0    | ↑         | Many ignored errors on writes                 | Major cleanup started in Phase 0. Still incomplete in some paths. |
| CLI / Command Surface            | 5.0     | 8.5    | →         | Stub commands (`delete`, `history`, `validate`) | Core commands (create/get/search/relate) functional. |
| Domain Policy Enforcement        | 4.0     | 8.0    | →         | Policies partially implemented + untested     | Basic validation exists. Full Drover rules need work. |
| Documentation & Observability    | 5.5     | 8.0    | →         | Weak ops/runbook content                      | Good README + new strategy docs. Missing recovery guides. |
| Deployability / Installability   | 7.0     | 9.0    | →         | Minor gaps in upgrade/migration docs          | Strong `make install` + script support. |
| Generative Testing Maturity      | 7.5     | 8.5    | ↑         | Early stage but growing fast                  | 3 fuzz targets live. Core replay PBT hardened + deterministic regression tests from actual failing seeds (TestRegression_ReplayMismatch_*) now guard the known drift. |

**Scoring Guide** (use consistently):
- 1–3: High risk / broken in important ways
- 4–6: Functional but fragile or incomplete
- 7–8: Solid for most uses, some rough edges
- 9–10: Production-grade, battle-tested

---

## History

| Date       | Overall | Key Changes |
|------------|---------|-------------|
| 2026-06-01 | 5.2     | Added deterministic regression tests (`TestRegression_ReplayMismatch_TombstoneInvalidateRelate` + short variant) directly from the rapid failing seeds. These are the highest-ROI guardrails while the applyToMemory vs log replay drift is fixed. |
| 2026-06-02 | 5.4     | Executed architecture review Candidates 01 (extracted single applyOperation reducer; eliminated 4 copies of op folding) + 02 (deleted internal/ontology/graph.go + internal/core compat shims; zero behavior change). Also 05 Load fallback per grilled Contract B. Regression seeds now pass; roundtrip test fixed+strengthened; all tests/E2E green. Architectural Quality + Durability scores bumped. |
| 2026-06-02 | 5.5     | Candidate 03 wired: ... |
| 2026-06-02 | 5.6     | Candidate 04: commands boilerplate collapsed into Ontology (new CreateEntity/Relate/Update/Invalidate/SoftDelete high-level methods + getOntology/propsFromStringMap helpers). 14 cmd files deduped (nil checks, prop convert, op build, Apply, prints). Better locality. |
| 2026-06-03 | 5.7     | RLM integration spike (bea_ont_010): landed drover-aware CodebaseLoader + go/ast CodeAnalysis helpers + DroverClient (direct + ForceExec binary modes) + full adapted system prompt + docs/rlm-integration.md + runnable tools/rlm-ontology spike. RLM positioned strictly as ingestion layer targeting the existing governed store (no parallel graphs, all writes through high-level API or real binary, respects CRITICAL RULE, OperationReducer, schema policies, fsck). |
| 2026-06-03 | 5.8     | bea_ont_008 marked complete: Production Operations Guide (OPERATIONS.md) substantially expanded and polished with full backup/restore (tar + verify), fsck recovery playbooks (specific errors + steps aligned to storage.go + commands), CheckAndRecover details, RLM ops section, CI patterns, monitoring/security/performance/migration. All original acceptance criteria met + exceeded. beads.json + beads.jsonl updated. Minor cross-ref polish in related docs. |
| 2026-05-31 | 5.1     | Significant hardening of LogReplayEquivalence PBT (stateful sequence generator with ID tracking, success-only equivalence, long-run support via RAPID_CHECKS + _Long test). `make test-generative` and `make test-replay-long` targets added with CI guidance. Test now actively surfaces real replay inconsistencies. |
| 2026-05-30 | 4.8     | Initial scorecard created. Phase 0 durability fixes started (silent writes + applyToMemory improvements). New fuzz targets + first PBT live. |
|            |         | |
|            |         | |

---

## Related Documents

- [PATH_TO_PRODUCTION.md](PATH_TO_PRODUCTION.md) — Full phased plan with workstreams and exit criteria.
- [TESTING_STRATEGY.md](TESTING_STRATEGY.md) — Detailed generative testing roadmap.
- [beads.json](/beads.json) — Trackable issues with acceptance criteria (source of truth for execution).

---

*This scorecard is intentionally lightweight. Update it frequently — even small 0.5 point movements are worth recording.*