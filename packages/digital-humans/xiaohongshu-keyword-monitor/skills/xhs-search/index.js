/**
 * XHS Search — browser_run script
 *
 * Executes inside the Xiaohongshu search results page context.
 * Checks login via direct fetch, installs XHR interceptor for search results,
 * triggers search via Pinia store, and collects structured post data.
 *
 * Contract:
 *   - Single async arrow function (invoked by browser_run via evaluateScript)
 *   - Receives params object as first argument
 *   - Returns JSON-serializable result
 *   - Page must already be navigated to XHS search results URL
 *   - User session/cookies are automatically available
 *   - Errors returned as { success: false, error: "..." }, never thrown
 */
async (params) => {
  const {
    sort_by = 'general',
    time_range = '不限',
    pages = 1
  } = params

  const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
  const log = (...a) => console.log('[xhs]', ...a)

  log('start', { sort_by, time_range, pages })

  // ------------------------------------
  // 1. Install XHR interceptor for search results
  // ------------------------------------
  window.__xhs_searchResps = []

  const origOpen = XMLHttpRequest.prototype.open
  const origSend = XMLHttpRequest.prototype.send

  XMLHttpRequest.prototype.open = function (method, url, ...rest) {
    this.__xhs_url = typeof url === 'string' ? url : String(url || '')
    return origOpen.call(this, method, url, ...rest)
  }

  XMLHttpRequest.prototype.send = function (body) {
    const url = this.__xhs_url || ''
    if (url.includes('search/notes')) {
      log('XHR matched:', url)
      this.addEventListener('load', function () {
        log('XHR load, status:', this.status, 'length:', this.responseText?.length)
        try { window.__xhs_searchResps.push(JSON.parse(this.responseText)) } catch (_) {}
      })
      this.addEventListener('error', () => log('XHR error'))
      this.addEventListener('abort', () => log('XHR abort'))
    }
    return origSend.call(this, body)
  }

  log('XHR interceptor installed')

  // ------------------------------------
  // 2. Check login via direct fetch
  // ------------------------------------
  try {
    // log('fetching user/me...')
    // const meData = await fetch('https://edith.xiaohongshu.com/api/sns/web/v2/user/me', { credentials: 'include' }).then(r => r.json())
    // log('user/me response:', meData.success, 'guest:', meData.data?.guest)

    // if (!meData.success || meData.data?.guest !== false) {
    //   return { success: false, logged_in: false, error: 'Not logged in to Xiaohongshu' }
    // }
    // const user = {
    //   user_id: meData.data.user_id || '',
    //   nickname: meData.data.nickname || ''
    // }
    // log('logged in:', user.nickname)

    // ------------------------------------
    // 3. Trigger search with filters
    // ------------------------------------
    log('accessing Pinia...')
    const pinia = document.querySelector('#app')?.__vue_app__?.config?.globalProperties?.$pinia
    log('pinia found:', !!pinia)
    const searchStore = pinia?._s.get('search')
    log('searchStore found:', !!searchStore)

    if (!searchStore) {
      return { success: false, logged_in: true, error: 'Search store not found' }
    }

    const sortTag = sort_by === 'general' ? 'general' : sort_by
    searchStore.searchContext.filters = [
      { tags: [sortTag], type: 'sort_type' },
      { tags: ['不限'], type: 'filter_note_type' },
      { tags: [time_range], type: 'filter_note_time' },
      { tags: ['不限'], type: 'filter_note_range' },
      { tags: ['不限'], type: 'filter_pos_distance' }
    ]
    searchStore.feeds = []
    searchStore.searchContext.page = 1
    window.__xhs_searchResps = []
    log('calling searchNotes...')
    searchStore.searchNotes()
    log('searchNotes called')

    // ------------------------------------
    // 4. Collect results (with pagination)
    // ------------------------------------
    const maxPages = Math.min(Math.max(pages, 1), 5)
    const allPosts = []

    for (let page = 1; page <= maxPages; page++) {
      log(`waiting for page ${page}...`)
      for (let i = 0; i < 15; i++) {
        await sleep(500)
        if (window.__xhs_searchResps.length >= page) break
      }
      log(`page ${page} captured: ${window.__xhs_searchResps.length}`)

      const resp = window.__xhs_searchResps[page - 1]
      if (!resp || !resp.data) {
        if (page === 1) return { success: false, logged_in: true, error: 'Search API did not respond within timeout' }
        break
      }

      const items = (resp.data.items || []).filter(i => i.model_type === 'note' && i.note_card)
      log(`page ${page} items:`, items.length)
      for (const item of items) allPosts.push(extractPost(item))

      const hasMore = resp.data.has_more === true
      if (!hasMore || page >= maxPages) break

      searchStore.searchContext.page = page + 1
      searchStore.searchNotes()
      await sleep(1500)
    }

    // ------------------------------------
    // 5. Cleanup and return
    // ------------------------------------
    XMLHttpRequest.prototype.open = origOpen
    XMLHttpRequest.prototype.send = origSend
    const lastResp = window.__xhs_searchResps?.[window.__xhs_searchResps?.length - 1]
    delete window.__xhs_searchResps

    log('done, total posts:', allPosts.length)
    return {
      success: true,
      logged_in: true,
      posts: allPosts,
      total: allPosts.length,
      has_more: lastResp?.data?.has_more === true
    }
  } catch (err) {
    XMLHttpRequest.prototype.open = origOpen
    XMLHttpRequest.prototype.send = origSend
    delete window.__xhs_searchResps
    log('error:', err?.message)
    return { success: false, error: String(err?.message || err) }
  }

  function extractPost(item) {
    const card = item.note_card
    const interact = card.interact_info || {}
    return {
      id: item.id,
      title: card.display_title || '',
      author: card.user?.nickname || card.user?.nick_name || '',
      liked_count: parseCount(interact.liked_count),
      collected_count: parseCount(interact.collected_count),
      comment_count: parseCount(interact.comment_count),
      type: card.type || 'normal',
      link: `https://www.xiaohongshu.com/explore/${item.id}`
    }
  }

  function parseCount(val) {
    if (val == null) return 0
    const str = String(val).trim()
    if (!str) return 0
    if (str.endsWith('万')) return Math.round(parseFloat(str) * 10000)
    const num = parseInt(str, 10)
    return isNaN(num) ? 0 : num
  }
}
