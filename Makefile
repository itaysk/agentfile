GO_SOURCES := $(shell find . -name '*.go' ! -name '*_test.go')

.PHONY: build test image-integration-test harness-integration-test integration-test sync-examples check-examples
build: af

test:
	go test ./...

image-integration-test:
	AF_INTEGRATION=1 go test -count=1 ./internal/image -run '^TestImageSmoke$$'

harness-integration-test:
	AF_INTEGRATION=1 go test -count=1 ./internal/runner -run '^TestHarnessCLIsWithMockLLM$$'

integration-test: image-integration-test harness-integration-test

sync-examples:
	python3 -B docs/sync-examples.py --write

check-examples:
	python3 -B docs/sync-examples.py

af: $(GO_SOURCES) go.mod go.sum
	go build -o $@ ./cmd/af
