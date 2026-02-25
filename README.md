<div align="center">

# AI Digital Human Protocol (DHP)

### Open protocol for autonomous AI agents

Build, distribute, and run production-ready digital humans with one portable spec.

[![GitHub Stars](https://img.shields.io/github/stars/openkursar/digital-human-protocol?style=social)](https://github.com/openkursar/digital-human-protocol/stargazers)
[![Spec License: CC0](https://img.shields.io/badge/spec-CC0%201.0-lightgrey.svg)](https://creativecommons.org/publicdomain/zero/1.0/)
[![Examples License: MIT](https://img.shields.io/badge/examples-MIT-blue.svg)](https://opensource.org/license/mit)
[![Registry](https://img.shields.io/badge/registry-live-brightgreen.svg)](https://openkursar.github.io/digital-human-protocol/index.json)

[Quickstart](#quickstart) · [Docs](#documentation-map) · [Examples](#examples) · [Contributing](#contributing) · **[中文](./docs/README.zh-CN.md)**

</div>

---

## Why AI DHP

Without a standard, autonomous agents are hard to share, hard to review, and hard to run consistently.
DHP turns ad-hoc prompts and scripts into a reproducible contract.

| Without protocol | With DHP |
|---|---|
| Prompt and behavior are implicit | Agent behavior is explicit in `spec.yaml` |
| Runtime assumptions are unclear | Triggers, memory, dependencies, and output are declared |
| Distribution is manual and fragile | Registry package format is standardized |
| Quality checks are inconsistent | Schema-based validation is automatable |

> DHP for autonomous agents is similar in spirit to what MCP did for AI tool interoperability.

---

## What Is a AI Digital Human

A AI digital human is an autonomous AI agent defined by one `spec.yaml` file.

It can run on a schedule, react to events, browse the web, read and write files, remember past runs,
notify users, and escalate when a human decision is needed.

```text
spec.yaml  ->  install  ->  run automatically  ->  notify user
```

DHP is **runtime-agnostic**. The spec defines what the agent does and when it runs; the runtime executes it.

**Compatible runtime:** [Halo](https://github.com/openkursar/hello-halo/)

---

## Quickstart

Copy this into `spec.yaml` and install it in a compatible runtime.

```yaml
spec_version: "1"
name: "HN Daily Brief"
version: "1.0.0"
author: "your-name"
description: "Summarizes top Hacker News stories every morning and sends an email digest."
type: automation
icon: "news"

system_prompt: |
  You are a concise technology analyst.

  On each run:
  1. Open https://news.ycombinator.com and identify the top 10 stories by score.
  2. Read each headline, linked article (if accessible), and top comments.
  3. Write a digest with:
     - One-sentence summary per story
     - A key insight or counterpoint where relevant
     - "Why it matters" for the 2-3 most important stories
  4. Keep it plain text and concise.
  5. Call report_to_user(type="run_complete") with the digest as summary.

requires:
  mcps:
    - id: ai-browser
      reason: "Read article pages and comment threads"

subscriptions:
  - id: morning-digest
    source:
      type: schedule
      config:
        cron: "0 8 * * *"

config_schema:
  - key: email
    label: "Recipient email"
    type: email
    required: true

output:
  notify:
    system: true
    channels:
      - email

store:
  slug: "hn-daily-brief"
  category: news
  tags: ["hn", "tech", "digest", "daily"]
  locale: en-US
  license: MIT
```

This is already production-ready.

---

## Core Capabilities

<table>
<tr>
<td width="50%">

### Deterministic Triggers
Use schedule, file change, webhook, or custom source triggers.

### Persistent Agent Memory
Carry context safely across runs with `memory_schema`.

### Human-in-the-Loop
Pause and escalate decisions when confidence is low.

</td>
<td width="50%">

### MCP-native Dependencies
Attach browser, database, and API tools through `requires`.

### Multi-channel Notifications
Deliver output to system, email, WeCom, DingTalk, Feishu, or webhook.

### Registry-ready Distribution
Publish reusable agents with store metadata and index automation.

</td>
</tr>
</table>

---

## App Types

| Type | Description | Required fields |
|---|---|---|
| `automation` | Autonomous background agent | `system_prompt`, `subscriptions` |
| `skill` | User-invoked capability | `system_prompt` |
| `mcp` | MCP server wrapper | `mcp_server` |
| `extension` | UI extension / theme | none |

---

## Examples

| Example | Description |
|---|---|
| [`examples/minimal/`](examples/minimal/) | Production-grade minimal automation |
| [`examples/price-hunter/`](examples/price-hunter/) | Production-grade complex automation (memory, filters, escalation, multi-channel output) |

Installable package set: [`packages/digital-humans/`](packages/digital-humans/)

---

## Documentation Map

- **Complete field reference:** [`spec/app-spec.md`](spec/app-spec.md)
- **Bundle/package format:** [`spec/package-format.md`](spec/package-format.md)
- **Registry index protocol:** [`spec/registry-protocol.md`](spec/registry-protocol.md)
- **Allowed categories:** [`spec/categories.md`](spec/categories.md)
- **Spec docs index:** [`spec/README.md`](spec/README.md)

---

## Publishing a Digital Human

```bash
# 1) create package directory
mkdir -p packages/digital-humans/my-agent

# 2) add your spec
$EDITOR packages/digital-humans/my-agent/spec.yaml

# 3) validate and rebuild index
node scripts/build-index.mjs

# 4) open pull request
```

After merge to `main`, GitHub Actions publishes the updated registry index.

---

## Contributing

We welcome production-grade specs, tooling improvements, and documentation quality upgrades.

- Contribution guide: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Open an issue for protocol proposals and edge cases
- Open a PR with spec updates and examples

---

## Registry

- **Index URL:** `https://openkursar.github.io/digital-human-protocol/index.json`
- **Protocol details:** [`spec/registry-protocol.md`](spec/registry-protocol.md)

Clients fetch `index.json`, then install each package via its `spec.yaml` path.

---

<div align="center">

### Build once, run anywhere — with AI DHP.

Protocol spec: [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) · Example specs: MIT

[Back to Top](#digital-human-protocol-dhp)

</div>
