# Agentfile - build your own agent

What is Agentfile?

Agentfile helps you build custom agents as portable container images.

- No code, declarative agents - driven by Markdown and YAML and managed in git.  
- Leverage agentic harness tools you already know and trust - Claude, Codex, Pi, and more.  
- Standard container images that run anywhere - locally, in cloud, Kubernetes, or CI/CD.

What can you do with Agentfile?

- Build custom agents specialized for each task. [Read more >>](docs/use-cases.md#customize)
- Automate business processes with scriptable agents. [Read more >>](docs/use-cases.md#automate)
- Organize skills across projects and teams. [Read more >>](docs/use-cases.md#organize)
- Operationalize agents for production and scale. [Read more >>](docs/use-cases.md#operationalize)

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

Have fun with it:

```sh
tail logfile.jsonl | af run log-triage
cd documents && af run grammar-checker --workspace .
cron "0 0 * * *" "af run daily-standup"
```

## Getting started

- [Installation](./docs/install.md)
- [Tutorial](./docs/tutorial.md)
- [Use cases](./docs/use-cases.md)
- [Examples](./docs/examples)
- [Reference documentation](./docs/reference/reference.md)
