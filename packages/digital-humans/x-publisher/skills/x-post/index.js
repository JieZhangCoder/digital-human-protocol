/**
 * x-post — browser_run 脚本
 *
 * 在 X 主页内联编辑器发表一条新推。
 * 通过合成 ClipboardEvent + DataTransfer 注入文字（Draft.js 兼容），
 * 点击 [data-testid="tweetButtonInline"]，并通过 DOM 检测新推文确认成功。
 *
 * 契约：
 *   - 单个 async 箭头函数，由 browser_run 通过 evaluateScript 调用
 *   - 接收 params 对象作为第一个参数
 *   - 返回 JSON 可序列化的结果
 *   - 页面必须已导航到 https://x.com/home
 *   - 用户 session/cookies 自动可用
 *   - 错误以 { success: false, error_type, error } 形式返回，不抛出异常
 */
async (params) => {
  const { content } = params || {}
  const log = (...a) => console.log('[x-post]', ...a)
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

  // ------------------------------------
  // 1. 参数校验
  // ------------------------------------
  if (!content || typeof content !== 'string' || !content.trim()) {
    return { success: false, error_type: 'content_rejected', error: '推文内容不能为空' }
  }
  // X 默认上限 280 字（Twitter 字符计数规则简化版：纯文本按 codepoint 算）
  const charCount = [...content].length
  if (charCount > 280) {
    return { success: false, error_type: 'content_rejected', error: `推文超过 280 字符（当前 ${charCount}）` }
  }

  // ------------------------------------
  // 2. 当前登录用户 handle —— 用于后续验证
  // ------------------------------------
  const myHandle = (() => {
    const link = document.querySelector('[data-testid="AppTabBar_Profile_Link"]')
      || document.querySelector('a[aria-label="Profile"]')
      || document.querySelector('[data-testid="SideNav_AccountSwitcher_Button"] a')
    const href = link?.getAttribute('href')
    if (href && /^\/[A-Za-z0-9_]{1,15}$/.test(href)) return href.slice(1)
    // Fallback: scan timeline for own avatar testid
    const ownArticle = document.querySelector('[data-testid="primaryColumn"] a[role="link"][href^="/"][href*="/status/"]')
    return null
  })()
  // 不强制 myHandle 必须找到（有时 sidenav 还没渲染），后面通过 timeText==="Now" 兜底

  // ------------------------------------
  // 3. 找到编辑器
  // ------------------------------------
  const editor = document.querySelector('[data-testid="tweetTextarea_0"]')
  if (!editor) {
    return { success: false, error_type: 'selector_not_found', error: '未找到推文编辑器（[data-testid=tweetTextarea_0]）' }
  }
  log('编辑器已找到')

  // ------------------------------------
  // 4. 注入文字（ClipboardEvent + DataTransfer，Draft.js 兼容）
  // ------------------------------------
  editor.focus()
  await sleep(120)
  try {
    const dt = new DataTransfer()
    dt.setData('text/plain', content)
    editor.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }))
  } catch (e) {
    return { success: false, error_type: 'selector_not_found', error: 'ClipboardEvent 注入异常: ' + e.message }
  }
  await sleep(600) // 等 Draft.js editorState 更新

  const editorText = editor.innerText || ''
  if (!editorText.trim() || !editorText.includes(content.slice(0, Math.min(20, content.length)))) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: 'ClipboardEvent 注入未生效，编辑器仍为空或内容不匹配',
      editor_preview: editorText.slice(0, 60),
    }
  }
  log('文字注入成功，长度:', editorText.length)

  // ------------------------------------
  // 5. 找到 Post 按钮，校验未 disabled
  // ------------------------------------
  const btn = document.querySelector('[data-testid="tweetButtonInline"]')
    || document.querySelector('[data-testid="tweetButton"]')
  if (!btn) {
    return { success: false, error_type: 'selector_not_found', error: '未找到 Post 按钮（[data-testid=tweetButtonInline]）' }
  }
  const isDisabled = btn.disabled || btn.getAttribute('aria-disabled') === 'true'
  if (isDisabled) {
    return {
      success: false,
      error_type: 'content_rejected',
      error: 'Post 按钮处于 disabled 状态（可能内容被风控/超长/含违规链接）',
    }
  }
  log('Post 按钮可点击')

  // ------------------------------------
  // 6. 记录点击前 timeline 顶部的 tweet IDs，避免误把旧推当新推
  // ------------------------------------
  const beforeIds = new Set(
    Array.from(document.querySelectorAll('[data-testid="tweet"] a[href*="/status/"]'))
      .map((a) => a.getAttribute('href')?.match(/\/status\/(\d+)/)?.[1])
      .filter(Boolean),
  )
  log('点击前 timeline 推文数:', beforeIds.size)

  // ------------------------------------
  // 7. 点击 Post
  // ------------------------------------
  btn.click()
  log('已点击 Post')

  // ------------------------------------
  // 8. 等待新推出现（最多 10 秒）
  // ------------------------------------
  const deadline = Date.now() + 10000
  let newArticle = null
  while (Date.now() < deadline) {
    await sleep(300)
    const articles = document.querySelectorAll('[data-testid="tweet"]')
    for (const art of articles) {
      const time = art.querySelector('time')
      const link = time?.closest('a')?.getAttribute('href') || ''
      const m = link.match(/^\/([A-Za-z0-9_]+)\/status\/(\d+)$/)
      if (!m) continue
      const [, handle, id] = m
      if (beforeIds.has(id)) continue // 已存在的推
      // 必须是自己发的：handle 匹配 myHandle 或 timeText === "Now"
      const timeText = (time?.textContent || '').trim()
      const isFresh = timeText === 'Now' || timeText === 'now' || timeText === '刚刚' || timeText === '现在'
      const isMine = myHandle && handle.toLowerCase() === myHandle.toLowerCase()
      if (isFresh || isMine) {
        newArticle = { id, handle, timeText, dateTime: time?.dateTime }
        break
      }
    }
    if (newArticle) break
  }

  if (!newArticle) {
    return {
      success: false,
      error_type: 'verification_timeout',
      error: '已点击 Post，但 10 秒内未在 timeline 检测到新推文（可能已发出但未刷新到本页）',
    }
  }

  log('成功:', newArticle)
  return {
    success: true,
    tweet_id: newArticle.id,
    tweet_url: `https://x.com${`/${newArticle.handle}/status/${newArticle.id}`}`,
    author_handle: newArticle.handle,
  }
}
