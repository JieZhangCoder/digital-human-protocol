---
name: meituan-submit-order
description: 在美团外卖结算预览（preview）页点击"提交订单"，生成待支付订单。返回 submit 响应或收银台 serialNum
allowed-tools: ai-browser
user-invocable: false
---

# Meituan Submit Order

在 preview 页（`/mindex/preview`）点击 `button.submit_QDYt9D` 提交订单。订单生成后页面会跨 host 跳转到 `sqt.meituan.com` 收银台。本 skill 通过 race 策略尝试在跳转前抓住 submit 响应；如果错过，会从 URL 提取 cashier serialNum 作为兜底。

> **必须 H5 mobile 模式**。

## 使用前提

- 已登录
- 当前 Tab 在 preview 页 `/mindex/preview`
- 已选好收货地址（否则提交按钮不可点）

## 调用步骤

```js
browser_run({ file: ".claude/skills/meituan-submit-order/index.js" })
```

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| wait_ms | number | 8000 | 等待 submit 响应 / 收银台跳转的最长时间 |

## 返回值

### 成功（抓到完整 submit 响应）
```json
{
  "success": true,
  "current_url": "https://sqt.meituan.com/c/finance/cashier/#/control-aggregate?serialNum=<serial>",
  "cashier_serial_num": "<serial>",
  "submit_response": {
    "url": "https://i.waimai.meituan.com/openh5/order/v2/submit?_=...",
    "status": 200,
    "response_parsed": { /* { code, data: { order_id, pay_url, ... } } */ }
  },
  "order_id": "<order id>",
  "pay_url": "<pay url>",
  "hint": "已抓到 submit 接口响应"
}
```

### 部分成功（响应没抓到，但订单已生成）
```json
{
  "success": true,
  "current_url": "https://sqt.meituan.com/c/finance/cashier/#/control-aggregate?serialNum=<serial>",
  "cashier_serial_num": "<serial>",
  "submit_response": null,
  "hint": "只拿到了收银台 serialNum, 完整响应需走 myuncompleteorder 查"
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| 当前不在 preview 页 | `{ success: false, error_type: "wrong_page" }` |
| 未找到提交按钮（可能未选地址） | `{ success: false, error_type: "no_submit_button" }` |
| 既没抓到响应也没跳收银台 | `{ success: false, error_type: "no_response" }` |

## 技术原理

- 触发：click `button.submit_QDYt9D`（Touch + Mouse 序列）
- 推测 endpoint：`POST /openh5/order/v2/submit` 或 `/openh5/order/wm/submit`（路径未实证完整）
- 跨 host 问题：submit 后页面跳 `sqt.meituan.com/c/finance/cashier`，原 origin 的 XHR loadend 可能在 ~50-200ms 跳转窗口内 fire。本 skill 同时写 `localStorage.__mt_last_submit_resp` 作为持久化兜底（同 origin 重访时可读）
- **风险**：提交后真出单, 用户需自行决定是否支付。建议配合 `meituan-order-status` 立刻确认订单 ID, 必要时手动撤销
