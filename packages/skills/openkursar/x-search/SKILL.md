---
name: x-search
description: 从 X 搜索结果页 DOM 提取推文列表，支持 Top/Latest/People 多种类型
allowed-tools: ai-browser
user-invocable: false
---

# X Search

读取 X 搜索结果页（`/search?q=...`）的推文列表。从已渲染的 timeline DOM 解析每条推文的作者、正文、时间、指标和 URL。

## 使用前提

- 浏览器 session 已登录 X
- 页面已导航到搜索结果页：`https://x.com/search?q={encoded_query}&src=typed_query&f={live|user}`
- 搜索结果至少加载完一屏

## 调用步骤

1. 编排层先构造 URL：
   - Top（默认）：`https://x.com/search?q={URL编码的query}&src=typed_query`
   - Latest（按时间）：`https://x.com/search?q={URL编码的query}&src=typed_query&f=live`
   - People（用户）：`https://x.com/search?q={URL编码的query}&src=typed_query&f=user`

2. `browser_navigate` 到该 URL

3. `browser_wait_for` 等待 `"Latest"` 或 `"Top"`（确认 tab 渲染）

4. `browser_run` 调用本 skill：
   ```
   browser_run({
     file: "{skill路径}/index.js",
     params: { limit: 20, scroll_pages: 1 }
   })
   ```

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| limit | number | 20 | 最多返回的推文数 |
| scroll_pages | number | 1 | 翻页次数；每页约滚动一屏触发懒加载 |
| min_likes | number | 0 | 仅返回 likes ≥ 此值的推文（用于话题监控时筛掉低质量） |

## 返回值

```json
{
  "success": true,
  "query": "anthropic",
  "search_type": "latest",
  "results": [
    {
      "id": "2052060350770515978",
      "url": "https://x.com/xai/status/2052060350770515978",
      "author": {
        "handle": "xai",
        "name": "xAI",
        "verified": true
      },
      "content": "SpaceXAI will provide @AnthropicAI with access to Colossus 1...",
      "created_at": "2026-05-06T16:18:07.000Z",
      "metrics": {
        "replies": 1086,
        "reposts": 3297,
        "likes": 25044,
        "views": null
      },
      "has_media": false
    }
  ],
  "total": 17,
  "has_more": true
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| 当前 URL 不是搜索页 | `{ success: false, error_type: "selector_not_found", error: "当前 URL 不是 X 搜索结果页", expected_url_pattern: "https://x.com/search?q=..." }` |
| 无搜索结果 | `{ success: true, results: [], total: 0, has_more: false }` |
| 未登录 / 受限 | `{ success: false, error_type: "auth_expired", error: "..." }` |
| timeline 未渲染 | `{ success: false, error_type: "selector_not_found", error: "未找到 timeline" }` |

## 技术原理

- 搜索结果页和首页、推文详情页共用同一套 timeline 渲染机制：每条推文是 `[data-testid="tweet"]` article
- 滚动触发懒加载：每次 `window.scrollTo({top: document.body.scrollHeight})` 后等 1.5 秒
- 与 x-read-tweet 共用相同的 article 解析逻辑（指标从按钮 aria-label 提取）
- 不依赖 GraphQL 拦截：`SearchTimeline` 的 queryId 会随 X 部署轮转
