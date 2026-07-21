# Harness Adapter Reference

This document defines the normative mapping between a bundle manifest and each supported harness.

The [agentfile specification](agentfile.md) defines the source schema, defaults, and discovery rules.

The [bundle format](bundle.md) defines bundle assets and relocatable templates.

The [image format](image.md) defines agent image construction.

## Terminology and scope

A **harness adapter** maps a bundle manifest and harness invocation to harness-specific bundle assets, profiles, and commands.

At build time, the adapter validates supported feature combinations and produces harness configuration templates.

At invocation time, the adapter receives a caller-selected private profile directory, renders harness configuration into it, copies skills into harness-specific profile directories where required, and prepares a harness command.

A **harness invocation** supplies an execution mode, workspace, environment, and any explicit prompt or model override for one harness run.

An **execution mode** defines how the harness interacts with its caller. The supported modes are one-shot, TUI, and ACP.

An **unpacked bundle** is the directory containing the bundle contents used for an invocation.

A **harness profile** is a caller-provided, private, per-invocation directory containing rendered harness configuration and copies of bundle assets installed for the selected harness. It is separate from the user's global harness configuration.

A **harness command** contains the harness executable, arguments, environment, and working directory for one invocation. It is data, not a shell command string or a running process.

A harness adapter does not define CLI behavior, construct an agent bundle or image, or launch a process.

The examples below use these symbolic values:

| Symbol | Meaning |
| --- | --- |
| `<bundle>` | unpacked bundle directory |
| `<profile>` | harness profile |
| `<workspace>` | workspace from the harness invocation |
| `<provider>` | selected model provider |
| `<model>` | selected model |
| `<prompt>` | selected one-shot prompt |
| `<system-prompt>` | selected system-prompt contents |
| `<system-prompt-asset>` | bundle-relative system-prompt asset path |
| `<skill-name>` | skill directory name |

Symbols enclosed in angle brackets are descriptive and are not literal paths or argument values.

## Build-time mappings

### Supported combinations

The bundle manifest's `harness` and `model.provider` must form a supported combination:

| Provider | Claude Code | Codex | Pi |
| --- | --- | --- | --- |
| `anthropic` | Supported | Unsupported | Supported |
| `openai` | Unsupported | Supported | Supported |
| `openrouter` | Unsupported | Unsupported | Supported |

Bundle construction and bundle readers reject unsupported combinations.

The following bundle-manifest combinations are also unsupported:

- `harness: pi` with `assets.configTemplate`;
- `harness: claudecode` and `bare: true` with a non-empty `assets.skills`; and
- `harness: claudecode` and `bare: true` with a `CLAUDE_CODE_OAUTH_TOKEN` target in `environment.defaults` or `environment.mappings`.

### Harness configuration templates

Bundle construction writes these harness-specific bundle assets:

| Harness | Template | Bundle-manifest field |
| --- | --- | --- |
| Claude Code | `harness/claudecode/mcp.json.tmpl` | `assets.configTemplate`, when the source agentfile declares MCP servers |
| Codex | `harness/codex/config.toml.tmpl` | `assets.configTemplate`, always |
| Pi | None | omitted |

Literal configuration values are written directly into a template.

A `runtimeEnv` reference is written as `__AGENTFILE_REF_<source-name>__`, where `<source-name>` is its declared source environment-variable name. The sorted set of these names is recorded in `assets.configEnv`.

Bundle and workspace paths are written as `__AGENTFILE_BUNDLE_ROOT__` and `__AGENTFILE_WORKSPACE__`.

Harness profile preparation replaces these reserved placeholders only in a private rendered copy.

#### Claude Code template

The Claude Code template has this JSON shape:

```json
{
  "mcpServers": {
    "time": {
      "type": "stdio",
      "command": "uv",
      "args": ["tool", "run", "mcp-server-time"],
      "env": {
        "TOKEN": "__AGENTFILE_REF_MCP_TOKEN__"
      }
    },
    "search": {
      "type": "http",
      "url": "https://example.com/mcp",
      "headers": {
        "Authorization": "__AGENTFILE_REF_SEARCH_MCP_AUTH__"
      }
    }
  }
}
```

For a `stdio.command` array, the first item becomes `command` and the remaining items become `args`.

#### Codex template

The Codex template uses `config.toml`:

```toml
project_doc_max_bytes = 0
model_instructions_file = "__AGENTFILE_BUNDLE_ROOT__/system-prompt.md"

[projects."__AGENTFILE_WORKSPACE__"]
trust_level = "trusted"

[mcp_servers.time]
command = "uv"
args = ["tool", "run", "mcp-server-time"]

[mcp_servers.time.env]
TOKEN = "__AGENTFILE_REF_MCP_TOKEN__"

[mcp_servers.search]
url = "https://example.com/mcp"
http_headers = { Authorization = "__AGENTFILE_REF_SEARCH_MCP_AUTH__" }
```

`model_instructions_file` is omitted when `assets.systemPrompt` is absent.

MCP tables are omitted when the source agentfile does not declare MCP servers.

For a `stdio.command` array, the first item becomes `command` and the remaining items become `args`.

## Invocation-time mappings

The adapter renders configuration and installs skills into the private harness profile, then returns a harness command.

The adapter does not start the harness process.

### Harness command preparation

Harness command preparation receives an unpacked bundle, a private harness profile directory, and a harness invocation.

During preparation, the adapter:

1. validates that every required environment variable is present;
2. uses the prompt and model in the bundle manifest unless the invocation explicitly overrides them;
3. prepares the supplied harness profile;
4. replaces bundle and workspace path placeholders in harness configuration templates;
5. substitutes referenced environment values into the rendered harness configuration;
6. applies declared environment defaults and mappings; and
7. produces the harness command.

The invocation supplies the initial environment.

A value in `environment.defaults` is set only when its target variable is absent.

A value in `environment.mappings` names a source variable. Its value is copied into the map key's target variable only when the target is absent.

Every source named by `environment.mappings` or `assets.configEnv` is required. A mapping source remains required even when its target variable already exists. An empty source value is valid.

One-shot mode requires an effective prompt.

TUI and ACP modes remove the prompt from the harness environment and start without an initial user message.

An environment value substituted into a JSON or TOML harness template must not contain a carriage return or newline.

### Harness profile

| Harness | Environment | Rendered configuration | Skills |
| --- | --- | --- | --- |
| Claude Code | `HOME=<profile>/claudecode/home` | `<profile>/claudecode/mcp.json` when `assets.configTemplate` is present | `<profile>/claudecode/home/.claude/skills/<skill-name>/` |
| Codex | `HOME=<profile>/codex/home`; `CODEX_HOME=<profile>/codex/home/.codex` | `<profile>/codex/home/.codex/config.toml` | `<profile>/codex/home/.agents/skills/<skill-name>/` |
| Pi | `PI_CODING_AGENT_DIR=<profile>/pi/home` | None | Referenced from `<bundle>/skills/<skill-name>` |

Claude Code and Codex skill directories are copied unchanged from their bundle assets.

Pi receives one `--skill` argument for each skill instead of copying skills into its profile.

### Provider and credentials

| Harness | Mapping |
| --- | --- |
| Claude Code | Pass `<model>` with `--model`. Anthropic credentials remain in `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN`. Subscription authentication is incompatible with bare mode. |
| Codex | Pass `<model>` with `--model`. If `CODEX_ACCESS_TOKEN` is set, remove `CODEX_API_KEY` from the harness environment. Otherwise, when `CODEX_API_KEY` is unset and `OPENAI_API_KEY` is set, copy its value to `CODEX_API_KEY`. |
| Pi | Pass `<provider>` with `--provider` and `<model>` with `--model`. Provider credentials remain in `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, or `OPENROUTER_API_KEY`. |

For image TUI and ACP invocation, the Codex adapter initializes the private profile with `codex login --with-access-token` or `codex login --with-api-key` when the corresponding credential is available.

Credential values must not be persisted outside the private harness profile.

### Permission flags

| Harness | Mapping |
| --- | --- |
| Claude Code | Add `--dangerously-skip-permissions`. The [image entrypoint](entrypoint.md) sets `IS_SANDBOX=1`; the host bundle runtime does not. |
| Codex | Add `--dangerously-bypass-approvals-and-sandbox`. |
| Pi | No permission flag. |

### System prompt

| Harness | Mapping |
| --- | --- |
| Claude Code | Add `--system-prompt-file <bundle>/<system-prompt-asset>`. |
| Codex | Set `model_instructions_file` in rendered `config.toml` to `<bundle>/<system-prompt-asset>`. |
| Pi | Add `--system-prompt <system-prompt>`. |

The mapping is omitted when `assets.systemPrompt` is absent.

Append-system-prompt behavior is not part of the agentfile schema.

### MCP servers

When `assets.configTemplate` is present, Claude Code receives:

```text
--mcp-config <profile>/claudecode/mcp.json --strict-mcp-config
```

Codex reads MCP configuration from `<profile>/codex/home/.codex/config.toml`.

Codex always renders `assets.configTemplate` as its private `config.toml`.

Pi does not support harness configuration templates.

### Harness commands

Each command below represents an executable and argument array, not a shell command string.

Harness command preparation supplies each symbolic value as one argument without shell interpolation.

Optional bracketed arguments are included only when the corresponding feature is present.

#### Claude Code

Required executable: `claude`.

The harness environment contains `IS_DEMO=1`.

One-shot:

```text
claude --print
  --model <model>
  --dangerously-skip-permissions
  [--bare]
  [--system-prompt-file <bundle>/<system-prompt-asset>]
  [--mcp-config <profile>/claudecode/mcp.json --strict-mcp-config]
  <prompt>
```

TUI:

```text
claude
  --model <model>
  --dangerously-skip-permissions
  [--bare]
  [--system-prompt-file <bundle>/<system-prompt-asset>]
  [--mcp-config <profile>/claudecode/mcp.json --strict-mcp-config]
```

ACP:

```text
claude
  --output-format stream-json
  --verbose
  --model <model>
  --dangerously-skip-permissions
  [--bare]
  [--system-prompt-file <bundle>/<system-prompt-asset>]
  [--mcp-config <profile>/claudecode/mcp.json --strict-mcp-config]
  --input-format stream-json
  --include-partial-messages
```

Top-level `bare: true` adds `--bare`.

Bare mode disables Claude Code auto-discovery of hooks, skills, plugins, MCP servers, memory, and `CLAUDE.md`. It also prevents use of `CLAUDE_CODE_OAUTH_TOKEN`.

#### Codex

Required executable: `codex`.

One-shot:

```text
codex exec
  --skip-git-repo-check
  --dangerously-bypass-approvals-and-sandbox
  --model <model>
  <prompt>
```

TUI:

```text
codex
  --dangerously-bypass-approvals-and-sandbox
  --model <model>
```

ACP:

```text
codex
  --dangerously-bypass-approvals-and-sandbox
  --model <model>
  app-server
```

The generated `config.toml` sets `project_doc_max_bytes = 0` so workspace `AGENTS.md` files do not change the packaged agent behavior.

It marks `<workspace>` as trusted so Codex does not request trust confirmation.

#### Pi

Required executable: `pi`.

One-shot:

```text
pi -p
  --provider <provider>
  --model <model>
  --no-context-files
  [--system-prompt <system-prompt>]
  [--skill <bundle>/skills/<skill-name> ...]
  <prompt>
```

TUI:

```text
pi
  --provider <provider>
  --model <model>
  --no-context-files
  [--system-prompt <system-prompt>]
  [--skill <bundle>/skills/<skill-name> ...]
```

ACP:

```text
pi
  --mode rpc
  --provider <provider>
  --model <model>
  --no-context-files
  [--system-prompt <system-prompt>]
  [--skill <bundle>/skills/<skill-name> ...]
```

`--no-context-files` prevents workspace `AGENTS.md` and `CLAUDE.md` files from changing the packaged agent behavior.

## Upstream references

These upstream harnesses change frequently. When their documented flags or configuration formats change, update this file and the corresponding harness mapping tests.

- Claude Code CLI: <https://code.claude.com/docs/en/cli-reference>
- Claude Code settings: <https://code.claude.com/docs/en/settings>
- Claude Code skills: <https://code.claude.com/docs/en/skills>
- Claude Code MCP: <https://code.claude.com/docs/en/mcp>
- Codex authentication: <https://developers.openai.com/codex/auth>
- Codex access tokens: <https://developers.openai.com/codex/enterprise/access-tokens>
- Codex non-interactive mode: <https://developers.openai.com/codex/noninteractive>
- Codex configuration: <https://developers.openai.com/codex/config-advanced>
- Codex skills: <https://developers.openai.com/codex/skills>
- Codex MCP: <https://developers.openai.com/codex/mcp>
- Codex app-server: <https://developers.openai.com/codex/app-server>
- Pi usage: <https://pi.dev/docs/latest/usage>
- Pi providers: <https://pi.dev/docs/latest/providers>
- Pi RPC: <https://pi.dev/docs/latest/rpc>
