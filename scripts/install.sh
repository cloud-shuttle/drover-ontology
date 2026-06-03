#!/usr/bin/env bash
#
# drover-ontology installer
# One-liner friendly installer for the native Go CLI.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/drover-org/drover-ontology/main/scripts/install.sh | bash
#   # or locally:
#   ./scripts/install.sh
#
set -euo pipefail

BINARY_NAME="ontology"
REPO="drover-org/drover-ontology"
CMD_PATH="cmd/ontology"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info()  { echo -e "${GREEN}[info]${NC}  $1"; }
warn()  { echo -e "${YELLOW}[warn]${NC}  $1"; }
error() { echo -e "${RED}[error]${NC} $1" >&2; }

# Detect preferred install location
choose_install_dir() {
    # 1. Respect explicit GOBIN
    if [[ -n "${GOBIN:-}" ]]; then
        echo "$GOBIN"
        return
    fi

    # 2. Prefer ~/.local/bin (modern, no sudo, works everywhere)
    local local_bin="$HOME/.local/bin"
    if [[ -d "$local_bin" ]] || mkdir -p "$local_bin" 2>/dev/null; then
        echo "$local_bin"
        return
    fi

    # 3. Fall back to Go's default (GOPATH/bin)
    if command -v go >/dev/null 2>&1; then
        local gopath
        gopath="$(go env GOPATH 2>/dev/null || echo "$HOME/go")"
        local gobin="$gopath/bin"
        mkdir -p "$gobin" 2>/dev/null || true
        echo "$gobin"
        return
    fi

    # 4. Last resort: /usr/local/bin (may need sudo)
    echo "/usr/local/bin"
}

main() {
    info "Installing drover-ontology (native Go CLI)..."

    if ! command -v go >/dev/null 2>&1; then
        error "Go is not installed. Please install Go 1.21+ first: https://go.dev/dl/"
        exit 1
    fi

    local go_version
    go_version="$(go version | awk '{print $3}')"
    info "Found Go: $go_version"

    local install_dir
    install_dir="$(choose_install_dir)"
    mkdir -p "$install_dir" 2>/dev/null || true

    info "Installing to: $install_dir/$BINARY_NAME"

    # Use go install for cleanliness (respects modules, caching, etc.)
    # We build from the current module if we're inside the repo,
    # otherwise we go get the latest.
    if [[ -f "go.mod" ]] && grep -q "drover-ontology" go.mod 2>/dev/null; then
        info "Installing from local source (development mode)..."
        go install -ldflags="-s -w" "./$CMD_PATH"
    else
        info "Installing latest version from GitHub..."
        go install -ldflags="-s -w" "github.com/${REPO}/${CMD_PATH}@latest"
    fi

    # The binary name produced by `go install ./cmd/ontology` is `ontology`
    local installed_path
    if command -v "$BINARY_NAME" >/dev/null 2>&1; then
        installed_path="$(command -v "$BINARY_NAME")"
    else
        # Try the target dir we chose
        installed_path="$install_dir/$BINARY_NAME"
    fi

    if [[ -x "$installed_path" ]]; then
        info "Successfully installed: $installed_path"
        "$installed_path" --help | head -8
        echo
        info "Make sure '$install_dir' is in your PATH."
        echo
        echo "Add this to your ~/.zshrc or ~/.bash_profile if needed:"
        echo "    export PATH=\"$install_dir:\$PATH\""
        echo
        echo "Then reload your shell:"
        echo "    source ~/.zshrc   # or ~/.bash_profile"
    else
        warn "Binary was built but not found in PATH yet."
        echo "You may need to add $install_dir to your PATH manually."
    fi
}

main "$@"
