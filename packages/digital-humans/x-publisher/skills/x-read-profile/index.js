/**
 * x-read-profile — browser_run 脚本
 *
 * 从 X 用户主页 (/{handle}) DOM 提取个人资料和最近推文。
 * 调用前编排层须 browser_navigate 到用户主页。
 *
 * 契约：
 *   - 单个 async 箭头函数
 *   - 接收 params 对象作为第一个参数
 *   - 返回 JSON 可序列化的结果
 *   - 错误以 { success: false, error_type, error } 形式返回，不抛出异常
 */
async (params) => {
  const { max_tweets = 10, scroll_pages = 1 } = params || {}
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

  // ------------------------------------
  // 1. URL 校验
  // ------------------------------------
  const handleMatch = location.pathname.match(/^\/([A-Za-z0-9_]{1,15})\/?$/)
  if (!handleMatch) {
    return {
      success: false,
      error_type: 'selector_not_found',
      error: '当前 URL 不是 X 用户主页（应为 /{handle}）',
      current_url: location.href,
    }
  }
  const handle = handleMatch[1]

  // ------------------------------------
  // 2. 错误页检测
  // ------------------------------------
  const bodyText = document.body?.innerText || ''
  if (/This account doesn.?t exist|This account is suspended|Account suspended|账号已暂停/i.test(bodyText)) {
    return { success: false, error_type: 'content_rejected', error: '用户不存在或账号被暂停' }
  }

  // ------------------------------------
  // 3. 解析 profile header
  // ------------------------------------
  const userNameEl = document.querySelector('[data-testid="UserName"]')
  if (!userNameEl) {
    return { success: false, error_type: 'selector_not_found', error: '未找到 UserName 节点（页面可能仍在加载）' }
  }
  // UserName 内含 display name 和 @handle 两段
  const userNameText = userNameEl.innerText || ''
  const lines = userNameText.split('\n').map((s) => s.trim()).filter(Boolean)
  const displayName = lines.find((l) => !l.startsWith('@')) || handle
  const verified = !!userNameEl.querySelector('svg[aria-label*="Verified"]')
    || !!userNameEl.querySelector('[aria-label*="Verified account"]')

  const bio = document.querySelector('[data-testid="UserDescription"]')?.innerText || ''
  const headerItemsText = document.querySelector('[data-testid="UserProfileHeader_Items"]')?.innerText || ''
  const headerLines = headerItemsText.split('\n').map((s) => s.trim()).filter(Boolean)
  const joined = headerLines.find((l) => /Joined|加入于/i.test(l))?.replace(/^Joined\s*/, '').replace(/^加入于\s*/, '') || null
  const website = headerLines.find((l) => /^[a-z0-9-]+\.[a-z]{2,}/i.test(l) && !/Joined|加入/i.test(l)) || null
  const location_ = headerLines.find((l) => l !== joined && l !== website && !/Joined|加入/i.test(l)) || null

  // 关注/粉丝链接
  const followingLink = document.querySelector(`a[href="/${handle}/following"]`)
  const followersLink = document.querySelector(`a[href="/${handle}/verified_followers"]`)
    || document.querySelector(`a[href="/${handle}/followers"]`)
  const followingCountText = parseFollowText(followingLink?.innerText || '', 'Following')
  const followersCountText = parseFollowText(followersLink?.innerText || '', 'Followers')

  // 私密账号检测
  const is_protected = /This account.s posts are protected|这是私密账号|protected/i.test(bodyText)
    && !document.querySelector('[data-testid="tweet"]')

  // ------------------------------------
  // 4. 收集最近推文（滚动加载）
  // ------------------------------------
  const seenIds = new Set()
  const recent_tweets = []
  const maxScrolls = Math.max(1, Math.min(scroll_pages, 10))

  if (!is_protected) {
    for (let s = 0; s < maxScrolls; s++) {
      for (const art of document.querySelectorAll('[data-testid="tweet"]')) {
        const parsed = parseTweetArticle(art, handle)
        if (!parsed || seenIds.has(parsed.id)) continue
        seenIds.add(parsed.id)
        recent_tweets.push(parsed)
        if (recent_tweets.length >= max_tweets) break
      }
      if (recent_tweets.length >= max_tweets) break
      if (s < maxScrolls - 1) {
        window.scrollTo({ top: document.body.scrollHeight })
        await sleep(1500)
      }
    }
  }

  return {
    success: true,
    profile: {
      handle,
      name: displayName,
      verified,
      bio,
      location: location_,
      website,
      joined,
      following_count_text: followingCountText,
      following_count: parseAbbrCount(followingCountText),
      followers_count_text: followersCountText,
      followers_count: parseAbbrCount(followersCountText),
      is_protected,
    },
    recent_tweets: recent_tweets.slice(0, max_tweets),
    total: recent_tweets.length,
  }

  // ------------------------------------
  // helpers
  // ------------------------------------
  function parseFollowText(text, suffix) {
    // text 形如 "1.2M Followers" 或 "36 Following"
    const m = text.match(/^([\d.,KMB万]+)/i)
    return m?.[1] || null
  }

  function parseAbbrCount(text) {
    if (!text) return null
    const t = text.trim().replace(/,/g, '')
    if (/万$/.test(t)) return Math.round(parseFloat(t) * 10000)
    if (/K$/i.test(t)) return Math.round(parseFloat(t) * 1000)
    if (/M$/i.test(t)) return Math.round(parseFloat(t) * 1000000)
    if (/B$/i.test(t)) return Math.round(parseFloat(t) * 1000000000)
    const n = parseInt(t, 10)
    return isNaN(n) ? null : n
  }

  function parseTweetArticle(article, expectedHandle) {
    const timeEl = article.querySelector('time')
    const tweetUrl = timeEl?.closest('a')?.getAttribute('href') || ''
    const idMatch = tweetUrl.match(/^\/([A-Za-z0-9_]+)\/status\/(\d+)$/)
    if (!idMatch) return null
    const [, tweetHandle, id] = idMatch

    const tweetTextEl = article.querySelector('[data-testid="tweetText"]')
    const content = tweetTextEl?.innerText || ''
    const truncated = !!article.querySelector('[data-testid="tweet-text-show-more-link"]')

    // 置顶 / 转推标记
    const socialCtx = article.querySelector('[data-testid="socialContext"]')?.innerText || ''
    const is_pinned = /Pinned|置顶/i.test(socialCtx)
    const is_repost = /reposted|转推/i.test(socialCtx) || tweetHandle.toLowerCase() !== expectedHandle.toLowerCase()

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
      url: `https://x.com/${tweetHandle}/status/${id}`,
      content,
      created_at: timeEl?.getAttribute('datetime') || null,
      metrics,
      truncated,
      is_pinned,
      is_repost,
      has_media,
    }
  }

  function parseAriaCount(el, regex) {
    if (!el) return null
    const m = (el.getAttribute('aria-label') || '').match(regex)
    return m ? parseInt(m[1].replace(/,/g, ''), 10) : null
  }
}
