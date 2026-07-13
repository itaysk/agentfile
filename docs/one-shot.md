# One-shot Scribtable Agents

AI assistants have changed our relationship with personal computing and productivity work, but there's still huge potential in using goal-driven, unattended agents within business automations, CI workflows and scripts (a.k.a "one-shot" agents). Think about an agent that process invoices, an agent that sends emails, an agent that classifies tickets - specialist agents that take part in a bigger workflow.  
These use cases introduces new challenges - how to create agents for unattended, non-interactive work? How to package agents and deploy them in remote environments? And what does the new Markdown programming toolchain looks like?

One-shot agents are:
- unattended, non-interactive
- reusable
- deployable
- packaged, self-contained
- goal-driven

[Claude's documentation](https://code.claude.com/docs/en/overview) showcases some one-shot examples.

```bash
# Analyze recent log output
tail -200 app.log | claude -p "Slack me if you see any anomalies"

# Automate translations in CI
claude -p "translate new strings into French and raise a PR for review"

# Bulk operations across files
git diff main --name-only | claude -p "review these changed files for security issues"
```

That is literally how it's put on the website, no further explanation. But these examples are naive.  
Take the first one "slack me if you see any anomalies":
- What is the log format? Is it JSON, text, nginx access logs? perhaps it's binary like Linux Journal?
- What is an "anomaly"? Is 1% increase in error rate significant? If we considered yesteday's log would we find other anomalies? Are we looking for technical failures, business anomalies, security signals, or customer-impacting symptoms?
- And how does "Slack me" supposed to work? Which slack? Who is "me"? Which credentials? 

To be fair, I suppose this example would work if:
- You already connected Slack to your local Claude Desktop application or Claude Code via MCP, and authenticated interactively via browser login.
- It would have analyzed the log file and guessed it's format on the fly.
- If the anomaly is contained to the input file, it could find something that stands out just by comparison.
- You've authenticated and configured Claude to your liking with global settings.

It's definitely a good start for a personal experimentation. But it's not a production-ready solution:
- You don't want the agent to spend time and money re-learning the log format (and potentially failing occasionally) - you want to give it the tools and instructions to be successful.
- You don't want to let the agent decide what matters for you - you want to define the criteria based on your business understanding, and you want to evolve it over time.
- You don't want the agent to rely your personal configuration - you want to predefine the required integrations and credentials.

Going down this path with claude is possible, but you'll end up engineering claude more than your agnet. 
That cute command like will quickly grow many flags, supporting files and environment variables. 
And that's just for claude. Codex is using toml configurations. Pi has a no-mcp agenda. Each harness is different, but they all share the same core principles. 

With Agentfile, you can focus on improving your agent. A single intuitive YAML file defines that agent. You can easily customize any aspect of the agent with a quick edit to this file.
The agent is fully portable - a container image you can simply run everywhere.
