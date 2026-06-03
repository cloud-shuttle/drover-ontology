# Common Ontology Queries

Knowledge-base and context-graph recipes. Workflow/delivery queries at the bottom.

## What do we know about a topic?

```bash
ontology search "data mesh retail" --limit 20
ontology query --type Concept --where '{"label":"Data mesh"}'
ontology related --id <concept_id> --rel "covers,references,related_to" --depth 2
```

## Everything captured for an engagement / series / conference

```bash
ontology related --id <project_id> --rel "contains,part_of,references,covers,member_of" --depth 3
```

## Documents that teach or cover a concept

```bash
ontology related --id <concept_id> --rel covers --depth 1
# Reverse: who references this decision doc?
ontology related --id <document_id> --rel references --depth 2
```

## People and orgs on a project

```bash
ontology query --type Person
ontology related --id <project_id> --rel "has_owner,member_of,serves" --depth 2
```

## Conference / workshop timeline

```bash
ontology query --type Event --where '{"kind":"workshop"}'
ontology related --id <series_project_id> --rel "contains,part_of" --depth 2
```

## Export context for an agent session

```bash
ontology related --id <project_id> --rel "contains,covers,references,member_of" --depth 2 > /tmp/context.json
```

---

## Delivery / workflow (optional schema)

### Open work for a Plan

```bash
ontology query --type Bead --where '{"status":"open"}' --related-to <plan_id> --rel implements
```

### Dependency / blocker graph

```bash
ontology related --id <bead_id> --rel "depends_on,blocks" --depth 3
```

### Terms in a domain (glossary)

```bash
ontology query --type Term --where '{"domain":"platform"}'
```

### Recent agent actions (audit)

```bash
ontology query --type Action | tail -20
```

### Policy check — approved plans without go review

```bash
ontology query --type Plan --where '{"status":"approved"}'
# then related --rel approves per plan id
```
