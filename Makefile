GO_SOURCES := $(shell find . -name '*.go' ! -name '*_test.go')

.PHONY: build test sync-examples
build: af

test:
	go test ./...

sync-examples:
	python3 docs/sync-examples.py --write

check-examples:
	python3 docs/sync-examples.py

af: $(GO_SOURCES) go.mod go.sum
	go build -o $@ ./cmd/af
