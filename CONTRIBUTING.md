# Contributing

Agentfile is docs-driven. The reference docs and UX are the contract: if docs describe behavior, code should implement it; if code behavior changes, update the docs in the same change.

Use `agentfile` for the YAML file and literal filenames like `agentfile.yaml`. Use `Agentfile` only for the project or product name.

## Development

Fast local checks:

```bash
make test
go run ./cmd/af --help
go install ./cmd/af
```

`make test` runs unit tests only and does not require Docker.

Docker is required for `af build`, `af run`, and `make integration-test`. Git sources require `git` on the build machine.

Docs examples are checked separately:

```bash
make check-examples
make sync-examples
```

`check-examples` verifies fenced code blocks that declare `source=...`; `sync-examples` rewrites those blocks from their source files.

## Integration Tests

Run Docker-backed harness tests with:

```bash
make integration-test
```

Integration tests build the harness base images from `images/*.Dockerfile`, build generated agent images through the normal runner, then run the generated images against a local mock LLM API.

The mock LLM returns `dummy response`, records requests, and speaks only the minimal protocols the supported harnesses need: Anthropic Messages, OpenAI Responses, and OpenAI Chat Completions. Tests must not call external model inference.

By default, integration tests remove their Docker image tags (`agentfile-integration-*`) after the run. To retain them:

```bash
AF_KEEP_INTEGRATION_IMAGES=1 make integration-test
```

Keep test-only routing out of generated entrypoints and user-facing schema. Integration tests may use test-only runner plumbing or Docker arguments, but packaged agent images should behave like normal product images.

## Harness Changes

Harness changes must go through `docs/reference/harness.md` first. That is the canonical reference for supported harnesses and their behavior.
