---
name: meituan-login-check
description: 检查美团外卖 H5 当前登录态，返回 userId / userName / dfpId 等关键身份信息
allowed-tools: ai-browser
user-invocable: false
---

# Meituan Login Check

读取美团外卖 H5 当前会话的登录态。**纯本地读 cookies + localStorage，不发任何请求**，不会触发反爬。

> **必须 H5 模式**：美团没有 PC 站点，所有 meituan-* skill 都依赖 H5。打开页面时必须 `browser_navigate({ url: "...", device: "h5" })`；PC 模式下 cookie/localStorage 域、DOM 结构均不同，会导致 skill 失败或返回脏数据。

## 使用前提

- 浏览器当前 Tab 在 `*.meituan.com` 任意页面（建议外卖 H5：`h5.waimai.meituan.com`），且页面以 H5 模式打开
- 不需要在特定路由

## 调用步骤

1. 编排层确认或导航到任意美团页面
2. `browser_run({ file: ".claude/skills/meituan-login-check/index.js" })`

## 参数

无。

## 返回值

```json
{
  "success": true,
  "logged_in": true,
  "userId": "<your-userId>",
  "userName": "<URL 解码后的用户名>",
  "uuid": "<iuuid，64 字符>",
  "openh5_uuid": "<H5 UUID，64 字符>",
  "dfpId": "<设备指纹，56 字符>",
  "webdfpid_len": 127
}
```

未登录情况：
```json
{ "success": true, "logged_in": false, "reason": "missing userId cookie" }
```

## 错误情况

| 情况 | 返回 |
|---|---|
| 当前域名不是美团 | `{ success: false, error_type: "wrong_domain", error: "...", current_host: "..." }` |
| document.cookie 异常 | `{ success: false, error_type: "internal_error", error: e.message }` |

## 技术原理

- 登录态判断依据：cookie `userId` 是否存在且非空（实测稳定）
- `userName` cookie 是 URL-encoded UTF-8
- `dfpId` 来自 localStorage，跨会话稳定，是美团 H5 设备指纹的核心
- 不调用 `/openh5/account/center` 接口——避免每次都请求增加风控曝露
