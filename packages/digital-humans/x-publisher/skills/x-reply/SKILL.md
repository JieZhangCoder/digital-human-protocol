---
name: x-reply
description: 在 X 推文详情页发表回复，通过 ClipboardEvent 注入 Draft.js 编辑器，点击 Reply，并通过 DOM 检测新回复确认成功
allowed-tools: ai-browser
user-invocable: false
---

# X Reply

在 X 推文详情页对目标推文发表一条回复。复用 x-post 的 Draft.js 注入方式（`ClipboardEvent`+`DataTransfer`），点击的按钮 testid 同样是 `tweetButtonInline`，但按钮文案在详情页变成 "Reply"。

## 使用前提

- 浏览器 session 已登录 X
- 页面已导航到目标推文详情页：`https://x.com/{author}/status/{tweet_id}`
- 回复编辑器（`[data-testid="tweetTextarea_0"]`）可见

## 调用步骤

1. `browser_navigate` 到目标推文：
   ```
   https://x.com/{author}/status/{tweet_id}
   ```

2. `browser_wait_for` 等待 `"Post your reply"` 或 `"发表你的回复"`（确认编辑器渲染）

3. `browser_run` 调用本 skill：
   ```
   browser_run({
     file: "{skill路径}/index.js",
     params: { content: "你的回复内容" }
   })
   ```

注：`target_url` 不传给 skill——skill 假定页面已经在目标推文详情页，由编排层负责导航。这样 skill 不重复做导航，避免与编排层职责冲突。

## 参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| content | string | 是 | 回复正文，1–280 字符 |

## 返回值

```json
{
  "success": true,
  "reply_id": "2053440000000000001",
  "reply_url": "https://x.com/MyHandle/status/2053440000000000001",
  "in_reply_to_tweet_id": "2053438009060831473",
  "in_reply_to_url": "https://x.com/FlynnWayne_Wang/status/2053438009060831473"
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| content 为空/超长 | `{ success: false, error_type: "content_rejected", error: "..." }` |
| 不在推文详情页 | `{ success: false, error_type: "selector_not_found", error: "当前 URL 不是 X 推文详情页" }` |
| 未找到编辑器 | `{ success: false, error_type: "selector_not_found", error: "未找到回复编辑器" }` |
| 注入失败 | `{ success: false, error_type: "selector_not_found", error: "ClipboardEvent 注入未生效" }` |
| Reply 按钮 disabled | `{ success: false, error_type: "content_rejected", error: "Reply 按钮处于 disabled 状态" }` |
| 验证超时 | `{ success: false, error_type: "verification_timeout", error: "已点击 Reply，未在 12 秒内检测到新回复" }` |

## 技术原理

- 详情页回复编辑器 testid 与发推一致（`tweetTextarea_0`），按钮 testid 一致（`tweetButtonInline`），仅按钮文案是 "Reply"
- 验证：点击 Reply 后，主推下方的对话流会插入一条新 article，作者为当前登录用户，时间戳为 "Now"。从其链接 `/{handle}/status/{id}` 提取 reply_id
- 由于详情页 SPA 局部刷新比 timeline 慢，验证超时设为 12 秒（比 x-post 的 10 秒长）
