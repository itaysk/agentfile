# Agentfile Introduction

Agentfile helps you build custom agents as portable container images.

- No code, declarative agents - driven by Markdown and YAML and managed in git.  
- Leverage agentic harness tools you already know and trust - Claude, Codex, Pi, and more.  
- Standard container images that run anywhere - locally, in cloud, Kubernetes, or CI/CD.

This is a tutorial that walks you through basic concepts of Agentfile. For the full reference manual see [here](./reference/reference.md).  
If you want to follow along, make sure you [install Agentfile](./install.md) first.

Let's get started.

---

## Hello World

Let's create a basic "Hello World" agent by creating an agentfile:

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

We've created an agent! Notice that we've given it a name, selected its model and harness, and gave it its task.

We can build this agent and get a runnable container image:

```bash
af build -f agentfile.yaml
docker images | grep 'hello-world'
```

To run it, you only need to provide your LLM provider credentials:

```bash
export ANTHROPIC_API_KEY='ant-...'
docker run -e ANTHROPIC_API_KEY hello-world:latest
```

Notice that by including a `prompt`, we've defined the agent's task, and thus created a [one-shot agent](./use-cases.md#automate). Running it resembles running a script or an executable binary - it will perform its task and exit, without requiring our input. This is useful in scripts and automations.

You can handle the agent image like any other container image:

```bash
docker tag hello-world:latest itaysk/hello-world:latest
docker push itaysk/hello-world:latest
kubectl run hello-world --image itaysk/hello-world:latest --env ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY
```

In the example, the prompt was defined inside the agentfile definition. In a real project it would often be managed in a dedicated file or a remote location. Let's see how Agentfile helps facilitate this.

---

## Asset Sources

Agent development involves writing a lot of Markdown: prompts, system prompts, context, skills, and related assets that together define an agent.
So far we've seen our prompt asset defined inside the agentfile, but assets can be sourced from different places, and Agentfile lets you mix them effortlessly.

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

Notice that we've added a skill to our agent, and we source it from the conventional skills directory structure, as indicated by the `fs` (filesystem) source.  
Also notice we've added a system prompt to our agent, and we source it from a remote repository, as indicated by the `git` source.

When you build this agent, assets are gathered and assembled automatically.

Real-world agents can be Markdown-heavy, with many files that make up the agent. Listing every single file in the agentfile would be painful, but luckily that's not required.  
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

Notice that we've omitted the `skills` field and just let it be discovered under the conventional `skills` directory.  
Similarly, we've removed the `prompt` field and converted it to a file `prompt.md`.

When you build the agent, auto-discovered assets and explicitly defined assets are merged together to form the complete agentfile.

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

For a complete reference of all sources and their configuration parameters, see [Sources Reference](./reference/reference.md#sources).

While Markdown assets define the core of the agent's behavior, the agent might need access to additional tools.

---

## Tools

When you build an agent, the agentfile's `harness` field selects the default base image for the resulting agent image. For example, if you chose `harness: claudecode`, the agent image uses `itaysk/claudecode:latest` as its base image. The default image names are listed in the [Harness reference](./reference/reference.md#harness).

You can extend the default base image to include anything else your agent might need. Create a custom image:

```Dockerfile source=/docs/examples/hello-world-image/Dockerfile1
FROM itaysk/claudecode:latest

ADD --unpack https://github.com/Code-Hex/Neo-cowsay/releases/download/v2.0.4/cowsay_2.0.4_Linux_arm64.tar.gz /usr/local/bin
```

Notice we started "from" the default Claude Code base image, meaning we're extending it. We've installed a custom binary which our agent can now use.

Build and tag this base image as `my-claudecode-base:latest`, then use the `image` field:

```yaml source=/docs/examples/hello-world-image/agentfile1.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world-cowsay
spec:
  harness:
    claudecode: {}
    image: cc-cowsay:latest
  llm:
    anthropic:
      model: claude-haiku-4-5
  prompt:
    text: |
      use the `cowsay` command to say hi!
```

CLI tools are straightforward for agents to use, but MCP servers require additional setup to register with the agent harness.  
Install an MCP server in your base image, and declare it in the agentfile:

```Dockerfile source=/docs/examples/hello-world-image/Dockerfile2
FROM itaysk/claudecode:latest

RUN apk update && apk add --no-cache uv
RUN uv tool install mcp-server-time
```

```yaml source=/docs/examples/hello-world-image/agentfile2.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world-time
spec:
  harness:
    claudecode: {}
    image: cc-time:latest
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

Notice the Dockerfile installed the MCP server into the agent image, and the agentfile registered it with the harness (Claude Code in this case).

---

## Workspace

The agent's "workspace" is the special directory `/agent/workspace` inside the agent container. The agent is configured to use it for work-in-progress, state, and artifacts storage.  
When running an agent, you can bind-mount the workspace to an existing directory. Do this if your agent needs to work in existing directory (input), or if you will want to access the agent's artifacts once it's done (output).

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
mkdir /tmp/greetings && cd /tmp/greetings
echo 'itay' > ./name
docker run --rm -v /tmp/greetings:/agent/workspace hello-world:latest
unzip -p ./greeting.zip # print the contents of the zip
```

So far we've built the agent image and ran it as a regular container. While that's useful for deploying agents, running agents with the CLI runner has some additional benefits.

---

## Run CLI

Use the `run` command to shorten long docker commands and register agents for repeatable execution:

```bash
af run --file agentfile.yaml # build & run in one go
af run --image hello-world:latest # run a built agent
af agents register -f agentfile.yaml # register an agent in the system
af run hello-world # run registered agent by name
```

You can override some agentfile fields at runtime. This lets you reuse agent as templates:

```bash
af run hello-world --prompt "say something else"
af run hello-world --model "claude-sonnet-4-5"
```

The `run` command can also facilitate runtime setup.  
For example, the `--workspace` flag lets you set a host directory to bind-mount to the workspace instead of writing the Docker mount manually. Use `--ws` as a shorter alias.

```bash
af run hello-world --workspace /tmp/greetings
git checkout fix-bug && af run hello-world --ws .
```

This pattern is especially useful when different agents contribute to the same directory. For example, a planner agent, coder agent, reviewer agent, all collaborating on the same code repository.

Another example, the `run` command lets you quickly set environment varialbes for the agent. The `--env` flag lets you set or export a variable for the agent. In addition, if agent declared its required environment variables, then the `--env-auto` flag will export them automatically from the host.

```source=/docs/examples/hello-world/agentfile2.yaml
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

```sh
af run hello-world --env-auto --env LOGNAME
```

Notice that our agentfile declared a required variable `CLAUDE_CODE_OAUTH_TOKEN`, which is supplied automatically from the host thanks to the `--env-auto` flag (it needs to be exported on the host first). In addition, we have forwarded the user's login name (`LOGNAME`) to the agent.

So far, our examples demonstrated "one-shot" agent - the agent's task was predefined and it ran to completion. But agents can also run interactively.

`af run --tui` lets you chat with the agent in the terminal. This will open the selected harness's native interactive terminal.

```bash
af run code-review --tui --workspace .
```

`af run --acp` lets you integrate your agents with your IDE, Terminal, or otehr [ACP](https://agentclientprotocol.com)-compatible application.  
For example, you could spawn your customized code review agent from your IDE, in the context of the project you're currently working on.

---

# Next steps

- [Examples](./examples/README.md)
- [Reference documentation](./reference/reference.md)
