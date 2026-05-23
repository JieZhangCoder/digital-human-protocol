/**
 * zhihu-publish-pin — browser_run 脚本
 *
 * 在知乎主页通过内联「发想法」入口发布一条短动态。
 *
 * 路径：
 *   1. 找当前用户 url_token（用于查 pin 列表）
 *   2. 点击发前先 GET /api/v4/members/{handle}/pins 拿到既有 pin id 集合
 *   3. 找首页 button[text="发想法"]，点击展开内联表单
 *   4. 找到 .public-DraftEditor-content 编辑器，先清空再 paste 注入
 *   5. 等发布按钮 enabled，点击
 *   6. 轮询 GET /pins 列表（最多 12 秒），新出现的 id 即为本次发布的 pin
 *
 * 不依赖 fetch/XHR 拦截 —— 知乎 SPA 在 chunk 加载早期缓存了 fetch 引用，
 * 后置 wrap 抓不到 publish 调用。直接查询 user pin 列表 API 100% 可靠。
 *
 * 契约：
 *   - 单个 async 箭头函数，由 browser_run 通过 evaluateScript 调用
 *   - 接收 params 对象作为第一个参数
 *   - 返回 JSON 可序列化的结果
 *   - 页面必须已导航到 https://www.zhihu.com/
 *   - 用户 session/cookies 自动可用
 *   - 错误以 { success: false, error_type, error } 形式返回，不抛出异常
 */
async (params) => {
  const { content } = params || {}
  const log = (...a) => console.log('[zhihu-publish-pin]', ...a)
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

  // ────────────────────────────────────────
  // 1. 参数校验
  // ────────────────────────────────────────
  if (!content || typeof content !== 'string' || !content.trim()) {
    return { success: false, error_type: 'content_rejected', error: '想法内容不能为空' }
  }
  const charCount = [...content].length
  if (charCount > 1000) {
    return {
      success: false,
      error_type: 'content_rejected',
      error: `想法超过 1000 字符（当前 ${charCount}）`,
    }
  }

  // ────────────────────────────────────────
  // 2. 取当前用户 url_token —— 用于查 pin 列表
  // ────────────────────────────────────────
  // 优先从 /api/v4/me 拿（最准确）；fallback 走 DOM
  async function fetchSelfHandle() {
    try {
      const r = await fetch('https://www.zhihu.com/api/v4/me?include=is_realname', {
        credentials: 'include',
      })
      if (r.ok) {
        const me = await r.json()
        if (me.url_token) return me.url_token
      }
    } catch {}
    // DOM fallback
    const a = document.querySelector('a[href^="/people/"]')
    const m = a?.getAttribute('href')?.match(/^\/people\/([^/?#]+)/)
    return m ? m[1] : null
  }
  const myHandle = await fetchSelfHandle()
  if (!myHandle) {
    return { success: false, error_type: 'auth_expired', error: '未检测到登录用户（无法获取 url_token）' }
  }
  log('当前用户 handle:', myHandle)

  async function listPins(limit = 5) {
    try {
      const r = await fetch(
        `https://www.zhihu.com/api/v4/members/${encodeURIComponent(myHandle)}/pins?limit=${limit}&offset=0`,
        { credentials: 'include' },
      )
      if (!r.ok) return null
      const d = await r.json()
      return Array.isArray(d.data) ? d.data : []
    } catch {
      return null
    }
  }

  // ────────────────────────────────────────
  // 3. 点击前的 pin 列表快照
  // ────────────────────────────────────────
  const beforePins = await listPins(10)
  if (beforePins === null) {
    return { success: false, error_type: 'auth_expired', error: '查询 pin 列表失败（可能登录态失效）' }
  }
  const beforeIds = new Set(beforePins.map((p) => String(p.id)))
  log('点击前 pin 数量:', beforeIds.size)

  // ────────────────────────────────────────
  // 4. 找「发想法」入口 —— 文字精确匹配
  // ────────────────────────────────────────
  function findEntryButton() {
    const btns = Array.from(document.querySelectorAll('button'))
    return btns.find((b) => (b.textContent || '').trim() === '发想法')
  }
  const entryBtn = findEntryButton()
  if (!entryBtn) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: '未找到「发想法」入口（未登录或入口隐藏）',
    }
  }
  log('找到「发想法」入口')

  // ────────────────────────────────────────
  // 5. 找编辑器（如未显示，点击入口展开）
  // ────────────────────────────────────────
  let editor =
    document.querySelector('.WritePinV2-Form .public-DraftEditor-content') ||
    document.querySelector('.public-DraftEditor-content')
  if (!editor) {
    entryBtn.click()
    const deadline = Date.now() + 4000
    while (Date.now() < deadline) {
      await sleep(150)
      editor =
        document.querySelector('.WritePinV2-Form .public-DraftEditor-content') ||
        document.querySelector('.public-DraftEditor-content')
      if (editor) break
    }
  }
  if (!editor) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: '点击发想法后未找到 Draft.js 编辑器（.public-DraftEditor-content）',
    }
  }
  log('编辑器已找到')

  // ────────────────────────────────────────
  // 6. 编辑器先清空（处理残留草稿）
  // ────────────────────────────────────────
  editor.focus()
  await sleep(100)

  const existingText = (editor.innerText || '').replace(/\u200B|\n/g, '').trim()
  if (existingText) {
    log('编辑器有残留:', existingText.slice(0, 40))
    try {
      const range = document.createRange()
      range.selectNodeContents(editor)
      const sel = window.getSelection()
      sel.removeAllRanges()
      sel.addRange(range)
      document.execCommand('delete', false, null)
      await sleep(200)
    } catch {}
    const after = (editor.innerText || '').replace(/\u200B|\n/g, '').trim()
    if (after) {
      return {
        success: false,
        error_type: 'selector_not_found',
        error: '编辑器有残留内容且无法清空（残留: ' + after.slice(0, 40) + '）',
      }
    }
  }

  // ────────────────────────────────────────
  // 7. ClipboardEvent + DataTransfer 注入文字
  // ────────────────────────────────────────
  try {
    const dt = new DataTransfer()
    dt.setData('text/plain', content)
    editor.dispatchEvent(
      new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }),
    )
  } catch (e) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: 'ClipboardEvent 注入异常: ' + e.message,
    }
  }
  await sleep(700)

  const editorText = (editor.innerText || '').replace(/\n+$/, '')
  if (
    !editorText.trim() ||
    !editorText.includes(content.slice(0, Math.min(20, content.length)))
  ) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: 'ClipboardEvent 注入未生效，编辑器仍为空或内容不匹配',
      editor_preview: editorText.slice(0, 60),
    }
  }
  log('文字注入成功，长度:', editorText.length)

  // ────────────────────────────────────────
  // 8. 找发布按钮 —— 在 .WritePinToolbar 范围内
  // ────────────────────────────────────────
  function findPublishButton() {
    const btns = Array.from(document.querySelectorAll('button'))
    const inToolbar = btns.filter((b) => b.closest('[class*="WritePinToolbar"]'))
    let b = inToolbar.find((b) => (b.textContent || '').trim() === '发布')
    if (b) return b
    b = inToolbar.find((b) => /^发布/.test((b.textContent || '').trim()))
    if (b) return b
    b = btns.find((b) => {
      const t = (b.textContent || '').trim()
      return t === '发布'
    })
    return b || null
  }
  let publishBtn = null
  const enableDeadline = Date.now() + 3000
  while (Date.now() < enableDeadline) {
    publishBtn = findPublishButton()
    if (publishBtn) {
      const dis = publishBtn.disabled || publishBtn.getAttribute('aria-disabled') === 'true'
      if (!dis) break
    }
    await sleep(150)
  }
  if (!publishBtn) {
    return { success: false, error_type: 'selector_not_found', error: '未找到「发布」按钮' }
  }
  if (publishBtn.disabled || publishBtn.getAttribute('aria-disabled') === 'true') {
    return {
      success: false,
      error_type: 'content_rejected',
      error: '发布按钮处于 disabled 状态（可能内容被风控/超长/含违规链接）',
    }
  }
  log('发布按钮可点击')

  // ────────────────────────────────────────
  // 9. 点击发布
  // ────────────────────────────────────────
  publishBtn.click()
  log('已点击发布')

  // ────────────────────────────────────────
  // 10. 轮询 pin 列表，发现新 id
  // ────────────────────────────────────────
  const deadline = Date.now() + 12000
  while (Date.now() < deadline) {
    await sleep(700)
    const after = await listPins(10)
    if (!after) continue
    const novel = after.filter((p) => !beforeIds.has(String(p.id)))
    if (novel.length > 0) {
      // 取最新（created 最大）的
      novel.sort((a, b) => (b.created || 0) - (a.created || 0))
      const fresh = novel[0]
      const pinId = String(fresh.id)
      // canonical 公开链接是 /pin/{id}（单数）。列表 API 返回的 fresh.url
      // 是 /pins/{id}（复数），是列表内部路径。统一构造为 /pin/{id}。
      const pinUrl = `https://www.zhihu.com/pin/${pinId}`
      const reviewing = !!fresh.reviewing_info?.is_reviewing
      log('发现新 pin:', pinId, 'reviewing:', reviewing)
      return {
        success: true,
        pin_id: pinId,
        pin_url: pinUrl,
        char_count: charCount,
        verified_via: 'pinlist',
        ...(reviewing
          ? {
              reviewing: true,
              reviewing_reason: fresh.reviewing_info?.reason,
              reviewing_tips: fresh.reviewing_info?.tips,
            }
          : {}),
      }
    }
  }

  return {
    success: false,
    error_type: 'verification_timeout',
    error: '已点击发布，但 12 秒内未在 pin 列表里发现新 id（可能被风控拦截，或网络异常）',
  }
}
