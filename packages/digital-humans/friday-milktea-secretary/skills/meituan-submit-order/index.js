// meituan-submit-order: 在 preview 页提交订单, 生成待支付订单
// 关键: submit 后页面会跳到 sqt.meituan.com cashier (跨 host), probe 会失效
// 解法: 在点击前安装 XHR loadend hook, 把 submit 响应同步写入 localStorage
//      (waimai.meituan.com origin); 同时把 hook 内同步 Promise resolve, 抓住 0~100ms 窗口
//
// params: { wait_ms?: number }
// returns: { success, submit_response?, order_id?, pay_url?, cashier_serial_num?, error? }

async (params) => {
  const { wait_ms = 8000 } = params || {}

  try {
    if (!location.pathname.includes('/mindex/preview')) {
      return { success: false, error_type: 'wrong_page', error: '当前不在 preview 页', current_url: location.href }
    }

    // 找提交按钮
    const btn = document.querySelector('button.submit_QDYt9D')
    if (!btn) {
      return { success: false, error_type: 'no_submit_button', error: '未找到 "提交订单" 按钮' }
    }

    // 安装 XHR loadend hook, 同步写 localStorage, race promise
    let submitResp = null
    const origOpen = XMLHttpRequest.prototype.open
    const origSend = XMLHttpRequest.prototype.send
    XMLHttpRequest.prototype.open = function (method, url, ...rest) {
      this.__mt_sb_meta = { method, url }
      return origOpen.call(this, method, url, ...rest)
    }
    XMLHttpRequest.prototype.send = function (body) {
      const meta = this.__mt_sb_meta
      // submit endpoint 未知, 用宽泛匹配: /submit, /order/v2/, /order/wm/, 排除 /preview /calculateprice
      if (meta && /openh5\/order/.test(meta.url) && /submit|wm\/submit|order\/v2\/submit/i.test(meta.url) === false) {
        // 等待 loadend 看是不是 submit
      }
      if (meta && /openh5\/order\/(v2\/submit|wm\/submit|submit)/i.test(meta.url)) {
        const bodyText = typeof body === 'string' ? body : ''
        this.addEventListener('loadend', () => {
          try {
            const text = this.responseText || ''
            let parsed = null
            try { parsed = JSON.parse(text) } catch (_) {}
            submitResp = {
              url: meta.url, status: this.status,
              request_body: bodyText.length > 30000 ? bodyText.slice(0, 30000) : bodyText,
              response_parsed: parsed,
              response_text: text.length > 30000 ? text.slice(0, 30000) : text,
            }
            try {
              localStorage.setItem('__mt_last_submit_resp', JSON.stringify({ ...submitResp, captured_at: Date.now() }))
            } catch (_) {}
          } catch (_) {}
        })
      }
      return origSend.call(this, body)
    }

    try {
      // click 序列
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

      // 等捕获响应 或 直到页面跳走 (window.location.href 变化)
      const startUrl = location.href
      const start = Date.now()
      while (!submitResp && Date.now() - start < wait_ms) {
        await new Promise((r) => setTimeout(r, 100))
        if (location.href !== startUrl && !location.href.includes('/preview')) {
          // 页面已经跳走, submit 响应若还没捕获, 说明 loadend 还没触发
          // 再等 200ms
          await new Promise((r) => setTimeout(r, 200))
          break
        }
      }
    } finally {
      try {
        XMLHttpRequest.prototype.open = origOpen
        XMLHttpRequest.prototype.send = origSend
      } catch (_) {}
    }

    // fallback: 从 localStorage 读
    if (!submitResp) {
      try {
        const raw = localStorage.getItem('__mt_last_submit_resp')
        if (raw) submitResp = JSON.parse(raw)
      } catch (_) {}
    }

    // 从 URL 提取 cashier serialNum (跨 host 跳转后)
    let cashierSerial = null
    const m = location.href.match(/[?&#]serialNum=([A-Z0-9]+)/i)
    if (m) cashierSerial = m[1]

    if (!submitResp && !cashierSerial) {
      return { success: false, error_type: 'no_response', error: '未捕获 submit 响应, 也未跳到收银台', current_url: location.href }
    }

    const data = submitResp?.response_parsed?.data
    return {
      success: true,
      current_url: location.href,
      cashier_serial_num: cashierSerial,
      submit_response: submitResp,
      order_id: data?.wm_order_id_view_str || data?.orderViewId || data?.order_id || null,
      pay_url: data?.pay_url || data?.cashier_url || null,
      hint: submitResp ? '已抓到 submit 接口响应' : '只拿到了收银台 serialNum, 完整响应需走 myuncompleteorder 查',
    }
  } catch (e) {
    return { success: false, error_type: 'internal_error', error: e.message, stack: e.stack }
  }
}
