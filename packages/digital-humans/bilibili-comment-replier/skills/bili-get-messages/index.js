/**
 * bili-get-messages — browser_run script
 *
 * 在 message.bilibili.com/#/reply 页面上下文中执行。
 * 通过直接 fetch 调用 B站 API 获取"回复我的"通知列表，
 * 并计算每条通知发回复所需的 oid / type / root / parent 参数。
 *
 * 契约：
 *   - 单个 async 箭头函数，由 browser_run 通过 evaluateScript 调用
 *   - 接收 params 对象作为第一个参数
 *   - 返回 JSON 可序列化的结果
 *   - 页面必须已导航到 message.bilibili.com（保证 cookies 可用）
 *   - 用户 session/cookies 自动可用
 *   - 错误以 { success: false, error: "..." } 形式返回，不抛出异常
 *
 * 返回的每条 notification：
 *   id            - 通知 ID（用于去重，存入 memory.replied_ids）
 *   user_nickname - 评论者昵称
 *   user_mid      - 评论者 UID
 *   comment       - 评论正文（source_content）
 *   replied_to    - 被回复的评论内容（target_reply_content，嵌套回复时有值）
 *   root_content  - 根评论内容（root_reply_content，嵌套时有值）
 *   notification_type - "video" | "reply" | "article"
 *   video_title   - 视频/文章标题
 *   video_desc    - 视频/文章描述（作为回复时的背景参考）
 *   video_url     - 视频/文章 URL（已可直接导航）
 *   oid           - 回复 API 的 oid 参数
 *   reply_type    - 回复 API 的 type 参数（1=视频, 12=文章）
 *   root          - 回复 API 的 root 参数
 *   parent        - 回复 API 的 parent 参数
 *   is_multi      - 是否为合并通知
 *   counts        - 合并通知的评论数
 *   reply_time    - 评论时间戳
 */
async (params) => {
  const {
    pages = 1,
    cursor_id = 0,
    cursor_time = 0
  } = params

  const log = (...a) => console.log('[bili-get-messages]', ...a)
  const sleep = (ms) => new Promise(r => setTimeout(r, ms))

  log('start', { pages, cursor_id, cursor_time })

  try {
    // ------------------------------------
    // 1. 检查登录状态
    // ------------------------------------
    const navResp = await fetch('https://api.bilibili.com/x/web-interface/nav', {
      credentials: 'include'
    }).then(r => r.json()).catch(() => null)

    if (!navResp || navResp.code !== 0 || !navResp.data?.isLogin) {
      return { success: false, logged_in: false, error: '未登录 B站，请先在浏览器中登录' }
    }

    const myMid = navResp.data.mid
    const myNickname = navResp.data.uname
    log('logged in:', myNickname, 'mid:', myMid)

    // ------------------------------------
    // 2. 分页拉取通知
    // ------------------------------------
    const maxPages = Math.min(Math.max(pages, 1), 5)
    const allNotifications = []
    let lastCursor = null

    let fetchUrl = 'https://api.bilibili.com/x/msgfeed/reply?platform=web&build=0&mobi_app=web'
    if (cursor_id && cursor_time) {
      fetchUrl += `&id=${cursor_id}&time=${cursor_time}`
    }

    for (let page = 1; page <= maxPages; page++) {
      log(`fetching page ${page}...`, fetchUrl)
      const resp = await fetch(fetchUrl, { credentials: 'include' }).then(r => r.json()).catch(() => null)

      if (!resp || resp.code !== 0) {
        if (page === 1) {
          return {
            success: false,
            logged_in: true,
            error: `API 返回错误: code=${resp?.code} msg=${resp?.message}`
          }
        }
        break
      }

      const data = resp.data
      lastCursor = data.cursor
      const items = data.items || []
      log(`page ${page}: ${items.length} items, cursor:`, lastCursor)

      for (const item of items) {
        allNotifications.push(extractNotification(item))
      }

      if (lastCursor?.is_end || items.length === 0 || page >= maxPages) break

      // 构造下一页 cursor URL
      fetchUrl = `https://api.bilibili.com/x/msgfeed/reply?platform=web&build=0&mobi_app=web&id=${lastCursor.id}&time=${lastCursor.time}`
      await sleep(500)
    }

    log('done, total:', allNotifications.length)
    return {
      success: true,
      logged_in: true,
      my_mid: myMid,
      my_nickname: myNickname,
      notifications: allNotifications,
      total: allNotifications.length,
      cursor: lastCursor,
      is_end: lastCursor?.is_end ?? true
    }

  } catch (err) {
    log('error:', err?.message)
    return { success: false, error: String(err?.message || err) }
  }

  // ------------------------------------
  // 辅助：从 API item 提取结构化通知
  // ------------------------------------
  function extractNotification(item) {
    const i = item.item

    // 计算 reply API 的 root 参数：
    //   root_id == 0 → 顶级评论，root = source_id
    //   root_id != 0 → 嵌套回复，root = root_id
    const rootParam = i.root_id && i.root_id !== 0 ? i.root_id : i.source_id
    const parentParam = i.source_id

    return {
      id: item.id,
      user_nickname: item.user?.nickname || '',
      user_mid: item.user?.mid || 0,
      comment: i.source_content || '',
      replied_to: i.target_reply_content || '',
      root_content: i.root_reply_content || '',
      notification_type: i.type || 'video',
      video_title: i.title || '',
      video_desc: i.desc || '',
      video_url: i.uri || '',
      is_multi: item.is_multi === 1,
      counts: item.counts || 1,
      oid: i.subject_id,
      reply_type: i.business_id,
      root: rootParam,
      parent: parentParam,
      reply_time: item.reply_time || 0
    }
  }
}
