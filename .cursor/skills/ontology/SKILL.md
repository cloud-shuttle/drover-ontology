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

> **⚠️ CRITICAL RULE — READ THIS FIRST**
>
> **The `ontology` command must be the native Go binary.**
>
> - All agents and all production usage **must** use the Go CLI (`ontology` or the `drover-ont` wrapper inside the monorepo).
> - This skill is implemented entirely in Go for maximum performance, strict type validation, and robust storage integrity.
> - This rule applies to every Drover project and every agent (Grok, Claude, drover-code, etc.).

# Ontology (Agent Guide)

**The `ontology` command is the native Go binary (primary implementation).**

## Multi-Project / Monorepo Usage

### Inside the drover-org monorepo
Use the `drover-ont` wrapper (usually at `~/bin/drover-ont`). It automatically points at the single central store:

```bash
drover-ont create --type Term ...
drover-ont relate "Drover Brain" broader_term "Product Contexts"
drover-ont fsck
```

This ensures every agent and every human working across drover-org writes to the same governed ontology.

### In a standalone project
1. Install the Go binary once:
   ```bash
   cd /path/to/drover-ontology
   make install-local
   export PATH="$HOME/.local/bin:$PATH"
   ```

2. Use the plain `ontology` command against your local store:
   ```bash
   ontology create --type Project --props '{"name":"My Project"}'
   ```

You can always override the store with flags:
```bash
ontology --graph ./memory/ontology/graph.jsonl --db ./memory/ontology/graph.db ...
```

See the full reference (`drover-ontology/SKILL.md`) for the one-liner installer and system-wide install options.

## Purpose

Build and query a **validated context graph** about whatever you're working on:

- Consulting business (clients, engagements, discovery notes, recommendations)
- Workshop series (sessions, curricula, runbooks, learning goals)
- Conferences (talks, speakers, CFPs, tracks, sponsors)
- Products and migrations (architecture concepts, ADRs, stakeholders)
- Taxonomy & controlled vocabularies (Terms from CONTEXT-MAP, `broader_term` trees, governance via `governed_by`)
- Anything else worth remembering as **typed, linked facts**

Ticket/plan/bead types exist for delivery workflow when needed — they are **not** the primary model.

## Core Mental Model

- **Entity** = typed node (Person, Organization, Project, Document, Note, Concept, Event, Goal, Term, …)
- **Relation** = typed edge (`covers`, `references`, `part_of`, `broader_term`, `tagged_with`, `governed_by`, `related_to`, `in_domain`, …)
- **Append-only log** = `memory/ontology/graph.jsonl` (source of truth)
- **Search** = semantic + graph (`ontology search`, `related`, `query`)

Scope initiatives with **Project** (`kind`: consulting, workshop_series, conference, product, …).

## Essential Commands

```bash
ontology create --type Organization --props '{"name":"Acme","kind":"client"}'
ontology create --type Project --props '{"name":"Q3 discovery","kind":"consulting","status":"active"}'
ontology create --type Concept --props '{"label":"Data mesh","definition":"..."}'
ontology create --type Document --props '{"title":"Interview notes","kind":"brief","path":"..."}'
ontology create --type Note --props '{"content":"Speaker prefers morning slots","tags":["conference"]}'
ontology create --type Event --props '{"title":"Module 2 lab","kind":"workshop","start":"2026-06-01T14:00:00Z"}'

ontology relate <doc_id> part_of <project_id>
ontology relate <doc_id> covers <concept_id>
ontology relate <person_id> member_of <org_id>
ontology relate <event_id> teaches <concept_id>
ontology relate "Drover Code" broader_term "Platform"
ontology relate <bead_id> tagged_with "ASDL C"
ontology search "stakeholder map retail" --limit 10
ontology related --id <project_id> --rel "contains,covers,references,broader_term" --depth 3

ontology validate --strict
ontology fsck
```

Global flags: `--graph`, `--db`, `--schema`.

## Agent Habits

| Situation | Action |
|-----------|--------|
| Durable fact from conversation | `Note` or `Document` + link to `Project` / `Concept` |
| New person or org | `Person` / `Organization` + `member_of` / `serves` |
| File worth re-finding | `Document` with `path` + `covers` topics |
| New initiative | `Project` with `kind` + `Goal` entities |
| Before deep work | `search` + `related` on key concepts |
| Formal delivery needed | Add workflow types (Plan, Review, Task) — see full SKILL |
| After creating a Bead/Plan/Project | Tag it with relevant Terms under the "Drover Core Concepts" facets using `tagged_with` (and structural relations like `implements`/`part_of`) |

## Entity Layers

| Layer | Types | Role |
|-------|-------|------|
| **Knowledge** | Concept, Term, Taxonomy, Document, Note, Message | What we know (use Term for controlled vocabularies & hierarchies) |
| **Context** | Person, Organization, Event, Location, Goal | Who / when / where / why |
| **Scope** | Project | Partition the graph |
| **Delivery** (optional) | Task, Bead, Plan, Review, Ticket, Change | How work gets done |

## Rules

- **Use the Go binary only**: All mutations and queries must go through the `ontology` Go CLI (or `drover-ont` inside the monorepo).
- Mutations only via `ontology` CLI — never hand-edit `graph.jsonl`.
- Prefer knowledge types first; add workflow types when gates matter.
- Do not assume product-specific repo paths unless they exist in the open workspace.
- Domain recipes: `references/knowledge-patterns.md`.
- For Term hierarchies & drover-org taxonomy work (broader_term, tagged_with, governed_by, in_domain): see full SKILL `references/taxonomy-relations.md`.
- **Governance rule for Terms**: You (as agent) may *propose* new Terms with `status="proposed"` and wire them (`broader_term` + `tagged_with "Drover Core Concepts"`). You **must not** set `status="accepted"` yourself. Only a human Taxonomy Curator may do that, after they (or you on their behalf) add a `governed_by` link to a curator Person. The initial curator is Peter.

## When to Use

- Capture or retrieve structured context
- Link people, docs, topics, and events
- Build Term hierarchies (`broader_term`) and classify work (`tagged_with`) for drover-org taxonomy — especially tag Beads, Plans, and Projects to the six main facets of "Drover Core Concepts"
- Answer “what do we know about X?”
- Share memory across skills with validation
- Optional: track plans/reviews/tasks on top of the same graph

Full reference: `drover-ontology/SKILL.md`.
