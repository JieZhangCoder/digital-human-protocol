---
name: meituan-order-status
description: 查询美团外卖账户的订单列表（待支付/进行中/已完成/已取消），返回订单 ID、状态、店铺、商品、总价、跳转链接
allowed-tools: ai-browser
user-invocable: false
---

# Meituan Order Status

调用 `GET /openh5/order/list`（**已实捕**）。GET 接口同样需要 `mtgsig` 签名，借用页面原生 fetch：点击底部 nav 的"订单"链接触发 SPA 内自动请求并拦截响应。

> **必须 H5 mobile 模式**。

## 使用前提

- 已登录
- 当前 Tab 在美团外卖域名下（任何页面均可，会自动 SPA 跳到订单页）

## 调用步骤

```js
// 默认行为: 自动跳到订单页, 抓首屏
browser_run({ file: ".claude/skills/meituan-order-status/index.js" })

// 已在订单页, 直接读最近响应
browser_run({ file: ".claude/skills/meituan-order-status/index.js", params: { navigate: false } })

// 只看待支付订单
browser_run({ file: ".claude/skills/meituan-order-status/index.js", params: { status_filter: "待支付" } })
```

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| navigate | boolean | true | 是否自动点底部"订单"跳转（不在订单页时建议 true） |
| wait_ms | number | 8000 | 等待响应的最长时间 |
| status_filter | string | "" | 按中文状态过滤（"待支付"/"已完成"/"已取消"/...） |

## 返回值

```json
{
  "success": true,
  "current_url": "https://h5.waimai.meituan.com/waimai/mindex/olist",
  "order_count": 3,
  "total_count_in_response": 3,
  "is_end": false,
  "next_start_index": 0,
  "orders": [
    {
      "order_id": "<order id>",
      "mt_order_view_id": "<order id>",
      "shop_name": "示例奶茶店（XX店）",
      "poi_id_str": "<shop poi id>",
      "mt_wm_poi_id": -100,
      "total_price": 38.9,
      "order_time": "<YYYY-MM-DD HH:mm>",
      "order_time_sec": 0,
      "pay_status": 1,
      "status_str": "已取消",
      "products": [{ "name": "示例商品", "count": 2, "spu_id": 24598527212 }],
      "buttons": [{ "title": "再来一单", "type": 1001 }],
      "scheme": "https://h5.waimai.meituan.com/waimai/mindex/menu?mtShopId=-100&poi_id_str=<shop poi id>",
      "pic_url": "http://p1.meituan.net/waimaipoi/...",
      "can_delete": true
    }
  ],
  "raw_response": { "url": "...", "status": 200, "parsed": { /* 完整原始响应 */ } }
}
```

## payStatus 取值（实捕）

| payStatus | status_str | 含义 |
|---|---|---|
| 1 | 已取消 | 待支付超时或手动取消 |
| 3 | 已完成 | 配送完成 |
| 其他 | 看 `status_str` | 待支付/制作中/配送中/已送达待评价... |

## 错误情况

| 情况 | 返回 |
|---|---|
| wait_ms 内未拦截到响应 | `{ success: false, error_type: "no_response", current_url }` |

## 技术原理

- 接口：`GET i.waimai.meituan.com/openh5/order/list?_={timestamp}`
- 触发：SPA 点击底部"订单" link → 路由到 `/mindex/olist` → 自动 fire 接口
- 签名：`mtgsig` GET 签名运行时生成
- 分页：响应里 `nextStartIndex` + `cursor` + `isEnd`，本 skill 只抓首屏
