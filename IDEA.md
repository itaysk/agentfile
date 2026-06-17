# Agentfile: an agent builder

Coding agents can be used in many scenarios outside of development, such as automating routine tasks. You may be familiar with `claude -p`, which is Claude's one-shot mode. Claude's documentation lists these use cases on the first page of the [Claude Overview doc](https://code.claude.com/docs/en/overview):

```bash
# Analyze recent log output
tail -200 app.log | claude -p "Slack me if you see any anomalies"

# Automate translations in CI
claude -p "translate new strings into French and raise a PR for review"

# Bulk operations across files
git diff main --name-only | claude -p "review these changed files for security issues"
```

These are compelling examples, but they are naive and unrealistic.

## The gaps

What is the log format? Should Claude just guess it based on the content? What if it guesses wrong? What if it guesses wrong only in some cases? And how much do you pay for this guesswork? In real life, you would instruct Claude on the incoming log format and how to parse it. In some cases, you would even need to provide a simple utility that helps with this. For example, the log could be a [Linux journal log](https://www.freedesktop.org/software/systemd/man/latest/systemd.journal-fields.html#), which is binary on disk, and the practical way to work with it is with the `journalctl` utility.
Gap: Document the input format, install tools (`journalctl`, `jq`, etc.), and document tool usage (skills).

What is an "anomaly" for us? Is a 1% increase in the error rate significant? Is a 10% increase in traffic after a promotion an anomaly? Do we care about technical or business metrics? In real life, an elaborate prompt with much more context would be provided. These kinds of prompts look more like long documents, take time and insight to create, and are continuously fine-tuned over time.
Gap: Prompt is a file (or multiple files), not a phrase. Needs to be tracked. Additional context needs to be pulled from organizational sources.

Telling Claude to "Slack you" is cool, but we'll need to set it up first. In your local development environment, you could run `claude plugin install slack`, but that is an interactive flow that logs your personal user into the development environment. In real life, we would need to set all this up.
Gap: Set up the Slack MCP server, authentication, and credential management.

Raising a PR is a multi-step process that requires organizational context and permissions. Where is the code hosted? What are the contribution guidelines?
Gap: Install tools (git, gh), organizational or project context (AGENTS.md or pulled from organizational knowledge base).

What model is this running? Do we want to use a leaner model for small tasks like drafting commit messages? In the real world, we would want to use the right model for the task.
Gap: Missing model selection (per task)

Claude Code is optimized for local interactive use. Running it in a job, worker, or webhook scenario requires adjustments to work efficiently. In addition, interactive use is forgiving of the nondeterministic nature of coding agents, since the user can steer the agent as needed. In real life, you would want to lock the agent behavior into a known good state that you control.
Gap: Missing many flags and configuration options.

## The real world

1. That cute one-liner becomes a monstrous command at best, or a lengthy script if we're being honest.
2. More files are added for Markdown context, skills, and instructions.
3. Setup scripts are added to ensure the environment is consistent.
4. The project directory grows and turns into a git repository, more people collaborate on it.
5. Deploying this directory becomes a manual nightmare.

These are not problems with Claude Code. Claude's job is to be a helpful coding agent, which it is. It is still your job, the user, to decide, instruct, connect, control, and manage the work for your coding agent. Agentfile gives you a system for organizing and packaging those prompts, scripts, agent settings, and tools. It takes the boilerplate decisions away so you can focus on what makes your agent great.
