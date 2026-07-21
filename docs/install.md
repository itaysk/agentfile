# Installation

## Prerequisites

- Supported harness (`claude`, `codex`, or `pi`).
- Docker for image builds and image runs.
- Git if fetching assets from Git repositories.

## Installation

Check out the [releases](https://github.com/itaysk/agentfile/releases) page for available versions.

To quickly install on macOS with Apple Silicon:

```shell
curl -sSfL https://github.com/itaysk/agentfile/releases/latest/download/agentfile_darwin_arm64.tar.gz | sudo tar xz -C /usr/local/bin af
```

Verify the installation:

```shell
af --help
```
