---
name: xhs-comment
description: 在小红书帖子详情页发表评论，通过 execCommand 注入文字 + XHR 拦截确认发布成功
allowed-tools: ai-browser
user-invocable: false
---

# XHS Comment

在小红书（小红书）帖子详情页发表一条评论。通过 `execCommand('insertText')` 注入文字触发 Vue 响应式，点击发送按钮，并拦截 `comment/post` XHR 响应确认成功。

## 使用前提

- 页面必须已导航到帖子详情页（`https://www.xiaohongshu.com/explore/{note_id}`）
- 用户必须已在浏览器 session 中登录小红书
- 页面需已加载完成（评论输入框可见）

## 调用步骤

1. `browser_navigate` 到帖子详情页：
   ```
   https://www.xiaohongshu.com/explore/{note_id}
   ```

2. `browser_wait_for` 等待 `"说点什么"` 出现（确认评论区已加载）

3. `browser_run` 调用本 skill：
   ```
   browser_run({
     file: "{skill路径}/index.js",
     params: {
       content: "你想发表的评论内容"
     }
   })
   ```

## 参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| content | string | 是 | 要发表的评论文字，不能为空 |

## 返回值

```json
{
  "success": true,
  "comment_id": "69aa4b17000000003103xxxx",
  "content": "你发表的评论内容",
  "toast": "评论已发布"
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| content 为空 | `{ "success": false, "error": "评论内容不能为空" }` |
| 未找到输入框 | `{ "success": false, "error": "未找到评论输入框（.content-input）" }` |
| 未找到发送按钮 | `{ "success": false, "error": "未找到发送按钮" }` |
| 按钮未激活 | `{ "success": false, "error": "发送按钮仍处于禁用状态（Vue 未激活）" }` |
| API 无响应 | `{ "success": false, "error": "API 超时：comment/post 未在 6 秒内响应" }` |
| 未登录 | `{ "success": false, "error": "未登录", "code": 401 }` |

## 技术原理

小红书 Web 端使用 Vue 3 + contenteditable 输入框。普通的 `input.value =` 赋值不会触发 Vue 响应式，导致发送按钮无法激活。

本 skill 采用：
1. `document.execCommand('insertText', false, content)` — 浏览器原生插入文字，兼容 contenteditable
2. `dispatchEvent(new Event('input', { bubbles: true }))` — 手动触发 Vue 监听的 input 事件
3. 等待 400ms Vue nextTick 异步更新后点击发送
4. XHR 拦截 `comment/post` 响应，通过 `success: true` 确认发布成功，而非依赖 DOM 变化判断
