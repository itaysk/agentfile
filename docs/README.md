- [Tutorial](tutorial.md) - Tutorial walking you through what Agentfile is, its design rationale and core concepts.
- [Reference Manual](reference/reference.md) - Describes every feature, behavior and specification of the project.
- [examples/](examples/) - Examples files for various features and scenarios which are also used for e2e/sanity tests.
- [Agentfile reference](reference/Agentfile.yaml) - All the Agentfile fields.
- [CLI reference](reference/cli.sh) - All the CLI commands and flags.

Examples synchronization:
Markdown documents reference examples by adding a `source=/path/to/file` annotation to the fenced code block. For example 

~~~markdown
```yaml source=/docs/examples/hello-world.yaml
apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello-world
spec:
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
