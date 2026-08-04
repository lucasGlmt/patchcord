GO         := go
BUILD_DIR  := bin
BINARY     := patchcord

.DEFAULT_GOAL := help

.PHONY: build
build: ## Build the patchcord binary into bin/patchcord
	$(GO) build -o $(BUILD_DIR)/$(BINARY) ./cmd/patchcord

.PHONY: build-plugins
build-plugins: ## Build every example plugin into bin/plugins/<name>
	@for dir in plugins/examples/*/; do \
		name=$$(basename "$$dir"); \
		echo "building plugin $$name"; \
		$(GO) build -o $(BUILD_DIR)/plugins/$$name ./$$dir || exit 1; \
	done

.PHONY: build-all
build-all: build build-plugins ## Build the agent and every example plugin

.PHONY: run
run: build ## Build then start the agent (patchcord serve)
	$(BUILD_DIR)/$(BINARY) serve

.PHONY: test
test: ## Run the full test suite
	$(GO) test ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format all Go source files
	$(GO) fmt ./...

.PHONY: fmt-check
fmt-check: ## Fail if any Go file needs gofmt
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then \
		echo "gofmt needed on:"; echo "$$files"; exit 1; \
	fi

.PHONY: proto
proto: ## Regenerate the plugin protocol's Go stubs (api/plugin/v1) with buf
	buf generate

.PHONY: swagger
swagger: ## Regenerate the OpenAPI spec (api/agent) from internal/api's swag annotations — requires `go install github.com/swaggo/swag/cmd/swag@latest`
	swag init --dir ./internal/api --generalInfo doc.go --parseInternal --output api/agent --outputTypes json,yaml --quiet

.PHONY: docker-build
docker-build: ## Build the agent's Docker image (see Dockerfile, ADR-0039)
	docker build -t patchcord-agent .

.PHONY: docker-run
docker-run: ## Run the agent via docker compose (build first with docker-build if needed)
	docker compose up --build

.PHONY: docs-build
docs-build: ## Build the mdBook documentation (requires `cargo install mdbook`)
	mdbook build docs/book

.PHONY: docs-serve
docs-serve: ## Serve the mdBook documentation locally with live reload (requires `cargo install mdbook`)
	mdbook serve docs/book

.PHONY: check
check: vet fmt-check test ## Run everything a change should pass before it's proposed as done

.PHONY: clean
clean: ## Remove build artifacts and local runtime data
	rm -rf $(BUILD_DIR) data

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
