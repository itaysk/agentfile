# runa

This document defines the normative behavior of `runa`, Agentfile's host runner for agent bundles.

CLI selection and temporary bundle creation are outside this contract.

## Terminology

`runa` invokes an agent bundle archive with a host-installed harness.

`runa` accepts an agent bundle and a [harness invocation](harness.md#terminology-and-scope).

## Invocation

`runa` supports one-shot and TUI modes.

For each invocation, `runa`:

1. safely extracts the agent bundle into a private temporary unpacked bundle;
2. uses the selected workspace or creates an empty temporary workspace;
3. assembles the invocation environment;
4. asks the harness adapter to prepare a private profile and harness command;
5. locates the harness executable on the host `PATH`;
6. starts the harness as the current user;
7. forwards standard streams and signals;
8. preserves the harness exit code; and
9. removes its temporary files on a best-effort basis.

The invocation environment begins with the complete parent environment.

Environment files are applied in order, followed by explicit environment values.

The selected harness and every declared MCP command or tool must already be installed on the host.

`runa` does not read or change user-level harness configuration, skills, plugins, hooks, or MCP registrations.

Only environment-based authentication is guaranteed.

ACP is not supported.

## Security

Every invocation writes:

```text
af: warning: runa runs the agent as the current user without isolation or approval gates
```

`runa` has no isolation boundary.

The harness runs as the current user with that user's access to files, credentials, processes, installed tools, and network resources.

Agentfile's non-interactive permission-bypass arguments remain enabled.

`runa` does not set Claude Code's `IS_SANDBOX=1`.

Use `runa` only with trusted bundles and workspaces.
