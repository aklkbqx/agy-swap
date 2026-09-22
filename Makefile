VERSION ?= 2.3.1
BUILD_ID ?= dev
GOCACHE ?= /tmp/agy-swap-go-cache
TARGET_DIR ?= $(HOME)/.local/bin

.PHONY: help build test race vet lint benchmark tui-smoke qa bump install release-assets

help:
	@echo "Available make targets:"
	@echo "  build           Build local agy-swap binary"
	@echo "  install         Build and install binary to $(TARGET_DIR)"
	@echo "  test            Run unit tests across all packages"
	@echo "  race            Run unit tests with Go race detector"
	@echo "  vet             Run go vet"
	@echo "  lint            Run golangci-lint (or vet/fmt fallback)"
	@echo "  benchmark       Run memory allocation benchmarks"
	@echo "  tui-smoke       Run terminal UI smoke tests"
	@echo "  qa              Run complete QA pipeline (gofmt, test, race, vet, tui-smoke)"
	@echo "  release-assets  Cross-compile release assets for all platforms"

build:
	GOCACHE=$(GOCACHE) go build -trimpath -ldflags "-s -w -X main.version=$(VERSION) -X main.buildID=$(BUILD_ID)" -o agy-swap ./cmd/agy-swap

install: build
	mkdir -p $(TARGET_DIR)
	install -m 755 ./agy-swap $(TARGET_DIR)/agy-swap
	@$(TARGET_DIR)/agy-swap --version

bump:
	go run ./cmd/releasetool bump $(if $(VERSION),$(VERSION),patch) .

release-assets:
	./scripts/build-release.sh $(VERSION) $(BUILD_ID) dist/release

test:
	GOCACHE=$(GOCACHE) go test ./...

race:
	GOCACHE=$(GOCACHE) go test -race ./...

vet:
	GOCACHE=$(GOCACHE) go vet ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		$(MAKE) vet; \
		test -z "$$(gofmt -l cmd internal)" || (echo "gofmt required:"; gofmt -l cmd internal; exit 1); \
	fi

benchmark:
	GOCACHE=$(GOCACHE) go test -run '^$$' -bench . -benchmem ./internal/app

tui-smoke: build
	./scripts/tui-smoke.sh ./agy-swap

qa:
	@test -z "$$(gofmt -l cmd internal)" || (echo "gofmt required:"; gofmt -l cmd internal; exit 1)
	$(MAKE) test
	$(MAKE) race
	$(MAKE) vet
	$(MAKE) tui-smoke
