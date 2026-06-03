#!/bin/bash
# Quick bootstrap for a new Drover workspace ontology
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MEM="$REPO_ROOT/memory/ontology"

mkdir -p "$MEM"

if [ ! -f "$MEM/schema.yaml" ]; then
  cp "$REPO_ROOT/references/drover-schema.yaml" "$MEM/schema.yaml"
  echo "Installed drover-schema.yaml"
fi

touch "$MEM/graph.jsonl"

go run "$REPO_ROOT/cmd/ontology/main.go" --graph "$MEM/graph.jsonl" --db "$MEM/graph.db" create --type Person --props '{
  "name": "Drover Builder",
  "role": "founder",
  "notes": "Seeded by drover-ontology skill"
}' || true

echo "✓ drover-ontology seed complete"
echo "  graph: $MEM/graph.jsonl"
echo "  schema: $MEM/schema.yaml"
