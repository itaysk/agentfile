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
- Project: directory where the agentfile lives. Used as the build context.
- Agent: container image produced from an agentfile.
- Harness: the agent runtime inside the image. Supported harnesses are `claudecode`, `codex`, and `pi`.
- LLM: the model provider and model used by the harness.
- Assets: prompt, system prompt, skill, and other markdown content that make up the agent.
- Sources: strategies for loading content into the build.
- Workspace: `/agent/workspace` inside the agent container.

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
`metadata.version` is optional. If present, it must be a non-empty string. Its
default is `latest`.

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

Choose a harness to set the runtime that executes the agent. The selected harness controls how the agentfile spec is translated into runtime-specific instructions, commands, and configuration.

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

- `claudecode`: `itaysk/claudecode:latest`
- `codex`: `itaysk/codex:latest`
- `pi`: `itaysk/pi:latest`

The selected base image must contain the selected harness executable. Agentfile adds the generated entrypoint during build.
The easiest way to create a custom image is to derive from an existing one.  
Images are built from Dockerfiles in [/images](/images).

Agentfile does not install tools declared elsewhere. Add tools to the base image.

### LLM

Use `spec.llm` to configure the model provider and model used by the harness.  
Exactly one provider key must be set.  
Supported providers are `anthropic`, `openai`, and `openrouter`.  
Each provider requires `model`.
`model` must be a non-empty string.

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

MCP `envs` entries use the same shape and name rules as `spec.envs`.

For `http`, `url` is required.  
`headers` is optional.

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
`name` must match `[A-Za-z_][A-Za-z0-9_]*`.  
Runtime environment variables take precedence over `spec.envs`.

### Workspace

The workspace is the agent's working directory for input, output, and temporary files.  
Inside the container, the path is always:

```text
/agent/workspace
```

The agent process runs with `/agent/workspace` as its working directory.  
The default workspace is empty and ephemeral.

`af run --workspace PATH` requests a host bind mount when the agent is run. `PATH` must be an existing directory. Relative paths are resolved from the current working directory. Use `--ws` as a shorter alias.

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

HTTP redirects are followed.  
Non-2xx HTTP responses are invalid.

## Discovery

Discovery populates agentfile assets based on project files automatically at build-time.
It is applied after reading the agentfile and before the effective agentfile is used.

Singular assets are discovered only when their `spec` field is absent. List assets append discovered entries after explicit entries.  
Each discovered asset is represented as an `fs` source in the effective agentfile YAML.

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

Build turns the effective agentfile into a runnable container image. Use `af build` to build an agent image.

```bash
af build [--file agentfile.yaml] [--project DIR] [--tag TAG]
```

Build steps:

1. Load the effective agentfile.
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

`--file` defaults to `agentfile.yaml`.
`--project` defaults to the current directory.

The default image tag is:

```text
metadata.name:metadata.version
```

### Run

Run starts an agent container and streams its output. `af run` is an alias for `af agents run`.

```bash
af agents run [NAME] [--file agentfile.yaml] [--project DIR] [--workspace DIR] [--ws DIR] [--env KEY[=VALUE]] [--env-file FILE] [field overrides]
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
`--workspace PATH` binds `PATH` to `/agent/workspace`. `PATH` must be an existing directory. Relative paths are resolved from the current working directory.  
`--ws PATH` is an alias for `--workspace PATH`.

`--file` defaults to `agentfile.yaml`.
`--project` defaults to the current directory.

`--env KEY[=VALUE]` sets an environment variable in the container. if `VALUE` is omitted, the value is taken from the current environment.
`--env-file FILE` loads environment variables from an `.env` file.

The following environment variables are passed through from the host environment to the container automatically:
- Current LLM provider default credentials. As described in [llm section](#llm)

When the run command receives piped input on stdin, that input is forwarded to the agent, so you can stream data to it:

```bash
tail -200 app.log | af run log-triage
```

#### Field Overrides

Field overrides change scalar `spec` fields for one run.  
Field overrides can override complete asset sources, in which case the `text` source is used in the effective agentfile (e.g `--prompt="example"` becomes `prompt: { text: "example" }`).  
Field overrides are applied after effective file configuration is loaded and before the run starts. They replace matching effective file values.  
After overrides are applied, the effective agentfile is validated again.  
Field overrides can set fields that weren't present in the agentfile, as long as the field path is valid and the resulting agentfile is valid.  
Overrides cannot set fields inside list items, append list items, or replace a list as a whole.  
Fields are referenced by their `spec` field path, with the `spec` prefix omitted. Use `--field.path value` or `--field.path=value`.  
Field overrides are only supported by `af run`. When run directly with Docker, the image uses the spec built into the image.

```bash
af run hello-world --llm.anthropic.model claude-sonnet-4-5
af run hello-world --prompt "say hi"
af run hello-world --prompt.text "say hi"
```

### Agents

The agent registry allows easy discovery and execution of agents. It maps user-local agent names to Agentfile projects.

The agent registry is stored in the [agentfile configuration directory](#configuration) under `/registry.json`.

The registry JSON uses a wrapped object shape:

```json
{
  "agents": {
    "hello": {
      "name": "hello",
      "projectDir": "/path/to/project",
      "agentfilePath": "/path/to/project/agentfile.yaml",
      "defaultImageTag": "hello:latest"
    }
  }
}
```

A registry entry stores:

1. name
2. project directory
3. agentfile path
4. default image tag

#### Register

Register an agent for later use by name.

```bash
af agents register [NAME] [--file agentfile.yaml] [--project DIR]
```

If `NAME` is omitted, `metadata.name` is used.

Registering the same name again replaces the previous registration.  

`--file` defaults to `agentfile.yaml`.
`--project` defaults to the current directory.

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

When run directly with Docker, the image uses the spec built into the image.

## Security

Agentfile agents are unattended processes and cannot interactively ask for approvals. They also assumed to run in conatiners which provide a natural isolation boundary. Therefore the harness runs with permission and approval gates disabled by default, the agent can read, write, and execute freely inside its container without asking. Additional isolation can be added at deploy-time using container runtime security features.
