/**
 * x-search — browser_run 脚本
 *
 * 从 X 搜索结果页 DOM 提取推文列表。
 * 调用前编排层须导航到 /search?q={query}&src=typed_query[&f=live|user]
 * skill 解析 timeline 中的所有 [data-testid="tweet"] article，可按 limit / scroll_pages 翻页。
 *
 * 契约：
 *   - 单个 async 箭头函数
 *   - 接收 params 对象作为第一个参数
 *   - 返回 JSON 可序列化的结果
 *   - 错误以 { success: false, error_type, error } 形式返回，不抛出异常
 */
async (params) => {
  const { limit = 20, scroll_pages = 1, min_likes = 0 } = params || {}
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
  const log = (...a) => console.log('[x-search]', ...a)

  // ------------------------------------
  // 1. URL 校验
  // ------------------------------------
  if (!/\/search/.test(location.pathname)) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: '当前 URL 不是 X 搜索结果页',
      current_url: location.href,
      expected_url_pattern: 'https://x.com/search?q=...',
    }
  }
  const usp = new URLSearchParams(location.search)
  const query = usp.get('q') || ''
  const f = usp.get('f') || 'top'
  const search_type = f === 'live' ? 'latest' : f === 'user' ? 'people' : 'top'

  // ------------------------------------
  // 2. 检测受限提示
  // ------------------------------------
  const bodyText = document.body?.innerText || ''
  if (/Something went wrong|Try reloading|Sign in to X|Log in/i.test(bodyText) && !document.querySelector('[data-testid="tweet"]')) {
    return { success: false, error_type: 'auth_expired', error: '搜索页提示登录或异常，可能未登录' }
  }

  // ------------------------------------
  // 3. 滚动加载 + 收集 article
  // ------------------------------------
  const seenIds = new Set()
  const collected = []
  const maxScrolls = Math.max(1, Math.min(scroll_pages, 10))

  for (let scrollIdx = 0; scrollIdx < maxScrolls; scrollIdx++) {
    // 解析当前可见的 article
    const articles = document.querySelectorAll('[data-testid="tweet"]')
    log(`scroll ${scrollIdx}: ${articles.length} articles in DOM`)
    for (const art of articles) {
      const parsed = parseArticle(art)
      if (!parsed) continue
      if (seenIds.has(parsed.id)) continue
      seenIds.add(parsed.id)
      if (parsed.metrics.likes != null && parsed.metrics.likes < min_likes) continue
      collected.push(parsed)
      if (collected.length >= limit) break
    }
    if (collected.length >= limit) break

    // 滚到底部触发懒加载
    if (scrollIdx < maxScrolls - 1) {
      window.scrollTo({ top: document.body.scrollHeight, behavior: 'auto' })
      await sleep(1500)
    }
  }

  // 不完全成功也返回（搜索本来就可能少于 limit）
  // 估算 has_more：如果还能往下滚 + 上一轮新增 > 0，认为还有
  const beforeHeight = document.body.scrollHeight
  window.scrollTo({ top: beforeHeight, behavior: 'auto' })
  await sleep(800)
  const has_more = document.body.scrollHeight > beforeHeight && collected.length >= limit

  return {
    success: true,
    query,
    search_type,
    results: collected.slice(0, limit),
    total: collected.length,
    has_more,
  }

  // ------------------------------------
  // helpers
  // ------------------------------------
  function parseArticle(article) {
    if (!article) return null
    const timeEl = article.querySelector('time')
    const timeLink = timeEl?.closest('a')
    const tweetUrl = timeLink?.getAttribute('href') || ''
    const idMatch = tweetUrl.match(/^\/([A-Za-z0-9_]+)\/status\/(\d+)$/)
    if (!idMatch) return null
    const [, handle, id] = idMatch

    const userNameEl = article.querySelector('[data-testid="User-Name"]')
    const nameSpan = userNameEl?.querySelector('a[href^="/"][role="link"] span')
    const authorName = nameSpan?.innerText?.trim() || userNameEl?.innerText?.split('\n')[0]?.trim() || handle
    const verified = !!userNameEl?.querySelector('svg[aria-label*="Verified"]')
      || !!userNameEl?.querySelector('[aria-label*="Verified account"]')

    const tweetTextEl = article.querySelector('[data-testid="tweetText"]')
    const content = tweetTextEl?.innerText || ''
    const lang = tweetTextEl?.getAttribute('lang') || null
    const truncated = !!article.querySelector('[data-testid="tweet-text-show-more-link"]')

    const metrics = {
      replies: parseAriaCount(article.querySelector('[data-testid="reply"]'), /(\d[\d,]*)\s+Repl/),
      reposts: parseAriaCount(article.querySelector('[data-testid="retweet"]'), /(\d[\d,]*)\s+repost/i),
      likes: parseAriaCount(
        article.querySelector('[data-testid="like"]') || article.querySelector('[data-testid="unlike"]'),
        /(\d[\d,]*)\s+Like/,
      ),
      views: parseAriaCount(
        article.querySelector('a[href*="/analytics"]') || article.querySelector('[aria-label*="views"]'),
        /(\d[\d,]*)\s+view/i,
      ),
    }

    const has_media = !!article.querySelector('[data-testid="tweetPhoto"]')
      || !!article.querySelector('video')

    return {
      id,
      url: `https://x.com/${handle}/status/${id}`,
      author: { handle, name: authorName, verified },
      content,
      lang,
      created_at: timeEl?.getAttribute('datetime') || null,
      metrics,
      truncated,
      has_media,
    }
  }

  function parseAriaCount(el, regex) {
    if (!el) return null
    const label = el.getAttribute('aria-label') || ''
    const m = label.match(regex)
    if (!m) return null
    return parseInt(m[1].replace(/,/g, ''), 10)
  }
}
