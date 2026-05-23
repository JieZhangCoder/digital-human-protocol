// meituan-shop-menu: 解析美团外卖店铺菜单页 DOM 输出结构化菜单
// 关键: 菜单是双栏虚拟滚动，右侧 spuList 容器需要分段滚动才能渲染全部商品
//
// params: { include_description?, only_available?, categories?, max_scroll_steps? }
// returns: { success, shop, categories[], total_items, category_names[], error? }

async (params) => {
  try {
    const {
      include_description = false,
      only_available = true,
      categories: catFilter = [],
      max_scroll_steps = 20,
    } = params || {}

    if (!location.pathname.includes('/mindex/menu')) {
      return {
        success: false,
        error_type: 'wrong_page',
        error: '当前不在菜单页',
        current_url: location.href,
      }
    }

    // 找右侧商品滚动容器（class 名以 spuList_ 开头）
    const spuList = (() => {
      const cands = Array.from(document.querySelectorAll('div')).filter((el) =>
        /spuList_/.test((el.className || '').toString()),
      )
      return cands[0] || null
    })()

    // 单次慢滚 + 边滚边 parse；不预 scroll-to-bottom（会让虚拟列表回收已渲染部分）
    const seenItems = new Map() // key: category|name → item

    const parsePrices = (text) => [...text.matchAll(/¥\s*([\d.]+)/g)].map((m) => parseFloat(m[1]))
    const parseMonthSold = (text) => text.match(/月售\s*(\d+\+?)/)?.[1] || null

    const parseDD = (dd, categoryName) => {
      const text = dd.innerText || ''
      const firstLine = text.split('\n').find((l) => l.trim().length > 0)
      const name = firstLine?.trim() || ''
      if (!name) return null
      const prices = parsePrices(text)
      const item = {
        name,
        price: prices[0] ?? null,
        original_price: prices.length >= 2 ? prices[1] : null,
        month_sold: parseMonthSold(text),
        needs_spec_selection: !text.includes('加入购物车') && !text.includes('已售罄'),
        sold_out: text.includes('已售罄'),
        in_cart_count: (() => {
          if (text.includes('移出购物车') && text.includes('加入购物车')) {
            const between = text
              .replace(/^[\s\S]*?移出购物车/, '')
              .replace(/加入购物车[\s\S]*$/, '')
              .trim()
            return parseInt(between.match(/\d+/)?.[0] || '0', 10) || 0
          }
          return 0
        })(),
      }
      if (include_description) {
        const lines = text.split('\n').map((l) => l.trim()).filter(Boolean)
        item.description = lines.slice(2, 8).join(' ')
      }
      item._category = categoryName
      return item
    }

    const wanted = new Set(catFilter || [])
    const allCategoryNames = new Set()

    const parseCurrentView = () => {
      const dls = document.querySelectorAll('dl')
      for (const dl of dls) {
        const dt = dl.querySelector('dt')
        const categoryName = dt?.innerText?.trim() || ''
        if (!categoryName) continue
        allCategoryNames.add(categoryName)
        if (wanted.size > 0 && !wanted.has(categoryName)) continue
        const dds = dl.querySelectorAll('dd')
        for (const dd of dds) {
          const it = parseDD(dd, categoryName)
          if (!it) continue
          if (only_available && it.sold_out) continue
          const key = categoryName + '|' + it.name
          if (!seenItems.has(key)) seenItems.set(key, it)
        }
      }
    }

    // 慢滚一次，每段 parse；scrollHeight 会随渲染膨胀，循环里实时取
    if (spuList) {
      spuList.scrollTop = 0
      await new Promise((r) => setTimeout(r, 400))
      parseCurrentView()
      const stepSize = Math.max(spuList.clientHeight * 0.5, 300)
      let lastTop = -1
      let stableTicks = 0
      for (let i = 0; i < max_scroll_steps; i++) {
        spuList.scrollTop = spuList.scrollTop + stepSize
        await new Promise((r) => setTimeout(r, 450))
        parseCurrentView()
        if (Math.abs(spuList.scrollTop - lastTop) < 4) {
          stableTicks++
          if (stableTicks >= 3) break
        } else {
          stableTicks = 0
        }
        lastTop = spuList.scrollTop
      }
      spuList.scrollTop = 0
    } else {
      parseCurrentView()
    }

    // 店铺顶部信息
    const article = document.querySelector('article')
    const bodyText = article?.innerText || document.body.innerText || ''
    const ratingMatch = bodyText.match(/([\d.]+)\s*配送约/)
    const deliveryMatch = bodyText.match(/配送约\s*([^\n]+?分钟)/)
    const discountMatch = bodyText.match(/([\d.]+折起)/)
    const shop = {
      name: document.title || '',
      rating: ratingMatch?.[1] || null,
      delivery_estimate: deliveryMatch?.[1]?.trim() || null,
      discount_label: discountMatch?.[1] || null,
    }

    // 按分类组装结果（保持原始顺序：用分类导航顺序，或 fallback 到出现顺序）
    const navLinks = Array.from(document.querySelectorAll('nav a, nav [role="link"]'))
      .map((el) => (el.innerText || '').trim().split('\n')[0].trim())
      .filter(Boolean)
    const order = navLinks.length > 0 ? navLinks : Array.from(allCategoryNames)

    const byCat = new Map()
    for (const item of seenItems.values()) {
      const c = item._category
      delete item._category
      if (!byCat.has(c)) byCat.set(c, [])
      byCat.get(c).push(item)
    }

    const categories = []
    let totalItems = 0
    for (const name of order) {
      if (wanted.size > 0 && !wanted.has(name)) continue
      const items = byCat.get(name)
      if (!items || items.length === 0) continue
      categories.push({ name, item_count: items.length, items })
      totalItems += items.length
    }
    // 兜底：order 之外的分类（不在 nav 里出现的）
    for (const [name, items] of byCat) {
      if (categories.find((c) => c.name === name)) continue
      categories.push({ name, item_count: items.length, items })
      totalItems += items.length
    }

    return {
      success: true,
      shop,
      categories,
      total_items: totalItems,
      category_names: Array.from(allCategoryNames),
    }
  } catch (e) {
    return { success: false, error_type: 'internal_error', error: e.message, stack: e.stack?.slice(0, 500) }
  }
}
