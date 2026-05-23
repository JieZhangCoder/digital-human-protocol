---
name: meituan-list-shops
description: 从美团外卖 H5 首页 DOM 解析店铺列表（含评分、月售、配送、距离、券标签），支持按关键词过滤
allowed-tools: ai-browser
user-invocable: false
---

# Meituan List Shops

从美团外卖 H5 首页（`h5.waimai.meituan.com/waimai/mindex/home`）的已渲染 DOM 解析店铺列表。

**纯 DOM 读取，零网络请求。**

> **必须 H5 模式**：美团没有 PC 站点。打开首页必须 `browser_navigate({ url: "...", device: "h5" })`。PC 模式下店铺卡片样式/容器结构与 H5 不同，本 skill 的 DOM 选择器只在 H5 视口下验证过。

## 使用前提

- 当前 Tab 是美团外卖首页 `/waimai/mindex/home`，**以 H5 模式打开**
- 已选好定位（顶部地址显示具体地点而非"定位中"）
- 列表已加载首屏

## 调用步骤

1. `browser_navigate({ url: "https://h5.waimai.meituan.com/waimai/mindex/home", device: "h5" })`
2. `browser_wait_for` 等待任一已知店铺名出现
3. 可选：滚动几屏触发懒加载 → 然后再调
4. `browser_run({ file: ".claude/skills/meituan-list-shops/index.js", params: { keyword: "奶茶", limit: 30 } })`

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| keyword | string | "" | 店名包含匹配（不区分大小写）。用于过滤奶茶/咖啡等品类 |
| min_rating | number | 0 | 评分下限（如 4.5） |
| max_distance_m | number | 0 | 最大距离（米）。0 表示不限 |
| limit | number | 30 | 返回最多多少家 |

## 返回值

```json
{
  "success": true,
  "location": "示例小区",
  "shops": [
    {
      "name": "霸王茶姬（XX店）",
      "rating": 4.9,
      "month_sold": "4000+",
      "avg_price": 19,
      "min_order": 20,
      "delivery_fee_text": "约¥1.8",
      "delivery_time": "15分钟",
      "distance_m": 172,
      "tagline": "新品也很好喝",
      "is_top_rated": true,
      "coupon_tags": ["领3元券", "返3元券"]
    }
  ],
  "total": 25,
  "total_in_dom": 28
}
```

## 错误情况

| 情况 | 返回 |
|---|---|
| 不在首页 | `{ success: false, error_type: "wrong_page", error: "...", current_url }` |
| 定位未确定 | `{ success: false, error_type: "no_location", error: "..." }` |
| DOM 未渲染 | `{ success: false, error_type: "dom_not_ready", error: "..." }` |

## 技术原理

- 列表是 `<article>` 下若干店铺卡片 group
- 评分通过 `<digit>.<digit>分` 文本提取；距离通过 `\d+m` 或 `\d+km` 提取
- 不依赖 class 名（hash 化的）
- 没有 `poi_id_str`——这个 ID 在 React state 里，需要进店后从 URL 拿。本 skill 只做"选店"决策辅助
- 关于"为何不直接调 shopList API"：调接口需要 `mtgsig` 签名，DOM 解析更简单稳健
