# Agentfile Introduction

Agentfile is a way to build agents that takes the technical boilerplate away so you can focus on what makes your agent great.

Agentfile is:

- A declarative definition of AI agents (IaC for agents).
- Built on agents you trust (Claude, Codex, Pi, etc.).
- Vendor agnostic (runs locally, on-prem, or in the cloud).

This is a tutorial that walks you through what Agentfile is, it's design rationale and core concepts. For the full manual see [./man.md](man.md)

---

## Hello World

Let's create a minimal "Hello World" Agent:

```yaml source=/docs/examples/hello-world/Agentfile.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  prompt:
    text: |
      say hi!
```

That's it! You can run this agent now:

```bash
af run -f hello-world.yaml
```

The agent runs, does its task, and exits.

This is an intentionally simple example. Authoring real-world agents can quickly grow into a Markdown behemoth. It would be inconvenient and impractical to manage all of this content in one file. Fetchers can help you streamline that. >>

---

## Fetchers

An agent's assets can be inlined or fetched from other places:

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  systemPrompt:
    git:
      url: https://github.com/jujumilk3/leaked-system-prompts//openai-chatgpt5-codex_20260325.md
  prompt:
    text: |
      say hi!
  skills:
    - fs:
        path: /skills/world-greetings
```

Notice that the system prompt was fetched from a git repository, and the skill was fetched from another directory. `git` and `fs` are just two common fetchers, but there are more types available and more ways to use them.

You may have considered that hardcoding an absolute file path like that can complicate packaging and deployment of the agent. A better way to fetch local files is from the local project. >>

---

## Project

The directory where the Agentfile lives is considered the project home.
When referring to a filesystem path, you can use project-relative assets:

```
Agentfile.yaml
skills/
  world-greetings/
    SKILL.md
```

```yaml source=/docs/examples/hello-world-project-skill/Agentfile1.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  systemPrompt:
    git:
      url: https://github.com/jujumilk3/leaked-system-prompts//openai-chatgpt5-codex_20260325.md
  prompt:
    text: |
      say hi!
  skills:
    - fs:
        projectPath: /skills/world-greetings
```

Notice that the skill path is now relative to the project root.

Assembling every individual project file in the Agentfile can quickly lead to bloated YAML that just mimics the project directory structure. Luckily, that's not needed with auto-discovery.

You can skip declaring project assets entirely, and let them be discovered automatically by naming convention.
Almost every field in the Agentfile has a corresponding file name convention.
If you simply create a file in the project with that name, it will be recognized automatically:

```
Agentfile.yaml
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
  prompt:
    text: |
      say hi like they do in Hawaii!
```

Notice that we've omitted the skills field and instead let it be discovered under the conventional `skills` directory.

In fact, an Agentfile could even be empty, as long as the project directory is fully set up:

```
Agentfile.yaml
prompt.md
skills/
  world-greetings/
    SKILL.md
```

```yaml source=/docs/examples/hello-world-project-skill/Agentfile3.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
```

Notice that we've omitted even the prompt field and let it be discovered under its conventional file name.

All these options and tricks are powerful but can introduce uncertainty about what will eventually happen. This is where the lock file helps. >>

---

## Lock file

The lock file is a fully defined and reproducible version of the Agentfile. Fully defined means every implicit value is explicitly defined. Reproducible means every asset gets a revision-stable identifier. The lock file is verbose, but it's generated for you.
By separating what you (the user) care about from what the computer (runtime) cares about, you benefit from a simple and intuitive authoring experience while retaining a consistent and reliable runtime.

If we examine one of our agents from earlier:

```
Agentfile.yaml
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
  prompt:
    text: |
      say hi like they do in Hawaii!
```

It would expand into the following lock file:

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  prompt:
    text: |
      say hi like they do in Hawaii!
    lock: sha256:a1b2c3
  skills:
    - fs:
        projectPath: /skills/world-greetings
      lock: sha256:a1b2c3
```

Notice that the skills section, which was auto-discovered, appears fully defined in the lock file. Also notice that every asset has a `lock` field that helps uniquely verify and identify the asset.

You should not conflate the lock field with version specifiers. The lock field is meant to be managed automatically. If you want to pin an asset to a specific version, you should use the relevant fetcher parameters. For example, to pin a git asset:

```yaml source=/docs/examples/hello-world-project-skill/Agentfile4.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  systemPrompt:
    git:
      url: https://github.com/jujumilk3/leaked-system-prompts//openai-chatgpt5-codex_20260325.md
      ref: v0.0.1
  prompt:
    text: |
      say hi like they do in Hawaii!
  skills:
    - fs:
        projectPath: /skills/world-greetings
```

Notice we pinned the git asset to a specific git tag.

The lock file has all the information needed to run an agent, but to do this, we first need to build it. >>

---

## Build

The build step takes a lock file and builds a container image that includes all of the assets and implements all of the configuration you defined in your agent. The resulting agent image is a regular container image: running it executes your agent!

So now you know the full lifecycle: Agentfile -> lock file -> container image -> running agent.
You can invoke any step directly, which starts the build process from that stage:

```bash
af build --lock-only -f hello-world.yaml   # lock
af build -f hello-world.yaml               # lock & build
af run -f hello-world.yaml                 # lock & build & run
af run -f hello-world.lock.yaml            # build & run
docker run hello-world:latest              # run
```

Running the agent image executes the agent as you defined it. There's a lot happening inside the agent image to make it work, and the heart of it all is the Harness. >>

---

## Harness

A harness is the program that implements the "agentic loop"; it orchestrates LLM inference, chain of thought, tool calling, and more. You might already be familiar with Claude Code, Codex, Pi, etc. These are all harnesses.

Every agent image is essentially a harness, pre-configured and bundled with your assets and instructions. [Pi](https://pi.dev) is the default harness, but you can change it to another harness if you prefer.

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  harness: claude-code
  prompt:
    text: |
      say hi!
```

Notice that we selected Claude Code as the harness.

Every harness has different usage, configuration, requirements, and tweaks, but you don't need to worry about any of this. Agentfile abstracts this away from you.
