# Agent Image Format

This document defines the normative format of an Agentfile agent image.

Container-start behavior is defined by the [image entrypoint reference](entrypoint.md).

## Terminology

An **agent image** is a container image that contains unpacked bundle contents, a harness and its operating-system environment, and an image entrypoint.

The **source bundle** is the agent bundle archive used to construct the agent image.

**Image construction** is the build-time operation that creates an agent image from a source bundle and a base image.

The **base image** supplies the operating-system environment and executables.

The **image entrypoint** is the executable file generated from the bundle manifest and configured as the image entrypoint.

## Construction

Image construction accepts one source bundle.

It must not read a source agentfile or project, discover or resolve assets, or fetch remote content.

Image construction accepts an optional `--base-image` value. When omitted, it selects the default for the bundle's harness:

- `claudecode`: `itaysk/claudecode:latest`
- `codex`: `itaysk/codex:latest`
- `pi`: `itaysk/pi:latest`

Image construction does not install executables. The base image must provide the selected harness and any other executables required by the agent.

Image construction unpacks the source bundle into `/agent/bundle`, generates the image entrypoint from its manifest, and writes the image configuration defined below.

The source bundle archive is not part of the required image contents.

## Filesystem layout

An agent image contains:

```text
/agent/bundle/
/agent/entrypoint
/agent/workspace/
```

`/agent/bundle` is the unpacked bundle.

`/agent/entrypoint` contains the executable image entrypoint.

Image construction creates `/agent/workspace` as an empty directory.

A container runtime may replace its contents or bind-mount another directory at that path.

At container start, the entrypoint reads `/agent/bundle` in place and writes its private harness profile under `/agent/profile`.

## Image configuration

The image working directory is `/agent/workspace`.

The image entrypoint is `/agent/entrypoint`.

Each entry in `environment.defaults` becomes an image environment default with the map key as its variable name.

## Labels

An agent image records:

| Label | Value |
| --- | --- |
| `build.agentfile.metadata` | JSON representation of `agent` from the bundle manifest |
| `build.agentfile.runtimeEnv` | JSON array containing the distinct source names in `environment.mappings` and `assets.configEnv` |
| `build.agentfile.harness` | `harness` from the bundle manifest |
| `build.agentfile.bundle.digest` | SHA-256 digest of the complete source bundle archive, prefixed with `sha256:` |

## Sensitive information

Agent image contents, configuration, labels, and build logs are not confidential.

Literal environment-variable values and any other sensitive information included in the source bundle or base image are visible in the resulting image.

Environment-variable values supplied at run time must not be written into image layers, image configuration, labels, or build logs.
