---
name: ontology
description: Typed context graph and knowledge base for agent memory — conferences, workshop series, consulting engagements, products, research, or any initiative. Capture people, organizations, documents, concepts, events, goals, and how they connect; optional workflow types for plans and reviews. Use for "remember", "what do we know about", link entities, semantic search, dependency/context traversal, and cross-skill shared state. Not a ticket tracker — work items are one optional layer.
triggers:
  - ontology
  - remember
  - knowledge graph
  - knowledge base
  - context graph
  - what do we know
  - entity
  - link to
  - conference
  - workshop
  - consulting
  - client
  - concept
---

> **Note**: Full developer reference. Agent contract: `.cursor/skills/ontology/SKILL.md`.

> **⚠️ CRITICAL RULE**
>
> **The `ontology` command must be the native Go binary.**
>
> This skill is implemented entirely in Go for maximum performance, strict type validation, and robust storage integrity.

# Ontology Skill (Developer Reference)

**The Go binary (`ontology`) is the primary and only implementation.**  
All production use, especially inside drover-org, must go through the native Go CLI (or the `drover-ont` wrapper when working across the monorepo).

**Context graph + knowledge base** for agents — append-only, validated, searchable. Models consulting engagements, workshop series, conferences, products, and research programs as **linked entities**, not just backlogs.

Work tracking (tasks, tickets, plans) is an **optional layer** when you need delivery discipline.

## Core Concept

```
Entity   = { id, type, properties, relations[], created, updated, source? }
Relation = { from_id, relation_type, to_id, properties? }
Mutation = { op: "create"|"update"|"relate"|"unrelate"|"invalidate", ... }
```

History: `memory/ontology/graph.jsonl` (append-only). Current state = fold of the log.

## Three Layers (use in this order)

| Layer | Purpose | Types |
|-------|---------|-------|
| **1. Knowledge** | What we know | Concept, Term, Taxonomy, Document, Note, Message, Thread |
| **2. Context** | Who / when / where / why | Person, Organization, Event, Location, Goal, Project |
| **3. Delivery** (optional) | How work gets done | Task, Bead, Ticket, Plan, Review, Change |

Most repos should start at layers 1–2. Add layer 3 when formal gates matter.

## Domain Patterns

See **`references/knowledge-patterns.md`** for worked examples:

- Consulting business (clients, engagements, discovery docs, `serves`, `covers`)
- Workshop series (curriculum, sessions, runbooks, learning goals)
- Conferences (talks, speakers, CFP, `teaches`, `part_of`)
- Product engineering (concepts + ADRs + optional workflow)
- Taxonomy building (Terms from CONTEXT-MAP + glossaries, `broader_term` hierarchies, `tagged_with` classification — see `references/taxonomy-relations.md`)

## Scoping (multi-initiative)

| Mechanism | Example |
|-----------|---------|
| `Project.kind` | `consulting`, `workshop_series`, `conference`, `product` |
| `part_of` / `contains` | Session → series; note → engagement |
| `covers` / `teaches` | Doc or talk → Concept |
| `references` | Note → Person, Document, prior decision |
| `related_to` | Associative navigation in the KB |
| Per-repo graph | `memory/ontology/` in each checkout |

## Entity Types

### Core — knowledge + context (`references/core-schema.yaml`)

**Knowledge:** Concept, Term, Taxonomy, Document, Note, Message, Thread, Policy  
**Context:** Person, Organization, Project, Goal, Event, Location  
**Audit:** Action  

**Key relations:** `contains`, `part_of`, `references`, `covers`, `teaches`, `related_to`, `member_of`, `serves`, `owns`, `has_owner`, `depends_on`, `blocks`

**Core taxonomy relations** (for Term hierarchies + knowledge linking in drover-org and similar): `broader_term`, `narrower_term`, `tagged_with`, `in_domain`, `governed_by` — see [references/taxonomy-relations.md](references/taxonomy-relations.md) for the full formalized set, usage patterns, **and the simple governance process** (anyone may propose with `status="proposed"` + wiring; only designated curators may promote to `accepted` after adding a `governed_by` link).

### Workflow extensions — optional (`references/drover-schema.yaml`)

Bead, Ticket, Plan, Review, Change — plus delivery relations (`implements`, `tracks`, `approves`, …).

Active schema: `memory/ontology/schema.yaml` (merge core + extensions as needed).

## Storage Layout

```
memory/ontology/
├── graph.jsonl
├── schema.yaml
├── views/
└── snapshots/
```

Never hand-edit `graph.jsonl`. Always use the CLI.

## Primary Workflows

### 1. Capture knowledge

```bash
ontology create --type Concept --props '{"label":"FinOps maturity","domain":"cloud"}'
ontology create --type Document --props '{"title":"Client workshop debrief","kind":"note","path":"..."}'
ontology create --type Note --props '{"content":"Audience skews platform engineers","tags":["workshop"]}'
ontology relate <doc_id> covers <concept_id>
ontology relate <doc_id> part_of <project_id>
```

### 2. Build social / event context

```bash
ontology create --type Organization --props '{"name":"RetailCo","kind":"client"}'
ontology create --type Person --props '{"name":"Alex","role":"CTO"}'
ontology create --type Event --props '{"title":"Cloud Shuttle kickoff","kind":"workshop","start":"..."}'
ontology relate <person_id> member_of <org_id>
ontology relate <event_id> part_of <project_id>
```

### 3. Explore context (before acting)

```bash
ontology search "stakeholder retail data" --limit 15
ontology related --id <project_id> --rel "contains,covers,references,member_of" --depth 3
ontology query --type Concept --where '{"domain":"cloud"}'
```

### 4. Validate & integrity

```bash
ontology validate --strict
ontology fsck
ontology fsck --check-invalidate
```

### 5. Delivery layer (when needed)

```bash
ontology create --type Plan --props '{"kind":"workshop","title":"Module 3","status":"draft"}'
ontology create --type Task --props '{"title":"Build lab repo","status":"open"}'
ontology relate <task_id> implements <plan_id>
```

## Skill Contracts

```yaml
ontology:
  reads: [Project, Document, Concept, Person, Organization]
  writes: [Note, Document, Action]
  postconditions:
    - "New Documents link to a Project or Event via part_of or contains"
```

Examples: `references/contracts.md`.

## Integrations (sync into the graph)

| Source | Target types |
|--------|----------------|
| Markdown docs, ADRs, runbooks | Document |
| Meeting notes, agent summaries | Note |
| Glossary / taxonomy work | Term, Concept, Taxonomy |
| Calendar / conference schedule | Event |
| CRM / contacts | Person, Organization |
| Issue trackers (optional) | Ticket |
| Review skills (optional) | Review, Plan |
| Full codebase + company docs via RLM (see docs/rlm-integration.md) | Package, Struct, Term, Plan, Bead, ... (recursive ingestion with governance) |

Provenance: entity `source` field.

## Installation

```bash
cd drover-ontology && make install-local
export PATH="$HOME/.local/bin:$PATH"
```

See the dedicated **Multi-Project / Monorepo Usage** section below for how to use the binary (and the `drover-ont` wrapper) across different projects.
## CLI Reference

```bash
ontology create | get | list | update | relate | invalidate
ontology search | query | related
ontology validate --strict | fsck [--check-invalidate]
```

Flags: `--graph`, `--db`, `--schema`.

**Durability:** JSONL authoritative; SQLite best-effort cache. Rollback = compensating append.

## Quick Start

```bash
# 1. Install the Go binary (once)
cd drover-ontology && make install-local
export PATH="$HOME/.local/bin:$PATH"

# 2. (Inside drover-org monorepo) use the wrapper for the central store
drover-ont create --type Project --props '{"name":"..."}'

# Or in a standalone project, use the plain binary against your local store
ontology create --type Project --props '{"name":"..."}'
ontology fsck
```

## References

| File | Content |
|------|---------|
| `references/core-schema.yaml` | Knowledge + context types & relations |
| `references/drover-schema.yaml` | Optional delivery workflow |
| `references/taxonomy-relations.md` | **Core formalized relations for Term hierarchies & drover-org taxonomy work** (`broader_term`, `tagged_with`, `governed_by`, ...) |
| `references/knowledge-patterns.md` | Consulting, workshops, conferences |
| `references/queries.md` | Query recipes |
| `references/contracts.md` | Skill integration |
| `references/migration.md` | Import external data |

## Implementation

| Path | Role |
|------|------|
| `SKILL.md` | This file (developer reference) |
| `.cursor/skills/ontology/SKILL.md` | Concise agent contract (what gets symlinked) |
| `cmd/ontology/` + `internal/ontology/` | **Go CLI (primary implementation — use this)** |
| `scripts/ontology.py` | Reference fallback only (do not use for real work) |

## Multi-Project / Monorepo Usage

### Inside the drover-org monorepo
Always use the `drover-ont` wrapper (usually installed at `~/bin/drover-ont`). It hard-codes the central store at:

- Graph: `~/consulting/drover-org/memory/ontology/graph.jsonl`
- DB: `~/consulting/drover-org/memory/ontology/graph.db`
- Schema: `~/consulting/drover-org/memory/ontology/schema.yaml`

This guarantees that every agent and every contributor across the entire organization writes to the same single source of truth.

### In a standalone project
Install the Go binary once:

```bash
cd drover-ontology
make install-local          # or use the one-liner installer script
export PATH="$HOME/.local/bin:$PATH"
```

Then use the plain `ontology` command against your project's local `memory/ontology/` directory.

You can always point at any store explicitly using the global flags:

```bash
ontology --graph ./memory/ontology/graph.jsonl \
         --db    ./memory/ontology/graph.db \
         --schema ./memory/ontology/schema.yaml \
         create ...
```

All agents working on drover-related work (Grok, Claude, drover-code, etc.) are expected to use the native Go binary.

## Provenance

From ClawHub (oswalpalash/ontology), extended with Go CLI, workflow types, and knowledge-graph patterns. Principles: typed entities, append-only log, validated relations, skill contracts.
