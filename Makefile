VERSION ?= 2.3.0
BUILD_ID ?= dev
GOCACHE ?= /tmp/agy-swap-go-cache
TARGET_DIR ?= $(HOME)/.local/bin

.PHONY: build test race vet benchmark tui-smoke qa bump install release-assets

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
