# Introduction to Agentfile

Agentfile helps you build custom and portable agents.

- No-code, declarative agents — driven by Markdown and YAML and managed in Git.
- Bring your own harness — Claude, Codex, Pi, and more.
- Deploy anywhere — locally, in the cloud, Kubernetes, or CI/CD.

This tutorial walks through the basic concepts of Agentfile. See the [product manual](./manual.md) for the complete workflow.
To follow along, [install Agentfile](./install.md) first.

Let's get started >>

---

## Hello World

Let's create a basic "Hello World" agent. Create a YAML file named `agentfile.yaml`:

```yaml source=/docs/examples/hello-world/agentfile1.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  prompt:
    text: |
      say hi!
```

We gave the agent a name, selected its model and harness, and assigned its task.

We can now build this agent:

```bash
af build --file agentfile.yaml
# Built hello-world__latest.tar.gz
```

We've created an agent bundle. A bundle is a simple packaging of agent's skills, tools, prompts and environment.  
Bundles are portable, you can run a bundle on another host as long as the required harness is installed (the bundle does not install anything on the host).
If you're a developer looking to integrate agentfile into other products or workflows, see the [bundle format specification](reference/bundle.md) and the [bundle runtime specification](reference/runa.md).

To run a bundle, you only need to provide LLM credentials. See the [authentication documentation](./manual.md#authentication) for details.

```bash
export ANTHROPIC_API_KEY='ant-...'
af run --bundle hello-world__latest.tar.gz
# Hi!
```

Notice that we're not "chatting" with the agent. By specifying a `prompt`, we've predefined the agent's task, and thus created a [one-shot agent](./use-cases.md#automate). Running it resembles running an executable binary or script in that it will perform its task and exit, without requiring our input.

If you are going to deploy your agents in production, automation, or cloud, it is useful to package them as container images:

```bash
af image build --bundle hello-world__latest.tar.gz --tag hello-world:latest
# Built hello-world:latest
```

Unlike the agent bundle we've created before, the agent image has the harness and required tools baked into the image. You can push and deploy it anywhere that can run container images:

```bash
export ANTHROPIC_API_KEY='ant-...'
docker run -e ANTHROPIC_API_KEY hello-world:latest

docker tag hello-world:latest itaysk/hello-world:latest
docker push itaysk/hello-world:latest
kubectl run hello-world --image itaysk/hello-world:latest --env ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY
```

In the example, the prompt was defined inside the agentfile. In a real project, it would often be managed in a dedicated file or a remote location. Let's see how Agentfile supports this.

---

## Asset Sources

Agent development involves writing a lot of Markdown: prompts, system prompts, context, skills, and related assets that together define an agent.
So far, our prompt asset has been defined inside the agentfile. Assets can also come from different sources, and Agentfile lets you mix them.

Consider the following project structure:

```
agentfile.yaml
prompt.md
skills/
  world-greetings/
    SKILL.md
```

And the following agentfile:

```yaml source=/docs/examples/hello-world-project-skill/agentfile1.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world-global
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  prompt:
    text: |
      say hi like they do in Hawaii!
  systemPrompt:
    git:
      url: https://github.com/itaysk/agentfile//docs/examples/test-sys-prompt.md
  skills:
    - fs:
        path: skills/world-greetings
```

We added a skill from the conventional skills directory structure, as indicated by the `fs` (filesystem) source.
We also added a system prompt from a remote repository, as indicated by the `git` source.

When you build this agent, assets are gathered and assembled automatically.

Real-world agents can be Markdown-heavy, with many files that make up the agent. You do not need to list every file in the agentfile.
Common fields such as prompt, system prompt, and skills are discovered automatically from conventional file names.

```
agentfile.yaml
prompt.md
skills/
  world-greetings/
    SKILL.md
```

```yaml source=/docs/examples/hello-world-project-skill/agentfile2.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world-global
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
```

We omitted the `skills` field and let Agentfile discover it under the conventional `skills` directory.
Similarly, we removed the `prompt` field and moved its content to `prompt.md`.

When you build the agent, auto-discovered and explicitly defined assets are merged into the bundle.

Asset sources can have different parameters that let you specifically control the source. For example:

```yaml
git:
  url: https://github.com/example/example
  commit: a1b2c3 # fetch from a specific commit

git:
  url: https://github.com/example/example
  ref: v0.0.1 # fetch from a specific ref (head or tag)

fs:
  absolutePath: /etc/file.md # fetch from absolute path

http:
  url: https://example.com/skills.tar.gz
  archive: true # extract the downloaded asset
```

For a complete reference of all sources and their configuration parameters, see the [agentfile sources specification](./reference/agentfile.md#sources).

While Markdown assets define the core of the agent's behavior, the agent might need access to additional tools.

---

## Tools

When running an agent bundle on another machine, you should ensure the required CLI and MCP tools are installed and configured. The agent bundle does not install anything on the host machine. 

If the agent needs an MCP server, you can declare it in the agentfile:

```yaml source=/docs/examples/hello-world-image/agentfile2.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world-time
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  prompt:
    text: |
      say hi! if it's AM time, say good morning.
  mcps:
    - name: time-mcp
      stdio:
        command: ["uv", "tool", "run", "mcp-server-time"]
```

For more details about MCP, see the [agentfile MCP specification](./reference/agentfile.md#mcp-servers).

When using agent images, all MCP and CLI tools should be available in the container image. You can build a base image with your tools and use it when packaging the agent image:

```Dockerfile source=/docs/examples/hello-world-image/Dockerfile2
FROM itaysk/claudecode:latest

RUN apk update && apk add --no-cache uv
RUN uv tool install mcp-server-time
```

We started from the Claude Code base image, which already includes the harness, and installed the MCP server the agent will use.

```bash
docker build --tag cc-time:latest .
af build --file agentfile.yaml
# Built hello-world-time__latest.tar.gz
af image build --bundle hello-world-time__latest.tar.gz --base-image cc-time:latest --tag hello-world-time:latest
# Built hello-world-time:latest
docker run -e ANTHROPIC_API_KEY hello-world-time:latest
# Good Morning!
```

---

## Workspace

The agent's workspace is its working directory for work in progress, state, and artifacts.

When running an agent, Agentfile creates a new ephemeral workspace by default. Select an existing directory with `--workspace` when the agent needs existing input or when you want its output to persist.

```yaml source=/docs/examples/hello-world-workspace/agentfile1.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world-zip
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  prompt:
    text: |
      get a name to greet from the file `name`.
      if the file is missing, abort.
      write a greeting to this name.
      write the result into a zip file called `greeting.zip`.
```

Notice the agent expects an input in the workspace, and will produce an artifact in the workspace that you can later review.

```bash
af build --file agentfile.yaml
# Built hello-world-zip__latest.tar.gz
mkdir /tmp/greetings
echo 'itay' > /tmp/greetings/name
af run --bundle hello-world-zip__latest.tar.gz --workspace /tmp/greetings
# Created greeting.zip
unzip -p /tmp/greetings/greeting.zip
# Hi Itay!
af image build --bundle hello-world-zip__latest.tar.gz --tag hello-world-zip:latest
# Built hello-world-zip:latest
docker run --rm -e ANTHROPIC_API_KEY -v /tmp/greetings:/agent/workspace hello-world-zip:latest
```

---

## Run CLI

The `af` CLI has 3 main entities:

- `af bundle` - for building and running agent bundles
- `af image` - for building and running agent images
- `af agents` - for registring and running named agents

`af run --bundle / --image  / --name` is a shortcut to the respective full command. `af build` is a shortcut for a bundle build. See [Run an agent](./manual.md#run-an-agent) for details.

You can run agent images and quickly bind the workspace:

```bash
af image run --image hello-world-zip:latest --workspace /tmp/greetings
# Created greeting.zip
```

You can register agents, give them a friendly name, and run them from anywhere:

```bash
af agents register --name hello-world --bundle /path/to/hello-world__latest.tar.gz
# Registered hello-world
af run --name hello-world
# Hi!
af agents register --name hello-world-zip --image hello-world-zip:latest
# Registered hello-world-zip
af run --name hello-world-zip --ws /tmp/greetings
# Created greeting.zip
```

You can quickly set environment variables for the agent:

```yaml source=/docs/examples/hello-world/agentfile2.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  prompt:
    text: |
      say hi to the user $LOGNAME
  envs:
    - name: CLAUDE_CODE_OAUTH_TOKEN
      runtimeEnv:
        name: CLAUDE_CODE_OAUTH_TOKEN
```

```bash
af build --file agentfile.yaml
# Built hello-world__latest.tar.gz
af image build --bundle hello-world__latest.tar.gz --tag hello-world-env:latest
# Built hello-world-env:latest
export CLAUDE_CODE_OAUTH_TOKEN='ant-...'
af image run --image hello-world-env:latest --env LOGNAME="itay" --env-auto
# Hi Itay!
```

`LOGNAME` was provided on the command line, while `CLAUDE_CODE_OAUTH_TOKEN` was forwarded from the host because the agentfile declares it as a `runtimeEnv`. See the [agentfile environment specification](./reference/agentfile.md#environment) for details.

You can override certain fields for a single invocation:

```bash
af run --name hello-world --prompt "say something else"
af run --name hello-world --model "claude-sonnet-4-5"
```

See [Field overrides and diagnostics](./manual.md#field-overrides-and-diagnostics) for details.

---

## Interactive Agents

So far, the examples have demonstrated one-shot agents: the task was predefined and ran to completion. Agents can also run interactively.

`af run --name hello-world --tui` opens the selected harness's native interactive terminal so you can chat with the registered agent.

---

## Next Steps

- [Examples](./examples/README.md)
- [Product manual](./manual.md)
- [Reference specifications](./README.md#reference)
