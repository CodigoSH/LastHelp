.PHONY: all build frontend backend run dev test test-arch test-cover lint fmt vet tidy clean docker help

APP        := lasthelp
MODULE     := git.codigosh.com/CodigoSH/LastHelp
BUILD_DIR  := bin
CMD        := ./cmd/lasthelp

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo v0.0.0-dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

# -trimpath keeps the build machine's absolute paths out of the binary, so a
# third party can rebuild a release and compare hashes. -s -w drops the symbol
# table; it stays out of dev builds so panics remain readable.
LDFLAGS    := -X $(MODULE)/internal/version.Current=$(VERSION) -X $(MODULE)/internal/version.Commit=$(COMMIT)
GOFLAGS    := -trimpath

all: build

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

frontend: ## Build the web assets and mirror them for embedding
	@command -v bun >/dev/null || { echo "bun is required to build the frontend"; exit 1; }
	@bun run build

backend: ## Build the Go binaries only
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP) $(CMD)
	@CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/healthcheck ./cmd/healthcheck
	@echo "built $(BUILD_DIR)/$(APP) $(VERSION)"

build: frontend backend ## Build everything

run: build ## Build and run
	@./$(BUILD_DIR)/$(APP)

dev: ## Run without rebuilding the frontend
	@go run $(CMD)

# -count=1 is not optional for the architecture rules: they read the repository
# from disk, and the test cache does not track source files it was never told
# about, so a cached "pass" there is worthless.
test-arch: ## Run the architecture rules
	@go test -count=1 ./internal/arch/

test: ## Run every test
	@go test ./...
	@go test -count=1 ./internal/arch/
	@bun test

test-cover: ## Run the Go tests with coverage
	@go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

fmt: ## Format the Go sources
	@gofmt -w ./cmd ./internal

vet: ## Run go vet
	@go vet ./...

lint: fmt vet ## Format and vet

tidy: ## Tidy go.mod
	@go mod tidy

# Building is not running, and a passing suite is not a working page. This
# starts the real binary on a scratch database, drives a real browser through
# the whole journey, and checks the interface in Arabic on the way out.
drive: build ## Walk the whole journey in a real browser
	@command -v google-chrome >/dev/null || { echo "google-chrome is required"; exit 1; }
	@scripts/drive.sh

# Screenshots are generated, never taken by hand: a hand-taken one is one nobody
# can reproduce when the interface changes, so the documentation ends up showing
# a version that no longer exists.
shots: build ## Regenerate the documentation screenshots
	@command -v google-chrome >/dev/null || { echo "google-chrome is required"; exit 1; }
	@scripts/docs-shots.sh

# The adapters were written from documentation, and for Gitea and Forgejo from
# reading the structs they serialise. Everything else that tests them uses a fake
# this repository also wrote, so both halves could be wrong the same way and
# agree. This is the only check that can tell.
live-forge: build ## Walk the forge bridge against a real Forgejo (FORGE_KIND=gitea for Gitea)
	@command -v podman >/dev/null || command -v docker >/dev/null || { echo "podman or docker is required"; exit 1; }
	@scripts/live-forge.sh

docker: ## Build the container image
	@docker build -t $(APP):$(VERSION) --build-arg VERSION=$(VERSION) .

clean: ## Remove build output
	@rm -rf $(BUILD_DIR) web/dist coverage.out
