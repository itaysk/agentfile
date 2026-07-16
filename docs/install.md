# Installation

## Prerequisites

- The selected harness (`claude`, `codex`, or `pi`) on `PATH` for `runa` invocations.
- Docker for image builds and image runs. Docker is not required to build bundles or use `runa`.
- Git if fetching assets from Git repositories.

## Installation

Check out the [releases](https://github.com/itaysk/agentfile/releases) page for available versions.

To quickly install on macOS with Apple Silicon:

```shell
curl -sSfL https://github.com/itaysk/agentfile/releases/latest/download/agentfile_darwin_arm64.tar.gz | sudo tar xz -C /usr/local/bin af
```
