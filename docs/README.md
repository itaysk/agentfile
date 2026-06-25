- [tutorial.md](intro.md) - Tutorial walking you through what Agentfile is, it's design rationale and core concepts.
- [man.md](man.md) - Full reference manual. Should document every single feature and behavior.
- [examples/](examples/) - Examples files for various features and scenarios which are also used for e2e/sanity tests.
- [Agentfile reference](./Agentfile.yaml) - All the Agentfile fields.
- [CLI reference](./cli.sh) - All the cli commands and flags.

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
