GO_SOURCES := $(shell find . -name '*.go' ! -name '*_test.go')

.PHONY: build test integration-test sync-examples check-examples
build: af

test:
	go test ./...

integration-test:
	AF_INTEGRATION=1 go test -count=1 ./...

sync-examples:
	python3 docs/sync-examples.py --write

check-examples:
	python3 docs/sync-examples.py

af: $(GO_SOURCES) go.mod go.sum
	go build -o $@ ./cmd/af
