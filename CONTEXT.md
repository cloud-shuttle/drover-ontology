# Ontology — domain language

Typed **context graph** and **knowledge base** for agent memory — not a ticket system. Captures whatever the agent is working on: a consulting engagement, workshop series, conference, product, or internal research program.

## What this is

- A **graph of entities and relations** agents can query, validate, and extend over time
- **Append-only audit trail** (`graph.jsonl`) so context is reproducible and never silently lost
- **Hybrid search** (semantic + graph) for “what do we know about X?”
- **Optional workflow layer** (plans, reviews, beads, tickets) when delivery discipline is needed — secondary to knowledge capture

## Core Concepts

- **Entity**: Node in the graph — id, type, properties, relations, temporal validity.

- **Relation**: Directed edge — structural (`part_of`, `contains`), semantic (`covers`, `teaches`, `references`), social (`member_of`, `serves`), or delivery (`implements`, `depends_on`).

- **Operation**: Append-only mutation (create, update, relate, unrelate, invalidate).

- **Concept / Term / Taxonomy**: Vocabulary and topic layer — how the graph encodes *meaning*, not just files and tasks.

- **Document / Note / Message / Thread**: Evidence and narrative — interviews, runbooks, slide decks, session notes, chat excerpts.

- **Person / Organization / Event / Location**: Social and temporal context — clients, speakers, conference sessions, venues.

- **Project / Goal**: Scope and intent — engagement, workshop series, conference program, product initiative.

- **Project (scoping)**: Partition the graph (`kind`: consulting, workshop_series, conference, product, …) without product-specific skill logic.

## Optional workflow types

When workflow schema is active: **Bead**, **Ticket**, **Plan**, **Review**, **Change** — for structured plan → review → execute loops. Use when you need gates; skip when you only need a knowledge base.

## Architectural principles

One module owns graph lifecycle (`internal/ontology`). Callers use a small CLI: create, relate, search, query, validate, fsck. Skills declare read/write **contracts** so many agents share one context graph safely.

## Storage and Integrity Terms

- **Durable audit log**: The append-only, line-delimited stream of mutations (`graph.jsonl`) that serves as the absolute source of truth for the context graph.

- **Query acceleration layer**: An ephemeral, read-optimized projection of the graph used to enable high-performance query, search, and validation. It is always fully reproducible by replaying the durable audit log.

- **Integrity check (fsck)**: The validation process that verifies consistency between the query acceleration layer and the latest state of the durable audit log.

## Architecture Review Terms (from improve-codebase-architecture process)
- **OperationReducer** (`applyOperation` in reducer.go): The single pure(ish) implementation of Operation folding logic. All create/update/relate/unrelate/invalidate/tombstone semantics for the entities map live here. Log.LoadAll and Ontology.applyToMemory (and thus live Apply) delegate to it. This was the primary target of Candidate 01 in the architecture review. Single source of truth for mutation semantics (greatly improves replay fidelity and Locality).
- **Load fidelity contract (B)**: Chosen contract for Storage.Load: must return complete, queryable current state (entities + current relations) or error. On relations query failure, fall back to pure log replay so callers (Ontology, IntegrityCheck, policy validation) always observe consistent fidelity. Contrasts with current best-effort partial projection.
- **Replay drift**: Observable divergence between state produced by live Apply + applyToMemory vs pure Log replay on Close+reopen. Surface by stateful rapid PBT + captured in deterministic TestRegression_ReplayMismatch_* seeds. Primary target of Candidate 01.

## Ingestion & Long-Context Terms (RLM integration)
- **RLM (Recursive Language Model)**: A model that receives a tiny prompt + a full REPL environment where a massive context (entire monorepo + company docs) lives as a normal variable. The model writes Go code to explore, chunk, parse (via go/ast helpers), recurse via sub-LLM calls, and mutate external state. Terminates with FINAL_ONTOLOGY(...). See docs/rlm-integration.md and docs/rlm-system-prompt.md.
- **CodebaseLoader + CodeAnalysis**: The drover-aware directory walker and the REPL-exposed helpers (`ListFiles`, `GetFileContent`, `ParseGoFile`, `FindReferences`, `LoadBeadsJSON`, etc.) that live in `internal/ingest/`. These are what the RLM "sees" and calls from inside its Go interpreter.
- **DroverClient**: The bridge registered into the RLM REPL. Provides `DroverCreate` / `DroverRelate` etc. It can delegate directly to `*Ontology` (high-level API) for trusted runs or exec the real `ontology`/`drover-ont` binary (the CRITICAL RULE path for untrusted model-generated code). Never creates a parallel graph.

Patterns by domain: `references/knowledge-patterns.md`.

