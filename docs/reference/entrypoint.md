# Agent Image Entrypoint

This document defines the normative container-start behavior of an [agent image](image.md).

It does not define image construction or CLI orchestration.

## Terminology

The **image entrypoint** is `/agent/entrypoint` in an agent image.

The **unpacked bundle** is the bundle content stored at `/agent/bundle`.

The **harness profile** is the private invocation state written under `/agent/profile`.

The **workspace** is `/agent/workspace`. A container runtime may bind-mount another directory at this path.

## Invocation

At container start, the image entrypoint:

1. validates the selected execution mode;
2. validates that every required environment variable is present;
3. prepares the harness profile under `/agent/profile`;
4. renders harness configuration with the unpacked bundle, workspace, and environment;
5. applies declared environment defaults and mappings;
6. prepares the [harness command](harness.md#terminology-and-scope); and
7. replaces itself with the harness process from `/agent/workspace`.

The entrypoint reads `/agent/bundle` in place and does not modify it.

The entrypoint supports one-shot, TUI, and ACP modes.

## Entrypoint environment

`AGENTFILE_RUN_MODE` selects `oneshot`, `tui`, or `acp`. It defaults to `oneshot`.

In one-shot mode, `AGENTFILE_PROMPT` overrides `assets.prompt`. When it is absent, the entrypoint reads that bundle asset.

In TUI and ACP modes, the entrypoint removes `AGENTFILE_PROMPT` and starts the harness without an initial user message.

`AGENTFILE_MODEL` overrides `model.name`. When it is absent or empty, the entrypoint uses that value from the bundle manifest.

`model.provider` cannot be overridden.

Every source named by `environment.mappings` or `assets.configEnv` must be present, although an empty value is valid.

An `environment.defaults` entry is applied only when its target variable is absent.

An `environment.mappings` entry copies its source variable into its target variable only when the target is absent.

An environment value substituted into a JSON or TOML harness template must not contain a carriage return or newline.

## Isolation

The entrypoint uses the container as its isolation boundary.

It sets `IS_SANDBOX=1` for Claude Code and applies the permission-bypass arguments defined by the [harness adapter reference](harness.md#permission-flags).

The entrypoint itself does not create or strengthen the container isolation boundary.
