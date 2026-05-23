// meituan-list-shops: 解析美团外卖 H5 首页店铺列表
// 数据源: DOM（已渲染的店铺卡片），不调用 shopList API（避免 _token 签名问题）
//
// params: { keyword?, min_rating?, max_distance_m?, limit? }
// returns: { success, location, shops[], total, total_in_dom, error? }

async (params) => {
  try {
    const { keyword = '', min_rating = 0, max_distance_m = 0, limit = 30 } = params || {}

    if (!location.pathname.includes('/mindex/home')) {
      return {
        success: false,
        error_type: 'wrong_page',
        error: '当前不在外卖首页',
        current_url: location.href,
        expected_pattern: '/waimai/mindex/home',
      }
    }

    // 顶部地址（顶部条第一行非搜索文本）
    const headerText = (document.querySelector('article')?.innerText || document.body.innerText || '').split('\n')
    const locName = headerText.find((l) => l.trim() && !l.includes('请输入') && !l.includes('搜索'))?.trim() || ''
    if (!locName) {
      return { success: false, error_type: 'no_location', error: '顶部未显示具体地址' }
    }

    // 店铺卡片识别：基于稳定的文本特征组合
    // 每张卡片同时含：评分 "X.X分"、月售、起送、配送、分钟
    // 不用 \b（对中文无效）
    const cardRoots = []
    document.querySelectorAll('*').forEach((el) => {
      const t = el.innerText || ''
      if (t.length < 50 || t.length > 1500) return
      if (!t.includes('月售')) return
      if (!t.includes('起送')) return
      if (!t.includes('分钟')) return
      if (!t.includes('配送')) return
      if (!/[\d.]+\s*分\s/.test(t) && !/[\d.]+\s*分\n/.test(t)) return // 评分行
      cardRoots.push(el)
    })

    // 取最深的（叶子级容器）：排除内嵌父容器
    const realCards = cardRoots.filter((el) => {
      for (const other of cardRoots) {
        if (other !== el && el.contains(other)) return false
      }
      return true
    })

    if (realCards.length === 0) {
      return { success: false, error_type: 'dom_not_ready', error: '未识别到店铺卡片，页面可能未加载完' }
    }

    const kw = (keyword || '').toLowerCase()
    const parseDistance = (text) => {
      const km = text.match(/(\d+(?:\.\d+)?)\s*km/i)
      if (km) return Math.round(parseFloat(km[1]) * 1000)
      const m = text.match(/(\d+)\s*m(?![a-zA-Z])/)
      if (m) return parseInt(m[1], 10)
      return null
    }

    const shops = []
    for (const card of realCards) {
      const text = card.innerText
      const lines = text.split('\n').map((l) => l.trim()).filter(Boolean)
      const name = lines[0]
      if (!name) continue
      if (kw && !name.toLowerCase().includes(kw)) continue

      const rating = (() => {
        const m = text.match(/([\d.]+)\s*分/)
        return m ? parseFloat(m[1]) : null
      })()
      if (min_rating > 0 && (rating == null || rating < min_rating)) continue

      const monthSold = text.match(/月售\s*(\d+\+?)/)?.[1] || null
      const avgPrice = parseFloat(text.match(/人均\s*¥\s*(\d+(?:\.\d+)?)/)?.[1] || '') || null
      const minOrder = parseFloat(text.match(/起送\s*¥\s*(\d+(?:\.\d+)?)/)?.[1] || '') || null
      const deliveryFeeText = text.match(/配送\s*(约?¥\s*[\d.]+)/)?.[1]?.replace(/\s+/g, '') || null
      const deliveryTime = text.match(/(\d+分钟)/)?.[1] || null
      const distance = parseDistance(text)
      if (max_distance_m > 0 && distance != null && distance > max_distance_m) continue

      // 推荐语（中文引号包裹）
      const taglineMatch = text.match(/[""]([^""]{2,30})[""]/)
      const tagline = taglineMatch ? taglineMatch[1] : null

      const isTopRated =
        text.includes('大众点评高分店铺') ||
        /好评榜|人气榜|复购榜|热销榜|品质榜/.test(text)

      // 券标签
      const couponTags = []
      const couponRe = /(领\d+元券|返\d+元券|新客减\d+|满\d+得[^\s\n]+|第\S+半价|买赠)/g
      let cm
      while ((cm = couponRe.exec(text)) !== null) {
        couponTags.push(cm[1])
        if (couponTags.length >= 10) break
      }

      shops.push({
        name,
        rating,
        month_sold: monthSold,
        avg_price: avgPrice,
        min_order: minOrder,
        delivery_fee_text: deliveryFeeText,
        delivery_time: deliveryTime,
        distance_m: distance,
        tagline,
        is_top_rated: isTopRated,
        coupon_tags: couponTags,
      })

      if (shops.length >= limit) break
    }

    return {
      success: true,
      location: locName,
      shops,
      total: shops.length,
      total_in_dom: realCards.length,
    }
  } catch (e) {
    return { success: false, error_type: 'internal_error', error: e.message, stack: e.stack?.slice(0, 500) }
  }
}
