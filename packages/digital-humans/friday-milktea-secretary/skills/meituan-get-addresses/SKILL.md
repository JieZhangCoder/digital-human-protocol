---
name: meituan-get-addresses
description: 从 localStorage 读取美团外卖账户下的全部收货地址，支持按关键词/电话过滤
allowed-tools: ai-browser
user-invocable: false
---

# Meituan Get Addresses

直接读取 `localStorage.addstore` 中缓存的地址簿。**纯本地读取，零网络请求**。

> **必须 H5 模式**：美团没有 PC 站点。前置的"访问首页让 addstore 写入"步骤必须在 H5 模式下完成（`browser_navigate({ url: "...", device: "h5" })`），PC 模式下不会触发 `/openh5/address/list` 的预加载。

## 使用前提

- 当前 Tab 在 `*.meituan.com`，**以 H5 模式打开**
- 此前至少访问过一次美团外卖 H5 首页或地址管理页（让 `addstore` 被写入）
- 通常无需特别准备——美团 H5 首页加载就会调 `/openh5/address/list` 并把结果写入 localStorage

## 调用步骤

1. 编排层确认浏览器在美团页面
2. `browser_run({ file: ".claude/skills/meituan-get-addresses/index.js", params: { keyword: "公司" } })`

## 参数

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| keyword | string | "" | 在 `poi` / `address` / `houseNumber` 上做包含匹配（不区分大小写） |
| phone | string | "" | 精确匹配手机号 |
| limit | number | 50 | 返回最多多少条 |

## 返回值

```json
{
  "success": true,
  "addresses": [
    {
      "addressId": 1000000001,
      "name": "张",
      "gender": 1,
      "phone": "<your-phone>",
      "poi": "示例科技园",
      "address": "",
      "houseNumber": "",
      "lat": 22.537028,
      "lng": 113.904215,
      "bindType": 12,
      "addressType": 0,
      "isDefault": 0,
      "isOutOfRange": 0
    }
  ],
  "total": 1,
  "total_stored": 28
}
```

注意：
- `lat` / `lng` 已自动从存储中的整数（×1e6）转换为十进制度
- `gender`: 1=先生, 2=女士
- `bindType`: 11=家, 12=公司, 15=其它

## 错误情况

| 情况 | 返回 |
|---|---|
| `localStorage.addstore` 不存在 | `{ success: false, error_type: "not_initialized", error: "addstore 未写入，需先访问美团外卖首页" }` |
| JSON parse 失败 | `{ success: false, error_type: "parse_error", error: e.message }` |

## 技术原理

- 数据源：美团 H5 自家把 `/openh5/address/list` 的响应缓存到 `localStorage.addstore`
- 缓存随登录态持久化，跨 Tab、跨刷新都在
- 比调 API 快得多，也不留风控痕迹
