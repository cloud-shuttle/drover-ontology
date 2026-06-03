# Knowledge graph patterns — context capture beyond work tracking

The ontology is a **context graph**: who, what, where, when, why, and how things connect. Ticket/work types are optional. Most value comes from **Documents, Notes, Concepts, Events, People, Organizations, and Goals** linked together.

Use `ontology search` + `related` + `covers` / `references` to answer “what do we know about X?” before acting.

---

## Consulting business

**Anchor**: `Organization` (your firm) + `Project` per client engagement.

```bash
# Firm and client
ontology create --type Organization --props '{"name":"Your Consulting Co","kind":"employer"}'
ontology create --type Organization --props '{"name":"Acme Retail","kind":"client"}'
ontology create --type Project --props '{"name":"Acme data platform assessment","kind":"consulting","status":"active","summary":"Q2 discovery + roadmap"}'

# People and roles
ontology create --type Person --props '{"name":"Jane Client","role":"sponsor","affiliation":"Acme Retail"}'
ontology relate <person_id> member_of <client_org_id>
ontology relate <person_id> has_owner <project_id>   # or serves / owns as appropriate

# Knowledge captured from discovery
ontology create --type Document --props '{"title":"Discovery interview notes","kind":"brief","path":"notes/acme-discovery.md"}'
ontology create --type Concept --props '{"label":"Event-sourced inventory","domain":"retail","definition":"..."}'
ontology relate <doc_id> part_of <project_id>
ontology relate <doc_id> covers <concept_id>
ontology relate <firm_id> serves <client_org_id>
```

**Typical questions the graph answers:**
- What did we learn about this client’s domain?
- Which documents support this recommendation?
- Who are the stakeholders on active engagements?

---

## Workshop series

**Anchor**: `Project` with `kind: workshop_series` containing `Event` sessions and `Document` curricula.

```bash
ontology create --type Project --props '{"name":"Kelpies Software Factory","kind":"workshop_series","status":"active"}'
ontology create --type Event --props '{"title":"Module 1 — ASDLC intro","kind":"workshop","start":"2026-06-15T09:00:00Z","format":"half-day"}'
ontology create --type Document --props '{"title":"Facilitator runbook","kind":"runbook","path":"workshops/kelpies/README.md"}'
ontology create --type Goal --props '{"description":"Attendees can run /implement on a sample issue"}'

ontology relate <event_id> part_of <series_project_id>
ontology relate <runbook_id> part_of <series_project_id>
ontology relate <event_id> covers <goal_id>
ontology relate <runbook_id> references <goal_id>
```

Link exercises, slide decks, and debrief notes the same way. Use **Note** for quick capture during a session; promote to **Document** when stable.

---

## Conference (single event or track)

**Anchor**: `Event` with `kind: conference` or `Project` wrapping the whole program.

```bash
ontology create --type Event --props '{"title":"DataEngBytes 2026","kind":"conference","start":"2026-09-10","end":"2026-09-12","location":"Melbourne"}'
ontology create --type Event --props '{"title":"Keynote — Context graphs for agents","kind":"talk","start":"2026-09-10T09:00:00Z"}'
ontology create --type Person --props '{"name":"Speaker Name","role":"speaker","expertise":"knowledge graphs"}'
ontology create --type Concept --props '{"label":"Agent memory","domain":"ai"}'

ontology relate <talk_id> part_of <conference_id>
ontology relate <speaker_id> member_of <talk_id>    # or link via Event.attendees in props
ontology relate <talk_id> teaches <concept_id>
ontology create --type Document --props '{"title":"CFP submission","kind":"brief"}'
ontology relate <cfp_id> part_of <conference_id>
```

Capture: CFP, schedule, speaker bios, sponsor orgs, recordings, and attendee feedback as **Document** / **Note** with `references` and `covers`.

---

## Product / engineering (when you also ship code)

Use **Task**, **Plan**, **Review**, **Change** from the workflow schema layer — but still anchor domain knowledge in **Concept**, **Document**, and **Term** (glossary). The graph should explain the *system*, not only the backlog.

---

## Agent habits (any domain)

| When | Do |
|------|-----|
| Learn a durable fact | `create Note` or `Document`, `relate` to `Project` / `Concept` |
| Meet someone new | `create Person`, `member_of` org, `references` from meeting notes |
| Read a file worth remembering | `create Document` with `path`, `covers` relevant concepts |
| Start a new initiative | `create Project` with `kind`, link goals and owners |
| Before a long task | `search` + `related --depth 2` on key concepts |
| After a session | Append **Action** + link new notes to the event |

---

## Schema tip

For full knowledge + optional delivery workflow, merge schemas:

```bash
# Active schema should include core types (this file) plus workflow extensions
# See references/core-schema.yaml + references/drover-schema.yaml
```

Prefer **core types first**; add Bead/Ticket/Plan only when you need formal delivery gates.
