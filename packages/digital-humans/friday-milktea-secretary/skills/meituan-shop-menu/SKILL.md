---
name: meituan-shop-menu
description: 解析美团外卖店铺菜单页 DOM，输出分类 + 商品 + 价格 + 是否需要选规格的结构化菜单
allowed-tools: ai-browser
user-invocable: false
---

# Meituan Shop Menu

从美团外卖 H5 店铺菜单页（`/waimai/mindex/menu?poi_id_str=...`）的已渲染 DOM 解析出菜单。

**纯 DOM 读取，零网络请求。**

> **必须 H5 模式**：美团没有 PC 站点。打开菜单页必须 `browser_navigate({ url: "...", device: "h5" })`。PC 模式下视口宽、滚动 step 大，虚拟列表节点会被 React 频繁复用，dt 文本瞬时被相邻商品名顶替，导致解析出"幻影分类 + 残片商品"。H5 模式实测干净。

## 使用前提

- 当前 Tab 是某家店的菜单页，**以 H5 模式打开**
- 菜单已经加载渲染（DescriptionList 列出现在 DOM 中）
- 已选中"点菜"标签（不是"评价"或"商家"）

## 调用步骤

1. `browser_navigate({ url: "<menu_url>", device: "h5" })`
2. `browser_wait_for` 等待商品标题文本（如"折扣"或店铺第一个分类名）
3. `browser_run({ file: ".claude/skills/meituan-shop-menu/index.js", params: { include_description: false } })`

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| include_description | boolean | false | 是否在 items 里附带长描述文本（菜品介绍） |
| only_available | boolean | true | 只返回未售罄的商品 |
| categories | string[] | [] | 仅返回指定分类（按名称匹配）；空数组返回全部 |

## 返回值

```json
{
  "success": true,
  "shop": {
    "name": "示例奶茶店（XX店）",
    "rating": "4.8",
    "delivery_estimate": "25分钟",
    "discount_label": "7.93折起"
  },
  "categories": [
    {
      "name": "霸气鲜果茶",
      "item_count": 8,
      "items": [
        {
          "name": "霸气杨枝甘露",
          "price": 21,
          "original_price": null,
          "month_sold": "600",
          "needs_spec_selection": true,
          "sold_out": false,
          "in_cart_count": 0
        }
      ]
    }
  ],
  "total_items": 42,
  "category_names": ["折扣", "新品联名", "..."]
}
```

字段说明：
- `needs_spec_selection`: true 表示卡片上没有"加入购物车"按钮，必须先打开规格弹窗才能加入（典型如奶茶有中杯/大杯、糖度、温度）
- `sold_out`: true 表示卡片上显示"已售罄"
- `in_cart_count`: 该商品在当前购物车中的数量（解析卡片上的"+/-"控件中间的数字）

## 错误情况

| 情况 | 返回 |
|---|---|
| 不在菜单页 | `{ success: false, error_type: "wrong_page", error: "..." }` |
| DOM 还没渲染完 | `{ success: false, error_type: "dom_not_ready", error: "未找到 dd 元素" }` |
| 当前不在"点菜"标签 | `{ success: false, error_type: "wrong_tab", error: "..." }` |

## 技术原理

- 美团 H5 菜单使用语义化 HTML：`<dl>` 是分类，`<dt>` 是分类名，`<dd>` 是商品
- 价格通过 `¥` 字符 + 数字正则提取
- 售罄、按钮通过文本内容匹配（class 名是 hash 化的，不可依赖）
- 不调用任何接口——菜单数据初次进店时一次性加载进前端

## Known Limitations

### 1. 拿不到 SPU/SKU/规格组 ID
DOM 只暴露文字（商品名、价格、月售、是否需选规格）。SPU/SKU/attr ID 在 React 内部状态里，DOM 不可见。

**为什么不调接口拿 ID**：菜单 API `/openapi/v1/poi/food` 和 `/openh5/v2/poi/menuproducts` 的 POST body 都需要 `_token`（H5guard SDK 实时签名），无法预生成。

**怎么补**：`meituan-modify-cart` 通过 click "加入购物车" 按钮触发，由 `/shoppingcart/wm/calculateprice` 响应回传 spu_id/sku_id/tag。SKU ID 在加菜时由平台自己补全，不需要 menu skill 提供。

### 2. 虚拟滚动 + 列表回收
美团 H5 用虚拟列表，屏外的 dd 会被 unrender。skill 内部用"分段滚动 + 边滚边 parse + 去重"来对抗，但：
- 极底部的几个分类（典型：零食/周边/快乐加料）有时抓不全
- `max_scroll_steps` 默认 20 够覆盖 ~10 个分类；超过的把这个参数调高（如 40）

### 3. 单杯多规格不暴露
带规格（杯型/糖度/温度）的商品在 DOM 上只显示一个"打开规格弹窗"的入口，本 skill 通过 `needs_spec_selection: true` 标记，但**不展开具体规格选项**。
规格枚举需要在加菜路径上靠"点商品打开弹窗 → 拦截 spec API 响应"完成。
