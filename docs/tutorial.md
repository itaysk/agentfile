# Agentfile Introduction

Agentfile is a way to build agents that takes the technical boilerplate away so you can focus on what makes your agent great.

Agentfile is:

- A declarative definition of AI agents (IaC for agents).
- Harness agnostic (Claude, Codex, Pi, etc.).
- Runtime agnostic (runs locally, on-prem, or in the cloud).

---

## Hello World

Let's define a minimal Agent:

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  prompt:
    text: |
      say hi!
```

Run this agent:

```bash
af run -f hello-world.yaml
```

Authoring agents can quickly turn you into a Markdown engineer. It's inconvenient and impractical to manage the complete context of an agent in one file. >>

---

## Sources

Values can be inlined or sourced externally.

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  systemPrompt:
    git: https://github.com/jujumilk3/leaked-system-prompts//openai-chatgpt5-codex_20260325.md
  prompt:
    text: |
      say hi like they do in Hawaii!
  skills:
    - oci: docker.io/itaysk/world-greetings-skill:latest
```

The system prompt is fetched from a central git repository. The skill is fetched from a container image registry. Fetching and assembling assets is handled automatically.
There are various ways to source assets. Assets can also be developed in the local project. >>

---

## Project

The directory where the Agentfile lives is considered the project home. While an Agentfile could define every single asset in YAML, it can also automatically discover assets by convention in the current project.

```.
Agentfile.yaml
skills/
  world-greetings/
    SKILL.md
```

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  prompt:
    text: |
      say hi like they do in Hawaii!
```

We've omitted the explicit skill declaration and instead let it be discovered under the conventional `skills` directory. There are many other conventional alternatives to Agentfile settings.
You can also mix explicit and implicit assets:

```.
Agentfile.yaml
prompt.md
```

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  skills:
    - oci: docker.io/itaysk/world-greetings-skill:latest
```

Here we fetch the skill from a container image registry again, but discover the prompt file by convention.
Again, fetching and assembling assets is handled automatically. If this feels unstable for production use, keep reading about the lock file. >>

---

## Lock file

The lock file is a complete and accurate version of the Agentfile. Technically, it's a superset of the Agentfile, so in theory, the Agentfile could be identical to the lock file, but that would be inconvenient to author. By separating what you (the user) care about from what the computer (runtime) cares about, you benefit from a simple and intuitive authoring experience while retaining a consistent and reliable runtime.

The following Agentfile:

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  systemPrompt:
    git: https://github.com/jujumilk3/leaked-system-prompts//openai-chatgpt5-codex_20260325.md
  prompt:
    text: |
      say hi!
```

Would be resolved into a lock file like:

```yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
  systemPrompt:
    git: https://github.com/jujumilk3/leaked-system-prompts//openai-chatgpt5-codex_20260325.md
    version: a1b2c3
    checksum: a1b2c3
  prompt:
    text: |
      say hi!
    checksum: a1b2c3
  skills:
    - file: /skills/world-greetings/
      checksum: a1b2c3
```

Notice that all assets now have a checksum, versions are fixed, and implicit assets are explicitly declared.
From this fully defined lock file, we can easily build a container image. >>

---

## Build

The Agentfile can be built, or packaged, into a runnable artifact:

```bash
af build -f hello-world.yaml
```

This would 1) resolve all undetermined references, 2) produce a lock file that captures the resolved state, and 3) build a container image from that lockfile that executes your agent.
You can invoke any of the intermediate artifacts, which would start the build process from that stage:

```bash
af run -f hello-world.yaml # lock & build & run
af run -f hello-world.lock.yaml # build, run
af build -f hello-world.yaml # build
docker run hello-world:latest # run
```

You read that right: the resulting Dockerfile is runnable right away, and will execute your agent as defined in the Agentfile. How does that work? >>

---

## Harness

Harness is the program that implements the "[agentic loop]()"; it orchestrates LLM inference, chain of thought, tool calling, and more.
You might already use a harness such as Claude Code, Codex, Pi, etc. Every harness has different usage, configuration, conventions, and tweaks; Agentfile abstracts this away from you. When building an image, we stitch together assets and configure the harness accordingly.

[Pi](https://pi.dev) is the default harness, but you can change it to another harness you prefer.
