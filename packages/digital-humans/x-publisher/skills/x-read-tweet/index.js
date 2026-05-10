/**
 * x-read-tweet — browser_run 脚本
 *
 * 在 X 推文详情页解析 DOM，返回结构化的焦点推文 + 可见回复列表。
 * 不发起任何 XHR；纯 DOM 读取，依赖稳定的 data-testid。
 *
 * 契约：
 *   - 单个 async 箭头函数
 *   - 接收 params 对象作为第一个参数
 *   - 返回 JSON 可序列化的结果
 *   - 错误以 { success: false, error_type, error } 形式返回，不抛出异常
 */
async (params) => {
  const { include_replies = true, max_replies = 20 } = params || {}

  // ------------------------------------
  // 1. URL 校验：必须是推文详情页
  // ------------------------------------
  const urlMatch = location.pathname.match(/^\/([A-Za-z0-9_]+)\/status\/(\d+)/)
  if (!urlMatch) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: '当前 URL 不是 X 推文详情页（应为 /{author}/status/{id}）',
      current_url: location.href,
    }
  }
  const focalTweetId = urlMatch[2]

  // ------------------------------------
  // 2. 检测错误页
  // ------------------------------------
  const bodyText = document.body?.innerText || ''
  if (/Hmm\.\.\.\s*this page doesn.?t exist|This Tweet was deleted|Tweet unavailable|This account doesn.?t exist|此推文已被删除/.test(bodyText)) {
    return {
      success: false,
      error_type: 'content_rejected',
      error: '推文不存在或已删除',
    }
  }
  if (/restricted|age-restricted|This Post is unavailable to you|该帖不可见/i.test(bodyText)) {
    return {
      success: false,
      error_type: 'auth_expired',
      error: '推文受限或需要登录才能查看',
    }
  }

  // ------------------------------------
  // 3. 找出所有 article（主推 + 回复）
  // ------------------------------------
  const articles = Array.from(document.querySelectorAll('[data-testid="tweet"]'))
  if (!articles.length) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: '未找到主推 article（[data-testid=tweet]）——页面可能仍在加载',
    }
  }

  // 焦点推文：链接里 status/{focalTweetId} 的那条
  const focalArticle = articles.find((art) => {
    const link = art.querySelector('a[href*="/status/"]')
    return link?.getAttribute('href')?.includes(`/status/${focalTweetId}`)
  }) || articles[0]

  const focal = parseArticle(focalArticle)
  if (!focal) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: '主推 article 解析失败（DOM 结构与预期不符）',
    }
  }

  // ------------------------------------
  // 4. 回复（焦点之后的 article）
  // ------------------------------------
  let replies = []
  if (include_replies) {
    const focalIndex = articles.indexOf(focalArticle)
    const candidates = articles.slice(focalIndex + 1, focalIndex + 1 + max_replies)
    replies = candidates
      .map(parseArticle)
      .filter(Boolean)
      .filter((r) => r.id !== focalTweetId)
  }

  return {
    success: true,
    tweet: focal,
    replies,
    reply_count_visible: replies.length,
  }

  // ------------------------------------
  // helpers
  // ------------------------------------
  function parseArticle(article) {
    if (!article) return null

    // tweet id + author handle from time link
    const timeEl = article.querySelector('time')
    const timeLink = timeEl?.closest('a')
    const tweetUrl = timeLink?.getAttribute('href') || ''
    const idMatch = tweetUrl.match(/^\/([A-Za-z0-9_]+)\/status\/(\d+)$/)
    if (!idMatch) return null
    const [, handle, id] = idMatch

    // author name + verified
    const userNameEl = article.querySelector('[data-testid="User-Name"]')
    const nameSpan = userNameEl?.querySelector('a[href^="/"][role="link"] span')
    const authorName = nameSpan?.innerText?.trim() || userNameEl?.innerText?.split('\n')[0]?.trim() || handle
    const verified = !!userNameEl?.querySelector('svg[aria-label*="Verified"]')
      || !!userNameEl?.querySelector('[aria-label*="Verified account"]')
      || !!userNameEl?.querySelector('[data-testid="icon-verified"]')

    // tweet text
    const tweetTextEl = article.querySelector('[data-testid="tweetText"]')
    const content = tweetTextEl?.innerText || ''
    const truncated = !!article.querySelector('[data-testid="tweet-text-show-more-link"]')

    // language (no direct attr in DOM; fallback null — was available via legacy.lang in GraphQL)
    const lang = tweetTextEl?.getAttribute('lang') || null

    // metrics from button aria-labels
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
      bookmarks: null, // Not exposed via aria-label in default state
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
