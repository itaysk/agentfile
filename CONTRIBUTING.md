# Contributing

Agentfile is docs-driven. The reference docs and UX are normative. Implement documented behavior and update the docs whenever observed behavior changes.

Use `agentfile` for the YAML format and filenames such as `agentfile.yaml`. Use `Agentfile` only for the project or product.

## Architecture

The agent bundle is the boundary between source processing and execution. Build-time packages produce bundles; runtime packages consume them.

- `internal/agentfile` defines the agentfile model, applies defaults, validates declarations, and resolves source content. Its contract is the [reference manual](docs/reference/reference.md).
- `internal/bundle` compiles source projects into portable bundles and validates, extracts, and identifies those bundles. Its contract is the [bundle format](docs/reference/bundle.md).
- `internal/harness` converts an unpacked bundle and invocation parameters into an isolated harness profile and command. It does not launch the harness. Its contract is the [harness reference](docs/reference/harness.md).
- `internal/runa` executes bundles with host-installed harnesses and owns the host process lifecycle. Its contract is the [`runa` reference](docs/reference/runa.md).
- `internal/image` constructs agent images from bundles and generates the container entrypoint. Its contracts are the [image format](docs/reference/image.md) and [entrypoint reference](docs/reference/entrypoint.md).
- `internal/runner` orchestrates execution modes, Docker containers, and ACP sessions. Its behavior is documented in the [reference manual](docs/reference/reference.md).
- `internal/cli` implements the command boundary and delegates work to the owning packages. `cmd/af` contains only the executable entry point. CLI behavior is documented in the [reference manual](docs/reference/reference.md) and [CLI examples](docs/reference/cli.sh).

Dependencies flow from `cmd/af` and `internal/cli` into orchestration packages, then into artifact and runtime packages. `internal/agentfile`, `internal/bundle`, and `internal/harness` must not depend on CLI, registry, or runner packages.

Changes that cross components must update every affected contract.

## Development

Run the default checks before submitting a change:

```bash
make test
make check-examples
go run ./cmd/af --help
```

`make check-examples` verifies documentation code blocks that declare `source=...`. Run `make sync-examples` to rewrite those blocks from their source files.

Docker is required for image operations and Docker-backed tests. It is not required for bundle builds or `runa`.

Git sources require `git` on the build machine.

## Testing

`make test` runs unit and contract tests. These tests may use fake executables for orchestration and must not require Docker, network access, or installed harnesses. A fake `docker` executable verifies Agentfile's Docker command construction, not Docker behavior.

`make image-integration-test` test image packaging but not the harness. It builds one small agent image from a bundle with a no-op harness and verifies behavior supplied by Docker: image labels, literal environment values, secret exclusion from image layers, generated-entrypoint execution, and parity with `runa` configuration rendering.

`make harness-integration-test` tests the harness configuration strategies. It builds the harness base images from `images/*.Dockerfile`, builds generated agent images through the normal runner, and executes the real Codex, Claude Code, and Pi CLIs against a local mock LLM. The mock implements the minimal Anthropic Messages, OpenAI Responses, and OpenAI Chat Completions protocols needed by the harnesses. These tests must not call external model inference and do not define Agentfile behavior.

`make integration-test` runs all integration tests. These tests remove their image tags after each run; retain them for debugging with:

```bash
AF_KEEP_INTEGRATION_IMAGES=1 make integration-test
```
