# Harness Map

Agentfile defines an agent once and builds it for a target harness. This maps each
capability to its Claude Code, Codex, and Pi equivalent: the flags, files, and
conventions the build emits.

These tools change frequently. Verify flags and paths against the linked references.

## System Prompt

Refs: [Claude CLI](https://code.claude.com/docs/en/cli-reference), [Codex config](https://developers.openai.com/codex/config-advanced), [Codex AGENTS.md](https://developers.openai.com/codex/guides/agents-md), [Pi usage](https://pi.dev/docs/latest/usage).

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| Replace base prompt | `--system-prompt`, `--system-prompt-file` | `model_instructions_file` | `--system-prompt`, `.pi/SYSTEM.md`, `~/.pi/agent/SYSTEM.md` |
| Append instructions | `--append-system-prompt`, `--append-system-prompt-file` | `developer_instructions` | `--append-system-prompt`, `APPEND_SYSTEM.md` |
| Project guidance | `CLAUDE.md` | `AGENTS.md` | `AGENTS.md`, `CLAUDE.md` |

## Prompt

Refs: [Claude CLI](https://code.claude.com/docs/en/cli-reference), [Codex non-interactive](https://developers.openai.com/codex/noninteractive), [Pi usage](https://pi.dev/docs/latest/usage).

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| Interactive | `claude` | `codex` | `pi` |
| One-shot | `claude -p` | `codex exec` | `pi -p` |
| Structured stream | `--output-format stream-json` | `codex exec --json` | `--mode json`, `--mode rpc` |
| File references | `@file` | `@file` | `@file` |
| Stdin | Yes | Yes | Yes |

## LLM

Refs: [Claude auth](https://code.claude.com/docs/en/authentication), [Claude model config](https://code.claude.com/docs/en/model-config), [Codex auth](https://developers.openai.com/codex/auth), [Codex models](https://developers.openai.com/codex/models), [Codex env vars](https://developers.openai.com/codex/environment-variables), [Pi providers](https://pi.dev/docs/latest/providers).

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| Model | `--model`, `settings.json` | `--model`, `config.toml` | `--model`, `/model` |
| Reasoning | `--effort` | `model_reasoning_effort` | `--thinking` |
| Providers | Anthropic, Bedrock, Vertex AI, Foundry | OpenAI, custom, Bedrock, local OSS | Built-in and custom |
| Custom providers | gateway/env configuration | `model_provider`, `model_providers` | `models.json`, extension |
| Subscription auth | Claude.ai | ChatGPT | ChatGPT, Claude, GitHub Copilot |
| API key auth | `ANTHROPIC_API_KEY` | `CODEX_API_KEY` (exec only), API-key login | `--api-key`, provider env vars, `/login` |
| Other credentials | `apiKeyHelper`, `CLAUDE_CODE_OAUTH_TOKEN` (`claude setup-token`) | seeded `~/.codex/auth.json` (`codex login --with-api-key` / `--with-access-token`) | seeded `~/.pi/agent/auth.json` |
| Auth cache | `~/.claude/.credentials.json` (relocate with `CLAUDE_CONFIG_DIR`) | `~/.codex/auth.json` or OS keychain | `~/.pi/agent/auth.json` |

Write provider and auth settings to user config, not project-local config (for Codex, `.codex/config.toml`). Keep credentials and auth caches out of images and version control.

## Non-Interactive Startup

Refs: [Claude CLI](https://code.claude.com/docs/en/cli-reference), [Codex CLI](https://developers.openai.com/codex/cli), [Pi usage](https://pi.dev/docs/latest/usage).

The one-shot modes (see Prompt) run a single turn and exit, with no login or trust prompt to fall back on. Credentials must already be present (see LLM); beyond that, these interactive-first gates can block an unattended run.

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| Startup gates | `--bare` accepts `ANTHROPIC_API_KEY` or `apiKeyHelper`, not `CLAUDE_CODE_OAUTH_TOKEN` | Git-repo check (`--skip-git-repo-check`); read-only unless `--sandbox workspace-write` | Project trust skipped by default; `--approve` to load project-local resources |

## Skills

Refs: [Claude skills](https://code.claude.com/docs/en/skills), [Codex skills](https://developers.openai.com/codex/skills), [Pi skills](https://pi.dev/docs/latest/skills).

All three support skills natively in `SKILL.md` format.

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| User location | `~/.claude/skills` | `~/.agents/skills` | `~/.agents/skills`, `~/.pi/agent/skills` |
| Project location | `.claude/skills` | `.agents/skills` | `.agents/skills`, `.pi/skills` |
| Explicit invocation | `/skill-name` | `/skills`, `$skill` | `/skill:name`, `--skill` |
| Disable discovery | `--bare`, `--safe-mode` | per-skill `[[skills.config]]` | `--no-skills` |

## MCP

Refs: [Claude MCP](https://code.claude.com/docs/en/mcp), [Codex MCP](https://developers.openai.com/codex/mcp), [Pi usage](https://pi.dev/docs/latest/usage).

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| Native MCP | Yes | Yes | No |
| Configure | `claude mcp add`, `.mcp.json`, settings | `codex mcp add`, `mcp_servers` | extension or skill |
| Transports | stdio, SSE, HTTP, WebSocket | stdio, streamable HTTP | extension-defined |
| Run harness as MCP server | Yes | `codex mcp-server` | SDK or extension |

## Security And Trust

Refs: [Claude settings](https://code.claude.com/docs/en/settings), [Codex config](https://developers.openai.com/codex/config-advanced), [Pi usage](https://pi.dev/docs/latest/usage).

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| Command safety | permission allow/deny lists | approvals, sandbox, rules | tool selection |
| Project trust | MCP and project approvals | trusted `.codex/` config | project trust |
