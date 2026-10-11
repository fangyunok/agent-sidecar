SHELL := /usr/bin/env bash
BIN  := bin/agent-sidecar
PKG  := ./cmd/aggregator ./internal/...

.PHONY: build test test-race vet fmt run clean

build:
	go build -trimpath -o $(BIN) ./cmd/aggregator

test:
	go test $(PKG)

test-race:
	go test -race $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -s -w .

run: build
	./$(BIN) --input testdata/sample_runs

clean:
	rm -rf bin
