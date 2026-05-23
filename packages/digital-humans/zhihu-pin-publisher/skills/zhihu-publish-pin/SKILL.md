---
name: zhihu-publish-pin
description: 在知乎主页发布一条「想法」（pin / 短动态），通过 ClipboardEvent 注入文字到 Draft.js 编辑器，点击发布按钮，并通过 XHR 拦截 + DOM 检测确认发布成功
allowed-tools: ai-browser
user-invocable: false
---

# Zhihu Publish Pin

在知乎主页通过内联「发想法」编辑器发布一条短动态：
1. 调用前查询自己的 pin 列表 `GET /api/v4/members/{handle}/pins?limit=10`，记录已存在 id 集合
2. 找到首页「发想法」入口 button，点击展开内联表单
3. 通过 `ClipboardEvent('paste')` + `DataTransfer` 把文本注入 Draft.js 编辑器（`.public-DraftEditor-content`）
4. 等发布按钮 enabled，点击发布
5. 轮询同一 pin 列表 API（最多 12 秒），首次出现的新 id 即为本次发布的 pin

**为什么不用 fetch 拦截**：知乎 SPA 在 chunk 加载早期缓存了 `window.fetch` 引用，后置注入的 wrap 抓不到 publish 调用。直接查询 user pin 列表 API 100% 可靠且无副作用。

## 使用前提

- 页面必须已导航到 `https://www.zhihu.com/`
- 用户已在浏览器 session 中登录知乎
- 主页可见「发想法」按钮（非匿名/未登录态会缺失）

## 调用步骤

1. `browser_navigate` 到主页：
   ```
   https://www.zhihu.com/
   ```

2. `browser_wait_for` 等待 `"发想法"` 出现（确认登录态 + 入口已渲染）

3. `browser_run` 调用本 skill：
   ```
   browser_run({
     file: "{skill路径}/index.js",
     params: { content: "你想发的想法内容" }
   })
   ```

## 参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| content | string | 是 | 想法正文，1–1000 字符；不能为空 |

## 返回值

成功：
```json
{
  "success": true,
  "pin_id": "<pin id>",
  "pin_url": "https://www.zhihu.com/pin/<pin id>",
  "char_count": 32,
  "verified_via": "pinlist"
}
```

`verified_via` 当前固定为 `"pinlist"`（通过查询用户 pin 列表确认）。

**审核中**（已发布但仅自己可见）：响应额外含
```json
{
  "reviewing": true,
  "reviewing_reason": "content_sensitive",
  "reviewing_tips": "待审核·仅自己可见"
}
```
此时 `success` 仍为 `true`（pin 已存在于知乎），但调用方应留意：在审核期间这条对其他人不可见。带"测试"、"自测"等关键词容易触发；正常 IP 内容不会。

## 错误情况

| 情况 | 返回 |
|---|---|
| content 为空 | `{ success: false, error_type: "content_rejected", error: "想法内容不能为空" }` |
| content 超长 | `{ success: false, error_type: "content_rejected", error: "想法超过 1000 字符" }` |
| 未找到「发想法」按钮 | `{ success: false, error_type: "selector_not_found", error: "未找到「发想法」入口（未登录或入口隐藏）" }` |
| 编辑器未弹出 | `{ success: false, error_type: "selector_not_found", error: "点击发想法后未找到 Draft.js 编辑器" }` |
| 编辑器有残留内容 | `{ success: false, error_type: "selector_not_found", error: "编辑器有残留内容，未先清空" }` |
| 文字注入失败 | `{ success: false, error_type: "selector_not_found", error: "ClipboardEvent 注入未生效" }` |
| 发布按钮 disabled | `{ success: false, error_type: "content_rejected", error: "发布按钮 disabled（内容被风控/含违规链接）" }` |
| 12 秒内无成功响应 | `{ success: false, error_type: "verification_timeout", error: "已点击发布，但未在 12 秒内确认成功" }` |
| API 返回 4xx/5xx | `{ success: false, error_type: "content_rejected" \| "auth_expired" \| "network_error", error: "..." }` |

## 技术原理

### 为什么是 ClipboardEvent

知乎想法用 Draft.js（同 X 推文编辑器）。直接 `value=` 或 `execCommand('insertText')` 写入 DOM 但 React/Draft state 不更新，发布按钮保持 disabled。

合成 `ClipboardEvent('paste')` + `DataTransfer.setData('text/plain', ...)` 触发 Draft.js 内部的 paste handler，正确更新 editorState。

### 为什么 paste 之前必须清空

Draft.js 的 paste 行为是**追加到当前光标位置**，不是替换。如果编辑器有残留草稿（知乎自动保留上次未发的），新内容会拼在后面导致内容污染。代码先用 `Ctrl+A` + `Backspace` 清空。

### 双路成功判定

- **XHR 拦截**：在执行前注入 fetch 包装，捕获 `POST /api/v4/pins`。这是知乎自身写入 API，2xx 响应体含 `id` 字段（pin id），最可靠。
- **DOM 兜底**：表单关闭 + 自己 pin 列表出现新链接。即使 API 路径变化或 wrap 失败，DOM 也能兜住。

发布成功后 pin URL 形式：`https://www.zhihu.com/pin/{numeric_id}`，用户自己的 pin 列表在 `/people/{user_url_token}/pins`。

### 不依赖 GraphQL queryId

知乎 PinV2 的发布走 REST `/api/v4/pins`，不像 X 用 GraphQL，没有 queryId 轮转问题，URL 模式稳定。
