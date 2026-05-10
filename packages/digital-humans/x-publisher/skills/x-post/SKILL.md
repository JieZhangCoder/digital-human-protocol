---
name: x-post
description: 在 X 主页发表一条新推，通过 ClipboardEvent 注入文字到 Draft.js 编辑器，点击 Post 按钮，并通过 DOM 检测新推文确认发布成功
allowed-tools: ai-browser
user-invocable: false
---

# X Post

在 X（推特）主页内联编辑器发布一条新推文。通过 `ClipboardEvent('paste')` + `DataTransfer` 注入文字（Draft.js 兼容方式），点击 `tweetButtonInline` 按钮，并通过 DOM 检测时间戳为 "Now" 且作者为当前登录用户的新推文确认发布成功。

## 使用前提

- 页面必须已导航到 `https://x.com/home`
- 用户已在浏览器 session 中登录 X
- 主页内联编辑器（`[data-testid="tweetTextarea_0"]`）可见

## 调用步骤

1. `browser_navigate` 到主页：
   ```
   https://x.com/home
   ```

2. `browser_wait_for` 等待 `"What's happening?"` 出现（确认编辑器已渲染）

3. `browser_run` 调用本 skill：
   ```
   browser_run({
     file: "{skill路径}/index.js",
     params: { content: "你想发的推文内容" }
   })
   ```

## 参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| content | string | 是 | 推文正文，1–280 字符（Premium 用户更多）；不能为空 |

## 返回值

```json
{
  "success": true,
  "tweet_id": "2053438009060831473",
  "tweet_url": "https://x.com/FlynnWayne_Wang/status/2053438009060831473",
  "author_handle": "FlynnWayne_Wang"
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| content 为空 | `{ success: false, error_type: "content_rejected", error: "推文内容不能为空" }` |
| content 超长 | `{ success: false, error_type: "content_rejected", error: "推文超过 280 字符" }` |
| 未找到编辑器 | `{ success: false, error_type: "selector_not_found", error: "未找到推文编辑器（[data-testid=tweetTextarea_0]）" }` |
| 文字注入失败 | `{ success: false, error_type: "selector_not_found", error: "ClipboardEvent 注入未生效，编辑器仍为空" }` |
| Post 按钮 disabled | `{ success: false, error_type: "content_rejected", error: "Post 按钮处于 disabled 状态" }` |
| 未在 10 秒内检测到新推 | `{ success: false, error_type: "verification_timeout", error: "已点击 Post，但 10 秒内未在 timeline 检测到新推文" }` |
| 未登录 | `{ success: false, error_type: "auth_expired", error: "未检测到登录用户" }` |

## 技术原理

X 的推文编辑器是 Draft.js（`.public-DraftEditor-content`），与知乎编辑器底层一致：
1. `document.execCommand('insertText', ...)` 能写入 DOM 但 React state 不更新（Post 按钮保持 disabled）
2. 合成 `ClipboardEvent('paste')` + `DataTransfer.setData('text/plain', ...)` 触发 Draft.js 的 paste handler，更新 editorState，使 Post 按钮激活

发布成功的判定：点击 Post 后，timeline 顶部会立即出现一条新 article，其中 `time.dateTime` 在最近 30 秒内、`<a>` 链接指向 `/{currentUserHandle}/status/{id}`。从 href 提取 tweet_id。

不依赖 GraphQL XHR 拦截：`CreateTweet` 的 queryId 会随 X 部署轮转，DOM 检测更稳定。
