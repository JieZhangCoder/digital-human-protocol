// meituan-get-cart: 读取美团外卖购物车（localStorage.cached_cart_data）
// 把按 poi_id_str 分仓的购物车展开成易消费的 items 列表
//
// params: { poi_id_str?: string, include_raw?: boolean }
// returns: { success, poi_id_str?, items[], item_count, ... }

async (params) => {
  try {
    const { poi_id_str: askPoi = '', include_raw = false } = params || {}

    const raw = localStorage.getItem('cached_cart_data')
    if (!raw) {
      return { success: true, is_empty: true, items: [], item_count: 0, reason: 'no cached_cart_data' }
    }

    let store
    try {
      store = JSON.parse(raw)
    } catch (e) {
      return { success: false, error_type: 'parse_error', error: e.message }
    }

    const ids = Array.isArray(store.ids) ? store.ids : []
    const lastPoiId = store.lastPoiId || ''

    // 计算每家店的摘要
    const summarize = (poi_id) => {
      const shop = store[poi_id]
      if (!shop || !Array.isArray(shop.cartData) || shop.cartData.length === 0) {
        return { poi_id_str: poi_id, item_count: 0, spu_count: 0, is_empty: true }
      }
      let itemCount = 0
      const spuSet = new Set()
      for (const cart of shop.cartData) {
        for (const bill of cart.bills || []) {
          itemCount += bill.count || 0
          spuSet.add(bill.spuId)
        }
      }
      return { poi_id_str: poi_id, item_count: itemCount, spu_count: spuSet.size, is_empty: itemCount === 0 }
    }

    const target = askPoi || lastPoiId
    if (!target) {
      return { success: true, is_empty: true, items: [], item_count: 0, reason: 'no target poi' }
    }

    const shop = store[target]
    if (!shop || !Array.isArray(shop.cartData)) {
      return { success: true, is_empty: true, items: [], item_count: 0, poi_id_str: target }
    }

    // 展开 bills 为 items
    const items = []
    let totalCount = 0
    for (const cart of shop.cartData) {
      for (const bill of cart.bills || []) {
        const attrValues = bill.attrValues || []
        const attrIds = bill.attrs || []
        const item = {
          spu_id: bill.spuId,
          sku_id: bill.skuId,
          name: bill.spuName,
          spec: bill.spec || '',
          count: bill.count || 0,
          unit_price: bill.originPrice,
          current_price: bill.currentPrice,
          attr_values: attrValues,
          attr_ids: attrIds,
          tag: bill.tag,
        }
        if (include_raw) item._raw = bill
        items.push(item)
        totalCount += bill.count || 0
      }
    }

    const result = {
      success: true,
      poi_id_str: target,
      mt_shop_id: shop.mtShopId,
      is_empty: totalCount === 0,
      items,
      item_count: totalCount,
      spu_count: new Set(items.map((i) => i.spu_id)).size,
    }

    // 如果没指定 poi，附带所有店的摘要
    if (!askPoi) {
      result.all_shops = ids.map(summarize)
      result.shop_count = result.all_shops.length
    }

    return result
  } catch (e) {
    return { success: false, error_type: 'internal_error', error: e.message }
  }
}
