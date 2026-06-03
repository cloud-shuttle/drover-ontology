# Contributing to Drover Ontology

Thank you for your interest in contributing to Drover Ontology! We welcome community contributions to help mature our local codebase governance, semantic modeling, and developer RLM loop integrations.

---

## 📂 Repository Structure

Drover Ontology is built as a pure, portable Go application. The codebase is organized as follows:

* **`cmd/ontology/`**: The CLI entry point.
* **`commands/`**: Subcommand implementations (`visualize`, `export`, `validate`, `fsck`).
* **`internal/ontology/`**: The core knowledge graph database. Handles SQLite projection seeding, `graph.jsonl` append-only logs, and active `schema.yaml` contract validations.
* **`internal/ingest/`**: The codebase scanner and token ingestion logic (includes the AST symbol parsers).
* **`tools/rlm-ontology/`**: The autonomous Real-time Learning Machine (RLM) Go REPL harness backed by the Yaegi interpreter.

---

## 🛠️ AST Symbol Ingestion & Adding New Languages

One of the most high-value areas to contribute is extending the **Language-Agnostic Symbol Parser** inside `internal/ingest/code_analysis.go`.

The CLI uses symbol scans to map classes, functions, structs, and configuration blocks to exact code line ranges (`line_start` and `line_end`) so they can be tagged as governed ontology terms. This metadata powers interactive IDE deep-linking buttons in our local web visualizer (`ontology visualize`).

### How the Symbol Parser Works

All symbol scanning delegates to the `ParseFileSymbols(relPath string)` method on `CodeAnalysis` inside [internal/ingest/code_analysis.go](file:///Users/peterhanssens/consulting/drover-org/drover-ontology/internal/ingest/code_analysis.go):

1. **Go (`.go`)**: Parses structural symbols natively using Go’s AST standard libraries (`go/parser` and `go/ast`).
2. **JavaScript / TypeScript (`.js`, `.jsx`, `.ts`, `.tsx`)**: Uses a lightweight brace-matching recursive scanner (`findBlockEnd`) to calculate accurate block spans for functions and classes.
3. **Python (`.py`)**: Uses an indentation-level block scanner (`getIndentLevel` and `findPythonBlockEnd`) to identify class and method boundaries without external CGO parser dependencies.
4. **YAML (`.yaml`, `.yml`)**: Parses top-level mappings and calculates spans based on line indentations.

---

### Step-by-Step Guide: Adding Rust (`.rs`) Support

If you want to contribute support for a new language like Rust, follow these steps:

#### Step 1: Detect File Extension
In `internal/ingest/code_analysis.go`, locate the `ParseFileSymbols` switch block and register the `.rs` file extension:

```go
case ".rs":
    return c.parseRustSymbols(content)
```

#### Step 2: Implement the Token-Brace Scanner
Write a parsing helper function `parseRustSymbols(content string) (string, error)` inside `code_analysis.go`. You can use a lightweight line and character scanner:
- Detect `struct`, `impl`, `trait`, and `fn` signature declarations.
- Track opening `{` and closing `}` characters dynamically to calculate the exact starting and ending line positions.
- Format and return a structured text log detailing extracted symbols and their ranges.

#### Step 3: Add Unit Tests
Add a test case in [internal/ingest/code_analysis_test.go](file:///Users/peterhanssens/consulting/drover-org/drover-ontology/internal/ingest/code_analysis_test.go) asserting correct symbol discovery and exact line ranges. For example:

```go
func TestParseRustSymbols(t *testing.T) {
    analysis := &CodeAnalysis{}
    content := `struct User {
    name: String,
}

fn get_name() -> String {
    "Peter".to_string()
}`

    symbols, err := analysis.parseRustSymbols(content)
    // Assert struct is found on lines 1-3, fn found on lines 5-7
}
```

---

## 🧪 Development Workflow

### Building Natively
We use a simple `Makefile` for compilation and local installations:
```bash
# Build binary under bin/
make build

# Install CLI locally to your system paths
make install-local
```

### Running Tests
Ensure all unit, integration, and fuzz suites compile and pass cleanly before submitting any pull requests:
```bash
# Run all standard tests
go test ./...

# Run the RLM Yaegi harness integration tests
go test -tags rlm ./tools/rlm-ontology/...
```
