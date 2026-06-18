- [tutorial.md](intro.md) - Walkthrough what Agentfile is. not supposed to cover every single feature, not even the important ones, just to give a an idea of what is Agentfile is and how it's like to use it. 
- [man.md](man.md) - Full reference manual. Should document every single feature and behavior.
- [examples/](examples/) - Examples files for various features and scenarios which are also used for e2e/sanity tests.

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

Sync examples with docs using [./sync-examples.py](./sync-examples.py).

```bash
python3 docs/examples.py
```
