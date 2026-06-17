# Harness Map

Concise feature map for common coding-agent harnesses.

## System Prompt

Refs: [Claude CLI](https://code.claude.com/docs/en/cli-reference), [Claude settings](https://code.claude.com/docs/en/settings), [Codex config](https://developers.openai.com/codex/config-advanced), [Codex AGENTS.md](https://developers.openai.com/codex/guides/agents-md), [Pi usage](https://pi.dev/docs/latest/usage).

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| Replace base prompt | `--system-prompt`, `--system-prompt-file` | `model_instructions_file` | `--system-prompt`, `.pi/SYSTEM.md`, `~/.pi/agent/SYSTEM.md` |
| Append instructions | `--append-system-prompt`, `--append-system-prompt-file` | `developer_instructions` | `--append-system-prompt`, `APPEND_SYSTEM.md` |
| Project guidance | `CLAUDE.md` | `AGENTS.md` | `AGENTS.md`, `CLAUDE.md` |

Codex provider and auth settings belong in user config, not project-local `.codex/config.toml`.

## Prompt Input

Refs: [Claude CLI](https://code.claude.com/docs/en/cli-reference), [Codex non-interactive](https://developers.openai.com/codex/noninteractive), [Pi usage](https://pi.dev/docs/latest/usage).

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| Interactive | `claude` | `codex` | `pi` |
| One-shot | `claude -p` | `codex exec` | `pi -p` |
| Structured stream | `--output-format stream-json` | `codex exec --json` | `--mode json`, `--mode rpc` |
| File references | `@file` | `@file` | `@file` |
| Stdin | Yes | Yes | Yes |

## Model And Auth

Refs: [Claude auth](https://code.claude.com/docs/en/authentication), [Claude model config](https://code.claude.com/docs/en/model-config), [Codex auth](https://developers.openai.com/codex/auth), [Codex models](https://developers.openai.com/codex/models), [Pi providers](https://pi.dev/docs/latest/providers).

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| Model | `--model`, `settings.json` | `--model`, `config.toml` | `--model`, `/model` |
| Reasoning | `--effort` | `model_reasoning_effort` | `--thinking` |
| Subscription auth | Claude.ai | ChatGPT | ChatGPT, Claude, GitHub Copilot |
| API key auth | `ANTHROPIC_API_KEY` | API-key login, `CODEX_API_KEY` for `codex exec` | `--api-key`, `/login`, provider env vars |
| Providers | Anthropic, Bedrock, Vertex AI, Foundry | OpenAI, custom providers, Bedrock, local OSS | Built-in and custom providers |
| Custom providers | gateway/env configuration | `model_provider`, `model_providers` | `models.json`, extension |

Do not commit API keys, auth caches, or local account state.

## Skills

Refs: [Claude skills](https://code.claude.com/docs/en/skills), [Codex skills](https://developers.openai.com/codex/skills), [Pi skills](https://pi.dev/docs/latest/skills).

| Feature | Claude Code | Codex | Pi |
|---|---|---|---|
| Support | Native | Native | Native |
| Format | `SKILL.md` | `SKILL.md` | `SKILL.md` |
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

| Area | Claude Code | Codex | Pi |
|---|---|---|---|
| Project trust | MCP/project approvals | trusted `.codex/` config | project trust |
| Command safety | permissions, allow/deny lists | approvals, sandbox, rules | tool selection, extensions |
| Risky extension points | plugins, MCP, hooks, skills | plugins, MCP, hooks, skills | extensions, skills |

Verify current flag names before scripting; all three harnesses change frequently.
