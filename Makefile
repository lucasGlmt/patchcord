GO         := go
BUILD_DIR  := bin
BINARY     := patchcord

# Version metadata embedded into internal/version at build time (see
# docs/adr/0056-versionnement-du-binaire-agent.md). VERSION is the last
# reachable tag plus a "-N-gHASH" suffix when HEAD is ahead of it, "dev"
# outside of a git checkout — never hand-set these, tag the release instead
# (`git tag vX.Y.Z && git push --tags`).
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE       := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -X github.com/lucasglmt/patchcord/internal/version.Version=$(VERSION) \
              -X github.com/lucasglmt/patchcord/internal/version.Commit=$(COMMIT) \
              -X github.com/lucasglmt/patchcord/internal/version.Date=$(DATE)

.DEFAULT_GOAL := help

.PHONY: build
build: ## Build the patchcord binary into bin/patchcord (embeds VERSION/COMMIT/DATE)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/patchcord

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
docker-build: ## Build the agent's Docker image (see Dockerfile, ADR-0039), embedding VERSION/COMMIT/DATE
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t patchcord-agent .

.PHONY: docker-run
docker-run: ## Run the agent via docker compose (build first with docker-build if needed)
	docker compose up --build

.PHONY: docker-run-tls
docker-run-tls: ## Run the agent behind Caddy with automatic HTTPS (see docker-compose.tls.yml, ADR-0041); requires PATCHCORD_DOMAIN
	docker compose -f docker-compose.tls.yml up --build

.PHONY: docs-build
docs-build: ## Build the mdBook documentation (requires `cargo install mdbook`)
	mdbook build docs/book

.PHONY: docs-serve
docs-serve: ## Serve the mdBook documentation locally with live reload (requires `cargo install mdbook`)
	mdbook serve docs/book

.PHONY: changelog
changelog: ## Regenerate CHANGELOG.md from Conventional Commits (requires `brew install git-cliff` or `cargo install git-cliff`) — see docs/adr/0056-versionnement-du-binaire-agent.md
	git-cliff --config cliff.toml --output CHANGELOG.md

.PHONY: check
check: vet fmt-check test ## Run everything a change should pass before it's proposed as done

.PHONY: clean
clean: ## Remove build artifacts and local runtime data
	rm -rf $(BUILD_DIR) data

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
