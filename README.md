# Agentfile - build reusable, scriptable, one-shot agents. 

AI assistents have changed our relationship with personal computing and productivity work, but there's still huge potential in using goal-driven, unattended agents within business automations, CI workflows and scripts (a.k.a "one-shot" agents).  
This use case introduces new challenges - how to create agents for unattended, non-interactive work, how to package agents and deploy them in remote environments in a repeatable way, and how does the new Markdown programming toolchain looks like.  
Agentfile is an opinionated framework that answers those questions.

- No code, declarative agents - driven by Markdown and YAML and managed in git.
- Leverage agentic harness tools you already know and trust - Claude, Codex, Pi, and more.
- Standard container images that run anywhere - locally, in cloud, Kubernetes, or CI/CD.

Agentfile makes agents familiar:

```shell
cd blog-post && af run grammer-check --here
tail logfile.jsonl | af run logtriage
cron "0 0 * * *" "af run daily-report"

# casually use it like any container image
af build -f agentfile.yaml --tag itaysk/my-agent:latest
docker push itaysk/my-agent:latest
kubectl run my-agent --image=itaysk/my-agent:latest
```

## Getting started

- Build the Go CLI locally:

  ```shell
  go install ./cmd/af
  af --help
  ```

  `af build` and `af run` require Docker. Git sources require `git` on the build machine.
- [Tutorial](./docs/tutorial.md)
- [Examples](./docs/examples)
- [Reference documentation](./docs/reference/reference.md)
