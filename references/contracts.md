# Ontology Skill Contracts

Skills that read or write the ontology must declare their contract in SKILL.md frontmatter (or a dedicated section). Examples below use Drover skill names for concreteness; the same pattern applies to DataEngBytes, Cloud Shuttle, workshop, or custom skills.

## Example: drover-wb-plan

```yaml
ontology:
  reads: [Plan, Review, Bead, Person]
  writes: [Plan, Action]
  preconditions:
    - "Plan.kind must be one of the approved kinds"
  postconditions:
    - "New Plan starts with status=draft"
    - "Plan creation emits an Action of type=plan_created"
```

## Example: taxonomy-architect

```yaml
ontology:
  reads: [Term, Taxonomy, Concept]
  writes: [Term, Taxonomy]
  preconditions:
    - "Term.label must be unique within its domain"
  postconditions:
    - "Accepted Terms are linked to at least one taxonomy root"
```

## Example: drover-beads-queue

```yaml
ontology:
  reads: [Bead, Ticket, Plan]
  writes: [Bead, Action]
  preconditions:
    - "Bead must have a valid kind and status"
  postconditions:
    - "Queue mutations are also written as Action entities for audit"
```

## Example: gstack review skills (design-review, plan-eng-review, etc.)

```yaml
ontology:
  reads: [Plan, Bead, Review]
  writes: [Review, Action]
  preconditions:
    - "The reviewed artifact (Plan/Bead) must exist"
  postconditions:
    - "Every Review has decision + reviewer + timestamp"
    - "High-severity issues create linked Beads of kind=fix"
```

## Contract Enforcement (future)

The ontology CLI / library can grow a `--check-contract <skill>` mode that:
1. Parses the calling skill's declared ontology section
2. Runs the requested mutation
3. Rejects or warns on violation at the skill boundary

For now, contracts are documentation + social contract between skill authors.
