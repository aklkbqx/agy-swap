VERSION ?= 2.1.1-dev
GOCACHE ?= /tmp/agy-swap-go-cache

.PHONY: build test race vet benchmark

build:
	GOCACHE=$(GOCACHE) go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o agy-swap ./cmd/agy-swap

test:
	GOCACHE=$(GOCACHE) go test ./...

race:
	GOCACHE=$(GOCACHE) go test -race ./...

vet:
	GOCACHE=$(GOCACHE) go vet ./...

benchmark:
	GOCACHE=$(GOCACHE) go test -run '^$$' -bench . -benchmem ./internal/app
