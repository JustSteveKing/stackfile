# stackfile — see `make help`.
#
# Targets carrying a `##` comment are the ones meant to be run by hand; help
# lists exactly those, so a new target documents itself or stays out of the way.

BIN     := bin/stackfile
PKG     := ./cmd/stackfile

# Stamped into main.version, without the leading "v", so a local build reads
# the same as a `go install` one.
VERSION := $(patsubst v%,%,$(shell git describe --tags --always --dirty 2>/dev/null || echo dev))
LDFLAGS := -X main.version=$(VERSION)
GO      ?= go

.DEFAULT_GOAL := help

.PHONY: build
build: ## Build the binary into bin/
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN) $(PKG)

.PHONY: install
install: ## Install stackfile into GOBIN
	$(GO) install -ldflags '$(LDFLAGS)' $(PKG)

.PHONY: check
check: fmt-check tidy-check vet test-race dogfood ## Everything CI runs

.PHONY: fmt
fmt: ## Format the code
	gofmt -w .

# Checked rather than applied: a build should tell you a file is unformatted,
# not quietly rewrite it underneath you.
.PHONY: fmt-check
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: tidy-check
tidy-check:
	@cp go.mod go.mod.bak && cp go.sum go.sum.bak
	@$(GO) mod tidy
	@if ! cmp -s go.mod go.mod.bak || ! cmp -s go.sum go.sum.bak; then \
		mv go.mod.bak go.mod; mv go.sum.bak go.sum; \
		echo "go.mod or go.sum is not tidy; run 'go mod tidy'"; exit 1; \
	fi
	@rm -f go.mod.bak go.sum.bak

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: test
test: ## Run the tests
	$(GO) test ./...

.PHONY: test-race
test-race:
	$(GO) test -race ./...

# The tests already hold these, but running the binary the way a user would
# is the check that the CLI and the tests have not drifted apart.
.PHONY: dogfood
dogfood: ## Validate our own stackfile, check it against go.mod, and check AGENTS.md
	$(GO) run $(PKG) validate --strict
	$(GO) run $(PKG) check --strict
	$(GO) run $(PKG) render --check

.PHONY: render
render: ## Regenerate the stack block in AGENTS.md
	$(GO) run $(PKG) render

.PHONY: clean
clean: ## Remove build output
	rm -rf bin/ dist/ coverage.out

.PHONY: help
help: ## Show this help
	@echo "stackfile $(VERSION)"
	@echo
	@awk 'BEGIN {FS = ":.*?## "} \
		/^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
