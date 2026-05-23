/**
 * x-reply — browser_run 脚本
 *
 * 在 X 推文详情页发表一条回复。
 * 调用前编排层须 browser_navigate 到目标推文详情页 (/{author}/status/{id})。
 * skill 通过合成 ClipboardEvent 注入文字到 Draft.js 编辑器，点击 Reply 按钮，
 * 并通过 DOM 检测对话流中新出现的本人 article 确认成功。
 *
 * 契约：
 *   - 单个 async 箭头函数，由 browser_run 通过 evaluateScript 调用
 *   - 接收 params 对象作为第一个参数
 *   - 返回 JSON 可序列化的结果
 *   - 错误以 { success: false, error_type, error } 形式返回，不抛出异常
 */
async (params) => {
  const { content } = params || {}
  const log = (...a) => console.log('[x-reply]', ...a)
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

  // ------------------------------------
  // 1. 参数 + URL 校验
  // ------------------------------------
  if (!content || typeof content !== 'string' || !content.trim()) {
    return { success: false, error_type: 'content_rejected', error: '回复内容不能为空' }
  }
  const charCount = [...content].length
  if (charCount > 280) {
    return { success: false, error_type: 'content_rejected', error: `回复超过 280 字符（当前 ${charCount}）` }
  }
  const urlMatch = location.pathname.match(/^\/([A-Za-z0-9_]+)\/status\/(\d+)/)
  if (!urlMatch) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: '当前 URL 不是 X 推文详情页（应为 /{author}/status/{id}）',
      current_url: location.href,
    }
  }
  const targetAuthor = urlMatch[1]
  const targetTweetId = urlMatch[2]
  log('目标推:', targetAuthor, targetTweetId)

  // ------------------------------------
  // 2. 自己的 handle
  // ------------------------------------
  const myHandle = (() => {
    const link = document.querySelector('[data-testid="AppTabBar_Profile_Link"]')
      || document.querySelector('a[aria-label="Profile"]')
    const href = link?.getAttribute('href')
    if (href && /^\/[A-Za-z0-9_]{1,15}$/.test(href)) return href.slice(1)
    return null
  })()

  // ------------------------------------
  // 3. 找到编辑器
  // ------------------------------------
  const editor = document.querySelector('[data-testid="tweetTextarea_0"]')
  if (!editor) {
    return { success: false, error_type: 'selector_not_found', error: '未找到回复编辑器' }
  }

  // ------------------------------------
  // 4. 注入文字
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
  await sleep(600)

  const editorText = editor.innerText || ''
  if (!editorText.trim() || !editorText.includes(content.slice(0, Math.min(20, content.length)))) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: 'ClipboardEvent 注入未生效',
      editor_preview: editorText.slice(0, 60),
    }
  }

  // ------------------------------------
  // 5. 点击 Reply
  // ------------------------------------
  const btn = document.querySelector('[data-testid="tweetButtonInline"]')
    || document.querySelector('[data-testid="tweetButton"]')
  if (!btn) {
    return { success: false, error_type: 'selector_not_found', error: '未找到 Reply 按钮' }
  }
  if (btn.disabled || btn.getAttribute('aria-disabled') === 'true') {
    return {
      success: false,
      error_type: 'content_rejected',
      error: 'Reply 按钮处于 disabled 状态',
    }
  }

  // 记录点击前的 tweet IDs
  const beforeIds = new Set(
    Array.from(document.querySelectorAll('[data-testid="tweet"] a[href*="/status/"]'))
      .map((a) => a.getAttribute('href')?.match(/\/status\/(\d+)/)?.[1])
      .filter(Boolean),
  )

  btn.click()
  log('已点击 Reply')

  // ------------------------------------
  // 6. 等待新回复出现（最多 12 秒）
  // ------------------------------------
  const deadline = Date.now() + 12000
  let replyArticle = null
  while (Date.now() < deadline) {
    await sleep(300)
    const articles = document.querySelectorAll('[data-testid="tweet"]')
    for (const art of articles) {
      const time = art.querySelector('time')
      const link = time?.closest('a')?.getAttribute('href') || ''
      const m = link.match(/^\/([A-Za-z0-9_]+)\/status\/(\d+)$/)
      if (!m) continue
      const [, handle, id] = m
      if (id === targetTweetId) continue
      if (beforeIds.has(id)) continue
      const timeText = (time?.textContent || '').trim()
      const isFresh = timeText === 'Now' || timeText === 'now' || timeText === '刚刚' || timeText === '现在'
      const isMine = myHandle && handle.toLowerCase() === myHandle.toLowerCase()
      if (isFresh || isMine) {
        replyArticle = { id, handle, timeText }
        break
      }
    }
    if (replyArticle) break
  }

  if (!replyArticle) {
    return {
      success: false,
      error_type: 'verification_timeout',
      error: '已点击 Reply，未在 12 秒内检测到新回复',
      in_reply_to_tweet_id: targetTweetId,
    }
  }

  return {
    success: true,
    reply_id: replyArticle.id,
    reply_url: `https://x.com/${replyArticle.handle}/status/${replyArticle.id}`,
    in_reply_to_tweet_id: targetTweetId,
    in_reply_to_url: `https://x.com/${targetAuthor}/status/${targetTweetId}`,
  }
}
