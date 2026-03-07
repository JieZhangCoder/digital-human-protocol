# DHP App Spec Reference

Every digital human is defined by a single `spec.yaml` file that conforms to this specification.
The spec declares what an agent does, when it runs, what it needs, and how it behaves — the runtime
handles execution.

This document is the normative reference for all fields in the **Digital Human Protocol (DHP)**.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Top-level Fields](#2-top-level-fields)
3. [subscriptions — Trigger Sources](#3-subscriptions--trigger-sources)
4. [config_schema — User Configuration](#4-config_schema--user-configuration)
5. [requires — Dependencies](#5-requires--dependencies)
6. [filters — Event Filtering](#6-filters--event-filtering)
7. [memory_schema — Persistent Memory](#7-memory_schema--persistent-memory)
8. [output — Notifications](#8-output--notifications)
9. [escalation — Human-in-the-Loop](#9-escalation--human-in-the-loop)
10. [mcp_server — MCP Server Definition](#10-mcp_server--mcp-server-definition)
11. [store — Registry Metadata](#11-store--registry-metadata)
12. [i18n — Localization Overrides](#12-i18n--localization-overrides)
13. [permissions — Runtime Permissions](#13-permissions--runtime-permissions)
14. [Type Constraints](#14-type-constraints)
15. [Backward Compatibility](#15-backward-compatibility)
16. [browser_login — Login Guidance](#16-browser_login--login-guidance)

---

## 1. Overview

### App Types

| `type` | Description | Requires `system_prompt` | Requires `mcp_server` | Allows `subscriptions` |
|---|---|---|---|---|
| `automation` | Autonomous agent triggered by events or schedule | Yes | No | Yes (at least 1) |
| `skill` | Capability invoked on demand by the user | Yes | No | No |
| `mcp` | Wraps an external MCP server | No | Yes | No |
| `extension` | UI extension or theme | No | No | No |

### Minimal Valid Spec

```yaml
spec_version: "1"
name: "My Agent"
version: "1.0.0"
author: "alice"
description: "Does something useful every hour."
type: automation
system_prompt: |
  You are a helpful agent. On each run, do X, then call
  report_to_user(type="run_complete") with a summary.
subscriptions:
  - source:
      type: schedule
      config:
        every: "1h"
```

---

## 2. Top-level Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `spec_version` | `string` | No | Protocol version. Defaults to `"1"`. Increment on breaking changes. |
| `name` | `string` | **Yes** | Display name. Must be non-empty. Duplicate names within a space are rejected. |
| `version` | `string` | **Yes** | App version. Loosely validated — `"1.0"`, `"1.0.0"`, `"0.1-beta"` are all valid. |
| `author` | `string` | **Yes** | Author name or organization handle. |
| `description` | `string` | **Yes** | One sentence describing what this agent does. |
| `type` | `"automation" \| "skill" \| "mcp" \| "extension"` | **Yes** | App type. Determines which fields are required and how the agent runs. |
| `icon` | `string` | No | Icon identifier (e.g. `"news"`, `"shopping"`) or image URL. |
| `system_prompt` | `string` | **Yes** for `automation` and `skill` | The agent's core instruction set. Injected as the system prompt on every run. This is the primary place to define agent behavior — be specific and comprehensive. |
| `subscriptions` | `SubscriptionDef[]` | Required for `automation` | Trigger sources. At least one required for automations. See [Section 3](#3-subscriptions--trigger-sources). |
| `config_schema` | `InputDef[]` | No | User-configurable fields shown at install time. See [Section 4](#4-config_schema--user-configuration). |
| `requires` | `Requires` | No | External MCP servers and skills the agent depends on. See [Section 5](#5-requires--dependencies). |
| `filters` | `FilterRule[]` | No | Pre-run event filter rules. Events not matching all rules are dropped. See [Section 6](#6-filters--event-filtering). |
| `memory_schema` | `Record<string, MemoryField>` | No | Declares the structure of the agent's persistent memory. `automation` only. See [Section 7](#7-memory_schema--persistent-memory). |
| `output` | `OutputConfig` | No | Post-run notification configuration. See [Section 8](#8-output--notifications). |
| `escalation` | `EscalationConfig` | No | Controls human escalation behavior. See [Section 9](#9-escalation--human-in-the-loop). |
| `mcp_server` | `McpServerConfig` | **Yes** for `mcp` | How to start the MCP server process. See [Section 10](#10-mcp_server--mcp-server-definition). |
| `permissions` | `string[]` | No | Runtime permission declarations. See [Section 13](#13-permissions--runtime-permissions). |
| `recommended_model` | `string` | No | Suggested model for this agent. Informational only — not enforced at runtime. |
| `store` | `StoreMetadata` | No | Registry and distribution metadata. See [Section 11](#11-store--registry-metadata). |
| `i18n` | `Record<string, I18nLocaleBlock>` | No | Locale-specific display text overrides for `name`, `description`, and `config_schema` labels. See [Section 12](#12-i18n--localization-overrides). |
| `browser_login` | `BrowserLoginEntry[]` | No | Websites the user should log into before the automation runs. `automation` only. See [Section 16](#16-browser_login--login-guidance). |

---

## 3. `subscriptions` — Trigger Sources

Subscriptions define when an automation runs. Each entry is a `SubscriptionDef`.

**Only `type: automation` may have subscriptions.** Other types will fail validation.

### SubscriptionDef

```yaml
subscriptions:
  - id: "my-trigger"           # optional — auto-generated if omitted
    source:                    # required
      type: schedule
      config:
        every: "1h"
    frequency:                 # optional — lets users adjust the interval
      default: "1h"
      min: "15m"
      max: "24h"
    config_key: "target_url"   # optional — passes a user config value to this trigger
```

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | `string` | No | Identifier unique within this spec. Auto-generated (`sub-0`, `sub-1`, …) if omitted. Used by user overrides to customize the frequency per subscription. |
| `source` | `SubscriptionSource` | **Yes** | The trigger source. Discriminated union on `source.type`. |
| `frequency` | `FrequencyDef` | No | Exposes a frequency control to the user. When set, the runtime UI shows a slider bounded by `min` and `max`. |
| `config_key` | `string` | No | References a key in `config_schema`. The user-supplied value for that key is passed as dynamic input to the trigger (e.g. the URL to watch). Must match an existing `config_schema` key. |

### Source Types

#### `schedule` — Time-based

```yaml
source:
  type: schedule
  config:
    every: "30m"        # interval — mutually exclusive with cron
    cron: "0 9 * * 1-5" # cron expression — mutually exclusive with every
```

At least one of `every` or `cron` must be present.

| Field | Type | Description |
|---|---|---|
| `every` | duration string | Fixed interval. Format: integer followed by `s`, `m`, `h`, or `d`. Examples: `"30m"`, `"2h"`, `"1d"`. |
| `cron` | string | Standard 5-field cron expression. Examples: `"0 8 * * *"` (daily 08:00), `"*/15 * * * *"` (every 15 min), `"0 9 * * 1-5"` (weekdays 09:00). |

---

#### `file` — File System Change

```yaml
source:
  type: file
  config:
    pattern: "src/**/*.ts"     # glob pattern matched against relative path
    path: "/home/user/project" # directory to watch (absolute path)
```

Both fields are optional. When both are omitted, any file change triggers the agent.

| Field | Type | Description |
|---|---|---|
| `pattern` | string | Glob pattern. Matched against the relative path of the changed file. |
| `path` | string | Absolute directory path. Triggers when the changed file path contains this string. |

Trigger events include created, modified, and deleted files.

---

#### `webhook` — HTTP Webhook

```yaml
source:
  type: webhook
  config:
    path: "github-events"     # mounts at /hooks/github-events
    secret: "my-secret-key"   # HMAC-SHA256 signature verification
```

| Field | Type | Description |
|---|---|---|
| `path` | string | URL path segment. The webhook is registered at `/hooks/{path}`. Triggers on HTTP POST to that path. Accepts all paths when omitted. |
| `secret` | string | When provided, incoming requests must include a valid `x-hub-signature-256` or `x-webhook-signature` header. Requests failing verification receive a 401 response. |

---

#### `webpage` — Web Page Change

```yaml
source:
  type: webpage
  config:
    watch: "price-element"  # natural language description of what to monitor
    selector: ".price"      # CSS selector to scope monitoring
    url: "https://..."      # target URL — prefer config_key over hardcoding
```

| Field | Type | Description |
|---|---|---|
| `watch` | string | Description of what to monitor. |
| `selector` | string | CSS selector to focus on a specific page region. |
| `url` | string | Target page URL. |

> **Status:** Schema-complete. Event producer support coming in a future release. Use `schedule` with AI Browser web automation as an equivalent pattern in the meantime.

---

#### `rss` — RSS Feed

```yaml
source:
  type: rss
  config:
    url: "https://news.ycombinator.com/rss"
```

| Field | Type | Description |
|---|---|---|
| `url` | string | RSS feed URL. |

> **Status:** Schema-complete. Event producer support coming in a future release. Use `schedule` with AI Browser to poll the feed as an equivalent pattern.

---

#### `custom` — Custom Event Source

```yaml
source:
  type: custom
  config:
    provider: "my-provider"
    key: "any-value"
```

`config` accepts any `Record<string, unknown>`. No field-level validation is applied — interpretation is left to the custom event source.

---

### `frequency` — User-adjustable Interval

```yaml
frequency:
  default: "1h"   # required
  min: "15m"      # optional
  max: "24h"      # optional
```

| Field | Type | Required | Description |
|---|---|---|---|
| `default` | duration string | **Yes** | Default execution interval. |
| `min` | duration string | No | Minimum interval users can set. |
| `max` | duration string | No | Maximum interval users can set. |

All values are duration strings (see [Section 14](#14-type-constraints)).

---

## 4. `config_schema` — User Configuration

Defines the form shown to users when installing the agent. Values are stored as `userConfig` and injected into the agent's context on every run.

```yaml
config_schema:
  - key: repo_url
    label: "Repository URL"
    type: url
    required: true
    placeholder: "https://github.com/owner/repo"
    description: "The GitHub repository to monitor"

  - key: threshold
    label: "Alert threshold (%)"
    type: number
    required: true
    default: 5

  - key: severity_filter
    label: "Minimum severity"
    type: select
    options:
      - label: "All"
        value: all
      - label: "High and critical only"
        value: high
```

| Field | Type | Required | Description |
|---|---|---|---|
| `key` | `string` | **Yes** | Unique identifier. The value is accessed as `userConfig[key]` at runtime. Can be referenced by `subscriptions[].config_key`. |
| `label` | `string` | **Yes** | Field label shown in the installation UI. |
| `type` | `InputType` | **Yes** | Input type — determines the UI control and basic validation. |
| `description` | `string` | No | Help text shown below the field. |
| `required` | `boolean` | No | Default `false`. When `true`, the user must supply a value to complete installation. |
| `default` | any | No | Pre-filled default value. |
| `placeholder` | `string` | No | Input placeholder text. |
| `options` | `SelectOption[]` | **Yes** when `type: select` | Dropdown options. Ignored for other types. |

### InputType Values

| Value | Control | Validation |
|---|---|---|
| `string` | Single-line text | None |
| `text` | Multi-line textarea | None |
| `number` | Number input | Numeric |
| `boolean` | Toggle / checkbox | Boolean |
| `url` | URL input | Basic URL format |
| `email` | Email input | Basic email format |
| `select` | Dropdown | Must match an option value |

### SelectOption

```yaml
options:
  - label: "Human-readable label"   # displayed in the UI
    value: machine-value            # string | number | boolean — stored in userConfig
```

---

## 5. `requires` — Dependencies

Declares external MCP servers and skills that this agent needs. The runtime uses this to inject the right tools and capabilities before each run.

```yaml
requires:
  mcps:
    - id: ai-browser
      reason: "Browse product pages and extract pricing data"
    - id: postgres-mcp
      reason: "Store and query historical price records"
  skills:
    - summarizer                     # shorthand form (string)
    - id: price-analysis             # object form
      reason: "Interpret pricing trends"
```

### MCP Dependencies

Each entry is a `McpDependency`:

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | `string` | **Yes** | MCP server identifier. |
| `reason` | `string` | No | Human-readable explanation shown to the user at install time. |
| `bundled` | `boolean` | No | Whether this MCP is bundled within the package rather than installed separately. |

**`ai-browser`** is a built-in MCP available in compatible runtimes. Declaring it ensures the agent has web browsing and automation capabilities.

### Skill Dependencies

Each entry is a `SkillDependency`, in either shorthand or object form:

```yaml
skills:
  - summarizer              # shorthand: just the skill id
  - id: price-analysis      # object form
    reason: "..."
    bundled: true
```

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | `string` | **Yes** (object form) | Skill identifier. |
| `reason` | `string` | No | Why this skill is needed. |
| `bundled` | `boolean` | No | Whether the skill files are co-located inside the package's `skills/{id}/` directory. When `true`, the runtime fetches files directly from the package instead of querying the store. |
| `files` | `string[]` | No (required when `bundled: true`) | Relative file paths within the `skills/{id}/` directory. Supports nested paths (e.g. `lib/utils.js`). Used by the runtime to download the skill files at install time. |

---

## 6. `filters` — Event Filtering

Filter rules are evaluated before the agent runs. If any rule fails, the event is discarded and no AI run is started — with zero token cost.

```yaml
filters:
  - field: payload.action
    op: eq
    value: "opened"
  - field: payload.pull_request.additions
    op: gt
    value: 10
```

All rules are combined with **AND logic** — all must pass for the event to trigger a run.

| Field | Type | Required | Description |
|---|---|---|---|
| `field` | `string` | **Yes** | Dot-notation path into the event object. Array indexing supported: `"payload.items[0].name"`. |
| `op` | `FilterOp` | **Yes** | Comparison operator. |
| `value` | any | **Yes** | Value to compare against. Type depends on `op`. |

### FilterOp Values

| Operator | Meaning | `value` type |
|---|---|---|
| `eq` | Strict equality (`===`) | any |
| `neq` | Strict inequality (`!==`) | any |
| `contains` | String includes substring, or array includes element | string or any |
| `matches` | String matches regular expression | string (regex pattern) |
| `gt` | Greater than | number |
| `lt` | Less than | number |
| `gte` | Greater than or equal | number |
| `lte` | Less than or equal | number |

---

## 7. `memory_schema` — Persistent Memory

Declares the structure of data the agent should persist across runs. The runtime maintains a
`memory.md` file per agent; the agent reads it at the start of each run and updates it at the end.

**Only `type: automation` may declare `memory_schema`.**

```yaml
memory_schema:
  watched_repos:
    type: array
    description: "List of repositories currently being monitored"
  last_run_summary:
    type: string
    description: "Summary of findings from the most recent run"
  open_issues:
    type: object
    description: "Map of issue ID to triage status"
```

`memory_schema` is a `Record<string, MemoryField>`:

| Field | Type | Required | Description |
|---|---|---|---|
| `type` | `string` | **Yes** | Describes the data type: `"string"`, `"number"`, `"boolean"`, `"array"`, `"object"`, `"date"`. Descriptive — not enforced at runtime. |
| `description` | `string` | No | Tells the agent what this field is for and how to use it. |

The memory file path and format are runtime-defined. The schema guides the agent; it does not enforce structure.

---

## 8. `output` — Notifications

Controls what happens after a run completes successfully.

```yaml
output:
  notify:
    system: true              # desktop notification
    channels:
      - email
      - wecom
  format: "Found {count} new issues in {repo}"
```

### `output.notify`

| Field | Type | Description |
|---|---|---|
| `system` | `boolean` | Send a system desktop notification. Enabled by default when `notify` is present; set to `false` to suppress. |
| `channels` | `NotificationChannelType[]` | External channels to notify. Credentials are configured by the user in runtime settings — not stored in the spec. |

### Notification Channels

| Value | Channel |
|---|---|
| `email` | Email |
| `wecom` | WeCom (Enterprise WeChat) |
| `dingtalk` | DingTalk |
| `feishu` | Feishu / Lark |
| `webhook` | Generic HTTP webhook |

Notifications are only sent when the run completes without error. The notification content is the
summary from the agent's final `report_to_user(type="run_complete")` call.

### `output.format`

A template string describing the expected output shape (e.g. `"Found {count} issues in {repo}"`).
Currently informational — displayed in the runtime UI. No interpolation is performed.

---

## 9. `escalation` — Human-in-the-Loop

Controls how the agent handles situations that require a human decision.

```yaml
escalation:
  enabled: true       # default true
  timeout_hours: 48   # default 24
```

When escalation is enabled, the agent may call `report_to_user(type="escalation")` to pause
execution and present a question to the user. The run resumes when the user responds.

| Field | Type | Description |
|---|---|---|
| `enabled` | `boolean` | Default `true`. Set to `false` to require the agent to make all decisions autonomously. |
| `timeout_hours` | `number` | Hours before an unanswered escalation automatically closes as an error. Default `24`. |

**Status flow:**

```
agent calls report_to_user(type="escalation")
  → status: waiting_user
    → user responds → run continues
    → timeout expires → status: error
```

---

## 10. `mcp_server` — MCP Server Definition

Defines how to start an MCP server process. Compatible with the MCP server configuration format used by Claude and other MCP clients.

**Only `type: mcp` may declare `mcp_server`.** Required when `type: mcp`.

```yaml
type: mcp
mcp_server:
  command: npx
  args:
    - "-y"
    - "@modelcontextprotocol/server-postgres"
  env:
    DATABASE_URL: "{{config.database_url}}"
  cwd: "/optional/working/dir"
```

| Field | Type | Required | Description |
|---|---|---|---|
| `command` | `string` | **Yes** | The executable to run, e.g. `"npx"`, `"python"`, `"node"`. |
| `args` | `string[]` | No | Command-line arguments. |
| `env` | `Record<string, string>` | No | Environment variables. Use `{{config.key}}` to reference a user config value — the runtime substitutes these at startup. |
| `cwd` | `string` | No | Working directory for the process (absolute path). |

---

## 11. `store` — Registry Metadata

Metadata used when publishing to a digital human registry. Not required for local use.

```yaml
store:
  slug: "github-pr-reviewer"
  category: dev-tools
  tags: ["github", "pr", "code-review", "automation"]
  locale: en-US
  min_app_version: "0.5.0"
  license: MIT
  homepage: "https://github.com/example/my-agent"
  repository: "https://github.com/example/my-agent"
```

| Field | Type | Required | Description |
|---|---|---|---|
| `slug` | `string` | No | URL-safe unique identifier. Must match the bundle directory name. See [slug format](#store-slug). |
| `category` | `string` | No | One of the allowed category values. See [`spec/categories.md`](categories.md). |
| `tags` | `string[]` | No | Free-form discovery tags. Defaults to `[]`. |
| `locale` | `string` | No | Primary language as a BCP 47 tag, e.g. `"en-US"`, `"zh-CN"`, `"ja-JP"`. |
| `min_app_version` | `string` | No | Minimum runtime version required to install this agent. |
| `license` | `string` | No | SPDX license identifier, e.g. `"MIT"`, `"Apache-2.0"`, `"CC0-1.0"`. |
| `homepage` | `string` | No | Product homepage URL. |
| `repository` | `string` | No | Source code repository URL. |
| `registry_id` | `string` | No | Set automatically by the registry at install time. Do not set this manually. |

---

## 12. `i18n` — Localization Overrides

Provides locale-specific overrides for the display text shown in store listings and install
forms. The canonical (top-level) `name`, `description`, and `config_schema` labels are the
authoritative English source. The `i18n` block lets authors supply translations without
duplicating the entire spec.

**Only display text is translated.** `system_prompt`, `store` metadata, subscription config,
and runtime behavior are never overridden by `i18n`.

```yaml
i18n:
  zh-CN:
    name: 京东价格猎手
    description: 监控京东商品页面的价格变动，在价格达到目标值或出现新低时发送通知，并避免重复提醒。
    config_schema:
      product_url:
        label: 商品链接
        description: 京东商品详情页链接
        placeholder: "https://item.jd.com/..."
      target_price:
        label: 目标价格
        description: 价格降至此值或以下时触发通知
      notify_on_every_drop:
        label: 每次降价均通知
        description: 开启后，每次出现新低价都会通知，而不只是达到目标价格时
      output_language:
        label: 输出语言
        options:
          en-US: 英文
          zh-CN: 中文
```

### `I18nLocaleBlock` — Per-locale Override Object

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | `string` | No | Translated display name. Falls back to canonical `name` when absent. |
| `description` | `string` | No | Translated description. Falls back to canonical `description` when absent. |
| `config_schema` | `Record<string, I18nConfigFieldOverride>` | No | Keyed by `config_schema[].key`. Each entry overrides display fields for that config field. |

### `I18nConfigFieldOverride` — Per-field Override Object

| Field | Type | Required | Description |
|---|---|---|---|
| `label` | `string` | No | Translated field label. Falls back to canonical `label`. |
| `description` | `string` | No | Translated help text. Falls back to canonical `description`. |
| `placeholder` | `string` | No | Translated placeholder text. Falls back to canonical `placeholder`. |
| `options` | `Record<string, string>` | No | Map of option `value` → translated label. Only values explicitly listed are overridden; others fall back to canonical labels. |

### Runtime Locale Resolution

Runtimes resolve display text using the following priority order:

1. **Exact locale match** — e.g. `i18n["zh-CN"]` for a user with locale `zh-CN`
2. **Language prefix match** — e.g. `i18n["zh-CN"]` also matches locale `zh-TW` if no
   `zh-TW` block exists and a `zh-*` block is found
3. **Canonical fallback** — the top-level `name` / `description` / `config_schema` labels
   (always English)

Runtimes that support AI-assisted translation may auto-translate the canonical text when no
`i18n` block exists for the user's locale, then cache the result.

### Registry Index (`i18n` in `RegistryEntry`)

The registry build script extracts a lightweight summary into `index.json` so the store UI
can display translated names and descriptions without fetching the full spec:

```json
{
  "slug": "jd-price-hunter",
  "name": "JD Price Hunter",
  "description": "Monitors a JD product page...",
  "i18n": {
    "zh-CN": {
      "name": "京东价格猎手",
      "description": "监控京东商品页面的价格变动..."
    }
  }
}
```

Only `name` and `description` are included in the index `i18n` summary. Full
`config_schema` overrides are only available after fetching the complete spec.

---

## 13. `permissions` — Runtime Permissions

Declares runtime capabilities the agent requests. Users may grant or revoke permissions after installation.

```yaml
permissions:
  - ai-browser
```

`permissions` is a string array of permission identifiers.

| Permission | Description | Default |
|---|---|---|
| `ai-browser` | Web browsing and page automation capability | Enabled for all `automation` apps |

**Permission resolution order** (first match wins):

1. Explicitly denied by the user → **denied**
2. Explicitly granted by the user → **allowed**
3. Declared in `spec.permissions` → **allowed**
4. Not declared → runtime default applies

---

## 14. Type Constraints

### Duration String

Used in `subscriptions[].source.config.every` and all `frequency.*` fields.

**Format:** one or more digits followed by a unit suffix.

```
^\d+[smhd]$
```

| Suffix | Unit |
|---|---|
| `s` | seconds |
| `m` | minutes |
| `h` | hours |
| `d` | days |

Valid: `"30s"`, `"5m"`, `"1h"`, `"12h"`, `"7d"`
Invalid: `"1hour"`, `"30 min"`, `"1.5h"`, `"1w"`

### Cron Expression

Must be at least 5 characters. Standard 5-field format:

```
┌─ minute (0–59)
│ ┌─ hour (0–23)
│ │ ┌─ day of month (1–31)
│ │ │ ┌─ month (1–12)
│ │ │ │ ┌─ day of week (0–7, 0 and 7 = Sunday)
│ │ │ │ │
* * * * *
```

Examples:
- `"0 8 * * *"` — daily at 08:00
- `"*/30 * * * *"` — every 30 minutes
- `"0 9 * * 1-5"` — weekdays at 09:00
- `"0 0 1 * *"` — first day of every month

### Store Slug

```
^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$
```

- Lowercase letters, digits, and internal hyphens only
- Cannot start or end with a hyphen
- Valid: `"hn-daily"`, `"github-pr-reviewer"`, `"postgres-mcp"`
- Invalid: `"-bad"`, `"bad-"`, `"MyAgent"`, `"my_agent"`

---

## 15. Backward Compatibility

The parser automatically normalizes legacy field names. New specs should use canonical names.

| Legacy field | Canonical field | Notes |
|---|---|---|
| `inputs` | `config_schema` | Top-level alias |
| `required_mcps` | `requires.mcps` | Accepts string array or object array |
| `required_skills` | `requires.skills` | Top-level alias |
| `requires.mcp` | `requires.mcps` | Singular → plural inside `requires` |
| `requires.skill` | `requires.skills` | Singular → plural inside `requires` |
| `subscriptions[].input` | `subscriptions[].config_key` | Per-entry alias |

### Subscription Shorthand

`type` and `config` may be written directly at the entry level instead of nested under `source`:

```yaml
# Shorthand (normalized automatically)
subscriptions:
  - type: schedule
    config:
      every: "1h"

# Canonical form
subscriptions:
  - source:
      type: schedule
      config:
        every: "1h"
```

Both forms are accepted and produce identical results.

---

## 16. `browser_login` — Login Guidance

Declares websites the user needs to log into before the automation can run. This is a declarative hint for the runtime UI — it does not affect execution logic or permissions.

**Only `type: automation` may declare `browser_login`.**

```yaml
browser_login:
  - url: "https://www.xiaohongshu.com"
    label: "小红书"
  - url: "https://creator.xiaohongshu.com"
    label: "小红书创作者中心"
```

### `BrowserLoginEntry`

| Field | Type | Required | Description |
|---|---|---|---|
| `url` | `string` | **Yes** | URL the user should navigate to for login. Must be a valid URL. |
| `label` | `string` | **Yes** | Website display name shown in the login guidance UI. |

### Semantics

- Purely declarative — the spec author states "this automation needs the user to be logged into these websites"
- The runtime uses this field to render login guidance UI (e.g. a notice bar, browser shortcuts)
- Does not affect execution logic, permissions, or state management
- Does not perform login detection

### Backward Compatibility

- Runtimes that do not recognize `browser_login` will strip it during schema validation — no impact on installation or execution
- Specs without `browser_login` will not show login guidance; the runtime may still show browser access based on `permissions: ["ai-browser"]`

### i18n Support

Labels can be translated via the `i18n` block, keyed by URL:

```yaml
i18n:
  zh-CN:
    browser_login:
      "https://www.xiaohongshu.com":
        label: "小红书"
      "https://creator.xiaohongshu.com":
        label: "小红书创作者中心"
```
