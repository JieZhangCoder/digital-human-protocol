# Bundled Skill Extraction Report

- Mode: `apply`
- Default author scope: `openkursar`
- Generated: 2026-05-23T06:54:32.724Z

## Summary

- Total bundled skill declarations: **28**
- Unique skill ids: **27**
- Auto-promotable groups: **27**
  - Single-host: 26
  - Multi-host identical: 1
  - Multi-host partial (majority wins, minority left for human): 0
- Full conflict groups (no automatic action): **0**

## Groups

| Skill id | Hosts | State | Canonical hosts | Conflict hosts | Action |
| --- | --- | --- | --- | --- | --- |
| `bili-get-messages` | 1 | single | bilibili-comment-replier | — | promote → `packages/skills/openkursar/bili-get-messages` |
| `bili-reply` | 1 | single | bilibili-comment-replier | — | promote → `packages/skills/openkursar/bili-reply` |
| `meituan-checkout-preview` | 1 | single | friday-milktea-secretary | — | promote → `packages/skills/openkursar/meituan-checkout-preview` |
| `meituan-get-addresses` | 1 | single | friday-milktea-secretary | — | promote → `packages/skills/openkursar/meituan-get-addresses` |
| `meituan-get-cart` | 1 | single | friday-milktea-secretary | — | promote → `packages/skills/openkursar/meituan-get-cart` |
| `meituan-list-shops` | 1 | single | friday-milktea-secretary | — | promote → `packages/skills/openkursar/meituan-list-shops` |
| `meituan-login-check` | 1 | single | friday-milktea-secretary | — | promote → `packages/skills/openkursar/meituan-login-check` |
| `meituan-modify-cart` | 1 | single | friday-milktea-secretary | — | promote → `packages/skills/openkursar/meituan-modify-cart` |
| `meituan-order-status` | 1 | single | friday-milktea-secretary | — | promote → `packages/skills/openkursar/meituan-order-status` |
| `meituan-shop-menu` | 1 | single | friday-milktea-secretary | — | promote → `packages/skills/openkursar/meituan-shop-menu` |
| `meituan-submit-order` | 1 | single | friday-milktea-secretary | — | promote → `packages/skills/openkursar/meituan-submit-order` |
| `x-post` | 1 | single | x-publisher | — | promote → `packages/skills/openkursar/x-post` |
| `x-read-profile` | 1 | single | x-publisher | — | promote → `packages/skills/openkursar/x-read-profile` |
| `x-read-tweet` | 1 | single | x-publisher | — | promote → `packages/skills/openkursar/x-read-tweet` |
| `x-reply` | 1 | single | x-publisher | — | promote → `packages/skills/openkursar/x-reply` |
| `x-search` | 1 | single | x-publisher | — | promote → `packages/skills/openkursar/x-search` |
| `xhs-comment` | 1 | single | xiaohongshu-ai-engager | — | promote → `packages/skills/openkursar/xhs-comment` |
| `xhs-search` | 2 | identical | xiaohongshu-ai-engager, xiaohongshu-keyword-monitor | — | promote → `packages/skills/openkursar/xhs-search` |
| `zhihu-creator-invited` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-creator-invited` |
| `zhihu-creator-search` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-creator-search` |
| `zhihu-fill-editor` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-fill-editor` |
| `zhihu-metrics-answers` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-metrics-answers` |
| `zhihu-metrics-overview` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-metrics-overview` |
| `zhihu-open-editor` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-open-editor` |
| `zhihu-publish-answer` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-publish-answer` |
| `zhihu-publish-pin` | 1 | single | zhihu-pin-publisher | — | promote → `packages/skills/openkursar/zhihu-publish-pin` |
| `zhihu-question-read` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-question-read` |

## Next steps

Extraction applied. Run `npm run build` to regenerate `index.json` and verify nothing broke.
