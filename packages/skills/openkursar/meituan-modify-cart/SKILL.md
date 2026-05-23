---
name: meituan-modify-cart
description: 在美团外卖店铺菜单页对一个商品做加 1 / 减 1 操作（含首次加菜、规格弹窗），返回 calculateprice API 实时响应
allowed-tools: ai-browser
user-invocable: false
---

# Meituan Modify Cart

在店铺菜单页（`/mindex/menu?poi_id_str=...`）对一个商品做购物车操作。**一个 skill 覆盖 4 种场景**：

| Mode | 触发条件 | 实测 |
|---|---|---|
| `in_cart_sub` | 商品在购物车 + action=sub | ✅ |
| `in_cart_add` | 商品在购物车 + action=add（DOM 是 `.plus_mbY0hg`） | ✅ |
| `first_add_no_spec_direct` | 商品不在购物车，DOM 是 `.plus_mbY0hg`（单规格商品） | ✅ |
| `first_add_with_spec` | 商品不在购物车，DOM 是 `.mBtnGroup_ho5pZr`（多规格商品弹 popup） | ⏳ 代码就位 |

**核心机制**：通过派发完整 Touch+Mouse 事件触发原生 React handler，借用页面 fetch 自动注入 `_token` 签名，拦截 `calculateprice` XHR 拿响应。

> **必须 H5 mobile 模式**：iPhone UA + 430×720 viewport。

## 使用前提

- 已登录
- 当前 Tab 在某店菜单页 `/mindex/menu?poi_id_str=...`，**以 h5 模式打开**
- 菜单页已完整渲染（无 loading 态）

## 调用步骤

```js
// 添加一个商品 (skill 自动判断走哪条路径)
browser_run({
  file: ".claude/skills/meituan-modify-cart/index.js",
  params: { product_name: "爆汁杏鲍菇", action: "add" }
})

// 减 1
browser_run({
  file: ".claude/skills/meituan-modify-cart/index.js",
  params: { product_name: "爆汁杏鲍菇", action: "sub" }
})

// 多规格商品: 指定规格选择
browser_run({
  file: ".claude/skills/meituan-modify-cart/index.js",
  params: {
    product_name: "辣翅烤翅",
    action: "add",
    spec_selections: ["香辣鸡翅"]  // 或 [0] (index), 或省略走默认
  }
})
```

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| product_name | string | (必填) | 商品名（支持 startsWith / includes 模糊匹配） |
| action | "add" \| "sub" | "add" | 加 1 或 减 1 |
| spec_selections | string[] \| number[] | [] | 多规格弹窗时每组要选的值（按字符串匹配或 index）。省略走默认选中或第 1 个 |
| wait_ms | number | 5000 | 等 calculateprice 响应的最长时间 |

## 返回值

```json
{
  "success": true,
  "mode": "in_cart_add",
  "product_name": "示例商品",
  "action": "add",
  "before_cart_count": 1,
  "after_cart_count": 2,
  "calculate_response": {
    "status": 200,
    "parsed": {
      "code": 0,
      "data": {
        "total_price": 51.1,
        "origin_total_price": 78.4,
        "discount_price": 27.3,
        "cart_info": [{ "product_list": [/* 当前购物车明细 */] }],
        "order_purchase_threshold_info": { "order_over_purchase_threshold": true }
      }
    },
    "request_body": "<URL-encoded form 完整请求体>",
    "response_text": "<完整 JSON 字符串>"
  }
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| 当前不在菜单页 | `{ success: false, error_type: "wrong_page", current_url }` |
| 没传 product_name | `{ success: false, error_type: "bad_param" }` |
| 菜单中找不到该商品 | `{ success: false, error_type: "product_not_found" }` |
| sub 但商品不在购物车 | `{ success: false, error_type: "not_in_cart" }` |
| 找不到 +/-/首加按钮 | `{ success: false, error_type: "no_add_button" / "no_minus_btn" }` |
| 规格弹窗没打开 | `{ success: false, error_type: "popup_did_not_open" }` |
| calculateprice 超时未返回 | `{ success: 视 after_cart_count 与 before 差异, calculate_response: null }` |

## 关键 DOM 锚点（实测稳定）

```
.plus_mbY0hg          : +/- 控件的 +（在购物车 OR 单规格商品首加）
.minus_yMNqWq         : +/- 控件的 -
.clickArea_qHUDMY     : +/- 内部实际接受事件的 span
.mBtnGroup_ho5pZr     : 多规格商品的首加按钮
.spec_AXWaUg          : 规格组 DL 容器
.specTitle_B3t6lm     : 规格组标题 (DT)
.tagContainer_5IqJXm  : 规格值容器 (DD)
.tag_nx79mf           : 单个规格值
.selected_G3xf3i      : 已选规格值的额外 class
.addToCartBtn_bQzcZn  : 规格 popup 里的"加入购物车"
[aria-label="关闭商品规格选择弹窗"] : 弹窗关闭
```

## 技术原理

- 接口：`POST i.waimai.meituan.com/openh5/v6/shoppingcart/wm/calculateprice`
- `data.modify_type`：1=加 / 2=减 / 3=删
- 签名：`_token` 由 H5guard 实时生成，不可重放——必须借用页面原生 fetch
- 触发：派发完整 TouchEvent + MouseEvent click 序列到 `.clickArea_qHUDMY` 内部 span（外层 `.plus_mbY0hg` 不直接接事件）
