# Testing Strategy: Property-Based & Fuzz Testing

**Status**: Proposed (2026-05)  
**Owner**: Ontology team  
**Goal**: Protect the most critical invariants in the ontology engine using generative testing techniques.

## Executive Summary

The `drover-ontology` core contains several high-risk, high-complexity areas that are difficult to adequately cover with traditional example-based unit tests:

- Append-only log as source of truth + hybrid SQLite materialization
- Graph acyclicity and schema/policy validation
- Temporal validity semantics
- Vector similarity mathematics

This document defines a concrete, prioritized plan to introduce **native Go fuzz testing** (`testing.F`) and **property-based testing** (using `github.com/flyingmutant/rapid`) to these areas.

The plan is designed to deliver maximum risk reduction with minimal new dependencies.

## Why This Matters for This Codebase

| Characteristic | Risk Level | Why Traditional Tests Struggle |
|----------------|------------|--------------------------------|
| Append-only log is source of truth | Critical | Must prove replay equivalence under arbitrary valid/invalid operation sequences |
| `createsCycle` DFS algorithm | High | Exhaustive manual cases for graphs are impractical |
| Drover domain policies (e.g. approved Plan → Review) | High | Combinatorial explosion of valid/invalid states |
| `Apply` + in-memory view + storage | High | `applyToMemory` is currently a simplified subset of `Log.LoadAll` |
| Pure vector math functions | Medium | Floating point + string-generated vectors have many edge cases |

## Prioritized Areas & Concrete Properties

### 1. Log Replay & State Consistency (Highest Priority)

**Core Invariant**: The JSONL log is the durable source of truth. Any sequence of operations must be replayable to produce equivalent state.

#### Property 1.1: Full Replay Equivalence
**Statement**:  
For any sequence of `Operation` values, the entity map produced by `Log.LoadAll()` after writing those operations must be equivalent (modulo non-deterministic timestamps) to the state obtained by:
1. Creating a fresh `Ontology` via `NewTestOntology`
2. Calling `Apply(op)` for each operation
3. Closing and re-opening from the same paths

**Suggested Generator**: Sequence of operations (create, update, relate, invalidate) with varying validity.

#### Property 1.2: Append is Durable
**Statement**:  
After a successful `storage.Apply(op)`, the operation must appear in a subsequent `Log.LoadAll()` even after process restart (simulated via new `Log` instance on same path).

#### Property 1.3: Log-Only Mode Consistency
**Statement**:  
When `UseSQLite: false`, state loaded via `hybridStorage.Load()` must match the result of `Log.LoadAll()`.

**Implementation Location**: `internal/ontology/log_replay_test.go` (new file using `rapid`).

### 2. Graph Validation & Acyclicity

#### Property 2.1: Acyclicity Soundness
**Statement**:  
`createsCycle(fromID, toID, rel, entities)` returns `true` **if and only if** adding the directed edge under that relation type would create a cycle in the current graph.

**Status**: ✅ Implemented in `validate_properties_test.go` (TestAcyclicitySoundness)
- Uses a clearly correct reference implementation (hypothetical edge + full DFS).
- Generates random graphs + candidate edges.
- Actively exercises the real `createsCycle` function used by validation.

### 3. Drover Domain Policies

#### Property 3.1: Approved Plan Requires Review
**Statement**:  
After any sequence of operations, if a `Plan` entity has `status == "approved"`, then `ValidateFull` (and `Ontology.Validate()`) must report no policy violation **only if** there exists at least one `approves` relation from a `Review` entity to that Plan.

**Status**: ✅ Implemented in `validate_properties_test.go` (TestApprovedPlanRequiresReview)
- Uses `NewTestOntology` + realistic sequences of create Plan/Review + relate "approves" + status updates.
- Asserts correct violation reporting via the shared `validateApprovedPlanRequiresReview` policy function.
- Also fixed a directionality bug in the original policy checker (relation is stored on the Review).

**Additional Policies** (future / partially done):
- Acyclicity is enforced on `depends_on` and `blocks` (soundness PBT + runtime validation)
- Required properties per type (lightweight enforcement in ValidateEntity)
- Term governance (`status=accepted` requires `governed_by`) — runtime policy + dedicated tests in `governance_test.go` + incidental coverage in the Plan PBT

### 4. Vector Mathematics (Fuzz + Light PBT)

#### Fuzz Targets
- `generateDummyVector(text)` with adversarial strings (very long, Unicode, control characters, empty).
- `cosineSimilarity(a, b)` with vectors containing `NaN`, `Inf`, zero-length, mismatched lengths, all-zero vectors.

#### Properties
- `cosineSimilarity(v, v) ≈ 1.0` for any non-zero vector `v` returned by `generateDummyVector`
- `cosineSimilarity(v, -v) ≈ -1.0`
- Result is always in `[-1.0, 1.0]` (within floating point tolerance)

**Implementation Location**: `internal/ontology/vector_fuzz_test.go` + properties in the same file or `math_properties_test.go`.

### 5. Temporal Validity Semantics

#### Property 5.1: Invalidated Entities Are Invisible
**Statement**:  
Once an entity has a `ValidUntil` in the past, it must not appear in results from `HybridSearch` (the primary public query surface that promises temporal filtering).

**Status**: ✅ Implemented in `validate_properties_test.go` (TestInvalidatedEntitiesInvisible)
- Exercises create + invalidate sequences.
- Verifies that invalidated entities no longer appear in HybridSearch results.
- Note: As of implementation, `All()` and `Get()` return raw entities (including invalidated ones). HybridSearch is the documented filtering surface.

---

## Recommended Tooling

| Technique | Tool | Why | Go Version Support |
|-----------|------|-----|--------------------|
| Fuzzing | `testing.F` (stdlib) | Mature since 1.18, excellent corpus management, no deps | 1.26 (current) |
| Property-Based Testing | `github.com/flyingmutant/rapid` | Best modern Go PBT library. Good shrinking, state machine support, active maintenance | All recent Go versions |

**Do not use** `testing/quick` for anything beyond toy examples.

Future option: `github.com/leanovate/gopter` if we need a more classic QuickCheck style.

---

## Phased Implementation Roadmap

### Phase 0 — Foundations (Already Done)
- [x] `NewTestOntology(t)` helper with automatic temp dir + cleanup
- [x] Basic smoke test in `ontology_test.go`

### Phase 1 — Fuzzing (Low effort, high value) — **Status: ✅ Complete**
1. ✅ `internal/ontology/log_fuzz_test.go` — `FuzzLogLoadAll` (highest priority target)
2. ✅ `internal/ontology/vector_fuzz_test.go` — `FuzzVectorMath`
3. ✅ `internal/ontology/operation_fuzz_test.go` — `FuzzOperationRoundtrip`

All targets are live and compile cleanly.

**How to run**:
```bash
go test -fuzz=FuzzLogLoadAll      -fuzztime=30s ./internal/ontology
go test -fuzz=FuzzVectorMath      -fuzztime=20s ./internal/ontology
go test -fuzz=FuzzOperationRoundtrip -fuzztime=15s ./internal/ontology
```

**Success Metric**: At least one interesting crash or useful corpus entry within the first week of running.

### Phase 2 — Core PBT Properties (Highest leverage) — **In Progress**
- ✅ Added `pgregory.net/rapid` as test dependency
- ✅ Implemented **Property 1.1 (Log Replay Equivalence)** in `replay_properties_test.go`
- ✅ Major generator hardening: stateful sequence generator that tracks created entity IDs → far more realistic relate/invalidate/unrelate/tombstone behavior.
- ✅ Improved error handling in the property (only successful `Apply()` operations are required to replay equivalently).
- ✅ Long-run support: `RAPID_CHECKS` environment variable + `TestLogReplayEquivalence_Long` variant.
- ✅ Integrated into `make test-generative` and dedicated `make test-replay-long` target.
- The test is now actively finding real replay inconsistencies (entity count, relation, and property drift) — strong validation signal.
- ✅ Added deterministic `TestRegression_ReplayMismatch_*` tests (from the exact rapid failing seeds) as permanent regression guards.

Next steps in this phase:
- Run at 5,000–10,000+ cases reliably (use `make test-replay-long RAPID_CHECKS=10000`).
- Add Property 2.1 (Acyclicity soundness).
- Root-cause and eliminate the remaining applyToMemory vs log replay drift (the regression seeds will turn green / can be removed when done).

**Success Metric**: The replay property runs thousands of cases reliably in CI and would have caught recent `applyToMemory` vs `LoadAll` drift.

### Phase 3 — Domain Policies & Temporal Rules — **Target: 2-4 days** (largely complete)
- ✅ Property 3.1 (Approved Plan Requires Review) — rapid PBT + shared policy implementation
- ✅ Property 5.1 (Invalidated Entities Invisible) — rapid PBT
- Term governance policy implemented + unit + incidental PBT coverage
- Runtime validation wired into `Ontology.Validate()` (via the Validator seam) and IntegrityCheck (on replayed state)

### Phase 4 — Math + Search Properties + Continuous Fuzzing
- Vector math properties
- Lightweight search ranking properties
- Long-running fuzz corpus in CI

---

## Integration with Existing Code

### Using `NewTestOntology`

All new generative tests should prefer `NewTestOntology(t)` (or small variants) for isolation:

```go
func TestLogReplayEquivalence(t *rapid.T) {
    ont := ontology.NewTestOntology(t) // temp files + cleanup for free

    // generate and apply operations
    // ...
}
```

We may need one small addition:

```go
// NewTestOntologyFromPaths allows reopening the same temp paths
// in a new process simulation for replay tests.
```

### Test File Naming Convention

- `*_fuzz_test.go` → native fuzz targets (can be run with `-fuzz`)
- `*_properties_test.go` or `*_rapid_test.go` → property-based tests

---

## Open Questions & Risks

1. **State Machine Generators**: `rapid` has good support, but complex `Operation` sequences may require careful modeling of validity (some operations are only legal on certain states).
2. **Floating Point Tolerance**: Need consistent epsilon strategy for cosine similarity properties.
3. **Performance**: Long PBT runs + SQLite may be slow. Consider in-memory storage seam for some properties.
4. **Maintenance**: Generative tests can be brittle if generators are too powerful. Start narrow.

---

## CI Integration

Add the following to your repository's CI (recommended pattern).  
The actual workflow used by this repo lives at `.github/workflows/test-generative.yml`.

```yaml
# Recommended structure (see the real file for the latest version)
name: Generative Tests

on:
  workflow_dispatch:   # Manual trigger (recommended for long runs)
  schedule:
    - cron: '0 6 * * 1'   # Weekly deep run (Monday morning)

jobs:
  generative:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Run bounded generative tests
        run: make test-generative

      - name: Deep replay property verification (long run)
        run: make test-replay-long RAPID_CHECKS=8000
        # Increase RAPID_CHECKS for even deeper verification on manual/scheduled runs
```

**Notes**:
- Full unbounded fuzzing (`-fuzztime` very high) is best left for local machines or dedicated long-running jobs.
- The `TestLogReplayEquivalence_Long` variant + `RAPID_CHECKS` env var gives you controllable depth.
- Consider caching Go modules for faster CI runs.

## How to Get Started (Immediate Next Steps)

1. Run `make test-generative` locally (safe, bounded fuzz + properties).
2. Run `make test-replay-long RAPID_CHECKS=5000` to exercise the core replay property deeply.
3. Add the recommended generative CI job (see **CI Integration** section above).
4. Feed findings back into this document and the relevant beads.

---

**References**
- Current only test: `internal/ontology/ontology_test.go`
- Key functions under test: `Log.LoadAll`, `hybridStorage.Apply`, `createsCycle`, `ValidateFull`, `cosineSimilarity`
- Existing test helper: `NewTestOntology` (see `seams.go` and `ontology.go`)

---

*This document is intended to be a living artifact. Update it as properties are implemented or invalidated.*

---

**Related Documents**
- [PATH_TO_PRODUCTION.md](PATH_TO_PRODUCTION.md) — Broader plan to reach production readiness (this testing work is a major component of Phase 0 and Phase 1).
- [PRODUCTION_READINESS_SCORECARD.md](PRODUCTION_READINESS_SCORECARD.md) — Track overall progress with the lightweight living scorecard.
- `Makefile` targets: `test-generative` and `test-replay-long` (with `RAPID_CHECKS` support).