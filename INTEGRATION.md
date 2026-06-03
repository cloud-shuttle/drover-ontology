# Integrating drover-ontology

> **⚠️ CRITICAL RULE**
>
> **You must use the native Go binary (`ontology` or the `drover-ont` wrapper).**
>
> This skill is implemented entirely in Go for maximum performance, strict type validation, and robust storage integrity.

This guide explains how other repositories (inside or outside drover-org) should adopt the ontology skill.

## Use the Go Binary

**The `ontology` command must be the native Go implementation.**

All real usage — especially by agents — must go through the Go CLI.

## Two Main Scenarios

### 1. Inside the drover-org monorepo

Use the `drover-ont` wrapper. It automatically targets the single central ontology store for the entire organization:

```bash
drover-ont create --type Bead --props '{"title":"..."}'
drover-ont relate <id> tagged_with "Drover Core Concepts"
drover-ont fsck
```

The wrapper is typically installed at `~/bin/drover-ont`.

### 2. In a standalone project

1. Install the Go binary once (from this repo):

   ```bash
   cd /path/to/drover-ontology
   make install-local
   export PATH="$HOME/.local/bin:$PATH"
   ```

   Or use the one-liner installer:

   ```bash
   curl -fsSL https://raw.githubusercontent.com/drover-org/drover-ontology/main/scripts/install.sh | bash
   ```

2. Bootstrap a local graph in your repo (first time only):

   ```bash
   ./scripts/seed-drover-ontology.sh
   ```

3. Use the plain `ontology` command. Your data lives in `memory/ontology/` inside your project.

## Cursor / Claude Code Integration

```bash
mkdir -p ~/.cursor/skills
ln -s /path/to/drover-ontology/.cursor/skills/ontology ~/.cursor/skills/drover-ontology
```

Then invoke with `/drover-ontology` or `/ontology`.

## Quick Reference

| Situation                    | Command to use      | Graph location                              |
|-----------------------------|---------------------|---------------------------------------------|
| Inside drover-org monorepo  | `drover-ont`        | Central store (`~/.../drover-org/memory/ontology/`) |
| Standalone project          | `ontology`          | `./memory/ontology/` in your repo           |
| Point at a specific store   | `ontology --graph ... --db ...` | Any location you choose                |

## Further Reading

- [SKILL.md](SKILL.md) — Full developer reference, including the dedicated **Multi-Project / Monorepo Usage** section
- [.cursor/skills/ontology/SKILL.md](.cursor/skills/ontology/SKILL.md) — Concise agent guide
- [README.md](README.md) — Installation options and overview
- [references/taxonomy-relations.md](references/taxonomy-relations.md) — How to work with Terms and governance

For questions about the central drover-org ontology or the `drover-ont` wrapper, see the main drover-ontology repo or ask in the relevant channel.