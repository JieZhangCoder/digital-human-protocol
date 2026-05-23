---
name: x-read-tweet
description: 从 X 推文详情页 DOM 提取主推文（作者、正文、时间、指标）和回复列表
allowed-tools: ai-browser
user-invocable: false
---

# X Read Tweet

读取 X 推文详情页的结构化数据。从已渲染的 DOM 中提取焦点推文（focal tweet）和对话流里可见的回复。指标数（点赞/转发/回复/浏览）通过解析按钮的 `aria-label`（"301 Replies. Reply"）拿到精确值，避免 DOM 显示的"301"或缩写"1.3K"丢精度。

## 使用前提

- 浏览器 session 已登录 X（公开推文不强制登录，但已登录可读到更多内容）
- 页面已导航到推文详情页：`https://x.com/{author}/status/{tweet_id}`
- 推文详情页已加载完成

## 调用步骤

1. `browser_navigate` 到推文：
   ```
   https://x.com/{author}/status/{tweet_id}
   ```

2. `browser_wait_for` 等待 `"Post your reply"` 或主推作者名出现

3. `browser_run` 调用本 skill：
   ```
   browser_run({
     file: "{skill路径}/index.js",
     params: { include_replies: true, max_replies: 20 }
   })
   ```

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| include_replies | boolean | true | 是否提取回复列表 |
| max_replies | number | 20 | 最多返回的回复数量；不滚动加载更多 |

## 返回值

```json
{
  "success": true,
  "tweet": {
    "id": "2052727053091475863",
    "url": "https://x.com/kimmonismus/status/2052727053091475863",
    "author": {
      "handle": "kimmonismus",
      "name": "Chubby ♨️",
      "verified": true
    },
    "content": "Anthropics co-founder Jack Clark: ...",
    "lang": "en",
    "created_at": "2026-05-08T12:27:21.000Z",
    "metrics": {
      "replies": 102,
      "reposts": 100,
      "likes": 765,
      "views": 65129,
      "bookmarks": null
    },
    "truncated": false,
    "has_media": true
  },
  "replies": [
    {
      "id": "...",
      "url": "...",
      "author": { "handle": "...", "name": "..." },
      "content": "...",
      "created_at": "...",
      "metrics": { "replies": 0, "reposts": 0, "likes": 12, "views": 340 }
    }
  ]
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| 当前 URL 不是推文详情页 | `{ success: false, error_type: "selector_not_found", error: "当前 URL 不是 X 推文详情页" }` |
| 主推 article 未渲染 | `{ success: false, error_type: "selector_not_found", error: "未找到主推 article" }` |
| 推文已删除/不存在 | `{ success: false, error_type: "content_rejected", error: "推文不存在或已删除" }` |
| 推文受限（私密/被屏蔽/年龄限制） | `{ success: false, error_type: "auth_expired", error: "推文受限，可能未登录或被作者屏蔽" }` |

## 字段说明

- `truncated: true` 表示主推被 "Show more" 折叠，DOM 中只能看到部分文本——若需完整内容，编排层应先 `browser_click` 主推内的 "Show more" 再调用本 skill
- `metrics.bookmarks` 多数情况下不在按钮 aria-label 中，未取到时返回 null
- `views` 来自 "X views. View post analytics" 的 aria-label 数字
- `verified` 来自 `User-Name` 内的 "Verified account" 图标存在性，含 blue check 和官方蓝勾——本 skill 不区分

## 技术原理

X 推文 DOM 通过稳定的 testid 暴露（探测于 2026-05），多语言界面下文本会变（"Post"/"发布"/"ポスト"），但 testid 不变：

| 元素 | testid |
|---|---|
| 推文 article | `[data-testid="tweet"]` |
| 推文正文 | `[data-testid="tweetText"]` |
| 用户卡片 | `[data-testid="User-Name"]` |
| 回复按钮 | `[data-testid="reply"]` |
| 转推按钮 | `[data-testid="retweet"]` |
| 点赞按钮 | `[data-testid="like"]` 或 `unlike`（已点赞时） |
| 媒体 | `[data-testid="tweetPhoto"]` |
| Show more | `[data-testid="tweet-text-show-more-link"]` |

精确指标从按钮 aria-label 解析：`"301 Replies. Reply"` → 301，`"9919 Likes. Like"` → 9919，`"880533 views. View post analytics"` → 880533。
