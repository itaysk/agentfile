# Agentfile Introduction

Agentfile is a declarative way to build agents - driven by YAML files and project conventions.  
It leverages agentic harness tools you already know and trust - Claude, Codex, Pi, and more.  
Agents become standard container images that can run anywhere - locally or on Kubernetes.  

This is a tutorial that walks you through basic concepts of Agentfile. For the full manual see [./man.md](man.md)

The basic anatomy of an agent includes:
1. **Prompt** - instructions for the agent
2. **LLM** - language model that infers the prompt
3. **Harness** - software that wires the LLM, prompt, and responses together

There can be many more agentic component, such as Skills, Tools, MCPs, and more, but at the bare minimum, an agent must have those three.

Let's see how to compose the core agent properties with an Agentfile. >>

---

## Hello World

Let's create a basic "Hello World" agent by creating an Agentfile:

```yaml source=/docs/examples/hello-world/Agentfile1.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: haiku-4.5
  prompt:
    text: |
      say hi!
```

We've created an agent! Notice that the agent defines the core component such as the prompt, model and harness.  
The prompt was defined inline, we'll later see other ways to manage prompts and additional markdown-driven assets.  
We selected Anthropic as the LLM provider, and specifically the Claude Haiku model for our agent.  
We also defined that our agent will be based on Claude Code harness. Since we want to keep this example simple, we don't set any further harness configuration. 

We can build this agent and get a runnable container image:

```bash
af build -f hello-world.yaml
docker images | grep 'hello-world'
```

The resulting image is self-sustained and contains everything the agent need to run.  
To run it, you only need to provide your LLM provider credentials:

```bash
export ANTHROPIC_API_KEY='ant-...'
docker run -e ANTHROPIC_API_KEY hello-world:latest
```

To keep it simple we use an environment variable, but keep in mind that there are additional credential management methods that we'll cover later.

You handle the agent image like any other container image:

```bash
docker tag hello-world:latest itaysk/hello-world:latest
docker push itaysk/hello-world:latest
kubectl run hello-world --image itaysk/hello-world:latest --env ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY
```

In the example, the prompt was defined inside the Agentfile, but in real world scenario it would likely be managed in a dedicated file, or in a remote location. Let's see how Agentfile helps facilitate this. >>

---

## Assets sources

Agent development involves writing a lot of Markdown: Prompts, System Prompt, Context, Skills, etc - these are all important "Assets" that together define an agent.  
So far we've seen our prompt asset defined inside the Agentfile, but assets can be sourced from different places, and Agentfile lets you mix them effortlessly.

Consider the following project structure:

```
Agentfile.yaml
prompt.md
skills/
  world-greetings/
    SKILL.md
```

And the following Agentfile:

```yaml source=/docs/examples/hello-world-project-skill/Agentfile1.yaml
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
      say hi like they do in Hawaii!
  systemPrompt:
    git:
      url: https://github.com.com/itaysk/agentfile//docs/examples/test-sys-prompt.md
  skills:
    - fs:
        path: skills/world-greetings
```

Notice that we've added a skill to our agent, and we source it from the conventional skills directory structure, as indicated by the `fs` (filesystem) source.  
Also notice we've added a system prompt to our agent, and we source it from a remote repository, as indicated by the `git` source.

When you build the agent, assets are gathered and assembled automatically!

Real world agents can be Markdown heavy, with hundreds of files that make up the agent. Listing every single file in the Agentfile would be painful, but luckily that's not required.  
Almost every field in the Agentfile has a corresponding file name convention. If you simply create a file in the project with that name, it will be recognized automatically.

```
Agentfile.yaml
prompt.md
skills/
  world-greetings/
    SKILL.md
```

```yaml source=/docs/examples/hello-world-project-skill/Agentfile2.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: haiku-4.5
```

Notice that we've omitted the `skills` field and just let it be discovered under the conventional `skills` directory.  
Similarly, we've removed the `prompt` field and converted it to a file `prompt.md`.

When you build the agent, auto-discovered assets and explicitly defined assets are merged together to form the complete Agentfile.  

Asset sources can have different parameters that lets you specifically control the source. For example:

```yaml
git:
  url: https://github.com/example/example
  commit: a1b2c3 # fetch from a specific commit

git:
  url: https://github.com/example/example
  ref: v0.0.1 # fetch from a specific ref (head or tag)

file:
  absolutePath: /etc/file.md # fetch from absolute path

http:
  url: https://example.com/skills.tar.gz
  unarchive: true # extract the downloaded asset
```

For a complete reference of all sources and their configuration parameters, see [here]().

While Markdown assets define the core of the agent's behavior, the agent might need access to additional tools. >>

---

## Tools

When you build an agent, the Agentfile's `harness` field implies the base image for resulting agent image. For example, if you chose `harness: claudecode`, the agent image would use `agentfile/claudecode:latest` as base image, which includes basic tools. You can examine the default Dockerfiles to understand what's built in [here]().

You can extend the default base image to include anything else your agent might need. Create a custom image:

```Dockerfile source=/docs/examples/hello-world-image/Dockerfile1
FROM itaysk/af-claudecode:latest

ADD --unpack https://github.com/Code-Hex/Neo-cowsay/releases/download/v2.0.4/cowsay_2.0.4_Linux_arm64.tar.gz /usr/local/bin
```

Notice we started "from" the original Claude Code base image, meaning we're extending it.  
We've added a binary from the web, extracted it, and placed it in the conventional system binaries location, so it should be ready to use.

To use it our custom image, use the `image` field:

```yaml source=/docs/examples/hello-world-image/Agentfile1.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  harness:
    claudecode: {}
    image: myimage:latest
  llm:
    anthropic:
      model: haiku-4.5
  prompt: |
    say hi using the cowsay command!
```

CLI tools are straightforward for agents to use, but MCP servers require additional setup to register with the agent harness.  
Install an MCP server in your base image, and declare it in the Agentfile:

```Dockerfile source=/docs/examples/hello-world-image/Dockerfile2
FROM itaysk/af-claudecode:latest

RUN apk update && apk add --no-cache uv
RUN uv tool install mcp-server-time
```

```yaml source=/docs/examples/hello-world-image/Agentfile2.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  harness:
    claudecode: {}
    image: myimage:latest
  llm:
    anthropic:
      model: haiku-4.5
  prompt: |
    say hi! if it's before 12AM say good morning.
  mcps:
    - command: ["uv", "tool", "run", "mcp-server-time"]
```

Notice the Dockerfile installed the MCP server in the agent image, and the Agentfile registers is with the harness (claude code in this case).

---

