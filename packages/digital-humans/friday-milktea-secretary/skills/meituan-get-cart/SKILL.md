---
name: meituan-get-cart
description: 从 localStorage 读取美团外卖购物车，可按店铺 poi_id_str 过滤；返回简洁的商品 + 数量 + 规格视图
allowed-tools: ai-browser
user-invocable: false
---

# Meituan Get Cart

读取 `localStorage.cached_cart_data` 中缓存的购物车。美团 H5 把每家店的购物车按 `poi_id_str` 分仓存放，本 skill 把结构展开成易消费的列表。

**纯本地读取，零网络请求。**

> **必须 H5 模式**：美团没有 PC 站点。前置的加菜/浏览动作必须在 H5 模式下完成（`browser_navigate({ url: "...", device: "h5" })`），购物车数据才能正确写入 `localStorage.cached_cart_data`。

## 使用前提

- 当前 Tab 在 `*.meituan.com`，**以 H5 模式打开**
- 此前在该浏览器会话内至少在某家店加过菜（否则 `cached_cart_data` 不存在或为空）

## 调用步骤

1. `browser_run({ file: ".claude/skills/meituan-get-cart/index.js", params: { poi_id_str: "<shop poi id>" } })`
2. 不传 `poi_id_str` 则返回所有店的购物车摘要

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| poi_id_str | string | "" | 指定店铺 ID。不传则返回 `lastPoiId` 对应店铺，且附 `all_shops` 摘要 |
| include_raw | boolean | false | 是否在返回里附带每个 bill 的原始 SPU/SKU 字段（用于后续加菜接口拼参数） |

## 返回值

指定 `poi_id_str` 时：

```json
{
  "success": true,
  "poi_id_str": "<shop poi id>",
  "is_empty": false,
  "items": [
    {
      "spu_id": 24598527212,
      "sku_id": 45536934479,
      "name": "示例商品名",
      "spec": "标准(1人份)(1人份)",
      "count": 2,
      "unit_price": 18.8,
      "current_price": 18.8,
      "attr_values": ["1人份"],
      "attr_ids": [15271680559],
      "tag": "1435338449"
    }
  ],
  "item_count": 2,
  "shop_count": 1
}
```

不指定时：

```json
{
  "success": true,
  "poi_id_str": "<lastPoiId>",
  "items": [...],
  "all_shops": [
    { "poi_id_str": "...", "item_count": 2, "spu_count": 1, "is_empty": false }
  ]
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| `cached_cart_data` 不存在 | `{ success: true, is_empty: true, items: [], item_count: 0, reason: "no cached_cart_data" }` |
| 指定店铺没数据 | `{ success: true, is_empty: true, items: [], poi_id_str }` |
| JSON parse 失败 | `{ success: false, error_type: "parse_error", error }` |

## 技术原理

- bills 数组是订单行，calculateAttrs 是规格 ID + 值对；attr_ids 是 ID 数组（加菜接口需要）
- `unit_price` 用 `originPrice`，`current_price` 用 `currentPrice`（折扣后）
