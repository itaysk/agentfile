# Agentfile - build your own agent

What is Agentfile?

Agentfile helps you build custom and portable agents.

- No code, declarative agents - Driven by Markdown and YAML and managed in git.  
- Bring your own harness - Claude, Codex, Pi, and more.  
- Deploy anywhere - Locally, in cloud, Kubernetes, or CI/CD.

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

Build it, run it, ship it:

```sh
# run an agentfile
af run -f myagent.yaml
# run it with a host-installed harness (unsandboxed)
af run -f myagent.yaml --host --workspace .
# build a portable bundle
af build --target bundle -f myagent.yaml --output my-agent.tar.gz
# build it to an image
af build --target image --bundle my-agent.tar.gz --tag itaysk/my-agent:latest
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
