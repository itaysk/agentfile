# Agentfile - package reusable agents

Agentfile packages a tuned agent—its system prompt, skills, MCP servers, model, and credential wiring—as a repeatable container image. Use the same agent unattended in automation, through its native terminal UI, or from an ACP client.

- No code, declarative agents - driven by Markdown and YAML and managed in git.
- Leverage agentic harness tools you already know and trust - Claude, Codex, Pi, and more.
- Standard container images that run anywhere - locally, in cloud, Kubernetes, or CI/CD.

Agentfile makes agents familiar:

```shell
cd blog-post && af run grammar-check --workspace .
tail logfile.jsonl | af run logtriage
cron "0 0 * * *" "af run daily-report"
af run code-review --tui --workspace .
af run --acp code-review # spawn this command from an ACP client

# casually use it like any container image
af build -f agentfile.yaml --tag itaysk/my-agent:latest
docker push itaysk/my-agent:latest
kubectl run my-agent --image=itaysk/my-agent:latest
```

## Getting started

- [Installation](./docs/install.md)
- [Tutorial](./docs/tutorial.md)
- [Examples](./docs/examples)
- [Reference documentation](./docs/reference/reference.md)
