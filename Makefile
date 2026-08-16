.PHONY: help build run run-sse validate test integration-test clean install lint fmt vet deps \
        docker-build docker-run docker-run-sse docker-stop docker-logs docker-push all

.DEFAULT_GOAL := help

# Load .env.dev if it exists (silently skip if missing)
-include .env.dev
export

# ── Docker config (override via env or CLI) ───────────────────────────────────
DOCKER_IMAGE  ?= bamboo-mcp
DOCKER_TAG    ?= latest
DOCKER_NAME   ?= bamboo-mcp
HTTP_PORT     ?= 8080
MCP_BASE_URL  ?= http://localhost:$(HTTP_PORT)

# ── Bamboo connection (never hardcode secrets here) ───────────────────────────
# Supply these via the environment or an untracked .env.dev file, e.g.:
#   BAMBOO_URL=https://bamboo.example.com
#   BAMBOO_TOKEN=your_bamboo_personal_access_token
#   BITBUCKET_URL=https://bitbucket.example.com
#   BAMBOO_PROXY=http://proxy.example.com:8080
BAMBOO_URL    ?=
BAMBOO_TOKEN  ?=
BAMBOO_PROXY  ?=
BITBUCKET_URL ?=
VERBOSE       ?= true

# ─────────────────────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "Bamboo MCP Server — Available Commands"
	@echo "────────────────────────────────────────────"
	@echo "  Build & Run"
	@echo "    make build            Build the binary (bin/bamboo-mcp)"
	@echo "    make run              Run in stdio mode (default)"
	@echo "    make run-sse          Run in SSE/HTTP mode on :$(HTTP_PORT)"
	@echo "    make validate         Run startup validation only (SKIP_VALIDATION=false)"
	@echo ""
	@echo "  Testing"
	@echo "    make test             Run unit tests"
	@echo "    make integration-test Run integration tests against real Bamboo"
	@echo ""
	@echo "  Code Quality"
	@echo "    make fmt              Format code"
	@echo "    make vet              Run go vet"
	@echo "    make lint             Run golangci-lint"
	@echo "    make all              fmt + vet + test + build"
	@echo ""
	@echo "  Docker"
	@echo "    make docker-build     Build Docker image ($(DOCKER_IMAGE):$(DOCKER_TAG))"
	@echo "    make docker-run       Run container in stdio mode"
	@echo "    make docker-run-sse   Run container in SSE/HTTP mode on :$(HTTP_PORT)"
	@echo "    make docker-stop      Stop and remove the container"
	@echo "    make docker-logs      Tail container logs"
	@echo "    make docker-push      Push image to registry"
	@echo ""
	@echo "  Maintenance"
	@echo "    make install          Install / tidy dependencies"
	@echo "    make deps             Download dependencies"
	@echo "    make clean            Remove build artifacts"
	@echo ""

# ── Build & Run ───────────────────────────────────────────────────────────────
build:
	@echo "Building bamboo-mcp..."
	@go build -ldflags="-s -w" -o bin/bamboo-mcp cmd/server/main.go
	@echo "Binary: bin/bamboo-mcp"

run: build
	@echo "Running bamboo-mcp (stdio)..."
	@MCP_TRANSPORT=stdio ./bin/bamboo-mcp

run-sse: build
	@echo "Running bamboo-mcp (SSE/HTTP on :$(HTTP_PORT))..."
	@MCP_TRANSPORT=sse MCP_HTTP_PORT=$(HTTP_PORT) MCP_BASE_URL=$(MCP_BASE_URL) ./bin/bamboo-mcp

validate: build
	@echo "Running startup validation..."
	@SKIP_VALIDATION=false ./bin/bamboo-mcp --validate-only 2>&1 || true

# ── Testing ───────────────────────────────────────────────────────────────────
test:
	@echo "Running tests..."
	@go test -v ./...

integration-test: build
	@echo "Running integration tests..."
	@go run scripts/integration_check.go

# ── Code Quality ──────────────────────────────────────────────────────────────
fmt:
	@echo "Formatting code..."
	@go fmt ./...

vet:
	@echo "Running go vet..."
	@go vet ./...

lint:
	@echo "Running linter..."
	@golangci-lint run || echo "golangci-lint not installed, skipping..."

all: fmt vet test build
	@echo "All tasks completed successfully!"

# ── Dependencies ──────────────────────────────────────────────────────────────
install:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

deps:
	@echo "Downloading dependencies..."
	@go mod download

clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@go clean

# ── Docker ────────────────────────────────────────────────────────────────────
docker-build:
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@podman build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "Image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

docker-run: docker-build
	@echo "Running container (stdio) — $(DOCKER_NAME)..."
	@podman run --rm --name $(DOCKER_NAME) \
		-e BAMBOO_URL=$(BAMBOO_URL) \
		-e BAMBOO_TOKEN=$(BAMBOO_TOKEN) \
		-e BAMBOO_PROXY=$(BAMBOO_PROXY) \
		-e BITBUCKET_URL=$(BITBUCKET_URL) \
		-e MCP_TRANSPORT=stdio \
		-e VERBOSE=$(VERBOSE) \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

docker-rm-container:
	@podman rm -f $(DOCKER_NAME) 2>/dev/null || true

docker-run-sse: docker-build docker-rm-container
	@echo "Running container (SSE/HTTP on :$(HTTP_PORT)) — $(DOCKER_NAME)..."
	@podman run  --name $(DOCKER_NAME) \
		-e BAMBOO_URL=$(BAMBOO_URL) \
		-e BAMBOO_TOKEN=$(BAMBOO_TOKEN) \
		-e BAMBOO_PROXY=$(BAMBOO_PROXY) \
		-e BITBUCKET_URL=$(BITBUCKET_URL) \
		-e MCP_TRANSPORT=sse \
		-e MCP_HTTP_PORT=8080 \
		-e MCP_BASE_URL=$(MCP_BASE_URL) \
		-e VERBOSE=$(VERBOSE) \
		-p $(HTTP_PORT):8080 \
		$(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "SSE server running at http://localhost:$(HTTP_PORT)"
	@echo "SSE endpoint: http://localhost:$(HTTP_PORT)/sse"

docker-stop:
	@echo "Stopping container $(DOCKER_NAME)..."
	@podman stop $(DOCKER_NAME) 2>/dev/null || true
	@podman rm $(DOCKER_NAME) 2>/dev/null || true

docker-logs:
	@podman logs -f $(DOCKER_NAME)

docker-push:
	@echo "Pushing $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@podman push $(DOCKER_IMAGE):$(DOCKER_TAG)
