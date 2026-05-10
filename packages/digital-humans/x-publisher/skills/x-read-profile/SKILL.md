---
name: x-read-profile
description: 从 X 用户主页 DOM 提取个人资料 + 最近推文
allowed-tools: ai-browser
user-invocable: false
---

# X Read Profile

读取 X 用户主页（`/{handle}`）：返回 bio、所在地、加入日期、关注/粉丝数、置顶推文，以及"Posts" tab 下的最近推文。

## 使用前提

- 浏览器 session 已登录 X
- 页面已导航到目标用户主页：`https://x.com/{handle}`
- 该用户公开（非私密账号）；私密账号需关注后才能看到推文

## 调用步骤

1. `browser_navigate` 到用户主页：
   ```
   https://x.com/{handle}
   ```

2. `browser_wait_for` 等待 `"Followers"` 出现（确认 profile header 渲染）

3. `browser_run` 调用本 skill：
   ```
   browser_run({
     file: "{skill路径}/index.js",
     params: { max_tweets: 10, scroll_pages: 1 }
   })
   ```

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| max_tweets | number | 10 | 最多返回的最近推文数 |
| scroll_pages | number | 1 | 翻页次数；增加 max_tweets 时可提高 |

## 返回值

```json
{
  "success": true,
  "profile": {
    "handle": "AnthropicAI",
    "name": "Anthropic",
    "verified": true,
    "bio": "We're an AI safety and research company that builds reliable, interpretable, and steerable AI systems.",
    "location": null,
    "website": "anthropic.com",
    "joined": "January 2021",
    "following_count_text": "36",
    "following_count": 36,
    "followers_count_text": "1.2M",
    "followers_count": 1200000,
    "is_protected": false
  },
  "recent_tweets": [
    {
      "id": "...",
      "url": "...",
      "content": "...",
      "created_at": "...",
      "metrics": { "replies": 12, "reposts": 45, "likes": 230, "views": 8400 },
      "is_pinned": false,
      "is_repost": false
    }
  ],
  "total": 10
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| 当前 URL 不是用户主页 | `{ success: false, error_type: "selector_not_found", error: "当前 URL 不是 X 用户主页" }` |
| 用户不存在 | `{ success: false, error_type: "content_rejected", error: "用户不存在" }` |
| 用户已被屏蔽/禁言 | `{ success: false, error_type: "content_rejected", error: "账号被屏蔽或暂停" }` |
| 私密账号且未关注 | `{ success: true, profile: {...is_protected: true}, recent_tweets: [] }` |
| profile header 未渲染 | `{ success: false, error_type: "selector_not_found", error: "未找到 UserName 节点" }` |

## 字段说明

- `followers_count` 由 `followers_count_text`（如 "1.2M"）解析得到，约 ±5% 误差（X 在百万级会四舍五入到一位小数）；要精确数字需切换到 GraphQL 抓 `UserByScreenName`
- `is_pinned` 通过查找推文上方的"Pinned"标记或 `[data-testid="socialContext"]` 判定
- `is_repost` 通过查找 `[data-testid="socialContext"]` 文案是否含 "reposted" / "转推" 判定
- `verified` 来自 `UserName` 内的 verified 图标存在性，含 blue check 和官方蓝勾本 skill 不区分

## 技术原理

| 元素 | testid |
|---|---|
| 显示名 + handle | `[data-testid="UserName"]` |
| Bio | `[data-testid="UserDescription"]` |
| 资料项（位置/网站/加入日期） | `[data-testid="UserProfileHeader_Items"]` |
| 关注/粉丝链接 | `a[href$="/following"]` / `a[href$="/verified_followers"]` |
| 置顶 / 转推标记 | `[data-testid="socialContext"]` |
