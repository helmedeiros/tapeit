.DEFAULT_GOAL := check

BINARY := tapeit
PKG := ./...

.PHONY: build
build:
	go build -o bin/$(BINARY) ./cmd/tapeit

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
