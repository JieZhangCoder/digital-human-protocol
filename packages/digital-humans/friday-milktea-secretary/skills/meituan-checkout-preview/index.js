// meituan-checkout-preview: 触发结算预览, 捕获 preview API 完整响应
// 三种模式:
//   on_menu  : 当前在店铺菜单页, 点 "去结算" 跳到 preview, 捕获接口
//   on_preview: 当前已在 preview 页, 选地址触发 re-preview, 或直接读最近响应
//   passive  : 只安装拦截器等待页面自然触发 preview
//
// params: { mode?: 'auto'|'on_menu'|'on_preview'|'passive', wait_ms?: number }
// returns: { success, preview_response: { request_body, response_parsed }, error? }

async (params) => {
  const { mode = 'auto', wait_ms = 8000 } = params || {}

  try {
    // 安装拦截器
    let lastPreview = null
    const origOpen = XMLHttpRequest.prototype.open
    const origSend = XMLHttpRequest.prototype.send
    XMLHttpRequest.prototype.open = function (method, url, ...rest) {
      this.__mt_pv_meta = { method, url, t0: Date.now() }
      return origOpen.call(this, method, url, ...rest)
    }
    XMLHttpRequest.prototype.send = function (body) {
      const meta = this.__mt_pv_meta
      if (meta && /openh5\/order\/v2\/preview/.test(meta.url)) {
        const bodyText = typeof body === 'string' ? body : ''
        this.addEventListener('loadend', () => {
          try {
            const text = this.responseText || ''
            const dataParam = decodeURIComponent((bodyText.match(/data=([^&]+)/) || [])[1] || '')
            let req = null
            try { req = JSON.parse(dataParam) } catch (_) {}
            let resp = null
            try { resp = JSON.parse(text) } catch (_) {}
            lastPreview = {
              url: meta.url,
              status: this.status,
              request_decoded: req,
              response_parsed: resp,
              response_text: text.length > 50000 ? text.slice(0, 50000) : text,
            }
          } catch (_) {}
        })
      }
      return origSend.call(this, body)
    }

    try {
      const path = location.pathname
      let resolvedMode = mode
      if (resolvedMode === 'auto') {
        if (path.includes('/mindex/menu')) resolvedMode = 'on_menu'
        else if (path.includes('/mindex/preview')) resolvedMode = 'on_preview'
        else resolvedMode = 'passive'
      }

      if (resolvedMode === 'on_menu') {
        const btn = document.querySelector('.goToPreview_oTQlfa')
        if (!btn) return { success: false, error_type: 'no_settle_button', error: '未找到 "去结算" 按钮，是否未达起送线？' }
        const r = btn.getBoundingClientRect()
        if (r.y < 0 || r.y > window.innerHeight) {
          btn.scrollIntoView({ block: 'center' })
          await new Promise((r) => setTimeout(r, 300))
        }
        const r2 = btn.getBoundingClientRect()
        const cx = r2.x + r2.width / 2, cy = r2.y + r2.height / 2
        const t = new Touch({ identifier: 1, target: btn, clientX: cx, clientY: cy, pageX: cx, pageY: cy, radiusX: 11, radiusY: 11, force: 1 })
        btn.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, cancelable: true, composed: true, touches: [t], targetTouches: [t], changedTouches: [t] }))
        btn.dispatchEvent(new TouchEvent('touchend', { bubbles: true, cancelable: true, composed: true, touches: [], targetTouches: [], changedTouches: [t] }))
        btn.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window, clientX: cx, clientY: cy }))
      } else if (resolvedMode === 'on_preview') {
        // 已经在 preview 页, 触发 re-preview: 重新点一次地址区域可能 OK
        // 但更可靠的策略是 wait, 因为打开 preview 页时本身会 fire preview
        // 这里不做额外触发, 等待 wait_ms 内的自然请求
      }

      // 等响应
      const start = Date.now()
      while (!lastPreview && Date.now() - start < wait_ms) {
        await new Promise((r) => setTimeout(r, 150))
      }
    } finally {
      XMLHttpRequest.prototype.open = origOpen
      XMLHttpRequest.prototype.send = origSend
    }

    if (!lastPreview) {
      return { success: false, error_type: 'no_response', error: 'preview API 未在等待时间内返回', current_url: location.href }
    }

    const data = lastPreview.response_parsed?.data
    return {
      success: lastPreview.response_parsed?.code === 0,
      current_url: location.href,
      request_summary: lastPreview.request_decoded ? {
        poi_id_str: lastPreview.request_decoded.poi_id_str,
        addr_id: lastPreview.request_decoded.addr_id,
        foodlist: lastPreview.request_decoded.foodlist,
        recipient_name: lastPreview.request_decoded.recipient_name,
        recipient_phone: lastPreview.request_decoded.recipient_phone,
        recipient_address: lastPreview.request_decoded.recipient_address,
        house_number: lastPreview.request_decoded.house_number,
      } : null,
      preview_response: lastPreview,
      summary: data ? {
        has_address: !!(lastPreview.request_decoded?.addr_id),
        coupon_count: (data.coupon_info_listIterator || []).length,
        unavailable_food_count: data.unAvailableFoodListSize || 0,
      } : null,
    }
  } catch (e) {
    return { success: false, error_type: 'internal_error', error: e.message, stack: e.stack }
  }
}
