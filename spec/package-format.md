# DHP Package Format

A **DHP** digital human package is always a **bundle** — a directory containing a `spec.yaml` and any
optional supporting files.

## Bundle Structure

```
packages/digital-humans/<slug>/
  spec.yaml               # required — the agent definition
  README.md               # optional — usage notes for this agent
  assets/                 # optional — icons, images, static resources
  skills/                 # optional — bundled skill directories
    <skill-id>/
      SKILL.md            # skill instruction file
      index.js            # skill script (optional)
      lib/                # nested directories supported
  mcps/                   # optional — bundled MCP server definitions
    <mcp-name>/
      spec.yaml
      <entrypoint>
```

The install entrypoint is always `spec.yaml`. All other files are optional.

## Required Fields in spec.yaml

Every spec must include:

| Field | Type | Notes |
|---|---|---|
| `name` | string | Non-empty display name |
| `version` | string | e.g. `"1.0.0"` |
| `author` | string | Author or organization |
| `description` | string | One sentence |
| `type` | string | `"automation"`, `"skill"`, `"mcp"`, or `"extension"` |

Type-specific requirements:

| Type | Also requires |
|---|---|
| `automation` | `system_prompt`, at least one `subscriptions` entry |
| `skill` | `system_prompt` |
| `mcp` | `mcp_server` |

## Recommended store Fields

When publishing to a registry, include a `store` block:

```yaml
store:
  slug: "my-agent"           # must match the bundle directory name
  category: productivity     # see spec/categories.md for allowed values
  tags: ["tag1", "tag2"]
  locale: en-US
  min_app_version: "0.5.0"
  license: MIT
  repository: "https://github.com/..."
```

The `slug` field must match the directory name under `packages/digital-humans/`.

## File Format

`spec.yaml` must be valid YAML. JSON is also accepted (the parser handles both), but YAML is
preferred for readability, especially for multi-line `system_prompt` values.

## Bundled Skills and MCPs

Skills and MCPs can be bundled inside the package instead of requiring separate installation.
Set `bundled: true` in the corresponding `requires` entry:

```yaml
requires:
  mcps:
    - id: my-database-connector
      bundled: true
      reason: "Custom connector — no separate installation needed"
  skills:
    - id: my-analysis-skill
      bundled: true
```

Bundled MCPs must have their own `spec.yaml` under `mcps/<id>/`.
Bundled skills must be directories under `skills/<id>/` containing the skill files (e.g. `SKILL.md`, `index.js`).
The `files` field in the `requires.skills` entry must list all files to be downloaded:

```yaml
requires:
  skills:
    - id: my-analysis-skill
      bundled: true
      files:
        - SKILL.md
        - index.js
```

## Full Field Reference

See [`spec/app-spec.md`](app-spec.md) for the complete list of all supported fields with types,
constraints, and examples.
