# Agentfile - build your own agent

Don't use the same generic agent for everything - create customized agents that are specialized for each task.

AI Agents are only as good as YOU make them. But how do you make an agent better? You give it skills, tools, and context. You iterate and improve it over time.

Agentfile lets you build portable and custom AI agents easily.

- No code, declarative agents - driven by Markdown and YAML and managed in git.  
- Leverage agentic harness tools you already know and trust - Claude, Codex, Pi, and more.  
- Standard container images that run anywhere - locally, in cloud, Kubernetes, or CI/CD.

Start with a minimal agentfile:

```yaml
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
```

Grow it to add Skills, MCPs, Custom tools, and more:

```yaml
spec:
  system prompt:
    git:
      url: https://github.com/itaysk/agentfile//docs/examples/test-sys-prompt.md
  skills:
    - fs:
        path: skills/world-greetings
  mcps:
    - name: time-mcp
      stdio:
        command: ["uv", "tool", "run", "mcp-server-time"]
```

Build it, run it, deploy it:

```sh
# run an agentfile
af run -f myagent.yaml
# build it to an image
af build -f myagent.yaml --tag itaysk/my-agent:latest
# it's a regular container image
docker push itaysk/my-agent:latest
# deploy it to Kuberntes or any container platform
kubectl run my-agent --image=itaysk/my-agent:latest
```

Build one-shot, scriptable agents:

```sh
tail logfile.jsonl | af run log-triage
cd documents && af run grammar-checker --workspace .
cron "0 0 * * *" "af run daily-standup"
```

## Getting started

- [Installation](./docs/install.md)
- [Tutorial](./docs/tutorial.md)
- [Examples](./docs/examples)
- [Reference documentation](./docs/reference/reference.md)
