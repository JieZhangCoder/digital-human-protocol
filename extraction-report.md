# Bundled Skill Extraction Report

- Mode: `dry-run`
- Default author scope: `openkursar`
- Generated: 2026-05-12T00:43:49.128Z

## Summary

- Total bundled skill declarations: **18**
- Unique skill ids: **17**
- Auto-promotable groups: **16**
  - Single-host: 16
  - Multi-host identical: 0
  - Multi-host partial (majority wins, minority left for human): 0
- Full conflict groups (no automatic action): **1**

## Groups

| Skill id | Hosts | State | Canonical hosts | Conflict hosts | Action |
| --- | --- | --- | --- | --- | --- |
| `bili-get-messages` | 1 | single | bilibili-comment-replier | — | promote → `packages/skills/openkursar/bili-get-messages` |
| `bili-reply` | 1 | single | bilibili-comment-replier | — | promote → `packages/skills/openkursar/bili-reply` |
| `x-post` | 1 | single | x-publisher | — | promote → `packages/skills/openkursar/x-post` |
| `x-read-profile` | 1 | single | x-publisher | — | promote → `packages/skills/openkursar/x-read-profile` |
| `x-read-tweet` | 1 | single | x-publisher | — | promote → `packages/skills/openkursar/x-read-tweet` |
| `x-reply` | 1 | single | x-publisher | — | promote → `packages/skills/openkursar/x-reply` |
| `x-search` | 1 | single | x-publisher | — | promote → `packages/skills/openkursar/x-search` |
| `xhs-comment` | 1 | single | xiaohongshu-ai-engager | — | promote → `packages/skills/openkursar/xhs-comment` |
| `xhs-search` | 2 | conflict | xiaohongshu-ai-engager | xiaohongshu-keyword-monitor | **[conflict]** no automatic action |
| `zhihu-creator-invited` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-creator-invited` |
| `zhihu-creator-search` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-creator-search` |
| `zhihu-fill-editor` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-fill-editor` |
| `zhihu-metrics-answers` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-metrics-answers` |
| `zhihu-metrics-overview` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-metrics-overview` |
| `zhihu-open-editor` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-open-editor` |
| `zhihu-publish-answer` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-publish-answer` |
| `zhihu-question-read` | 1 | single | zhihu-ai-answerer | — | promote → `packages/skills/openkursar/zhihu-question-read` |

## Conflicts / Partial diffs

Diffs are truncated to the first 40 lines per file. Resolve conflicts by manually choosing the authoritative version and re-running the script.

### `xhs-search` (conflict)

Canonical: `xiaohongshu-ai-engager`
Conflict: `xiaohongshu-keyword-monitor`

```diff
--- xiaohongshu-ai-engager/skills/xhs-search/index.js
+++ xiaohongshu-keyword-monitor/skills/xhs-search/index.js
  /**
   * XHS Search — browser_run script
   *
   * Executes inside the Xiaohongshu search results page context.
   * Checks login via direct fetch, installs XHR interceptor for search results,
   * triggers search via Pinia store, and collects structured post data.
   *
   * Contract:
   *   - Single async arrow function (invoked by browser_run via evaluateScript)
   *   - Receives params object as first argument
   *   - Returns JSON-serializable result
   *   - Page must already be navigated to XHS search results URL
   *   - User session/cookies are automatically available
   *   - Errors returned as { success: false, error: "..." }, never thrown
   */
  async (params) => {
    const {
      sort_by = 'general',
      time_range = '不限',
      pages = 1
-   } = params || {}
+   } = params
  
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
    const log = (...a) => console.log('[xhs]', ...a)
  
    log('start', { sort_by, time_range, pages })
  
    // ------------------------------------
    // 1. Install XHR interceptor for search results
    // ------------------------------------
    window.__xhs_searchResps = []
  
    const origOpen = XMLHttpRequest.prototype.open
    const origSend = XMLHttpRequest.prototype.send
  
    XMLHttpRequest.prototype.open = function (method, url, ...rest) {
      this.__xhs_url = typeof url === 'string' ? url : String(url || '')
      return origOpen.call(this, method, url, ...rest)
    }
... (truncated)
```

## Next steps

Re-run with `--apply` to perform the promotions listed above. Conflicts must be resolved manually first.
