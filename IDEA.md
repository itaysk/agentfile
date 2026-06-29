# Why Agentfile?

The idea started when I wanted to leverage agents in my note-taking system (a.k.a. second-brain/llm-wiki). I wanted to let pre-defined agents routinely triage, curate and improve my notes for me. That meant I needed to create "agents" that are reusable, self-contained, and I can easily invoke them as needed.  
I didn't want to code an agent with an SDK, I only care about the markdown. I didn't want to commit to one harness, LLM provider, or a cloud company. And I wanted to run this on my computer, but also elsewhere.  
What I needed is something like docker for agents. I actually started with just a Dockerfile for the agent, but it quickly grew - With many files and skills that I needed to manage, and harness-specific flags and quirks I needed to learn, and scripts and wrappers I needed to write to fit in my workflow. And then repeat it consistently for every new agent.  
You can think of Agentfile as a unified abstraction on top of harnesses that compiles into Docker.

My personal need also tapped into a broader observation, that people are looking for ways to deploy agents and operationalize them in production. While several cloud/PaaS agentic platform exists, I thought there's room for a Kubernetes-native experience. People trust Kubernetes, it runs practically everywhere, and it also provides many important facilities out of the box that are usefule for agents.

One-shot agents are:
- unattended, non-interactive
- reusable
- deployable
- packaged, self-contained
- goal-driven

I found the [Claude Overview doc](https://code.claude.com/docs/en/overview) showcases some one-shot examples:

```bash
# Analyze recent log output
tail -200 app.log | claude -p "Slack me if you see any anomalies"

# Automate translations in CI
claude -p "translate new strings into French and raise a PR for review"

# Bulk operations across files
git diff main --name-only | claude -p "review these changed files for security issues"
```

But these examples are naive and unrealistic. Take the first one "slack me if you see any anomalies". 

There are too many unknowns:
- What is the log format? Is it JSON, text, nginx access logs? perhaps it's binary like Linux Journal?
- What is an "anomaly"? Is 1% increase in error rate significant? If we considered yesteday's log would we find other anomalies? Are we looking for technical failures, business anomalies, security signals, or customer-impacting symptoms?
- And how does "Slack me" supposed to work? Which slack? Who is "me"? Which credentials? 

To their credit, I suppose this would actually work in a local setup, because:
- You already have Claude installed, authenticated and configured to your liking.
- You already connected Slack to your local Claude Desktop application or Claude Code via MCP, and authenticated interactively via browser login.
- It would have analyzed the log file and guessed it's format on the fly.
- If the anomaly is contained to the input file, it could find something that stands out just by comparison.

And it's definitely a good start for a personal use case or for experimentation.  
But it's not a production-ready solution:
- You don't want the agent to spend time and money re-learning the log format (and potentially failing occasionally) - you want to give it the tools and instructions to be successful.
- You don't want to let the agent decide what matters for you - you want to define the criteria based on your business understanding, and you want to evolve it over time.
- You don't want the agent to use your personal Slack or computer - you want to predefine the required integrations and credentials.

Going down this path with claude is possible, but you'll end up with a monstrous command line with many flags, supporting files and environment variables.  
And that's just for claude. Codex is using toml configurations. Pi has a no-mcp agenda. Each harness is different, but they all share the same core principles.  
All of this boilerplate screams for a tool, convention and abstration to simplify and standardize the process of creating one-shot agents.
