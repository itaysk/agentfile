What can you do with Agentfile?

- Build custom agents specialized for each task. [Read more >>](#customize)
- Automate business processes with scriptable agents. [Read more >>](#automate)
- Organize skills across projects and teams. [Read more >>](#organize)
- Operationalize agents for production and scale. [Read more >>](#operationalize)

# Customize

Don't use the same generic agent for everything - create custom agents specialized for each task.

AI agents are only as good as you make them. But how do you make an agent better? You give it skills, tools, and context. You iterate and improve it over time.

> "The highest leverage thing you have to optimize for AI agents is their context. If you’re not getting enough out of AI agents, you probably have a context problem."

[Aaron Levie, CEO @ Box](https://x.com/levie/status/1962319895070458241)

> A sharp model with thin context loses to an average model with the right context, every time.

[Montana Labs Applied AI Research](https://x.com/montana_labs/status/2072413861211414931)

**Agentfile lets you easily build custom AI agents. Each agent is purpose-built for a task and carries its own skill set and configuration.**

# Automate

There is huge potential for using AI agents in business automation, CI/CD workflows, and scripts.

These so-called "one-shot agents" introduce a different set of requirements from familiar chat assistants.

One-shot agents need to be:

- Unattended and non-interactive
- Reusable and deployable
- Goal-driven and independent

[Claude's documentation](https://code.claude.com/docs/en/overview) showcases examples of one-shot agents using the `claude -p` command. For example: `Slack me if you see any anomalies`. This is an inspiring demonstration, but it leaves important questions unanswered:

- What is the log format: JSON, text, an access log, or a binary journal?
- What constitutes an anomaly? Consider technical, business, or security signals? What is the baseline for comparison? What is the alert threshold?
- How does the agent connect to Slack? Which workspace and channel should it use, and how are credentials supplied?

The assumption is that you run the command on your personal computer, where you have already configured and authenticated the Slack MCP server, installed skills and tools globally, and have source code available to clarify any ambiguity. And let's be honest: even without these assumptions, the model is pretty smart and could probably make progress on its own if it had to. But you will end up spending a lot of time and tokens helping the agent relearn the task every time it is triggered, not to mention the output variance this introduces.

For ad hoc exploration, it's a fair tradeoff, but for business automation at scale, you can set your agent up for success: Document the log format and include the schemas and tools to parse it efficiently. Define the anomaly criteria and provide access to historical records. Explicitly install the Slack MCP server and instruct it where and how to alert you.

**Agentfile lets you build scriptable agents. The prompt and goal are baked in. The harness is configured and tuned to run to completion. Standard input, standard output, and exit codes behave as you'd expect from any other process.**

# Organize

- "What's the best way to install skills into my coding agent and keep them updated?"
- "How do I manage skills and configurations for each project I'm working on?"
- "How do I share my setup with my teammates? How do we collaborate and improve our skill set?"
- "How can I reuse my setup across different models, harnesses, and vendors?"

These are real questions that users ask as they advance in their agentic coding journey. There are typically two options:

User-level: Everything is in your local development environment at the user level, typically in your home directory. This has the obvious downside of being personal to you. In addition, there's no separation between projects and use cases, so as you work across different projects, you lose track of what is installed and what it was meant for. You let the agent spontaneously decide whether and when to use skills. You also expose yourself to cross-skill and cross-session contamination.

Repo-level: Everything is in the code repository you're working on, typically in a .agents directory or equivalent. But you still have one pool of skills for every possible task, and you rely on your agent to pick the relevant ones every single turn. You also need to duplicate shared capabilities in every other repository your team maintains. And importantly, not everything lives in a canonical repository.

The logical solution is to organize agent skills and capabilities around the job itself.

**Agentfile lets you build agents that carry their own skill sets. It's the only organization system that makes sense at scale.**

# Operationalize

Getting an agent to work the way you want is only one part of the job. What happens when it's finally time to deploy the agent to a production environment?

Most agentic platforms and services offer cloud-like hosting for agents, but you probably already have a preferred stack, cloud vendor, or self-hosted infrastructure. Your agents probably need to follow the same production-readiness requirements and be compatible with your tools and practices—monitoring, security, policy, and so on.

**Agentfile lets you build portable agents as containers. Deploy agents anywhere—your cloud, on-premises Kubernetes, GitHub Actions, or workflow engines. Use your favorite tools and practices to manage agents like any other containerized workload.**
