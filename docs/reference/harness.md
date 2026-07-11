# Harness Reference

The [reference manual](./reference.md) defines the agentfile schema and effective agentfile algorithm.
This file defines how agentfile features map to each harness's configuration and runtime behavior.

A "harness adapter" is the implementation that performs the mapping and setup.
Adapter work happens at build time, with access to the Agentfile project. The adapter produces a layout that is fully set up and ready to run.

If a selected harness cannot represent an effective agentfile capability listed here, the build must fail with a clear unsupported-combination error.

## Runtime Layout

The agent process runs with `/agent/workspace` as its working directory. The workspace is considered purely under the user's and agent's control and is meant for input/output. It does not contain agentfile setup files.

The build stages agentfile assets under `/agent/agentfile`.  
The staged files are implementation input for the harness and the image entrypoint.

| Asset | Staged location |
| --- | --- |
| Effective agentfile | `/agent/agentfile/agentfile.effective.json` |
| Prompt | `/agent/agentfile/prompt.md` |
| System Prompt | `/agent/agentfile/system-prompt.md` |
| Skills | `/agent/agentfile/skills/<skill-name>/...` |
| Harness config | `/agent/agentfile/<harness>/...` |

The effective agentfile is the fully resolved, explicit and complete agentfile serialized as JSON. It is the machine-readable runtime specification for the image.

Harness config files are staged into the image at their `/agent/agentfile/<harness>/` paths with a placeholder token in place of each `runtimeEnv` reference. At container start, the entrypoint reads the effective agentfile and substitutes the tokens with values from the container environment, in place.
Runtime values never appear in image layers, regardless of harness capabilities.

Harness config files are staged into the image at their `/agent/agentfile/<harness>/` paths with a placeholder token in place of each `runtimeEnv` reference. At container start, the generated entrypoint substitutes the tokens with values from the container environment, in place. A config file without runtime references is final as staged. Runtime values never appear in image layers, regardless of harness capabilities.

The image entrypoint must:

1. Validate that every runtime variable is provided. Empty string is a value and considered provided.
2. Substitute placeholder values in the staged harness config files, escaped for the config format (JSON/TOML).
3. Apply `spec.envs` as default environment variables. `runtimeEnv` entries resolve from their source variable.
4. Identify and setup runtime overrides to agentfile fields:
  1. `AGENTFILE_PROMPT` (default to `spec.prompt`)
  2. `AGENTFILE_MODEL` (default to `spec.llm.model`)
5. Set the harness home and config environment described below.
6. Run from `/agent/workspace`.
7. Exit before launching the harness when `AGENTFILE_RENDER_ONLY` is set and non-empty.
8. Setup harness stdout and stderr streaming.
9. Invoke harness command with correct flags and variables.
10. Exit with the harness process exit code.

The entrypoint owns the `AGENTFILE_` environment variable namespace: it accepts `AGENTFILE_PROMPT` and `AGENTFILE_MODEL` as run overrides, publishes their effective values along with `AGENTFILE_PROVIDER` and `AGENTFILE_SYSTEM_PROMPT`, reads `AGENTFILE_RENDER_ONLY`, and may use further `AGENTFILE_`-prefixed variables internally (for example during config substitution). This is why agentfile entry names and `runtimeEnv` names must not start with `AGENTFILE_`.

The entrypoint resolves each runtime variable once and substitutes that single resolution into every config token that references it.

## Harness Homes

Each harness gets an image-local generated home directory. The generated home is where Agentfile writes harness config and installs harness-local assets. It is not copied from the build host.

| Harness | Generated home | Runtime environment |
| --- | --- | --- |
| Claude Code | `/agent/agentfile/claudecode/home` | `HOME=/agent/agentfile/claudecode/home` |
| Codex | `/agent/agentfile/codex/home` | `HOME=/agent/agentfile/codex/home`, `CODEX_HOME=/agent/agentfile/codex/home/.codex` |
| Pi | `/agent/agentfile/pi/home` | `PI_CODING_AGENT_DIR=/agent/agentfile/pi/home` |

Generated homes may contain config, copied skills, and other Agentfile-owned runtime assets. They must not contain LLM credentials, host auth caches, or host user-level harness configuration.

## Provider Support

`spec.llm` declares the provider expected by the user. The selected build-time adapter must either implement that provider exactly as listed here or reject the effective agentfile at build time.

| Provider | Claude Code | Codex | Pi |
| --- | --- | --- | --- |
| `anthropic` | Use `--model`; credentials from `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN` (requires non-[bare mode](#bare-mode)). | Unsupported. | Use `--provider anthropic --model`; credentials from `ANTHROPIC_API_KEY`. |
| `openai` | Unsupported. | Use `--model`; derive `CODEX_API_KEY` from `OPENAI_API_KEY` when unset. | Use `--provider openai --model`; credentials from `OPENAI_API_KEY`. |
| `openrouter` | Unsupported. | Unsupported. | Use `--provider openrouter --model`; credentials from `OPENROUTER_API_KEY`. |

Unsupported providers may be added later by extending this file. Until then, they are invalid for the listed harness.

## Permissions

Agentfile agents run in containers, and the harness is launched with permission and approval gating disabled by default.

| Harness | Mapping |
| --- | --- |
| Claude Code | Use `--dangerously-skip-permissions`, with environment variable `IS_SANDBOX=1` |
| Codex | Use `--dangerously-bypass-approvals-and-sandbox` |
| Pi | No action needed. |

## System Prompt

`spec.systemPrompt` replaces the selected harness's default base prompt when the harness exposes a replacement mechanism. If `spec.systemPrompt` is absent, the harness default applies.

| Harness | Mapping |
| --- | --- |
| Claude Code | Use `--system-prompt-file /agent/agentfile/system-prompt.md`. |
| Codex | Set `model_instructions_file = "/agent/agentfile/system-prompt.md"` in generated `config.toml`. |
| Pi | Use `--system-prompt "$AGENTFILE_SYSTEM_PROMPT"` with the resolved contents. |

Agentfile does not currently model append-system-prompt behavior. Harness append flags such as Claude Code `--append-system-prompt-file`, Codex `developer_instructions`, or Pi `--append-system-prompt` are outside the reference schema.

## Prompt

All harnesses must run in one-shot mode and receive the resolved prompt text as the user task.

| Harness | One-shot command |
| --- | --- |
| Claude Code | `claude --print <prompt-text>` |
| Codex | `codex exec <prompt-text>` |
| Pi | `pi -p <prompt-text>` |

The implementation must pass prompt text as an argument or stdin without shell interpolation. Prompt text is not a path and must not be converted to an `@file` reference.

## Skills

Skill directories from the agentfile are copied unchanged, including all files below the resolved skill directory.

| Harness | Mapping |
| --- | --- |
| Claude Code | Install each skill at `/agent/agentfile/claudecode/home/.claude/skills/<skill-name>/`. |
| Codex | Install each skill at `/agent/agentfile/codex/home/.agents/skills/<skill-name>/`. |
| Pi | Use `--skill /agent/agentfile/skills/<skill-name>` once per skill. |

The `<skill-name>` directory is the `name` field in `SKILL.md` front matter. The name must be a single path segment.

## MCP Servers

`spec.mcps` is supported by Claude Code and Codex. It is unsupported by the default Pi adapter because Pi does not include built-in MCP support.

If `spec.mcps` is non-empty and `spec.harness.pi` is selected, the build must fail unless a future Agentfile version defines a Pi extension adapter.

### Claude Code MCP

Write `/agent/agentfile/claudecode/mcp.json` and invoke Claude Code with:

```text
--mcp-config /agent/agentfile/claudecode/mcp.json --strict-mcp-config
```

The generated JSON shape is:

```json
{
  "mcpServers": {
    "time": {
      "type": "stdio",
      "command": "uv",
      "args": ["tool", "run", "mcp-server-time"],
      "env": {
        "EXAMPLE": "value"
      }
    },
    "search": {
      "type": "http",
      "url": "https://example.com/mcp",
      "headers": {
        "Authorization": "Bearer token"
      }
    }
  }
}
```

For a `stdio.command` array, the first item becomes `command` and remaining items become `args`.

### Codex MCP

Write MCP servers to `/agent/agentfile/codex/home/.codex/config.toml` using `mcp_servers` tables.

```toml
project_doc_max_bytes = 0

[mcp_servers.time]
command = "uv"
args = ["tool", "run", "mcp-server-time"]

[mcp_servers.time.env]
EXAMPLE = "value"

[mcp_servers.search]
url = "https://example.com/mcp"
http_headers = { Authorization = "Bearer token" }
```

For a `stdio.command` array, the first item becomes `command` and remaining items become `args`.

## Harness Commands

The following commands define the runtime launch commands emitted by the default harness adapters. Implementations may construct equivalent `execve` argument arrays, but must preserve these semantics.

### Claude Code

Required executable: `claude`.

Runtime environment:

```text
HOME=/agent/agentfile/claudecode/home
IS_SANDBOX=1
```

Command:

```bash
claude \
  --print \
  --model "$AGENTFILE_MODEL" \
  --no-session-persistence \
  --dangerously-skip-permissions \
  [--bare] \
  [--system-prompt-file /agent/agentfile/system-prompt.md] \
  [--mcp-config /agent/agentfile/claudecode/mcp.json --strict-mcp-config] \
  "$AGENTFILE_PROMPT"
```

Flags passed explicitly still apply, and all of the necessary features can be configured with explicit flags except skills.

#### Bare mode

Bare mode (claude's `--bare` flag) minimizes claude's footprint and startup time by disabling auto-discovery for hooks, skills, plugins, MCP servers, memory, and `CLAUDE.md`. It also automatically sets `CLAUDE_CODE_SIMPLE=1` which simplifies the system prompt.

You can control bare mode with `spec.harness.claudecode.bare` which is a boolean flag.

Bare mode is off by default and used only when `bare: true` is set.

Bare mode does not load skills (see below). `bare: true` with skills is rejected (validation error).

Bare mode does not read `CLAUDE_CODE_OAUTH_TOKEN`. Do not set `bare: true` when the agent authenticates with a Claude subscription token. `bare: true` with a `spec.envs` entry named `CLAUDE_CODE_OAUTH_TOKEN` is rejected (validation error); a token supplied only at run time cannot be validated.

##### Bare mode and Skills

Verified empirically against Claude Code 2.1.204 (by capturing the API request bodies claude sends under each flag combination):

- In bare mode, skill discovery ignores `~/.claude/skills` entirely — skills staged in the generated home are invisible (`/skill-name` returns "Unknown command").
- `--add-dir <dir>` does make bare mode discover skills, but only from `<dir>/.claude/skills` (so `--add-dir /agent/agentfile/claudecode/home` would re-expose the staged skills; pointing `--add-dir` at a skills directory itself does nothing).
- Discovered or not, bare mode never advertises skills to the model: the request's tool list is only `Bash`, `Edit`, `Read` — no `Skill` tool — and skill names/descriptions appear nowhere in the request.

Rejected feature request: https://github.com/anthropics/claude-code/issues/37207

### Codex

Required executable: `codex`.

Runtime environment:

```text
HOME=/agent/agentfile/codex/home
CODEX_HOME=/agent/agentfile/codex/home/.codex
```

If `CODEX_ACCESS_TOKEN` is unset, `OPENAI_API_KEY` is set, and `CODEX_API_KEY` is unset, the entrypoint must set `CODEX_API_KEY="$OPENAI_API_KEY"` for the Codex process. Do not persist these variables.
If `CODEX_ACCESS_TOKEN` is set, it takes precedence over `CODEX_API_KEY` and `OPENAI_API_KEY`.

Command:

```bash
codex exec \
  --skip-git-repo-check \
  --dangerously-bypass-approvals-and-sandbox \
  --model "$AGENTFILE_MODEL" \
  "$AGENTFILE_PROMPT"
```

Codex reads generated system prompt, MCP, and other adapter config from `$CODEX_HOME/config.toml`. The generated config must include `project_doc_max_bytes = 0` so workspace `AGENTS.md` files do not change the packaged agent behavior.

### Pi

Required executable: `pi`.

Runtime environment:

```text
PI_CODING_AGENT_DIR=/agent/agentfile/pi/home
```

Command:

```bash
pi \
  -p \
  --provider "$AGENTFILE_PROVIDER" \
  --model "$AGENTFILE_MODEL" \
  --no-context-files \
  [--system-prompt "$AGENTFILE_SYSTEM_PROMPT"] \
  [--skill /agent/agentfile/skills/<skill-name> ...] \
  "$AGENTFILE_PROMPT"
```

`--no-context-files` prevents workspace `AGENTS.md` and `CLAUDE.md` files from changing the packaged agent behavior. Skills declared by the agentfile are still loaded through explicit `--skill` flags.

## Upstream References

These upstream harnesses change frequently. When their documented flags or config formats change, update this file and the corresponding harness mapping tests.

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
- Pi usage: <https://pi.dev/docs/latest/usage>
- Pi providers: <https://pi.dev/docs/latest/providers>
