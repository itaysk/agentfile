- [Tutorial](tutorial.md) - Walkthrough of Agentfile design rationale and core concepts.
- [Examples](examples/) - Example files for feature walkthroughs and sanity tests.
- [Reference Manual](reference/reference.md) - Product manual and implementation specification.
- [Harness Reference](reference/harness.md) - Harness-specific runtime mappings.
- [agentfile schema](reference/agentfile.schema.json) - JSON Schema for agentfile documents.
- [agentfile schema example](reference/agentfile.yaml) - Example agentfile with all possible fields and features.
- [CLI reference](reference/cli.sh) - CLI commands and flags.

Examples synchronization:
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

Check examples against docs using [./sync-examples.py](./sync-examples.py) or via the Makefile:

```bash
make sync-examples # only check for discrepacies
make check-examples # check for discrepacies and sync discrepant examples
```
