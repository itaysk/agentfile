# Agentfile Manual

Agentfile helps you build custom, portable agents from declarative projects.

- No-code agents driven by Markdown and YAML and managed in Git.
- Bring your own harness: Claude Code, Codex, or Pi.
- Run locally, in a container, Kubernetes, or CI/CD.

## Orientation

The project is made of the following areas:

1. An agent bundle packages the agent's configuration and assets in a portable format. Any compatible runtime can use it to re-invoke the agent.
2. Agentfile builds those bundles from declarative projects and can package them as container images.

Terminology:

- An **agentfile** is the YAML file that declares a source project, commonly named `agentfile.yaml`.
- A **project** is the directory containing the agentfile and local source assets.
- An **agent bundle** is a `.tar.gz` archive containing a compiled manifest and materialized assets, but no provisioned executables.
- A **bundle manifest** is the compiled `manifest.json` definition and its runtime requirements. It is not an agentfile.
- An **agent image** is a container image that contains an unpacked bundle, a harness, tools, and an image entrypoint.
- A **harness** is the Claude Code, Codex, or Pi executable that runs the agent.
- A **workspace** is the working directory supplied to the harness.
- **Bundle execution** uses a host-installed harness and has no isolation.

## Create a project

A minimal `agentfile.yaml` selects a harness and model and can optionally include a prompt:

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  prompt:
    text: |
      say hi!
```

Prompts, system prompts, skills, MCP servers, and environment variables can be declared in the same file.

Prompt and skill content can be literal text or loaded from [external sources](reference/agentfile.md#sources).

Conventional `prompt.md`, `system-prompt.md`, and `skills/*` paths are discovered automatically.

See the [agentfile specification](reference/agentfile.md) and [complete YAML example](reference/agentfile.yaml) for more details.

## Build an agent bundle

The agentfile is source code, not a runnable artifact. Build it into an agent bundle before running it.

```bash
af bundle build --file agentfile.yaml --output reviewer.tar.gz
```

`af build` is the equivalent top-level shortcut.

A bundle build applies defaults and discovery, resolves sources, compiles the manifest and harness configuration templates, stages the materialized assets, and writes a reproducible `.tar.gz` file.

`--file` defaults to `agentfile.yaml`. Relative paths are resolved from the current directory. Absolute paths are used as-is.

`--output` defaults to `<metadata.name>__<metadata.version>.tar.gz`, with path-separator characters replaced by `-`. A successful build prints `Built <path>`.

The [agent bundle format](reference/bundle.md) is the normative definition of the archive layout and manifest. The [harness adapter reference](reference/harness.md#build-time-mappings) defines the generated harness configuration.

## Build an agent image

Agent bundles are portable and reusable, but they rely on the host to provide the harness and tools. For production deployments, you can package the bundle, harness, and runtime environment together as an agent image.

```bash
af image build --bundle reviewer.tar.gz --tag reviewer:latest
```

An image is built only from a bundle. Image construction does not revisit the source project, rediscover assets, or refetch sources.

`--base-image` can override the base image. Otherwise, the selected harness determines the default:

- `claudecode`: `itaysk/claudecode:latest`
- `codex`: `itaysk/codex:latest`
- `pi`: `itaysk/pi:latest`

The base image must contain the selected harness and any tools or MCP executables the agent needs. Agentfile does not install them. The easiest way to customize the environment is to derive from an image in the repository's [images directory](../images).

`--tag` defaults to `<agent.name>:<agent.version>` from the bundle manifest. A successful build prints `Built <tag>`.

The image contains:

- Unpacked bundle at `/agent/bundle`
- Generated entrypoint at `/agent/entrypoint`
- Placeholder `/agent/workspace` (typically overridden at runtime)

See the [agent image format](reference/image.md) for construction and layout and the [image entrypoint reference](reference/entrypoint.md) for container-start behavior.

## Run an agent

Choose the artifact based on where its dependencies should live:

- A bundle uses the harness and tools installed on the host. It is convenient for local use, but provides no isolation.
- An image uses the harness and tools packaged in the container. Docker provides the isolation boundary.

Run a bundle:

```bash
af bundle run --bundle reviewer.tar.gz
```

Run an image:

```bash
af image run --image reviewer:latest
```

The shorter `af run` form accepts exactly one artifact selector:

```bash
af run --bundle reviewer.tar.gz
af run --image reviewer:latest
af run --name reviewer
```

The command hierarchy is:

| Canonical command | Top-level equivalent |
| --- | --- |
| `af bundle build [--file FILE] [--output FILE]` | `af build [--file FILE] [--output FILE]` |
| `af bundle run --bundle FILE [RUN FLAGS]` | `af run --bundle FILE [RUN FLAGS]` |
| `af image build --bundle FILE [--base-image REF] [--tag TAG]` | — |
| `af image run --image REF [RUN FLAGS]` | `af run --image REF [RUN FLAGS]` |
| `af agents run --name NAME [RUN FLAGS]` | `af run --name NAME [RUN FLAGS]` |
| `af agents register [--name NAME] (--bundle FILE \| --image REF)` | — |
| `af agents list` | — |
| `af agents remove --name NAME` | — |

Each scoped run command accepts only its own selector. `af run` rejects missing or conflicting selectors before accessing the filesystem, registry, network, or Docker.

See the [CLI reference](reference/cli.sh) for complete command and flag examples.

### Bundle execution

A bundle run:

- Extracts the bundle and creates a private harness profile
- Uses an empty temporary workspace unless you selected
- Finds the harness on `PATH` and starts it as the current user
- Removes temporary files after the run

The selected harness, MCP commands, and other tools must already be installed on the host.

See the [bundle runtime reference](reference/runa.md) for the complete runtime contract.

### Image execution

An image run uses the local image when available and pulls it when absent. Images support one-shot, TUI, and ACP modes.

See the [image entrypoint reference](reference/entrypoint.md) for container startup behavior and the [harness adapter reference](reference/harness.md#invocation-time-mappings) for the exact harness commands and configuration.

### Execution modes

There are three execution modes:

| Mode | Selection | Task source | Lifecycle |
| --- | --- | --- | --- |
| One-shot | default | agentfile prompt or `--prompt` | performs one task and exits |
| TUI | `--tui` | user interacts in the harness terminal | lasts for the terminal session |
| ACP | `--acp` | messages from an ACP client | controlled by the client |

`--tui`, `--acp`, and `--prompt` are mutually exclusive.

One-shot mode requires a prompt from the bundle or `--prompt`. Piped input is forwarded along with that prompt:

```bash
tail -200 app.log | af run --name log-triage
```

`--tui` opens the harness's native interactive terminal without an initial message. The built prompt is ignored. For images, TUI requires the `build.agentfile.harness` label added by current Agentfile builds.

`--acp` exposes a bundle or image to an [Agent Client Protocol](https://agentclientprotocol.com)-compatible client over standard input and output. Each client session starts a separate harness process.

The client supplies the prompt and workspace, so `--prompt`, `--workspace`, and `--ws` are rejected. Client-provided MCP servers are also rejected because MCP configuration belongs to the agentfile.

The ACP bridge accepts text and resource-link prompts and supports streamed messages, thoughts, tool calls, cancellation, and close. File resource links inside the session workspace are translated to `/agent/workspace` for images and the resolved host workspace for bundles.

### Workspace

Agents start in an empty, ephemeral workspace. Select an existing directory when the agent needs input from the host or its output should persist:

```bash
af run --bundle reviewer.tar.gz --workspace ./project
```

`--ws` is the short form of `--workspace`. Relative paths are resolved from the current directory.

For images, Agentfile bind-mounts the selected directory at `/agent/workspace`. For bundles, the harness uses the host path directly. In both cases, the selected directory becomes the harness working directory.

### Environment

Runtime values can come from the command line, an environment file, or the host environment:

- `--env KEY=VALUE` sets a value for this run
- `--env KEY` reads the value from the current environment
- `--env-file FILE` loads values from an `.env` file
- `--env-auto` forwards variables declared by the agentfile from the host into an image

A bundle run already inherits the complete parent environment. Explicit `--env` values take precedence over the other sources.

Referenced runtime values are required. Empty value is valid. See [`runtimeEnv` in the agentfile specification](reference/agentfile.md#runtime-variables) for declaration rules and the [bundle manifest environment fields](reference/bundle.md#bundle-manifest) for their compiled form.

### Authentication

LLM credentials are runtime secrets. Supply them through the invocation environment instead of storing them in the agentfile or bundle.

| Provider | Environment variable |
| --- | --- |
| Anthropic | `ANTHROPIC_API_KEY` |
| OpenAI | `OPENAI_API_KEY` |
| OpenRouter | `OPENROUTER_API_KEY` |

For Claude Code, `CLAUDE_CODE_OAUTH_TOKEN` uses a Claude subscription instead of an API key. Generate one with `claude setup-token`. Subscription authentication is incompatible with Claude Code bare mode. When both credentials are set, `ANTHROPIC_API_KEY` takes precedence.

For Codex, `CODEX_ACCESS_TOKEN` uses ChatGPT-managed subscription or workspace access. When present, it takes precedence over `CODEX_API_KEY` and `OPENAI_API_KEY`.

The [harness credential mappings](reference/harness.md#provider-and-credentials) define the exact per-harness behavior.

### Field overrides

You can customize an individual run without rebuilding the agent:

- `--prompt` replaces the built prompt in one-shot mode or supplies a missing prompt
- `--model` replaces the model name while keeping the provider declared in the agentfile

These overrides apply only to the current run. They do not modify the agentfile, bundle, or image.

For direct container execution, `AGENTFILE_PROMPT` and `AGENTFILE_MODEL` are the equivalent entrypoint-level overrides.

### Input and output

Streams depend on the execution mode:

| Mode | Standard input | Standard output | Standard error |
| --- | --- | --- | --- |
| One-shot | piped input is forwarded to the harness | harness output | hidden unless `--debug`; printed automatically if the run fails |
| TUI | attached to the terminal | attached to the terminal | attached to the terminal |
| ACP | client protocol messages | protocol messages only | diagnostics |

Image pull progress is always written to standard error.

One-shot and TUI runs preserve the harness exit code. An ACP run exits when the client closes the protocol connection; session harness failures are reported through ACP. Errors detected by `af` before the harness starts exit with status 1. Docker invocation failures may use Docker's own exit code.

## Register agents by name

Register bundles or images that you run frequently. The agent inventory maps a user-friendly name to the artifact, so you can run it with `af run --name NAME`.

Register a bundle or image:

```bash
af agents register [--name NAME] (--bundle FILE | --image REF)
```

If `--name` is omitted, Agentfile infers it from the bundle manifest or the local image's `build.agentfile.metadata` label.

Registration behaves differently for each artifact type:

- A bundle is validated and copied into managed storage under its SHA-256 digest. Identical archives share one managed copy.
- An image must be a valid local Agentfile image. Registration does not pull it.

Registering the same name replaces the existing entry. Unreferenced managed bundle copies are removed. A successful registration prints `Registered <name>`.

List or remove registrations:

```bash
af agents list
af agents remove --name NAME
```

The list has `NAME`, `VERSION`, `HARNESS`, and `DIGEST` columns. `DIGEST` is the source bundle's SHA-256 digest shortened to its first 12 hexadecimal characters. Metadata unavailable appears as `-`.

Removing a bundle registration removes its managed copy only when no other entry refers to it. Removing an image registration does not remove the image itself.

The registry index is `$UserConfigDir/agentfile/registry.json`. Managed bundles are stored at `$UserConfigDir/agentfile/bundles/<sha256>.tar.gz`, where `$UserConfigDir` is the [OS user configuration directory](https://pkg.go.dev/os#UserConfigDir).

## Run an image directly

Agent images are ordinary container images, so you can run them without the `af` CLI:

```bash
docker run --rm -e ANTHROPIC_API_KEY hello-world:latest
```

Mount a workspace when needed:

```bash
docker run --rm -e ANTHROPIC_API_KEY -v "$PWD:/agent/workspace" hello-world:latest
```

Set the entrypoint mode to start the harness TUI:

```bash
docker run --rm -it -e AGENTFILE_RUN_MODE=tui -e ANTHROPIC_API_KEY -v "$PWD:/agent/workspace" hello-world:latest
```

ACP requires protocol translation from `af image run --image REF --acp`, so it cannot be started with direct `docker run`.

## Security

An agent bundle is an artifact, not a sandbox. Its instructions and skill scripts can cause arbitrary actions.

The trust boundary depends on how you run the agent:

- A bundle launches the harness as the current user without isolation or approval gates. It inherits the parent environment and can access the user's files, credentials, processes, tools, and network resources. Run only trusted bundles and workspaces.
- An image uses the container as its isolation boundary. Harness approval gates remain disabled inside that boundary. Add container-runtime security controls when you need stronger isolation.

Use `runtimeEnv` for secrets. Bundle and image contents, metadata, layers, and build logs are not confidential.

See the [bundle](reference/bundle.md#sensitive-information), [image](reference/image.md#sensitive-information), [bundle runtime](reference/runa.md#security), and [entrypoint](reference/entrypoint.md#isolation) security contracts for more details.

## Reference map

| Reference | Owns |
| --- | --- |
| [agentfile specification](reference/agentfile.md) | source YAML, defaults, sources, discovery, and validation |
| [JSON Schema](reference/agentfile.schema.json) | machine-readable agentfile shape |
| [Agent bundle format](reference/bundle.md) | archive and manifest contract |
| [Harness adapter reference](reference/harness.md) | harness mappings, profiles, commands, and credentials |
| [Agent image format](reference/image.md) | image construction, layout, configuration, and labels |
| [Agent image entrypoint](reference/entrypoint.md) | container-start behavior |
| [Bundle runtime](reference/runa.md) | unsandboxed host bundle execution |
| [CLI reference](reference/cli.sh) | command and flag examples |
