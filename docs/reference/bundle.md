# Agent Bundle Format

This document defines the normative format for Agentfile bundle version `agentfile.build/bundle/v1`.

An agent bundle is a deterministic `.tar.gz` archive whose contents use portable, bundle-relative paths.

It contains a bundle manifest and bundle assets. It does not provision a harness executable, operating-system packages, or environment values supplied at run time.

Bundle format versions are independent of agentfile API versions.

## Terminology

An **agent bundle** is the complete archive.

An **unpacked bundle** is a directory containing the archive contents.

The **bundle manifest** is the required `manifest.json` file at the archive root. It is the bundle's complete agent definition and identifies the content needed at run time.

A **bundle asset** is a file or directory stored in the agent bundle for use at run time. The bundle manifest is not a bundle asset.

A **bundle-relative path** identifies content from the archive root. It is never an absolute filesystem path.

## Archive layout

The archive root may contain:

```text
manifest.json
prompt.md
system-prompt.md
skills/<skill-name>/...
harness/<harness>/...
```

The bundle manifest is required.

Optional bundle assets are omitted when they are not declared or generated.

Harness configuration templates are relocatable bundle assets.

Agentfile-generated filesystem paths in those templates must not be absolute. Runtime paths are represented by reserved placeholders.

## Bundle manifest

The bundle manifest is a compiled description of the bundle and its runtime requirements. It is not an agentfile and does not preserve source declarations.

For example:

```json
{
  "bundleVersion": "agentfile.build/bundle/v1",
  "agent": {
    "name": "reviewer",
    "version": "1.0"
  },
  "harness": "codex",
  "model": {
    "provider": "openai",
    "name": "gpt-5"
  },
  "assets": {
    "prompt": "prompt.md",
    "systemPrompt": "system-prompt.md",
    "skills": [
      "skills/demo"
    ],
    "configTemplate": "harness/codex/config.toml.tmpl",
    "configEnv": [
      "MCP_TOKEN"
    ]
  },
  "environment": {
    "defaults": {
      "LOG_LEVEL": "info"
    },
    "mappings": {
      "GH_TOKEN": "GITHUB_TOKEN"
    }
  }
}
```

`bundleVersion` is required and must be `agentfile.build/bundle/v1`.

`agent.name` and `agent.version` identify the built agent. Defaults have already been applied to these values.

`harness` is required and selects exactly one runtime: `claudecode`, `codex`, or `pi`. A bundle targets one harness in the same way that a container image targets one platform.

`bare` is an optional boolean. It is valid only with the `claudecode` harness and enables Claude Code's bare mode. Omission means `false`.

`model.provider` and `model.name` are required. The provider is `anthropic`, `openai`, or `openrouter`.

`assets` identifies materialized bundle content:

- `prompt` is the optional one-shot prompt file;
- `systemPrompt` is the optional system-prompt file;
- `skills` is the optional list of skill directories;
- `configTemplate` is the optional generated harness configuration template; and
- `configEnv` is the optional sorted list of distinct run-time environment-variable names referenced by that template.

The `prompt`, `systemPrompt`, and `configTemplate` values and every `skills` item are canonical bundle-relative paths. Every skill path is exactly `skills/<skill-name>`, and skill names are unique. `configEnv` is omitted when no configuration template needs environment substitution.

`environment.defaults` optionally maps a target environment-variable name to its literal default value.

`environment.mappings` optionally maps a target environment-variable name to a run-time source environment-variable name. A target cannot appear in both maps. Every mapping source and every name in `assets.configEnv` is required at run time; an empty value is valid.

Every environment-variable name must match `[A-Za-z_][A-Za-z0-9_]*` and must not start with the reserved `AGENTFILE_` prefix.

The manifest does not contain `apiVersion`, `kind`, `spec`, source unions, or MCP declarations. Sources have already been materialized, defaults have already been applied, and MCP declarations have already been compiled into the harness configuration template.

A bundle reader does not perform discovery, filesystem resolution, Git fetches, or HTTP requests.

## Reproducibility

Archive entries are sorted.

Archive headers use user ID `0`, group ID `0`, and empty owner and group names. Modification, access, and change times are set to the Unix epoch.

File modes are assigned by content type rather than copied verbatim. Directories use `0755`, regular files use `0644`, and files that were executable at resolution time use `0755`.

The gzip timestamp is zero.

Identical inputs produce identical archive bytes and SHA-256 digests.

## Safety

Every archive path must be non-empty, bundle-relative, use `/` separators, and already be in canonical form.

Canonical form has no `.` or `..` segments, repeated separators, backslashes, or filesystem volume prefix. One trailing `/` is ignored when validating a directory entry.

Readers reject duplicate paths, symlinks, hard links, devices, sockets, and every entry type other than a regular file or directory.

An agent bundle is not a security boundary.

Bundle assets can cause code execution, so run a bundle only when its contents and origin are trusted.

## Sensitive information

Agent bundle contents are not confidential.

Values in `environment.defaults` and any other sensitive information included in source content are visible in the resulting bundle.

For environment variables supplied at run time, `environment.mappings` and `assets.configEnv` record source names only.

Their values must not be written into the agent bundle.
