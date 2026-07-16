# Agentfile documentation

- [Installation](install.md) — Install the Agentfile CLI.
- [Tutorial](tutorial.md) — Walk through Agentfile's core concepts.
- [Examples](examples/) — Explore example agentfiles and assets.
- [Use cases](use-cases/) — What you can do with Agentfile.

## Reference

- [Reference manual](reference/reference.md) — Product manual and implementation specification.
- [Harness reference](reference/harness.md) — Harness-specific mappings.
- [Agent bundle format](reference/bundle.md) — Portable artifact spec.
- [Agent image format](reference/image.md) — Container image contract.
- [Agent image entrypoint](reference/entrypoint.md) — Container-start behavior.
- [`runa`](reference/runa.md) — Host bundle runner.
- [agentfile schema](reference/agentfile.schema.json) — agentfile JSON Schema.
- [agentfile schema example](reference/agentfile.yaml) — Full agentfile yaml reference.
- [CLI reference](reference/cli.sh) — CLI commands and flags.

## Examples and snippets

Markdown documents reference examples by adding a `source=/path/to/file` annotation to the fenced code block. For example:

~~~markdown
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
~~~

Check examples against the docs using [sync-examples.py](./sync-examples.py) or the Makefile:

```bash
make check-examples # check for discrepancies
make sync-examples  # update discrepant examples
```
