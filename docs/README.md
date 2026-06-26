- [Tutorial](tutorial.md) - Walkthrough of Agentfile design rationale and core concepts.
- [Examples](examples/) - Example files for feature walkthroughs and sanity tests.
- [Reference Manual](reference/reference.md) - Product manual and implementation specification.
- [Harness Reference](reference/harness.md) - Harness-specific runtime mappings.
- [Agentfile schema](reference/agentfile.schema.json) - JSON Schema for Agentfile documents.
- [Agentfile schema example](reference/Agentfile.yaml) - Example Agentfile with all possible fields and features.
- [CLI reference](reference/cli.sh) - CLI commands and flags.
- [Conformance](reference/conformance.md) - Validation and golden-test expectations for implementations.

Examples synchronization:
Markdown documents reference examples by adding a `source=/path/to/file` annotation to the fenced code block. For example:

~~~markdown
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
      model: claude-haiku-4-5
  prompt:
    text: |
      say hi!
```
~~~

Check examples against docs using [./sync-examples.py](./sync-examples.py).

```bash
python3 docs/sync-examples.py
```

Sync discrepant examples into the docs with `--write`.

```bash
python3 docs/sync-examples.py --write
```
