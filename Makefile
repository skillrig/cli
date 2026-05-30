# skillrig-cli build & test tasks.
# Test tiers map to the package layout (Constitution III):
#   unit        -> ./internal/... ./pkg/...  (presentation-free logic + skillcore
#                  ground-truth/table-driven tests, fast; no real binary)
#   integration -> ./test/...                (TestQuickstart_*; builds & execs the binary)

BINARY := skillrig

.PHONY: build test test-unit test-integration lint fmt vet check clean

build: ## Build the skillrig binary into ./$(BINARY)
	go build -o $(BINARY) .

test: ## Run the full test suite (unit + integration)
	go test ./...

test-unit: ## Run unit tests only (presentation-free logic in internal/ + pkg/skillcore)
	go test ./internal/... ./pkg/...

test-integration: ## Run the quickstart acceptance suite (builds & execs the binary)
	go test ./test/...

lint: ## Run golangci-lint (v2 config in .golangci.yml)
	golangci-lint run

fmt: ## Format all Go sources
	gofmt -w .

vet: ## Run go vet
	go vet ./...

check: fmt vet lint test ## Pre-merge gate: fmt + vet + lint + full test suite

clean: ## Remove build artifacts
	rm -f $(BINARY)
