VERSION ?= 2.1.2-dev
BUILD_ID ?= dev
GOCACHE ?= /tmp/agy-swap-go-cache

.PHONY: build test race vet benchmark tui-smoke qa

build:
	GOCACHE=$(GOCACHE) go build -trimpath -ldflags "-s -w -X main.version=$(VERSION) -X main.buildID=$(BUILD_ID)" -o agy-swap ./cmd/agy-swap

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
