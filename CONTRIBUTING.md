# Contributing

Agentfile is docs-driven. The reference docs and UX are normative. Implement documented behavior and update the docs whenever observed behavior changes.

Use `agentfile` for the YAML format and filenames such as `agentfile.yaml`. Use `Agentfile` only for the project or product.

## Orientation

The project is made of the following areas:

1. An agent bundle packages the agent's configuration and assets in a portable format. Any compatible runtime can use it to re-invoke the agent.
   1. The bundle format is defined in the [bundle specification](docs/reference/bundle.md).
   2. Runtime is defined in the [bundle runtime specification](docs/reference/runa.md).
2. Agentfile builds those bundles from declarative projects and can package them as container images.
   1. The agentfile YAML format is defined in the [agentfile specification](docs/reference/agentfile.md).
   2. The image format is defined in the [image specification](docs/reference/image.md).
   3. The command-line interface and user experience are defined in the [product manual](docs/manual.md).

Terminology:

- An **agentfile** is the YAML file that declares a source project, commonly named `agentfile.yaml`.
- A **project** is the directory containing the agentfile and local source assets.
- An **agent bundle** is a `.tar.gz` archive containing a compiled manifest and materialized assets, but no provisioned executables.
- A **bundle manifest** is the compiled `manifest.json` definition and its runtime requirements. It is not an agentfile.
- An **agent image** is a container image that contains an unpacked bundle, a harness, tools, and an image entrypoint.
- A **harness** is the Claude Code, Codex, or Pi executable that runs the agent.
- A **workspace** is the working directory supplied to the harness.
- **Bundle execution** uses a host-installed harness and has no isolation.

## Architecture

The agent bundle separates build-time processing from runtime execution.

- Build: `internal/agentfile`, `internal/bundle`, and `internal/image`.
- Runtime: `internal/harness` and `internal/runa`.
- Product: `internal/registry`, `internal/runner`, and `internal/cli`; `cmd/af` is the executable entry point.

Dependencies flow from the product packages into the build and runtime packages. `internal/agentfile`, `internal/bundle`, and `internal/harness` must not depend on CLI, registry, or runner packages.

Changes that cross components must update every affected contract.

## Development

Run the default checks before submitting a change:

```bash
make test
make check-examples
go run ./cmd/af --help
```

`make check-examples` verifies documentation code blocks that declare `source=...`. Run `make sync-examples` to rewrite those blocks from their source files.

Docker is required for image operations and Docker-backed tests. It is not required for bundle builds or bundle runtime execution.

Git sources require `git` on the build machine.

## Testing

`make test` runs unit and contract tests. These tests may use fake executables for orchestration and must not require Docker, network access, or installed harnesses. A fake `docker` executable verifies Agentfile's Docker command construction, not Docker behavior.

`make image-integration-test` test image packaging but not the harness. It builds one small agent image from a bundle with a no-op harness and verifies behavior supplied by Docker: image labels, literal environment values, secret exclusion from image layers, generated-entrypoint execution, and parity with `runa` configuration rendering.

`make harness-integration-test` tests the harness configuration strategies. It builds the harness base images from `images/*.Dockerfile`, builds generated agent images through the normal runner, and executes the real Codex, Claude Code, and Pi CLIs against a local mock LLM. The mock implements the minimal Anthropic Messages, OpenAI Responses, and OpenAI Chat Completions protocols needed by the harnesses. These tests must not call external model inference and do not define Agentfile behavior.

`make integration-test` runs all integration tests. These tests remove their image tags after each run; retain them for debugging with:

```bash
AF_KEEP_INTEGRATION_IMAGES=1 make integration-test
```
