// meituan-order-status: 查询美团外卖订单列表 (含待支付/进行中/已完成/已取消)
// 实捕 endpoint: GET https://i.waimai.meituan.com/openh5/order/list
// 触发方式: 点底部 nav "订单", SPA 跳到 /mindex/olist 自动 fire 接口, 拦截响应
//
// payStatus 取值: 1=已取消, 3=已完成, 其他状态见 orderStatusStr (中文)
//
// params: { wait_ms?: number, navigate?: boolean, status_filter?: string }
// returns: { success, order_count, orders[], raw_response?, error? }

async (params) => {
  const { wait_ms = 8000, navigate = true, status_filter = '' } = params || {}

  try {
    let lastResp = null
    const origOpen = XMLHttpRequest.prototype.open
    const origSend = XMLHttpRequest.prototype.send
    XMLHttpRequest.prototype.open = function (method, url, ...rest) {
      this.__mt_os_meta = { method, url }
      return origOpen.call(this, method, url, ...rest)
    }
    XMLHttpRequest.prototype.send = function (body) {
      const meta = this.__mt_os_meta
      if (meta && /openh5\/order\/list/i.test(meta.url)) {
        this.addEventListener('loadend', () => {
          try {
            const text = this.responseText || ''
            let parsed = null
            try { parsed = JSON.parse(text) } catch (_) {}
            // 累积多次响应 (分页时会有多次)
            if (!lastResp) lastResp = { url: meta.url, status: this.status, parsed, text: text.length > 50000 ? text.slice(0, 50000) : text }
            else lastResp = { url: meta.url, status: this.status, parsed, text: text.length > 50000 ? text.slice(0, 50000) : text, _multi: true }
          } catch (_) {}
        })
      }
      return origSend.call(this, body)
    }

    try {
      if (navigate && !location.pathname.includes('/mindex/olist') && !location.pathname.includes('/order/list')) {
        // 找底部 nav "订单" link, 走 SPA 内导航
        const orderLink = Array.from(document.querySelectorAll('a, [role="link"]')).find((a) => (a.innerText || '').trim() === '订单')
        if (orderLink) {
          const r = orderLink.getBoundingClientRect()
          const cx = r.x + r.width / 2, cy = r.y + r.height / 2
          const t = new Touch({ identifier: 1, target: orderLink, clientX: cx, clientY: cy, pageX: cx, pageY: cy, radiusX: 11, radiusY: 11, force: 1 })
          orderLink.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, cancelable: true, composed: true, touches: [t], targetTouches: [t], changedTouches: [t] }))
          orderLink.dispatchEvent(new TouchEvent('touchend', { bubbles: true, cancelable: true, composed: true, touches: [], targetTouches: [], changedTouches: [t] }))
          orderLink.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window, clientX: cx, clientY: cy }))
        }
      }

      const start = Date.now()
      while (!lastResp && Date.now() - start < wait_ms) {
        await new Promise((r) => setTimeout(r, 200))
      }
    } finally {
      XMLHttpRequest.prototype.open = origOpen
      XMLHttpRequest.prototype.send = origSend
    }

    if (!lastResp) {
      return { success: false, error_type: 'no_response', error: 'order/list 未在等待时间内返回', current_url: location.href }
    }

    const data = lastResp.parsed?.data || {}
    const rawList = Array.isArray(data.orderList) ? data.orderList : []

    const orders = rawList.map((o) => ({
      order_id: o.orderId,
      mt_order_view_id: o.mtOrderViewId,
      shop_name: o.shopName,
      poi_id_str: o.poi_id_str,
      mt_wm_poi_id: o.mtWmPoiId,
      total_price: parseFloat(o.totalPrice) || 0,
      order_time: o.orderTime,
      order_time_sec: o.orderTimeSec,
      pay_status: o.payStatus,            // 1=已取消, 3=已完成 (其余看 status_str)
      status_str: o.orderStatusStr,
      products: (o.productList || []).map((p) => ({ name: p.productName, count: p.productCount, spu_id: p.spuId })),
      buttons: (o.buttonList || []).map((b) => ({ title: (b.title || '').trim(), type: b.type })),
      scheme: o.scheme,
      pic_url: o.img,
      can_delete: o.canDelete === 1,
    }))

    const filtered = status_filter ? orders.filter((o) => o.status_str === status_filter) : orders

    return {
      success: lastResp.parsed?.code === 0,
      current_url: location.href,
      order_count: filtered.length,
      total_count_in_response: orders.length,
      is_end: data.isEnd,
      next_start_index: data.nextStartIndex,
      orders: filtered,
      raw_response: lastResp,
    }
  } catch (e) {
    return { success: false, error_type: 'internal_error', error: e.message, stack: e.stack }
  }
}
