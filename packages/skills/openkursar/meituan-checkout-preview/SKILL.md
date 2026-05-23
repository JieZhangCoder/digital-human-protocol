---
name: meituan-checkout-preview
description: 触发美团外卖结算预览（preview）接口，返回完整结算数据：优惠券、配送时段、地址、备注、总价细分等
allowed-tools: ai-browser
user-invocable: false
---

# Meituan Checkout Preview

触发 `POST /openh5/order/v2/preview` 接口，捕获完整结算页数据。可在三种状态下调用：店铺菜单页（自动点"去结算"跳到 preview）、preview 页（等待自然请求）、或纯被动等待。

> **必须 H5 mobile 模式**。

## 使用前提

- 已登录
- 购物车里有商品且过起送线（否则 "去结算" 按钮不可点）

## 调用步骤

```js
// 场景 1: 当前在菜单页, 想触发结算
browser_run({
  file: ".claude/skills/meituan-checkout-preview/index.js",
  params: { mode: "on_menu" }
})

// 场景 2: 当前在 preview 页, 拿最近一次响应
browser_run({
  file: ".claude/skills/meituan-checkout-preview/index.js",
  params: { mode: "on_preview", wait_ms: 5000 }
})

// 场景 3: auto (默认), 根据当前 URL 自动判断
browser_run({ file: ".claude/skills/meituan-checkout-preview/index.js" })
```

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| mode | "auto" \| "on_menu" \| "on_preview" \| "passive" | "auto" | 触发模式 |
| wait_ms | number | 8000 | 等待 preview 响应的最长时间 |

## 返回值

```json
{
  "success": true,
  "current_url": "https://h5.waimai.meituan.com/waimai/mindex/preview?placeholder=1&redirectfrom=1",
  "request_summary": {
    "poi_id_str": "<shop poi id>",
    "addr_id": 1000000001,
    "foodlist": [{ "skuId": 45536934479, "count": 2, "attr_ids": [], "activityTag": "" }],
    "recipient_name": "张",
    "recipient_phone": "<your-phone>",
    "recipient_address": "示例小区 4 单元",
    "house_number": "101"
  },
  "preview_response": {
    "url": "https://i.waimai.meituan.com/openh5/order/v2/preview?_=...",
    "status": 200,
    "request_decoded": { /* 完整请求体 */ },
    "response_parsed": { /* 完整响应 */ }
  },
  "summary": {
    "has_address": true,
    "coupon_count": 6,
    "unavailable_food_count": 0
  }
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| 未达起送线，"去结算"按钮不存在 | `{ success: false, error_type: "no_settle_button" }` |
| wait_ms 内未捕获到 preview 响应 | `{ success: false, error_type: "no_response", current_url }` |

## 技术原理

- 接口：`POST i.waimai.meituan.com/openh5/order/v2/preview`
- 请求体 `data` 字段：URL-encoded JSON，含 `foodlist`、`addr_id`、`recipient_*`、`callback_info`（带回上次活动/订单信息）
- 触发方式：菜单页 click `.goToPreview_oTQlfa`；或地址选择后页面自动 re-preview
- 签名：`_token` 由 H5guard 实时生成
