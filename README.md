# Agentfile - build your own agent

What is Agentfile?

Agentfile helps you build custom and portable agents.

- No-code, declarative agents — driven by Markdown and YAML and managed in Git.
- Bring your own harness — Claude, Codex, Pi, and more.
- Deploy anywhere — locally, in the cloud, Kubernetes, or CI/CD.

What can you do with Agentfile?

- Build custom agents specialized for each task. [Read more >>](docs/use-cases.md#customize)
- Automate business processes with scriptable agents. [Read more >>](docs/use-cases.md#automate)
- Organize skills across projects and teams. [Read more >>](docs/use-cases.md#organize)
- Operationalize agents for production and scale. [Read more >>](docs/use-cases.md#operationalize)

Start with a minimal agentfile:

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: my-agent
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
```

Grow it to add skills, MCP servers, custom tools, and more:

```yaml
spec:
  systemPrompt:
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

Build it, run it, ship it:

```sh
# build and run a bundle
af build --file myagent.yaml --output my-agent.tar.gz
af run --bundle my-agent.tar.gz
# package and run it as an image
af image build --bundle my-agent.tar.gz --tag itaysk/my-agent:latest
af run --image itaysk/my-agent:latest
# push the regular container image
docker push itaysk/my-agent:latest
# deploy it to Kubernetes or any container platform
kubectl run my-agent --image=itaysk/my-agent:latest
```

Have fun with it:

```sh
tail logfile.jsonl | af run --name log-triage
cd documents && af run --name grammar-checker --workspace .
cron "0 0 * * *" "af run --name daily-standup"
```

## Getting started

- [Installation](./docs/install.md)
- [Tutorial](./docs/tutorial.md)
- [Use cases](./docs/use-cases.md)
- [Examples](./docs/examples)
- [Product manual](./docs/manual.md)
