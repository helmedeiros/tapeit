.DEFAULT_GOAL := check

BINARY := tapeit
PKG := ./...

# VERSION comes from the nearest tag, so a built binary can say what it is.
# cmd/tapeit declares `version = "dev"` and nothing was injecting it, which meant
# every build — including a released one — reported itself as "dev".
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/tapeit

.PHONY: version
version:
	@echo $(VERSION)

.PHONY: test
test:
	go test -race $(PKG)

.PHONY: fmt
fmt:
	gofmt -l -w .

.PHONY: vet
vet:
	go vet $(PKG)

.PHONY: lint
lint:
	golangci-lint run

.PHONY: check
check: fmt vet lint test

.PHONY: clean
clean:
	rm -rf bin dist
