# Agentfile Introduction

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

This is a tutorial that walks you through basic concepts of Agentfile. For the full manual see the [Reference Manual](./reference/reference.md).  
If you want to follow along, make sure you [install Agentfile](./installation.md).

The basic anatomy of an agent includes:
1. **Prompt** - instructions for the agent
2. **LLM** - language model for inference
3. **Harness** - software that wires the LLM, prompt, and responses together

There can be many more agentic components, such as skills, tools, MCP servers, and context files, but at the bare minimum an agent needs those three.

Let's see how to compose the core agent properties with an agentfile.

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

We've created an agent! Notice that the agentfile defines the prompt, model, and harness.
The prompt was defined inline; later we'll see other ways to manage prompts and additional markdown-driven assets.
We selected Anthropic as the LLM provider, and specifically the Claude Haiku model for our agent.  
We also defined that our agent will be based on the Claude Code harness. Since we want to keep this example simple, we don't set any further harness configuration.

We can build this agent and get a runnable container image:

```bash
af build -f agentfile.yaml
docker images | grep 'hello-world'
```

The resulting image contains the packaged agent definition and runtime setup.
To run it, you only need to provide your LLM provider credentials:

```bash
export ANTHROPIC_API_KEY='ant-...'
docker run -e ANTHROPIC_API_KEY hello-world:latest
```

You handle the agent image like any other container image:

```bash
docker tag hello-world:latest itaysk/hello-world:latest
docker push itaysk/hello-world:latest
kubectl run hello-world --image itaysk/hello-world:latest --env ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY
```

In the example, the prompt was defined inside the agentfile. In a real project it is often managed in a dedicated file or a remote location. Let's see how Agentfile helps facilitate this.

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

When you build the agent, assets are gathered and assembled automatically!

Real-world agents can be Markdown-heavy, with many files that make up the agent. Listing every single file in the agentfile would be painful, but luckily that's not required.
Common prompt, system prompt, and skill assets have file and directory conventions. If you create files in those conventional locations, they are recognized automatically.

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

Notice we started "from" the default Claude Code base image, meaning we're extending it.  
We've added a binary from the web, extracted it, and placed it in the conventional system binaries location, so it should be ready to use.

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

Notice the Dockerfile installed the MCP server in the agent image, and the agentfile registers it with the harness (Claude Code in this case).

---

## Workspace

The agent's "workspace" is the special directory `/agent/workspace` inside the agent container. The agent is configured to use it for work-in-progress, state, and artifacts storage.  
You can bind-mount the workspace to an existing directory. Do this if you want to seed agent's workspace (input), or access the agent's work once it's done (output).

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
      get a name to greet from the file @name.
      if the file is missing, abort.
      write a greeting to this name.
      write the result into a zip file called `greeting.zip`.
```

Notice the agent handles input and output via the workspace.

```bash
mkdir /tmp/greetings && cd /tmp/greetings
echo 'itay' > ./name
docker run --rm -v /tmp/greetings:/agent/workspace hello-world:latest
unzip -p ./greeting.zip # print the contents of the zip
```

So far we've built the agent image and ran it as a regular container. While that's useful for deploying agents, running agents with the CLI runner has some benefits.

---

## Run CLI

Use the `run` command to shorten long docker commands and register agents for repeatable execution:

```bash
af run -f agentfile.yaml # build & run in one go
af agents register -f agentfile.yaml # register hello-world agent
af run hello-world # run agent by name, no need to locate the agentfile.
```

You can override agentfile fields at runtime:

```bash
af run hello-world --llm.anthropic.model "claude-sonnet-4-5" # change model for single run
```

This feature can be utilized for creating ad-hoc agents.  
Create a general agent as a template:

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: cc
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
```

Then launch ad-hoc agents based on it:

```bash
af run cc --prompt "say hi!"
af run cc --prompt "say bye!"
```

The run CLI can also facilitate runtime setup.  
For example, the `--in` flag lets you set a host directory to bind-mount to the workspace instead of writing the Docker mount manually, and the `--here` flag sets the workspace to the current working directory.

```bash
af run hello-world --in /tmp/greetings

cd /tmp/greetings && af run hello-world --here
```

This pattern is especially useful for agents that perform the same work on different working directories. For example, a planner agent, coder agent, reviewer agent, all collaborating on the same code repository.

---

# Next steps

- [Examples](./examples/examples.md)
- [Reference documentation](./reference/reference.md)
