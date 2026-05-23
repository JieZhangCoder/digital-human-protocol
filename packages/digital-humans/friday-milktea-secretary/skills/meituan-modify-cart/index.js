// meituan-modify-cart: 美团外卖店铺菜单页购物车操作 (含规格弹窗)
//
// 覆盖 3 种场景:
//   A. 商品已经在购物车 -> 点 .plus_mbY0hg / .minus_yMNqWq 调整数量
//   B. 商品不在购物车, 直接加 (无规格) -> 点 .mBtnGroup_ho5pZr, calculateprice 立即触发
//   C. 商品不在购物车, 带规格 -> 点 .mBtnGroup_ho5pZr 弹规格 popup
//      -> 选规格 -> 点 .addToCartBtn_bQzcZn -> calculateprice 触发
//
// 关键 DOM 锚点:
//   .plus_mbY0hg          : 已加入购物车的 +
//   .minus_yMNqWq         : 已加入购物车的 -
//   .clickArea_qHUDMY     : +/- 内部实际接事件的 span
//   .mBtnGroup_ho5pZr     : 未加入购物车的"+"按钮 (单按钮)
//   .spec_AXWaUg          : 规格组 DL 容器
//   .specTitle_B3t6lm     : 规格组标题 (DT)
//   .tagContainer_5IqJXm  : 规格值容器 (DD)
//   .tag_nx79mf           : 单个规格值
//   .selected_G3xf3i      : 已选规格值的额外 class
//   .addToCartBtn_bQzcZn  : 规格 popup 内的"加入购物车"按钮
//
// params: { product_name: string, action: 'add'|'sub', spec_selections?: string[]|number[], wait_ms?: number }
//   product_name      : 商品名 (必填, 用来定位商品卡片)
//   action            : 'add' = 加 1, 'sub' = 减 1 (减只对已在购物车的 SKU 有效)
//   spec_selections   : 可选, 数组. 每项对应一个规格组, 可以是值的字符串/index/或不传走默认
//                       例: ["香辣鸡翅", "标准冰"] 或 [0, 0] 或省略走 default selected
//   wait_ms           : 等 calculateprice 响应的最长时间, 默认 5000
//
// returns: { success, mode, calculate_response, before_cart_count, after_cart_count, error? }

async (params) => {
  const { product_name, action = 'add', spec_selections, wait_ms = 5000 } = params || {}

  try {
    if (!location.pathname.includes('/mindex/menu')) {
      return { success: false, error_type: 'wrong_page', error: '当前不在店铺菜单页', current_url: location.href }
    }
    if (!product_name) {
      return { success: false, error_type: 'bad_param', error: '必须传 product_name' }
    }
    if (action !== 'add' && action !== 'sub') {
      return { success: false, error_type: 'bad_param', error: 'action 只能是 add 或 sub' }
    }

    // ---------- helpers ----------
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

    // 点击: 总是优先点元素内部的 .clickArea_qHUDMY (如果存在), 因为美团 React handler 绑在内层
    const fireClick = async (el) => {
      const target = el.querySelector('.clickArea_qHUDMY') || el
      target.scrollIntoView({ block: 'center', behavior: 'instant' })
      await sleep(250) // 等 scroll + React render 稳定
      const r = target.getBoundingClientRect()
      const cx = r.x + r.width / 2
      const cy = r.y + r.height / 2
      const t = new Touch({ identifier: 1, target, clientX: cx, clientY: cy, pageX: cx, pageY: cy, radiusX: 11, radiusY: 11, force: 1 })
      target.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, cancelable: true, composed: true, touches: [t], targetTouches: [t], changedTouches: [t] }))
      target.dispatchEvent(new TouchEvent('touchend', { bubbles: true, cancelable: true, composed: true, touches: [], targetTouches: [], changedTouches: [t] }))
      target.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window, clientX: cx, clientY: cy }))
    }

    // 匹配规则: 优先精确匹配, 否则 startsWith / includes (兼容用户传简称)
    const cartCountForProduct = (name) => {
      try {
        const raw = localStorage.getItem('cached_cart_data')
        if (!raw) return 0
        const store = JSON.parse(raw)
        const candidates = []
        for (const k of Object.keys(store)) {
          if (k === 'ids' || k === 'lastPoiId') continue
          const shop = store[k]
          if (!shop?.cartData) continue
          for (const c of shop.cartData) {
            for (const b of c.bills || []) {
              if (b.spuName === name) return b.count || 0
              if (b.spuName && (b.spuName.startsWith(name) || b.spuName.includes(name))) {
                candidates.push(b.count || 0)
              }
            }
          }
        }
        return candidates.length ? candidates[0] : 0
      } catch (_) { return 0 }
    }

    // 找包含 product_name 的 product card
    // 策略: 反向遍历所有 +/-/首加按钮, 取其最近祖先 (含 ¥ 的) 作为该商品的 card
    // 再判断该 card 的 innerText 是否含 product_name. 命中最短文本的 card 即正解
    const findProductCard = (name) => {
      const buttons = document.querySelectorAll('.plus_mbY0hg, .minus_yMNqWq, .mBtnGroup_ho5pZr')
      let bestCard = null
      let bestLen = Infinity
      for (const btn of buttons) {
        let c = btn
        for (let i = 0; i < 8 && c; i++) {
          const tt = c.innerText || ''
          if (tt.includes(name) && /¥/.test(tt) && tt.length < bestLen && tt.length < 400) {
            bestCard = c
            bestLen = tt.length
            break
          }
          c = c.parentElement
        }
      }
      return bestCard
    }

    // 装拦截器
    let calcResp = null
    const origOpen = XMLHttpRequest.prototype.open
    const origSend = XMLHttpRequest.prototype.send
    XMLHttpRequest.prototype.open = function (method, url, ...rest) {
      this.__mc_meta = { method, url }
      return origOpen.call(this, method, url, ...rest)
    }
    XMLHttpRequest.prototype.send = function (body) {
      const meta = this.__mc_meta
      if (meta && /calculateprice/.test(meta.url)) {
        const bodyText = typeof body === 'string' ? body : ''
        this.addEventListener('loadend', () => {
          try {
            const text = this.responseText || ''
            let parsed = null
            try { parsed = JSON.parse(text) } catch (_) {}
            calcResp = { status: this.status, parsed, request_body: bodyText.slice(0, 30000), response_text: text.slice(0, 30000) }
          } catch (_) {}
        })
      }
      return origSend.call(this, body)
    }

    const beforeCount = cartCountForProduct(product_name)
    let mode = ''

    try {
      const card = findProductCard(product_name)
      if (!card) {
        return { success: false, error_type: 'product_not_found', error: `菜单中找不到包含 "${product_name}" 的商品卡片`, before_cart_count: beforeCount }
      }

      if (action === 'sub') {
        // sub 只对已在购物车的有效
        if (beforeCount <= 0) {
          return { success: false, error_type: 'not_in_cart', error: '商品不在购物车, 不能 sub', before_cart_count: 0 }
        }
        const minus = card.querySelector('.minus_yMNqWq')
        if (!minus) return { success: false, error_type: 'no_minus_btn', before_cart_count: beforeCount }
        mode = 'in_cart_sub'
        await fireClick(minus)
      } else {
        // action = add: 看 DOM 实际是 +/- 控件 (.plus_mbY0hg) 还是首加按钮 (.mBtnGroup_ho5pZr)
        // 美团对单规格商品始终用 .plus_mbY0hg (即使 count=0); 仅多规格商品用 .mBtnGroup_ho5pZr 弹 popup
        const plus = card.querySelector('.plus_mbY0hg')
        const firstAdd = card.querySelector('.mBtnGroup_ho5pZr')
        if (plus) {
          mode = beforeCount > 0 ? 'in_cart_add' : 'first_add_no_spec_direct'
          await fireClick(plus)
        } else if (firstAdd) {
          await fireClick(firstAdd)
          // 等 popup 出现 (最多 1.5s)
          let popupOpened = false
          for (let i = 0; i < 15; i++) {
            await sleep(100)
            if (document.querySelector('.addToCartBtn_bQzcZn') || document.querySelector('[aria-label="关闭商品规格选择弹窗"]')) {
              popupOpened = true
              break
            }
            // 也可能没规格直接加成功了
            if (cartCountForProduct(product_name) > beforeCount) {
              mode = 'first_add_no_spec'
              break
            }
          }
          if (mode === '') {
            if (popupOpened) {
              mode = 'first_add_with_spec'
              // 处理每个规格组, 选择对应值
              const specGroups = document.querySelectorAll('.spec_AXWaUg')
              const sel = spec_selections || []
              for (let i = 0; i < specGroups.length; i++) {
                const group = specGroups[i]
                const tags = group.querySelectorAll('.tag_nx79mf')
                if (tags.length === 0) continue
                // 如果当前组已有 selected, 且 sel[i] 未指定, 跳过
                const alreadySelected = group.querySelector('.tag_nx79mf.selected_G3xf3i')
                const wanted = sel[i]
                let target = null
                if (wanted === undefined || wanted === null) {
                  if (alreadySelected) continue
                  target = tags[0]
                } else if (typeof wanted === 'number') {
                  target = tags[wanted]
                } else if (typeof wanted === 'string') {
                  for (const tag of tags) {
                    if ((tag.innerText || '').includes(wanted)) { target = tag; break }
                  }
                }
                if (!target) {
                  // 找不到目标值, 选默认第一个
                  if (alreadySelected) continue
                  target = tags[0]
                }
                await fireClick(target)
                await sleep(200)
              }
              // 点 加入购物车
              const addBtn = document.querySelector('.addToCartBtn_bQzcZn')
              if (!addBtn) return { success: false, error_type: 'no_popup_add_btn', mode, before_cart_count: 0 }
              await fireClick(addBtn)
            } else {
              return { success: false, error_type: 'popup_did_not_open', mode: 'first_add_unknown', before_cart_count: 0 }
            }
          }
        } else {
          return { success: false, error_type: 'no_add_button', error: '商品卡片中找不到 +/- 或首加按钮', before_cart_count: beforeCount }
        }
      }

      // 等 calculateprice 响应
      const start = Date.now()
      while (!calcResp && Date.now() - start < wait_ms) {
        await sleep(100)
      }
    } finally {
      XMLHttpRequest.prototype.open = origOpen
      XMLHttpRequest.prototype.send = origSend
    }

    const afterCount = cartCountForProduct(product_name)

    return {
      success: calcResp !== null && calcResp.parsed?.code === 0,
      mode,
      product_name,
      action,
      before_cart_count: beforeCount,
      after_cart_count: afterCount,
      calculate_response: calcResp,
    }
  } catch (e) {
    return { success: false, error_type: 'internal_error', error: e.message, stack: e.stack }
  }
}
