VERSION ?= 2.17.0
BUILD_ID ?= dev
GOCACHE ?= /tmp/agy-swap-go-cache
TARGET_DIR ?= $(HOME)/.local/bin
GOLANGCI_LINT_VERSION ?= v2.14.0
GOVULNCHECK_VERSION ?= v1.9.0

.PHONY: help status release-check release-notes release-evidence build test race vet lint vuln benchmark tui-smoke qa bump install release-assets publish-homebrew-tap live site-preview site-release site-verify-live

help:
	@echo "Available make targets:"
	@echo "  status          Read-only overview: repo sync, versions, live site, installed binary, open work"
	@echo "  release-check   Every pre-gate check: qa, vuln, install, Site build and tests, production preview"
	@echo "  release-notes   Print the CHANGELOG section for VERSION (use as gh release --notes-file)"
	@echo "  release-evidence  Post-publication checks for VERSION, printed as delivery-record Markdown"
	@echo "  build           Build local agy-swap binary"
	@echo "  install         Build and install binary to $(TARGET_DIR)"
	@echo "  test            Run unit tests across all packages"
	@echo "  race            Run unit tests with Go race detector"
	@echo "  vet             Run go vet"
	@echo "  lint            Run golangci-lint (installed, or the pinned version via go run)"
	@echo "  vuln            Run govulncheck against reachable code (needs network)"
	@echo "  benchmark       Run memory allocation benchmarks"
	@echo "  tui-smoke       Run terminal UI smoke tests"
	@echo "  qa              Run complete QA pipeline (formatting, test, race, vet, lint, tui-smoke)"
	@echo "  release-assets  Cross-compile release assets for all platforms"
	@echo "  publish-homebrew-tap  Explicitly commit/push the tap after release approval"
	@echo "  live            Run AGY Live locally (demo gateway + Vite, first free ports)"
	@echo "  site-preview    Build and serve the production site image locally on :4796"
	@echo "  site-release    Build, smoke-test, and (PUSH=1) publish the site image for VERSION"
	@echo "  site-verify-live  Wait for production to serve VERSION, then compare it with the tag build"

build:
	GOCACHE=$(GOCACHE) go build -trimpath -ldflags "-s -w -X main.version=$(VERSION) -X main.buildID=$(BUILD_ID)" -o agy-swap ./cmd/agy-swap

install: build
	mkdir -p $(TARGET_DIR)
	install -m 755 ./agy-swap $(TARGET_DIR)/agy-swap
	@$(TARGET_DIR)/agy-swap --version

bump:
	go run ./cmd/releasetool bump $(if $(VERSION),$(VERSION),patch) .

release-assets:
	GOVULNCHECK_VERSION=$(GOVULNCHECK_VERSION) ./scripts/build-release.sh $(VERSION) $(BUILD_ID) dist/release

publish-homebrew-tap:
	./scripts/update-homebrew-tap.sh $(VERSION) dist/release --publish

live:
	./scripts/agy-live-dev.sh

site-preview:
	./scripts/site-image.sh preview

site-release:
	./scripts/site-image.sh release $(VERSION) $(if $(PUSH),--push,)

site-verify-live:
	GOCACHE=$(GOCACHE) ./scripts/site-verify-live.sh $(VERSION)

status:
	@./scripts/project-status.sh

release-check: qa vuln install
	cd site && bun install --frozen-lockfile && bun run build && bun run test
	./scripts/site-image.sh preview

release-notes:
	@GOCACHE=$(GOCACHE) go run ./cmd/releasetool release-notes $(VERSION) CHANGELOG.md

release-evidence:
	@GOCACHE=$(GOCACHE) ./scripts/release-evidence.sh $(VERSION)

test:
	GOCACHE=$(GOCACHE) go test ./...

race:
	GOCACHE=$(GOCACHE) go test -race ./...

vet:
	GOCACHE=$(GOCACHE) go vet ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		GOCACHE=$(GOCACHE) golangci-lint run ./...; \
	else \
		GOCACHE=$(GOCACHE) go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...; \
	fi

vuln:
	GOCACHE=$(GOCACHE) go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

benchmark:
	GOCACHE=$(GOCACHE) go test -run '^$$' -bench . -benchmem ./internal/app

tui-smoke: build
	./scripts/tui-smoke.sh ./agy-swap

qa:
	@test -z "$$(gofmt -l cmd internal)" || (echo "gofmt required:"; gofmt -l cmd internal; exit 1)
	$(MAKE) test
	$(MAKE) race
	$(MAKE) vet
	$(MAKE) lint
	$(MAKE) tui-smoke
