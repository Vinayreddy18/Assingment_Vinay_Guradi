# Samādhān — Makefile
# Common developer tasks. Run `make help` for the list.

APP        := samadhan
PKG        := ./...
BIN_DIR    := bin
BIN        := $(BIN_DIR)/$(APP)
MAIN       := ./cmd/samadhan
PORT       ?= 8080

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: run
run: ## Run the server (offline provider unless ANTHROPIC_API_KEY is set)
	go run $(MAIN)

.PHONY: run-offline
run-offline: ## Force the deterministic offline provider
	SAMADHAN_PROVIDER=mock go run $(MAIN)

.PHONY: build
build: ## Build a static-ish binary into ./bin
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o $(BIN) $(MAIN)
	@echo "built $(BIN)"

.PHONY: test
test: ## Run all tests
	go test $(PKG)

.PHONY: test-v
test-v: ## Run all tests (verbose)
	go test -v $(PKG)

.PHONY: cover
cover: ## Run tests with a coverage summary
	go test -cover $(PKG)

.PHONY: race
race: ## Run tests under the race detector
	go test -race $(PKG)

.PHONY: vet
vet: ## go vet
	go vet $(PKG)

.PHONY: fmt
fmt: ## Format the code
	gofmt -s -w .

.PHONY: tidy
tidy: ## Tidy go.mod
	go mod tidy

.PHONY: check
check: fmt vet test ## Format, vet and test

.PHONY: docker-build
docker-build: ## Build the Docker image
	docker build -t $(APP):latest .

.PHONY: docker-run
docker-run: ## Run the container on $(PORT) (pass ANTHROPIC_API_KEY through if set)
	docker run --rm -p $(PORT):8080 -e ANTHROPIC_API_KEY="$$ANTHROPIC_API_KEY" $(APP):latest

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
