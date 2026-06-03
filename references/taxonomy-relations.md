# Core Relations for Taxonomy & Knowledge Graphs

**Purpose**: A small, canonical, reusable set of relations for building and navigating **Term** hierarchies and linking knowledge artifacts (Documents, ADRs, Beads, Plans, Concepts) in the Drover org.

These relations are the primary mechanism for turning the flat list of 37+ Terms (populated from `CONTEXT-MAP.md`, per-repo glossaries, and `docs/taxonomy.yaml`) into a living, queryable knowledge graph.

Use the `ontology` CLI (`relate`, `related`, `search`, `query`) against the central store at `drover-org/memory/ontology/`.

## Design Principles (small set)

- **Hierarchy first**: One primary broader/narrower mechanism.
- **Classification over free tagging**: `tagged_with` + well-governed Term entities beats ad-hoc string properties or `domain` relations.
- **Governance explicit**: Relations (not just status) record how Terms become accepted.
- **Reuse over invention**: Prefer `part_of`, `references`, `covers` from the core schema when they fit.
- **Validation where it matters**: Acyclicity on hierarchy; type constraints on governance links.
- **Invertible where useful**: `broader_term` has an optional explicit inverse `narrower_term`.

## The Core Set (6 relations)

| Relation       | From Types                  | To Types                     | Acyclic | Cardinality (recommended) | Purpose & Example |
|----------------|-----------------------------|------------------------------|---------|---------------------------|-------------------|
| `broader_term` | Term                        | Term                         | Yes     | many-to-1                 | "A is narrower than / a kind of B". Primary hierarchy edge.<br/>`Drover Code` broader_term `Platform` |
| `narrower_term`| Term                        | Term                         | Yes     | 1-to-many                 | Explicit inverse of broader_term (optional but encouraged for downward queries).<br/>`Platform` narrower_term `Drover Code` |
| `tagged_with`  | Term, Concept, Document, Bead, Plan, Review, Note | Term | No | many-to-many | Cross-cutting classification / facet. Replaces misuse of `domain` as a relation.<br/>`Bea_98e4249` tagged_with `ASDL C`<br/>`SomeTerm` tagged_with `governance` |
| `in_domain`    | Term                        | Term (domain roots)          | No      | many-to-1                 | Explicit domain scoping for Terms (clean replacement for the ad-hoc `domain` relation seen in early data).<br/>`ASDL C` in_domain `governance` |
| `related_to`   | Term, Concept, Document     | Term, Concept, Document      | No      | many-to-many              | Associative "see also" / navigational link. Symmetric in practice.<br/>`Bounded Context` related_to `ADR` |
| `governed_by`  | Term                        | Person, Review, Policy       | No      | many-to-many              | Records accountability / acceptance process for a Term. Directly supports the schema policy: "Term.status=accepted requires review by taxonomy skill or designated curator".<br/>`Drover Muster` governed_by `Peter` |

### Strongly Recommended Companion Relations (from core-schema)

These are already formalized and should be used heavily when linking **work** to the taxonomy:

- `part_of` / `contains` — Structural scoping (e.g. a specific ADR Document part_of a Project; a Term part_of a Taxonomy root).
- `references` — Explicit citation (ADR references a Term it defines or relies on).
- `covers` — A Document / Event / Plan addresses or teaches the Term.
- `implements` — A Bead / Ticket / Change delivers or realizes the Term (e.g. a new capability described by a Term).
- `owns` / `has_owner` — Accountability for Terms or the Taxonomies that contain them.

## Usage Patterns for Drover-org

### 1. Building the Primary Hierarchy (broader_term)

```bash
# From the drover-org root (using drover-ont wrapper or direct with --graph/--db)
ontology relate "Drover Code" broader_term "Platform"
ontology relate "Drover Brain" broader_term "Platform"
ontology relate "ASDL C" broader_term "governance"
ontology relate "Workflow step" broader_term "Platform workflow"
```

Query downward:
```bash
ontology related --id "Platform" --rel "narrower_term,broader_term" --depth 2
```

### 2. Cross-cutting Classification (tagged_with + in_domain)

```bash
# Replace the early ad-hoc "domain" relation
ontology relate "ASDL C" in_domain "governance"

# Tag work items and artifacts
ontology relate "Bea_98e4249" tagged_with "ASDL C"
ontology relate "some-adr-doc-id" tagged_with "governance-policy"
```

### 3. Governance Trail (governed_by)

```bash
ontology create --type Person --props '{"name":"Peter Hanssens","role":"taxonomy-curator"}'
ontology relate "Drover Muster" governed_by "per_..."   # or link a Review entity later
ontology update "Drover Muster" --props '{"status":"accepted"}'
```

This makes the "accepted" policy auditable.

### 4. Linking Concrete Work to Concepts

```bash
# An ADR that defines or relies on a Term
ontology relate "adr-0007-dbos-placement" references "DBOS runtime owner"
ontology relate "adr-0007-dbos-placement" covers "Orchestrator workflow"

# A Bead implementing a defined concept
ontology relate "Bea_new_provisioner" implements "Platform workflow"
ontology relate "Bea_new_provisioner" tagged_with "provisioning"
```

### 5. Exploration Recipes

```bash
# What narrower concepts exist under Platform?
ontology related --id "Platform" --rel "narrower_term" --depth 1

# All work tagged with governance concerns
ontology query --type Bead --where '{"tagged_with":"governance"}'   # (or use related + filter)

# Context around a Term before writing an ADR
ontology search "DBOS" --limit 10
ontology related --id "DBOS runtime owner" --rel "broader_term,related_to,references,covers" --depth 2
```

## Governance & Lifecycle for Relations Themselves

- New Terms start with `status: proposed` (per Term type in schema).
- Promotion to `accepted` should be accompanied by at least one `governed_by` link (or a linked Review with `approves`).
- Hierarchy changes (`broader_term`) should be reviewed for acyclicity (enforced by the validator when the relation is registered in schema).
- `tagged_with` and `related_to` are intentionally loose — use them for discovery, not hard contracts.
- Deprecated Terms: set `status: deprecated` + add `related_to` to the replacement Term. Do not delete edges; use `invalidate` on the relation if truly erroneous (rare).

## Migration Notes (Early Data)

Early population of the central `drover-org/memory/ontology/` graph used a few ad-hoc relations:

- `domain` (Term → string-ish target) → migrate to `in_domain` (Term → Term) or `tagged_with`.
- `part_of` used on product slugs (drover-code etc.) → these are reasonable for now; consider promoting the product slugs to proper `Project` entities + `part_of` or introducing a `Product` Term type.
- Duplicate `part_of` entries for `drover-ontology` → clean via `invalidate` + re-relate if needed.

Run `ontology fsck --check-invalidate` after any bulk cleanup.

## Full Schema Registration

See:
- `references/core-schema.yaml` (portable knowledge layer)
- `references/drover-schema.yaml` (adds workflow + Term governance extensions)
- `memory/ontology/schema.yaml` (active for the central drover-org store)

The relations above are registered with `from_types`, `to_types`, `acyclic`, and `description` so that `ontology relate` and `ontology validate --strict` enforce them.

## Future Small Extensions (do not add lightly)

Only when real usage demonstrates pain:

- `synonym_of` (for alias Terms)
- `replaces` (deprecation path with stronger semantics than `related_to`)
- `scope_of` (Term → Project or Taxonomy root)

Keep the set small. Most value comes from consistent use of the six above plus the four core companions (`part_of`, `references`, `covers`, `implements`).

## Live Top-Level Structure in drover-org

The central ontology at `drover-org/memory/ontology/` now contains a top-level **Taxonomy** entity:

- **"Drover Core Concepts"** (ID: `Drover Core Concepts`, scheme: `drover-org/v1`)
  - The official top-level Taxonomy **container** for drover-org.
  - `root_term`: Product Contexts
  - `contains` the six main facets (explicitly defined with mappings to `docs/taxonomy.yaml` and CONTEXT-MAP.md):
    - **Product Contexts** — bounded contexts/services (maps to `product` facet)
    - **Roles & Actors** — humans + agents (maps to `audience` facet)
    - **Documentation Types** — Diátaxis + ADR (maps to `doc_type` facet)
    - **Cross-cutting Topics** — shared concerns (maps to `topic` facet)
    - **Publication Surfaces** — publishing locations (maps to `surface` facet)
    - **Workflow & Runtime Concepts** — durable execution (practical extension for Kelpies, DBOS, etc.)

Hundreds of concrete Terms (the original 37 + new facet containers) are organized underneath the facets using `broader_term` (the primary hierarchy relation). This turns the flat list of Terms into a navigable tree aligned with `docs/taxonomy.yaml` facets and `CONTEXT-MAP.md`.

Explore with:
```bash
drover-ont get "Drover Core Concepts"
drover-ont list Term
# Use SQLite or future `related` command for tree traversal under the facets
```

See the actual relations table or run integrity checks with `drover-ont fsck`.

## Governance Process for Terms (Simple & Agent-Friendly)

The goal is **low-friction proposal + accountable acceptance** so the taxonomy stays high-quality without becoming a bottleneck.

### Roles
| Role              | Who                                      | Powers                                      |
|-------------------|------------------------------------------|---------------------------------------------|
| **Proposer**      | Any human in drover-org or any agent (Grok, Claude, drover-code, etc.) following this skill | Create Terms with `status="proposed"`, wire `broader_term` + `tagged_with`, suggest `governed_by` links |
| **Taxonomy Curator** (Acceptor) | Designated humans (initially **Peter Hanssens**). Domain owners may be added later for their bounded context. | Promote `status` to `accepted` (only after adding a `governed_by` link to themselves or a Review) |
| **Taxonomy skill / agents** | Automated agents                         | Propose, validate, run fsck, suggest structure — **never** unilaterally accept |

The role itself is modeled in the graph as the Term **"Taxonomy Curator"** (under Roles & Actors, tagged to Drover Core Concepts). Peter is currently the only person tagged with it.

### The 5-Step Process

1. **Discover**
   - New concept appears in `CONTEXT-MAP.md`, a repo `CONTEXT.md`, `docs/taxonomy.yaml`, an ADR, meeting notes, or agent conversation.

2. **Propose** (anyone)
   ```bash
   drover-ont create Term "My Concept" \
     -p label="My Concept" \
     -p definition="One clear sentence. What it is, not what it does." \
     -p status="proposed" \
     -p domain="asdlc|product|infra|governance" \
     -p aliases="old name, synonym"

   # Immediately place it in the living taxonomy
   drover-ont relate "My Concept" broader_term "Cross-cutting Topics"   # or other facet
   drover-ont relate "My Concept" tagged_with "Drover Core Concepts"
   ```

3. **Review** (lightweight)
   - Run `drover-ont fsck` and `drover-ont validate`.
   - Check for duplicates, clarity of definition, correct facet placement, and alignment with source docs.
   - Proposer (or another agent) can create a `Review` entity of kind `asdlc` or `taxonomy` and link it if formal sign-off is desired.

4. **Accept** (only a Taxonomy Curator)
   ```bash
   # Curator records accountability
   drover-ont relate "My Concept" governed_by "Peter"

   # Then (and only then) promote the status
   drover-ont update "My Concept" -p status="accepted"
   ```

   The central schema policy now enforces:
   > "Term.status=accepted requires at least one governed_by link to a Taxonomy Curator (Person, Review, or Policy) and must be performed by a designated curator"

5. **Evolve**
   - Deprecate: `update ... status="deprecated"` + add `related_to` (or future `replaces`) to the successor.
   - Alias: create a Term with `status="alias"` pointing at the canonical Term.

### What "Review by taxonomy skill or designated curator" Means in Practice
- The **skill** (this documentation + the `drover-ont` binary + the agent habits in SKILL.md) handles proposal, wiring, validation, and suggestion of `governed_by` links.
- A **designated human curator** provides the final accountability via the `governed_by` edge before status changes to `accepted`.
- This satisfies both the schema policy and the needs of a multi-agent environment.

### Initial Seeded Governance (live in the graph)
- Taxonomy "Drover Core Concepts" is `governed_by` "Peter".
- "Peter" is `tagged_with` "Taxonomy Curator".
- The role "Taxonomy Curator" exists as a first-class Term.

Future curators for specific domains can be added the same way (e.g., a drover-brain maintainer becomes governor of certain sub-facets).

---

## Real Work Linkages (Examples)

We are actively connecting delivery work to the taxonomy using `tagged_with` (primary classification), plus `implements`, `part_of`, `covers`, and `contains` for richer structure.

### Example 1: Knowledge Graph Project (drover-ontology)
```bash
drover-ont relate "drover-ontology" tagged_with "Brain"
drover-ont relate "drover-ontology" tagged_with "Indexing"
drover-ont relate "drover-ontology" tagged_with "Indexing pipeline"
drover-ont relate "drover-ontology" tagged_with "Hybrid Memory"
drover-ont relate "drover-ontology" tagged_with "Capability registry"
drover-ont relate "drover-ontology" tagged_with "Documentation Types"
drover-ont relate "drover-ontology" implements "Brain"
drover-ont relate "drover-ontology" covers "Documentation Types"
```

### Example 2: Core Agent Workbench + Related Bead
```bash
# Project
drover-ont relate "drover-code" tagged_with "Agent MCP access"
drover-ont relate "drover-code" tagged_with "Hybrid Memory"
drover-ont relate "drover-code" tagged_with "Input guard"
drover-ont relate "drover-code" tagged_with "Output guard"
drover-ont relate "drover-code" tagged_with "Action guard"
drover-ont relate "drover-code" tagged_with "Capability binding"
drover-ont relate "drover-code" tagged_with "Muster integration gate"

# Specific work item
drover-ont relate "Bead_bb21fd31" tagged_with "Brain"
drover-ont relate "Bead_bb21fd31" tagged_with "Hybrid Memory"
drover-ont relate "Bead_bb21fd31" tagged_with "Indexing"
drover-ont relate "Bead_bb21fd31" tagged_with "Input guard"
drover-ont relate "Bead_bb21fd31" tagged_with "Output guard"
drover-ont relate "Bead_bb21fd31" tagged_with "Action guard"
drover-ont relate "Bead_bb21fd31" implements "Hybrid Memory"
drover-ont relate "Bead_bb21fd31" part_of "drover-code"
```

### Example 3: Top-Level Ecosystem
```bash
drover-ont relate "Drover" tagged_with "Brain"
drover-ont relate "Drover" tagged_with "Crew"
drover-ont relate "Drover" tagged_with "Policy evaluation"
drover-ont relate "Drover" tagged_with "Drover Core Concepts"
drover-ont relate "Drover" contains "drover-ontology"
```

Other product Projects (`drover-cloud`, `drover-gateway`, `drover-cli`, etc.) are tagged with "Product Contexts" (and can be further specialized as more Terms are added).

These linkages make questions like the following answerable directly from the graph:
- "What work is touching the knowledge system / Brain / Indexing?"
- "Which projects and Beads involve safety (Guard/Warden) or Muster capability governance?"
- "What is the drover-ontology project itself implementing and covering?"

**Pattern for the future**: When new Beads or Plans are created, immediately tag them with the most specific Terms under the six main facets of the "Drover Core Concepts" Taxonomy.

---

**Owner**: drover-org taxonomy curators (see "Governance Process for Terms" section above; initially Peter Hanssens as the sole Taxonomy Curator).  
**Review cadence**: Quarterly or when adding a new bounded context (see `CONTEXT-MAP.md`).  
**Source of truth**: This file + the central `graph.jsonl` at `drover-org/memory/ontology/`.