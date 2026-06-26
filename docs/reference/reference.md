# Agentfile Manual

This file is the product manual and implementation spec.  
Agentfile builds runnable agent container images from one YAML file and project conventions.  
An implementation must do the behavior described here. Nothing else is part of Agentfile.

## Table of Contents

- [Terms and Concepts](#terms-and-concepts)
- [Agentfile](#agentfile)
- [Agent Specification](#agent-specification)
  - [Harness](#harness)
  - [LLM](#llm)
  - [Prompt](#prompt)
  - [System Prompt](#system-prompt)
  - [Skills](#skills)
  - [MCP Servers](#mcp-servers)
  - [Environment](#environment)
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
  - [Register](#register)
  - [List](#list)
  - [Field Overrides](#field-overrides)
- [Direct Docker Use](#direct-docker-use)
- [Security](#security)

## Terms and Concepts

These terms are used by the rest of the manual.

- Agentfile: YAML file that declares and describes an agent. Commonly named `Agentfile.yaml`.
- Project: directory where the Agentfile lives. Used as the build context.
- Agent: container image produced from an Agentfile.
- Harness: the agent runtime inside the image. Supported harnesses are `claudecode`, `codex`, and `pi`.
- LLM: the model provider and model used by the harness.
- Assets: prompt, system prompt, skill, and other markdown content that make up the agent.
- Sources: strategies for loading content into the build.
- Workspace: `/agent/workspace` inside the agent container.

## Agentfile

An Agentfile declares an agent. It is a YAML document modeled like a Kubernetes resource.

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
  workspace: {}
```

`apiVersion`, `kind`, `metadata.name`, `spec.harness`, and `spec.llm` are required.  
`apiVersion` must be `agentfile.build/v1`.  
`kind` must be `Agent`.  
`metadata.version` is optional. Its default is `latest`.

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
      model: haiku-4.5
  prompt:
    text: |
      say hi!
```

## Agent Specification

### Harness

Choose a harness to set the runtime that executes the agent. The selected harness controls how the Agentfile spec is translated into runtime-specific instructions, commands, and configuration.

The [Harness reference](./harness.md) is the normative companion for harness-specific behavior. It defines the runtime files, environment variables, commands, and unsupported combinations an implementation must use for each harness.

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

The optional `image` field sets the base image for the resulting agent image:

```yaml
spec:
  harness:
    image: my-agent-base:latest
    claudecode: {}
```

If `image` is omitted, the default base image is selected automatically:

- `claudecode`: `itaysk/claudecode`
- `codex`: `itaysk/codex`
- `pi`: `itaysk/pi`

The selected base image must contain the selected harness and Agentfile entrypoint.  
The easiest way to create a custom image is to derive from an existing one.  
Images are built from Dockerfiles in [/images](/images).

Agentfile does not install tools declared elsewhere. Add tools to the base image.

### LLM

Use `spec.llm` to configure the model provider and model used by the harness.  
Exactly one provider key must be set.  
Supported providers are `anthropic`, `openai`, and `openrouter`. Each provider requires `model`.

```yaml
spec:
  llm:
    anthropic:
      model: haiku-4.5
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
      model: anthropic/claude-haiku-4.5
```

Model names are strings. Agentfile does not validate model catalogs.

Credentials are runtime input. They are not stored in the image.  
Default credential environment variables are:

| Provider | Environment variable |
| --- | --- |
| `anthropic` | `ANTHROPIC_API_KEY` |
| `openai` | `OPENAI_API_KEY` |
| `openrouter` | `OPENROUTER_API_KEY` |

### Prompt

Use `spec.prompt` to give the agent its one-shot task.  
Prompt content is supplied with a [source object](#sources).

```yaml
spec:
  prompt:
    text: |
      summarize the files in the workspace
```

### System Prompt

Use `spec.systemPrompt` for standing instructions that apply before the user prompt.  
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
`spec.skills` is a list of [source objects](#sources). Each source must resolve to one skill directory. A skill directory must contain `SKILL.md`.

```yaml
spec:
  skills:
    - fs:
        path: bundles/world-greetings
```

Duplicate skill names are invalid.  
The skill name is read from `SKILL.md` front matter when present. Otherwise it is the skill directory name.

### MCP Servers

Register MCP servers to make external tools available to the harness.  
`spec.mcps` is a list of server registrations.  
Each MCP server requires `name`.  
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
            value: Bearer token
```

For `stdio`, `command` is required and must be a non-empty string array. `envs` is optional:

```yaml
stdio:
  command: ["tool"]
  envs:
    - name: EXAMPLE
      value: value
```

For `http`, `url` is required. `headers` is optional.

MCP commands run inside the agent container. Agentfile only registers MCP servers, it does not install MCP server binaries.

### Environment

Use `spec.envs` to set environment variables in the agent.

```yaml
spec:
  envs:
    - name: LOG_LEVEL
      value: info
```

Each environment entry requires `name` and `value`.  
Runtime environment variables take precedence over `spec.envs`.

### Workspace

The workspace is the agent's working directory for input, output, and temporary files.  
Inside the container, the path is always:

```text
/agent/workspace
```

The agent process runs with `/agent/workspace` as its working directory.  
The default workspace is empty and ephemeral.

`spec.workspace.hostBindPath` requests a host bind mount when the agent is run:

```yaml
spec:
  workspace:
    hostBindPath: /tmp/work
```

`hostBindPath` must be an absolute host path.

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

`path` is relative to the Agentfile directory.  
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
Supported archive formats are `zip`, `tar`, `tar.gz`, and `tgz`.

HTTP redirects are followed.  
Non-2xx HTTP responses are invalid.

## Discovery

Discovery populates Agentfile assets based on project files automatically at build-time.  
It is applied after reading the Agentfile and before the effective Agentfile is used.

If `spec.prompt` is absent and `prompt.md` exists, it becomes:

```yaml
prompt:
  fs:
    path: prompt.md
```

If `spec.systemPrompt` is absent and `system-prompt.md` exists, it becomes:

```yaml
systemPrompt:
  fs:
    path: system-prompt.md
```

If `skills/` exists, each immediate child directory containing `SKILL.md` is added to `spec.skills`.

```yaml
skills:
  - fs:
      path: skills/myskill
```

Explicit skills come before discovered skills.  
No recursive skill discovery is performed below `skills/*`.

## CLI

Use the CLI to build, register, list, and run agents. Use `af --help` to show help and usage.

### Build

Build turns the effective Agentfile into a runnable container image. Use `af build` to build an agent image.

```bash
af build [--file Agentfile.yaml] [--project DIR] [--tag TAG]
```

Build steps:

1. Load the effective Agentfile.
2. Resolve all sources.
3. Select the base image.
4. Copy assets into the image.
5. Write harness configuration according to the [Harness reference](./harness.md).
6. Set the image entrypoint.
7. Tag the image.

The image entrypoint runs the selected harness in one-shot mode.  
The image working directory is `/agent/workspace`.

Build does not require LLM credentials.  
Build does not run the agent.  
Build must not modify the project directory.

`--file` defaults to `Agentfile.yaml`.  
`--project` defaults to the current directory.

The default image tag is:

```text
metadata.name:metadata.version
```

### Run

Run starts an agent container and streams its output. `af run` is an alias for `af agents run`.

```bash
af agents run [NAME] [--file Agentfile.yaml] [--project DIR] [--in DIR] [--here]
```

```bash
af run [NAME] [--file Agentfile.yaml] [--project DIR] [--in DIR] [--here]
```

Agent selection:

1. If `--file` or `--project` is set, load that project.
2. Otherwise, if `NAME` is set, load the registered agent named `NAME`.
3. Otherwise, load the current project.

Run steps:

1. Load or build the image.
2. Bind the workspace if requested.
3. Pass runtime environment variables.
4. Start the container.
5. Stream stdout and stderr.
6. Exit with the container exit code.

The run command requires an effective prompt.  
`--in PATH` sets `spec.workspace.hostBindPath`.  
`--here` sets `spec.workspace.hostBindPath` to the current directory.  
`--in` and `--here` cannot be used together.

`--file` defaults to `Agentfile.yaml`.  
`--project` defaults to the current directory.

### Register

Register an agent project for later use by name.

```bash
af agents register [NAME] [--file Agentfile.yaml] [--project DIR]
```

If `NAME` is omitted, `metadata.name` is used.

The registry maps user-local agent names to Agentfile projects.

A registry entry stores:

```text
name
project directory
Agentfile path
default image tag
```

Registering the same name again replaces the previous registration.  
The registry is not copied into images.

`--file` defaults to `Agentfile.yaml`.  
`--project` defaults to the current directory.

### List

List registered agents.

```bash
af agents list
```

### Field Overrides

Field overrides change scalar spec fields for one run.  
Runtime overrides are CLI flags named after `spec` field paths.  
Do not include the leading `spec`.

Field overrides are applied after effective file configuration is loaded and before the run starts. They replace matching effective file values.

The full run form with field overrides is:

```bash
af agents run [NAME] [--file Agentfile.yaml] [--project DIR] [--in DIR] [--here] [field overrides]
```

Examples:

```bash
af run hello-world --llm.anthropic.model sonnet-4.5
af run hello-world --prompt.text "say hi"
af run hello-world --workspace.hostBindPath /tmp/work
```

`--prompt TEXT` is an alias for `--prompt.text TEXT`.  
`--in PATH` is an alias for `--workspace.hostBindPath PATH`.  
Setting `--prompt.text` replaces the whole prompt source with a text source.

Only scalar string fields can be overridden.  
Overrides cannot append list items.  
Field overrides are only supported by `af run`.  
When run directly with Docker, the image uses the spec built into the image.

## Direct Docker Use

Agent images are normal container images. They can run directly with Docker without the `af` runner.

```bash
docker run --rm -e ANTHROPIC_API_KEY hello-world:latest
```

Use a bind mount for workspace input and output:

```bash
docker run --rm -e ANTHROPIC_API_KEY -v "$PWD:/agent/workspace" hello-world:latest
```

When run directly with Docker, the image uses the spec built into the image.