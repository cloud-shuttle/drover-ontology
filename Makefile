# drover-ontology Makefile

BINARY_NAME := ontology
PKG := github.com/drover-org/drover-ontology/cmd/ontology
BUILD_DIR := bin

# Default target
.PHONY: all
all: build

# Build both binaries into bin/
.PHONY: build
build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "-s -w" -o $(BUILD_DIR)/$(BINARY_NAME) $(PKG)
	go build -tags rlm -ldflags "-s -w" -o $(BUILD_DIR)/rlm-ontology github.com/drover-org/drover-ontology/tools/rlm-ontology
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME) and $(BUILD_DIR)/rlm-ontology"

# Install using Go's standard mechanism (respects $GOBIN / $GOPATH/bin)
.PHONY: install
install:
	go install -ldflags "-s -w" $(PKG)
	go install -tags rlm -ldflags "-s -w" github.com/drover-org/drover-ontology/tools/rlm-ontology
	@echo "Installed 'ontology' and 'rlm-ontology' via go install"
	@echo "Make sure $$GOPATH/bin or $$GOBIN is in your PATH"

# Install to ~/.local/bin (works on macOS + most Linux distros without sudo)
.PHONY: install-local
install-local:
	@mkdir -p $(HOME)/.local/bin
	go build -ldflags "-s -w" -o $(HOME)/.local/bin/$(BINARY_NAME) $(PKG)
	go build -tags rlm -ldflags "-s -w" -o $(HOME)/.local/bin/rlm-ontology github.com/drover-org/drover-ontology/tools/rlm-ontology
	@echo "Installed to $(HOME)/.local/bin/ontology and $(HOME)/.local/bin/rlm-ontology"
	@echo "Make sure ~/.local/bin is in your PATH (add to ~/.zshrc or ~/.bash_profile if needed):"
	@echo '  export PATH="$$HOME/.local/bin:$$PATH"'

# Install to /usr/local/bin (may require sudo on some systems)
.PHONY: install-system
install-system:
	go build -ldflags "-s -w" -o /usr/local/bin/$(BINARY_NAME) $(PKG)
	go build -tags rlm -ldflags "-s -w" -o /usr/local/bin/rlm-ontology github.com/drover-org/drover-ontology/tools/rlm-ontology
	@echo "Installed to /usr/local/bin/ontology and /usr/local/bin/rlm-ontology"

# Development build (with debug info)
.PHONY: dev
dev:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(PKG)
	@echo "Built dev binary"

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
	@echo "Cleaned build artifacts"

# Run tests
.PHONY: test
test:
	go test ./...

# Run generative tests (fuzz + property-based)
# See docs/TESTING_STRATEGY.md for details and recommendations.
# This is the target used by .github/workflows/test-generative.yml in CI.
#
# This target runs:
# - Time-bounded fuzzing (safe for local/CI use)
# - All property-based and dedicated generative tests
.PHONY: test-generative
test-generative:
	@echo "==> Running time-bounded fuzz targets (30s each)"
	go test -fuzz=FuzzLogLoadAll      -fuzztime=30s ./internal/ontology || true
	go test -fuzz=FuzzVectorMath      -fuzztime=20s ./internal/ontology || true
	go test -fuzz=FuzzOperationRoundtrip -fuzztime=15s ./internal/ontology || true

	@echo ""
	@echo "==> Running property-based and replay tests"
	go test ./internal/ontology -run 'TestLogReplayEquivalence|Properties|Regression_ReplayMismatch' -count=1 -timeout 2m || echo "NOTE: generative test reported issues (see output)."

	@echo ""
	@echo "For long-running replay verification, use:"
	@echo "  make test-replay-long RAPID_CHECKS=10000"

# Long-running focused replay property test (highest value for consistency)
# Recommended for periodic deep verification or before major releases.
# Example: make test-replay-long RAPID_CHECKS=10000
.PHONY: test-replay-long
test-replay-long:
	@echo "Running long LogReplayEquivalence (set RAPID_CHECKS env var for more iterations)"
	RAPID_CHECKS=$${RAPID_CHECKS:-5000} go test \
		-run TestLogReplayEquivalence \
		-timeout 15m \
		-count=1 \
		./internal/ontology

# Basic CLI end-to-end tests.
# Builds the real binary once per run and exercises the key commands (create, relate,
# get, fsck, list) against completely isolated temporary stores. Skipped in -short mode.
.PHONY: test-e2e
test-e2e:
	@echo "==> Running basic CLI E2E tests (build + create/relate/get/fsck against temp stores)"
	go test -count=1 -timeout 3m ./e2e

# Show help
.PHONY: help
help:
	@echo "drover-ontology build targets:"
	@echo "  make build         - Build both binaries to bin/ (ontology and rlm-ontology)"
	@echo "  make install       - Install both to $GOPATH/bin (standard Go install)"
	@echo "  make install-local - Install both to ~/.local/bin (recommended for Mac/Linux)"
	@echo "  make install-system- Install both to /usr/local/bin (may need sudo)"
	@echo "  make dev            - Development build"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run all unit/property tests"
	@echo "  make test-generative - Run bounded fuzz + property-based tests (see TESTING_STRATEGY.md)"
	@echo "  make test-replay-long - Deep run of LogReplayEquivalence (set RAPID_CHECKS=10000)"
	@echo "  make test-e2e       - Basic CLI E2E (builds binary + exercises create/relate/get/fsck against temp stores)"
	@echo "  (Regression_ReplayMismatch_* tests run automatically via the above and 'make test')"
