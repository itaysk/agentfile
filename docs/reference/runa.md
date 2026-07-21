# Bundle Runtime

This document defines the normative behavior of Agentfile's host runtime for agent bundles (a.k.a runa).

CLI selection and temporary bundle creation are outside this contract.

## Terminology

The bundle runtime invokes an agent bundle archive with a host-installed harness.

It accepts an agent bundle and a [harness invocation](harness.md#terminology-and-scope).

## Invocation

The bundle runtime supports one-shot, TUI, and ACP modes.

For each one-shot or TUI invocation, the bundle runtime:

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

The bundle runtime does not read or change user-level harness configuration, skills, plugins, hooks, or MCP registrations.

Only environment-based authentication is guaranteed.

For an ACP run, the runtime extracts the bundle and assembles the inherited environment once for the bridge lifetime. Each `session/new` request:

1. requires an existing absolute workspace directory supplied by the client;
2. creates a private harness profile for that session;
3. prepares the harness in ACP mode with the session workspace; and
4. starts a separate host harness process connected to the ACP bridge.

Closing a session stops its harness process. Closing the ACP connection stops every remaining session and removes the extracted bundle and private profiles on a best-effort basis. Standard output is reserved for protocol messages; warnings and harness diagnostics use standard error.

## Security

Every bundle run writes:

```text
af: warning: bundle execution uses the current user without isolation or approval gates
```

The bundle runtime has no isolation boundary.

The harness runs as the current user with that user's access to files, credentials, processes, installed tools, and network resources.

Agentfile's non-interactive permission-bypass arguments remain enabled.

The bundle runtime does not set Claude Code's `IS_SANDBOX=1`.

Use the bundle runtime only with trusted bundles and workspaces.
