# Agentfile documentation

- [Installation](install.md) — Install the Agentfile CLI.
- [Tutorial](tutorial.md) — Walk through Agentfile's core concepts.
- [Examples](examples/) — Explore example agentfiles and assets.
- [Use cases](use-cases.md) — What you can do with Agentfile.

## Reference

- [Product manual](manual.md) — Product concepts and workflows.
- [agentfile specification](reference/agentfile.md) — Source YAML, defaults, sources, and discovery.
- [Harness reference](reference/harness.md) — Harness-specific mappings.
- [Agent bundle format](reference/bundle.md) — Portable artifact spec.
- [Agent image format](reference/image.md) — Container image contract.
- [Agent image entrypoint](reference/entrypoint.md) — Container-start behavior.
- [Bundle runtime](reference/runa.md) — Host bundle execution.
- [agentfile schema](reference/agentfile.schema.json) — agentfile JSON Schema.
- [agentfile schema example](reference/agentfile.yaml) — Full agentfile YAML reference.
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
