# agentfile Specification

This document defines the normative build-time input format for agentfile API version `agentfile.build/v1`.

An agentfile is a YAML project-authoring format. Agentfile resolves its sources and compiles it into an [agent bundle](bundle.md); the agentfile is not the bundle manifest and is not read when that bundle runs or when an image is built from it.

Field names are case-sensitive. Unknown fields are invalid.

## Document shape

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

The machine-readable [JSON Schema](agentfile.schema.json) and [full YAML example](agentfile.yaml) accompany this specification.

## Agent specification

### Harness

`spec.harness` selects the harness executable that runs the built agent. Exactly one selector key must be set.

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

The `claudecode` harness accepts an optional `bare` field:

```yaml
spec:
  harness:
    claudecode:
      bare: true
```

`bare` opts into Claude Code's bare mode. `bare: true` cannot be combined with `spec.skills` or with Claude subscription authentication through `CLAUDE_CODE_OAUTH_TOKEN`.

The [harness adapter reference](harness.md) defines supported harness/provider combinations and the exact build-time and invocation-time mappings.

### LLM

`spec.llm` selects the model provider and model. Exactly one provider key must be set.

Supported providers are `anthropic`, `openai`, and `openrouter`. Each provider requires a non-empty `model`. Agentfile treats model names as strings and does not validate provider model catalogs.

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

Credentials are invocation input, not part of `spec.llm`. See the [manual's authentication section](../manual.md#authentication) and the [harness credential mappings](harness.md#provider-and-credentials).

### Prompt

`spec.prompt` declares the default task for one-shot execution. It is optional and uses a [source object](#sources).

```yaml
spec:
  prompt:
    text: |
      summarize the files in the workspace
```

A one-shot invocation must obtain a prompt from `spec.prompt` or an invocation override. TUI and ACP invocations ignore it.

### System prompt

`spec.systemPrompt` declares standing instructions that define the agent's character and behavior. It is optional and uses a [source object](#sources). If omitted, the harness default applies.

```yaml
spec:
  systemPrompt:
    fs:
      path: prompts/system.md
```

### Skills

`spec.skills` is an optional list of [source objects](#sources). Each source must resolve to one skill directory.

A skill directory must contain `SKILL.md`, and its skill name is the `name` field in that file's YAML front matter.

Skill names must be unique within `spec.skills`. Each name must be a single path segment: it cannot contain `/` or `\` and cannot be exactly `.` or `..`.

```yaml
spec:
  skills:
    - fs:
        path: bundles/world-greetings
```

### MCP servers

`spec.mcps` is an optional list of MCP server registrations. Each registration requires a non-empty, unique `name` and exactly one transport: `stdio` or `http`.

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

MCP `envs` entries use the same shape and name rules as [`spec.envs`](#environment).

For `http`, `url` is required. `headers` is optional. Header entries use the same value rules as `spec.envs` entries. A header name may be any valid HTTP header name that does not start with `AGENTFILE_`.

MCP commands run in the same environment as the harness. Agentfile registers MCP servers but does not install their executables in a bundle or on the host.

Claude Code performs its own `${VAR}` expansion on some `mcp.json` fields after Agentfile renders them, so a literal or runtime value containing `${...}` may be expanded again by Claude Code.

### Environment

`spec.envs` is an optional list of environment variables for the agent.

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

- `value` is a public literal default stored in the bundle and materialized as an image environment default.
- `runtimeEnv` reads a value from the invocation environment.

`name` must match `[A-Za-z_][A-Za-z0-9_]*` and must not start with the reserved prefix `AGENTFILE_`.

A literal `value` is applied only when the variable is not already set.

### Runtime variables

A `runtimeEnv` entry declares that a value is unknown at build time and must be read from the invocation environment. Runtime values do not appear in bundles or image layers, so `runtimeEnv` is the appropriate value source for secrets.

`runtimeEnv.name` must match `[A-Za-z_][A-Za-z0-9_]*` and must not start with the reserved prefix `AGENTFILE_`.

Runtime variables are supported in `spec.envs[]`, `spec.mcps[].stdio.envs[]`, and `spec.mcps[].http.headers[]`.

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

A referenced runtime variable is required at invocation time. An empty value is valid; only an unset variable is missing.

## Sources

A source object declares how Agentfile loads content during a bundle build. Exactly one source type must be set.

### Text source

`text` embeds literal content.

```yaml
text: |
  say hi
```

### Filesystem source

`fs` reads from the build machine's filesystem. Exactly one path field must be set.

```yaml
fs:
  path: assets/content.md
```

```yaml
fs:
  absolutePath: /opt/agentfile/content.md
```

`path` is relative to the directory containing the agentfile. `absolutePath` is an absolute path on the build machine.

### Git source

`git` reads from a Git repository.

```yaml
git:
  url: https://github.com/example/repo.git//path/in/repo
  ref: main
```

`url` is required and must start with a repository location using an HTTP or SSH scheme. Append `//path/in/repo` to select a file or directory inside the repository; the separator is the last `//` in the URL.

Exactly one of `ref` or `commit` may be set. If neither is set, Agentfile uses the remote default branch at build time.

Sources without `commit` use shallow clones. A `commit` source first tries a shallow clone and shallow fetch of the requested commit, then falls back to a full clone if the remote does not support fetching by commit.

### HTTP source

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

`url` is required. `archive` is optional and defaults to `false`.

When `archive` is `false`, the response body is used as one file. When it is `true`, the response body is extracted.

Supported archive formats are `zip`, `tar`, `tar.gz`, and `tgz`. The format is detected from the URL suffix first, then by common magic bytes such as zip and gzip when the URL has no useful extension.

Archive extraction writes only directories and regular files. Symlinks and other special entries are skipped, and archive mode bits are reduced to regular permission bits.

HTTP redirects are followed. A fetch must complete within 60 seconds, its response must be at most 100 MiB, and its HTTP status must be in the 2xx range.

## Discovery

Discovery populates agentfile assets from conventional project paths during a bundle build. It runs after the agentfile is read and before validation and source resolution.

Singular assets are discovered only when their `spec` field is absent. List assets append discovered entries after explicit entries. Each discovered asset becomes an `fs` source.

`prompt.md` is discovered as `spec.prompt`:

```yaml
spec:
  prompt:
    fs:
      path: prompt.md
```

`system-prompt.md` is discovered as `spec.systemPrompt`.

Each `skills/<name>` directory is discovered as a `spec.skills` entry. Discovered skills are sorted by path, and discovery does not recurse below `skills/*`.
