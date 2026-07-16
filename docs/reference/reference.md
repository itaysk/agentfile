# Agentfile Manual

Agentfile helps you build custom and portable agents.

- No code, declarative agents - Driven by Markdown and YAML and managed in git.  
- Bring your own harness - Claude, Codex, Pi, and more.  
- Deploy anywhere - Locally, in cloud, Kubernetes, or CI/CD.
 
This file is the product manual and implementation spec.  

## Table of Contents

- [Terms and Concepts](#terms-and-concepts)
- [Agentfile](#agentfile)
- [Agent Specification](#agent-specification)
  - [Harness](#harness)
    - [Claude Code](#claude-code)
  - [LLM](#llm)
    - [Subscription plans](#subscription-plans)
  - [Prompt](#prompt)
  - [System Prompt](#system-prompt)
  - [Skills](#skills)
  - [MCP Servers](#mcp-servers)
  - [Environment](#environment)
  - [Runtime Variables](#runtime-variables)
  - [Workspace](#workspace)
- [Sources](#sources)
  - [Text Source](#text-source)
  - [Filesystem Source](#filesystem-source)
  - [Git Source](#git-source)
  - [HTTP Source](#http-source)
- [Discovery](#discovery)
- [CLI](#cli)
  - [Build](#build)
  - [Run](#run)
    - [TUI Mode](#tui-mode)
    - [ACP Mode](#acp-mode)
    - [Field Overrides](#field-overrides)
  - [Agents](#agents)
    - [Register](#register)
    - [List](#list)
    - [Remove](#remove)
  - [Configuration](#configuration)
- [Direct Docker Use](#direct-docker-use)
- [Security](#security)

## Terms and Concepts

These terms are used by the rest of the manual.

- agentfile: YAML file that declares and describes an agent. Commonly named `agentfile.yaml`.
- Project: directory where the agentfile and local source assets live.
- Agent bundle: deterministic `.tar.gz` archive containing a bundle manifest and materialized assets, but no provisioned executables.
- Bundle manifest: compiled `manifest.json` definition of an agent bundle and its run-time requirements. It is not an agentfile.
- Unpacked bundle: directory containing an agent bundle's archive contents.
- Agent image: container image containing unpacked bundle contents, a harness, tools, and a generated image entrypoint.
- Harness adapter: component that maps a bundle manifest and harness invocation to harness-specific assets, configuration, and a harness command.
- Harness invocation: execution mode, workspace, environment, and prompt or model overrides for one harness run.
- `runa`: Agentfile's unsandboxed host runner for agent bundles.
- Execution mode: one-shot, TUI, or ACP interaction between the harness and its caller.
- Harness: Claude Code, Codex, or Pi executable that runs the agent.
- LLM: the model provider and model used by the harness.
- Assets: prompt, system prompt, skill, and other markdown content that make up the agent.
- Sources: strategies for loading content into the build.
- Workspace: working directory supplied to the harness; `/agent/workspace` in an agent image and the selected or temporary host directory with `runa`.

## Agentfile

An agentfile declares an agent. It is a YAML document modeled like a Kubernetes resource.

Field names are case-sensitive.  
Unknown fields are invalid.

The top-level shape is:

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: string
  version: string
spec:
  harness: {}
  llm: {}
  prompt: {}
  systemPrompt: {}
  skills: []
  mcps: []
  envs: []
```

`apiVersion`, `kind`, `metadata.name`, `spec.harness`, and `spec.llm` are required.  
`apiVersion` must be `agentfile.build/v1`.  
`kind` must be `Agent`.  
`metadata.name` must be a non-empty string.  
`metadata.version` is optional. If omitted or empty, it defaults to `latest`.

Example:

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

## Agent Specification

### Harness

Choose the harness executable that runs the agent.

The selected harness controls how the agentfile spec is translated into harness-specific instructions, commands, and configuration.

The [Harness reference](./harness.md) is the normative companion for this behavior. It defines profile files, environment variables, commands, and unsupported combinations for each harness.

Exactly one harness selector key must be set.

```yaml
spec:
  harness:
    claudecode: {}
```

```yaml
spec:
  harness:
    codex: {}
```

```yaml
spec:
  harness:
    pi: {}
```

Image builds accept `--base-image` to override the base image. If omitted, the default is selected from the bundle's harness:

- `claudecode`: `itaysk/claudecode:latest`
- `codex`: `itaysk/codex:latest`
- `pi`: `itaysk/pi:latest`

The selected base image must contain the selected harness executable. Agentfile adds the generated entrypoint during image construction.
The easiest way to create a custom image is to derive from an existing one.  
Images are built from Dockerfiles in [/images](/images).

Agentfile does not install tools declared elsewhere. Add tools to the base image.

#### Claude Code

The `claudecode` harness accepts an optional `bare` field:

```yaml
spec:
  harness:
    claudecode:
      bare: true
```

`bare` opts into claude's bare mode, which minimizes claude's footprint and startup time.  
`bare: true` cannot be combined with `spec.skills` or with Claude subscription auth (`CLAUDE_CODE_OAUTH_TOKEN`).
For more information, see [Bare mode](./harness.md#bare-mode) in the Harness reference.

### LLM

Use `spec.llm` to configure the model provider and model used by the harness.  
Exactly one provider key must be set.  
Supported providers are `anthropic`, `openai`, and `openrouter`.  
Each provider requires `model`. Valid model names are determined by the provider.
The model can be overridden for a single agent run using the `--model` flag.

```yaml
spec:
  llm:
    anthropic:
      model: claude-haiku-4-5
```

```yaml
spec:
  llm:
    openai:
      model: gpt-5-mini
```

```yaml
spec:
  llm:
    openrouter:
      model: anthropic/claude-haiku-4-5
```

Model names are strings. Agentfile does not validate model catalogs.

LLM credentials are invocation input, read from the environment when the agent runs.

See [Runtime Variables](#runtime-variables) for how to automate this.

Well-known harness/LLM provider environment variables:

| Provider | Environment variable |
| --- | --- |
| `anthropic` | `ANTHROPIC_API_KEY` |
| `openai` | `OPENAI_API_KEY` |
| `openrouter` | `OPENROUTER_API_KEY` |

#### Anthropic/Claude Subscription

`CLAUDE_CODE_OAUTH_TOKEN` bills usage to a Claude subscription plan instead of an API key. 

Generate it with `claude setup-token`.

[Bare mode](./harness.md#bare-mode) does not support Claude subscription auth, so the agent must not set `spec.harness.claudecode.bare: true`.  

If both `CLAUDE_CODE_OAUTH_TOKEN` and `ANTHROPIC_API_KEY` are set, `ANTHROPIC_API_KEY` takes precedence.

### OpenAI/Codex Subscription

`CODEX_ACCESS_TOKEN` uses Codex with ChatGPT-managed subscription or workspace access instead of an API key. 

Business and Enterprise workspaces can create Codex access tokens in ChatGPT.

If `CODEX_ACCESS_TOKEN` is set, it takes precedence over `CODEX_API_KEY` and `OPENAI_API_KEY`.

### Prompt

In one-shot mode, use `spec.prompt` to specify the agent's task.

This is the one and only prompt in one-shot mode. A one-shot run must receive a prompt from `spec.prompt` or an invocation prompt override.
Prompt content is supplied with a [source object](#sources).
The prompt can be overridden for a single agent run using the `--prompt` flag.

```yaml
spec:
  prompt:
    text: |
      summarize the files in the workspace
```

### System Prompt

Use `spec.systemPrompt` for standing instructions that define the agent's charecter and behavior.
System prompt content is supplied with a [source object](#sources).

```yaml
spec:
  systemPrompt:
    fs:
      path: prompts/system.md
```

`systemPrompt` is optional. If omitted, the harness default applies.

### Skills

Skills add reusable instruction bundles to the agent.  
`spec.skills` is a list of [source objects](#sources). Each source must resolve to one skill directory.  
A skill directory must contain `SKILL.md`, and the skill name is the `name` field in its YAML front matter.
Skill names must be unique within `spec.skills`.
Skill names must be single path segments: no `/` or `\`, and not exactly `.` or `..`.

```yaml
spec:
  skills:
    - fs:
        path: bundles/world-greetings
```

### MCP Servers

Register MCP servers to make external tools available to the harness.  
`spec.mcps` is a list of server registrations.  
Each MCP server requires `name`.  
`name` must be a non-empty string.  
MCP server names must be unique within `spec.mcps`.
Exactly one transport must be set.  
Supported transports are `stdio` and `http`.

```yaml
spec:
  mcps:
    - name: time
      stdio:
        command: ["uv", "tool", "run", "mcp-server-time"]
```

```yaml
spec:
  mcps:
    - name: search
      http:
        url: https://example.com/mcp
        headers:
          - name: Authorization
            runtimeEnv:
              name: SEARCH_MCP_AUTH
```

For `stdio`, `command` is required and must be a non-empty string array. `envs` is optional:

```yaml
stdio:
  command: ["tool"]
  envs:
    - name: EXAMPLE
      value: value
    - name: GITHUB_PERSONAL_ACCESS_TOKEN
      runtimeEnv:
        name: GITHUB_TOKEN
```

MCP `envs` entries use the same shape and name rules as `spec.envs`, see [Environment](#environment).

For `http`, `url` is required.  
`headers` is optional. Header entries use the same value rules as `spec.envs` entries, see [Environment](#environment). Name may be any valid HTTP header name (not starting with `AGENTFILE_`).

MCP commands run in the same environment as the harness.

Agentfile only registers MCP servers; it does not install MCP server binaries in a bundle or on the host.

An agent image must provide them through its base image.

Note that Claude Code performs its own `${VAR}` expansion on some mcp.json fields after Agentfile renders them, so a value (literal or runtime) whose content contains `${...}` may be further expanded by Claude Code.

### Environment

Use `spec.envs` to set environment variables in the agent.

```yaml
spec:
  envs:
    - name: LOG_LEVEL
      value: info
    - name: GITHUB_TOKEN
      runtimeEnv:
        name: GITHUB_TOKEN
```

Each entry requires `name` and exactly one value source:

- `value` — a public literal default stored in the bundle and materialized as an image `ENV` default.
- `runtimeEnv` — read from the invocation environment. See [Runtime Variables](#runtime-variables).

`name` must match `[A-Za-z_][A-Za-z0-9_]*` and must not start with the reserved prefix `AGENTFILE_`.

Literal values are defaults: a `value` entry is applied only when the variable is not already set.

`runa` checks the inherited environment. An agent image stores the value as an overridable image environment default.

### Runtime Variables

A `runtimeEnv` entry declares that a value is unknown at build time and is read from the invocation environment.

Environment values supplied at invocation time never appear in bundles or image layers, which makes `runtimeEnv` the right choice for secrets.

`runtimeEnv.name` is the environment variable to read. It must match `[A-Za-z_][A-Za-z0-9_]*` and must not start with the reserved prefix `AGENTFILE_`.

Runtime variables work in the following name/value entries: `spec.envs[]`, `spec.mcps[].stdio.envs[]`, `spec.mcps[].http.headers[]`.

```yaml
spec:
  envs:
    - name: GH_TOKEN
      runtimeEnv:
        name: GITHUB_TOKEN
  mcps:
    - name: search
      http:
        url: https://example.com/mcp
        headers:
          - name: Authorization
            runtimeEnv:
              name: SEARCH_MCP_AUTH
```

Runtime variables are required at invocation time: harness command preparation fails when a runtime variable is not provided.
Empty is a value: a variable set to the empty string is used verbatim; only an unset variable is considered not provided.  

For Docker runs, `af run --env-auto` forwards declared runtime variables from the host environment.

`runa` already inherits the complete parent environment. See [Run](#run).

### Workspace

The workspace is the agent's working directory for input, output, and temporary files. The default is empty and ephemeral.

Inside an agent image, the path is always:

```text
/agent/workspace
```

The harness in an agent image runs with `/agent/workspace` as its working directory.

`af run --workspace PATH` uses an existing directory.

Docker bind-mounts it at `/agent/workspace`; `runa` uses the host path directly.

`PATH` must be an existing directory. Relative paths are resolved from the current working directory. Use `--ws` as a shorter alias.

When using `docker run` directly, you still need to mount the workspace yourself.

```bash
docker run --rm -v "$PWD:/agent/workspace" hello-world:latest
```

## Sources

Assets can be loaded from a variety of sources.  
Exactly one source type must be set.

### Text Source

`text` embeds literal content.

```yaml
text: |
  say hi
```

### Filesystem Source

`fs` reads from the filesystem of the build machine.  
Exactly one path field must be set.

```yaml
fs:
  path: assets/content.md
```

```yaml
fs:
  absolutePath: /opt/agentfile/content.md
```

`path` is relative to the agentfile directory.
`absolutePath` is an absolute path on the build machine.

### Git Source

`git` reads from a Git repository.

```yaml
git:
  url: https://github.com/example/repo.git//path/in/repo
  ref: main
```

`url` is required.  
URL must start with a repository location using `http` or `ssh` schemes.  
Append `//path/in/repo` to select a file or directory inside the repository.  
The separator is the last `//` in the URL.

Exactly one of `ref` or `commit` may be set.  
If neither is set, the remote default branch is used at build time.
Sources without `commit` use shallow clones. `commit` sources first try a shallow clone plus a shallow fetch of the requested commit, then fall back to a full clone if the remote does not support fetching by commit.

### HTTP Source

`http` reads from a URL.

```yaml
http:
  url: https://example.com/content.md
```

```yaml
http:
  url: https://example.com/skill.tar.gz
  archive: true
```

`url` is required.  
`archive` is optional. Its default is `false`.

If `archive` is `false`, the response body is used as one file.  
If `archive` is `true`, the response body is extracted.  
Supported archive formats are `zip`, `tar`, `tar.gz`, and `tgz`. Archive format is detected from the URL suffix first, then by common magic bytes such as zip and gzip when the URL does not include a useful extension.
Archive extraction writes only directories and regular files. Symlinks and other special entries are skipped, and archive mode bits are reduced to regular permission bits.

HTTP redirects are followed.  
HTTP source fetches must complete within 60 seconds and responses must be at most 100 MiB.  
Non-2xx HTTP responses are invalid.

## Discovery

Discovery populates agentfile assets based on project files automatically at build-time.
It is applied after reading the agentfile and before validation and source resolution.

Singular assets are discovered only when their `spec` field is absent. List assets append discovered entries after explicit entries.  
Each discovered asset is added to the loaded agentfile as an `fs` source.

`prompt.md` discovered as `spec.prompt`.
`system-prompt.md` is discovered as `spec.systemPrompt`.

```yaml
spec:
  prompt:
    fs:
      path: prompt.md
```

`skills/<name>` directories are discovered as `spec.skills[name]` and sorted in path order.

```yaml
spec:
  skills:
    - fs:
        path: skills/name
```

No recursive skill discovery is performed below `skills/*`.

## CLI

Use the CLI to build, register, list, and run agents. Use `af --help` to show help and usage.

### Build

Build targets describe the artifact to create. The default target is `image`.

```bash
af build --target bundle --file agentfile.yaml --output reviewer.tar.gz
af build --target image --file agentfile.yaml --base-image example/agent-base:latest --tag reviewer:latest
af build --target image --bundle reviewer.tar.gz --base-image example/agent-base:latest --tag reviewer:latest
```

A bundle build loads the source agentfile, applies defaults and discovery, resolves every source once, compiles the bundle manifest and harness configuration template, stages materialized assets, and writes a reproducible `.tar.gz`. The bundle manifest contains normalized run-time values and direct bundle-relative asset paths rather than agentfile source declarations. See the normative [Agent bundle format](bundle.md). `--output` is valid only for bundle builds. Its default is `<metadata.name>-<metadata.version>.tar.gz`, with path-separator characters replaced by `-`.

The current bundle reader accepts archives up to 1 GiB and 100,000 entries. Each entry and the total extracted content are limited to 512 MiB.

An image build from an agentfile creates a temporary bundle, then constructs the image from that bundle. An image build from `--bundle` does not read a source project, rediscover assets, or refetch sources. `--base-image` and `--tag` are valid only for image builds. `--base-image` defaults from the selected harness, and `--tag` defaults to `<agent.name>:<agent.version>` from the bundle manifest.

`--file` and `--bundle` are mutually exclusive. `--file` defaults to `agentfile.yaml` when the input is a project. Builds do not require LLM credentials, do not run the harness, and do not modify the project.

Images contain the unpacked bundle at `/agent/bundle`, the generated image entrypoint, and `/agent/workspace`. The entrypoint reads the bundle contents in place and writes its private harness profile under `/agent/profile`.

They record agent identity, harness, run-time environment-variable names, and bundle digest labels. See the [Agent image format](image.md).

### Run

Run starts an agent from a source agentfile, agent bundle, or agent image.

`af run` is an alias for `af agents run`.

Source agentfiles use Docker by default. `--host` selects unsandboxed host execution through `runa`.

The run command supports three execution modes:

| Mode | Selection | Task source | Lifecycle |
| --- | --- | --- | --- |
| One-shot | default | `spec.prompt` or `--prompt` | performs one task and exits |
| TUI | `--tui` | user interacts with the agent in terminal UI | lasts for the terminal session |
| ACP | `--acp` | messages from an ACP client | controlled by the client |

The three modes are mutually exclusive, therefore the flags `--tui`, `--acp`, and `--prompt` are mutually exclusive.

```bash
af agents run [NAME | --file agentfile.yaml | --bundle FILE | --image REF] [--host] [--tui | --acp | --prompt TEXT] [--model MODEL] [--workspace DIR] [--ws DIR] [--env KEY[=VALUE]] [--env-file FILE] [--env-auto] [--debug]
```

Agent selection:

1. `--image REF` runs an image with Docker.
2. `--bundle FILE` runs a bundle with `runa`.
3. `--file FILE` builds a temporary image, or a temporary bundle when `--host` is present.
4. `NAME` runs a registered agentfile or image; `--host` is invalid for registered images.
5. With no selection, `agentfile.yaml` in the current directory is used.

`NAME`, `--file`, `--bundle`, and `--image` are mutually exclusive. Bundle registration is not supported.

For source agentfiles, the CLI normally builds a temporary image.

It selects or builds the image, bind-mounts the workspace when requested, and invokes Docker. Images are pulled if absent.

When `--host` selects a source agentfile, the CLI creates a temporary bundle and passes it to `runa`.

`runa` creates a private unpacked bundle, uses an empty temporary workspace unless one is selected, creates a private [harness profile](harness.md#terminology-and-scope), and locates the harness on `PATH`.

It starts the harness as the current user, forwards streams and signals, preserves its exit code, and removes temporary files.

`runa` supports one-shot and TUI modes; ACP is rejected. It never imports or modifies global harness configuration. See the [`runa` reference](runa.md).

Every `runa` invocation warns that it runs without isolation or approval gates. Host mode is for trusted bundles and environments only.

`--workspace PATH` uses an existing workspace.

Docker binds it to `/agent/workspace`; `runa` uses it directly. Relative paths are resolved from the current working directory.
`--ws PATH` is an alias for `--workspace PATH`.

`--file` defaults to `agentfile.yaml` in the current directory. Relative paths are resolved from the current directory; absolute paths are used as-is.

`--env KEY[=VALUE]` sets an environment variable for the invocation. If `VALUE` is omitted, the value is taken from the current environment.
`--env-file FILE` loads environment variables from an `.env` file.
`--env-auto` forwards declared runtime variables to a container when present in the host.

`runa` inherits the parent environment already. Explicit `--env` values take precedence.

`--debug` streams build progress and agent stderr to stderr (which aren't streamed by default). If a one-shot run exits with non-zero code, its captured stderr is printed automatically. TUI mode always attaches stderr and shows build progress. ACP mode always reserves stdout for protocol messages and sends diagnostics to stderr. Image pull progress is always printed to stderr.


In one-shot mode, piped stdin is forwarded to the agent as input in addition to its effective prompt:

```bash
tail -200 app.log | af run log-triage
```

#### TUI Mode

`--tui` opens the selected harness's native interactive terminal.

```bash
af run code-review --tui --workspace .
```

TUI mode starts without an initial user message: `spec.prompt` is ignored, and `--prompt` cannot be combined with `--tui`.

For image-based selection, TUI mode requires the `build.agentfile.harness` label added by current Agentfile builds. Host TUI requires the harness executable on `PATH`.

#### ACP Mode

`--acp` flag allows integrating the agent with an [Agent Client Protocol](https://agentclientprotocol.com)-compatible client. This allows you to use your agents with your IDE, Terminal or agent management UI.  
Configuration varies based on client - where client asks for a command to run, supply the `af run` command that runs your agents, and add the `--acp` flag.

ACP is supported only through Docker.

The ACP client supplies a workspace for each session. The request's absolute `cwd` is mounted at `/agent/workspace`. `--workspace` and `--ws` are not supported with `--acp`.

The ACP client supplies the user input. `spec.prompt` is ignored in ACP mode, and `--prompt` is rejected.

The ACP bridge accepts text and resource-link prompts and supports streamed messages, thoughts, tool calls, cancellation, and close. It does not advertise other ACP features.

File resource links inside the session workspace are translated to their `/agent/workspace` paths.

Client-provided MCP servers are rejected since MCP server definition and configuration belong in the agentfile.

#### Field Overrides

Run supports overriding certain agentfile fields:

- `--prompt` replaces the bundle or image default prompt in one-shot mode. It can also supply a missing prompt.
- `--model` replaces the default model. The provider remains the one declared in the agentfile.

These values apply only to the harness invocation.

They do not modify the agentfile, bundle, or image. Other agentfile fields cannot be overridden.

When running an agent image directly with a container runtime, `AGENTFILE_PROMPT` and `AGENTFILE_MODEL` are the equivalent entrypoint-level overrides.

```bash
af run hello-world --prompt "say hi"
af run hello-world --model claude-sonnet-4-5
```

### Agents

The agent registry allows easy discovery and execution of agents. It maps user-local agent names to agentfile projects or agent images.

The agent registry is stored in the [agentfile configuration directory](#configuration) under `/registry.json`.

The registry JSON uses a wrapped object shape:

```json
{
  "agents": {
    "hello": {
      "name": "hello",
      "agentfilePath": "/path/to/project/agentfile.yaml"
    },
    "hello-world": {
      "name": "hello-world",
      "image": "itaysk/agentfile-hello-world:0.1"
    }
  }
}
```

A registry entry stores:

1. name
2. exactly one of `agentfilePath` or `image`

For agentfile entries, image tags are derived from the current registered agentfile metadata when needed. For image entries, the stored image reference is used directly.

#### Register

Register an agent for later use by name.

```bash
af agents register [NAME] [--file agentfile.yaml | --image myimage:tag]
```

If `NAME` is omitted, `metadata.name` is used.

Registering the same name again replaces the previous registration.  

`--file` defaults to `agentfile.yaml` in the current directory. Relative paths are resolved from the current directory; absolute paths are used as-is.  
`--image REF` requires an image built by `af build`.  

Image registration validates the `build.agentfile.*` labels.  
The image must be present locally, pull it first if you need to.

#### List

List registered agents.

```bash
af agents list
```

#### Remove

Remove a registered agent.

```bash
af agents remove [NAME]
```

### Configuration

Agentfile CLI stores state and configuration under the OS user configuration directory. See [here](https://pkg.go.dev/os#UserConfigDir) for details.

## Direct Docker Use

Agent images are normal container images. They can run directly with Docker without the `af` runner.

```bash
docker run --rm -e ANTHROPIC_API_KEY hello-world:latest
```

Use a bind mount for workspace input and output:

```bash
docker run --rm -e ANTHROPIC_API_KEY -v "$PWD:/agent/workspace" hello-world:latest
```

Run interactive agent:

```bash
docker run --rm -it -e AGENTFILE_RUN_MODE=tui -e ANTHROPIC_API_KEY -v "$PWD:/agent/workspace" hello-world:latest
```

You cannot run an agent in ACP mode directly with docker run, since the agent image isn't aware of ACP protocol. The protocol translation is handled by the `af run --acp` command.

## Security

Docker uses the container as its isolation boundary.

Harness permission and approval gates are disabled so the agent can read, write, and execute freely inside that boundary without asking. Additional isolation can be added at deploy time using container runtime security features.

`runa` has no isolation boundary.

It launches the harness as the current user with access to that user's files, credentials, processes, installed tools, and network resources, while retaining Agentfile's permission-bypass behavior.

Host mode must be selected explicitly for source agentfiles and prints a warning on every invocation. Use it only for trusted bundles, prompts, skills, MCP registrations, and workspaces.

Bundles are artifacts, not sandboxes. Safe extraction prevents archive path attacks, but bundle instructions and skill scripts may still cause arbitrary actions when executed.

Secrets should use `runtimeEnv` and be provided at invocation time. They must not be stored in bundles or image layers.

`runa` inherits the complete parent environment, including undeclared secrets. See [Runtime Variables](#runtime-variables), the [bundle format](bundle.md#sensitive-information), and the [`runa` security rules](runa.md#security).
