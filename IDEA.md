# Agentfile: declarative agent builder

One-shot agents are easy to demo. They are much harder to scale and  run repeatadly and conssistently.

The moment an agent leaves an interactive chat and enters a server, a workflow, a script or CI, the prompt is no longer enough. The agent needs context, skills, tools, credentials, configurations and flags. Those pieces need to be reviewed, versioned, reproduced, and shipped together.

Agentfile helps you facilitate that process. It makes working with agentic tools like Claude Code or Codex explicit, portable, and deployable. 

## The premise

You may be familiar with `claude -p`, Claude's one-shot mode. Claude's documentation lists examples like these on the first page of the [Claude Overview doc](https://code.claude.com/docs/en/overview):

```bash
# Analyze recent log output
tail -200 app.log | claude -p "Slack me if you see any anomalies"

# Automate translations in CI
claude -p "translate new strings into French and raise a PR for review"

# Bulk operations across files
git diff main --name-only | claude -p "review these changed files for security issues"
```

These examples are compelling because they show the shape of the future: agents that can be invoked by scripts, jobs, and events and magically get a job done by a casual description.

They are also demos. In a real environment, each of those one-liners hides a lot of decisions that cannot be left implicit.

## The contract

Take the log example: "Slack me if you see any anomalies"

What is the log format? Is it JSON, text, nginx access logs, application logs, or Linux journal output? Should the agent infer the format from the sample? What happens when it guesses correctly most of the time, but incorrectly on the incident that matters?

In a real system, the input format is part of the agent contract. You would document it. You might provide examples. You might install tools like `jq` or `journalctl`. You might add a small parser or a skill that explains how to inspect the data. Without that contract, the agent spends tokens rediscovering facts the system already knows, and the result is harder to trust.

The same issue appears in the word "anomaly." Is a 1% increase in error rate significant? Is a 10% traffic spike expected because marketing launched a campaign? Are we looking for technical failures, business anomalies, security signals, or customer-impacting symptoms?

That judgment does not live in the one-line prompt. It lives in operational context: runbooks, alert definitions, service ownership, historical baselines, release calendars, business rules, and team-specific conventions. For serious work, the prompt becomes a document, often a set of documents, refined over time.

## The integration

"Slack me" sounds simple until you need to run it outside your laptop.

In a local interactive setup, you might install a Slack plugin and authenticate as yourself. In automation, that is not enough. You need an MCP server or tool integration, credentials, scopes, secret management, channel policy, and a runtime environment where all of that is present before the agent starts.

"Raise a PR" has the same issue. The agent needs `git` and `gh`. It needs repository access. It needs to know the contribution rules, branch naming conventions, review expectations, and what kind of changes are acceptable to submit automatically.

These are not exotic requirements. They are the normal requirements that appear when a useful demo becomes a team workflow.

## The runtime

One-shot agents still run on models, harnesses, and configuration.

What model should handle this task? A cheap model might be enough for drafting release notes, while a code migration or security review may require a stronger one. What flags should the harness use? What tools should be available? Which filesystem paths can the agent read or write?

**Interactive agent use is forgiving because the user can steer the agent when something is underspecified.** Automation is less forgiving. You cannot make agent behavior deterministic, but you can lock down the inputs you control: prompts, tools, versions, model settings, credentials, runtime image, permissions, and deployment shape.

That is the difference between "run this prompt" and "run this agent."

## What actually happens

The cute one-liner becomes a long command, then it becomes a script. Then the script grows flags, setup steps, prompt files, helper utilities, and local conventions. Someone adds a README. Someone else adds a Dockerfile. The directory becomes a git repository because the team needs review, history, and collaboration.

At that point, the agent already has a project. And every team would rediscover it from scratch. 

Agentfile defines that format once and for all.  

Agentic tools are already good when you use and guide them yourself. The next step is making agents that you would be confident plugging into real workflows, agents that are reviewable, repeatable, portable, and scalable.
