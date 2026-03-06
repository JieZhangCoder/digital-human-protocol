/**
 * bili-reply — browser_run script
 *
 * 在任意 bilibili.com 页面上下文中执行（cookies 自动携带）。
 * 通过直接 fetch POST 到 B站评论 API 发表回复，不依赖任何 DOM 操作。
 *
 * 契约：
 *   - 单个 async 箭头函数，由 browser_run 通过 evaluateScript 调用
 *   - 接收 params 对象作为第一个参数
 *   - 返回 JSON 可序列化的结果
 *   - 页面必须是 bilibili.com 域名（保证 bili_jct cookie 可用）
 *   - 错误以 { success: false, error: "..." } 形式返回，不抛出异常
 *
 * params（全部来自 bili-get-messages 返回的 notification 字段，直接传入，不要修改）：
 *   oid        - notification.oid
 *   reply_type - notification.reply_type（1=视频, 12=文章）
 *   root       - notification.root
 *   parent     - notification.parent
 *   message    - 回复内容文字
 */
async (params) => {
  const { oid, reply_type, root, parent, message } = params
  const log = (...a) => console.log('[bili-reply]', ...a)

  // ------------------------------------
  // 1. 参数校验
  // ------------------------------------
  if (!oid) return { success: false, error: '缺少参数: oid' }
  if (!reply_type) return { success: false, error: '缺少参数: reply_type' }
  if (!root) return { success: false, error: '缺少参数: root' }
  if (!parent) return { success: false, error: '缺少参数: parent' }
  if (!message?.trim()) return { success: false, error: '回复内容不能为空' }
  if (message.trim().length > 500) {
    return { success: false, error: `回复内容过长: ${message.trim().length} 字（上限 500）` }
  }

  log('start', { oid, reply_type, root, parent, msgLen: message.length })

  // ------------------------------------
  // 2. 从 cookie 获取 CSRF Token（bili_jct）
  // ------------------------------------
  const csrf = document.cookie
    .split(';')
    .map(c => c.trim())
    .find(c => c.startsWith('bili_jct='))
    ?.split('=')[1]

  if (!csrf) {
    return { success: false, error: '未找到 bili_jct cookie，请确认已登录 B 站' }
  }

  log('csrf obtained')

  try {
    // ------------------------------------
    // 3. 构造表单数据并 POST
    //    参数顺序严格复刻 B站前端 message-reply-box 的真实请求
    // ------------------------------------
    const formData = new URLSearchParams()
    formData.append('oid', String(oid))
    formData.append('type', String(reply_type))
    formData.append('message', message.trim())
    formData.append('scene', 'msg')
    formData.append('plat', '1')
    formData.append('from', 'im-reply')
    formData.append('build', '0')
    formData.append('mobi_app', 'web')
    formData.append('root', String(root))
    formData.append('parent', String(parent))
    formData.append('csrf', csrf)

    log('sending reply...')
    const fetchResp = await fetch('https://api.bilibili.com/x/v2/reply/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      credentials: 'include',
      body: formData.toString()
    })

    // ------------------------------------
    // 4. 解析并验证响应
    // ------------------------------------
    const respData = await fetchResp.json().catch(() => null)
    log('response:', fetchResp.status, respData?.code, respData?.message)

    if (!respData) {
      return { success: false, error: '回复 API 返回无法解析的响应' }
    }

    if (respData.code !== 0) {
      const errHints = {
        '-101': '账号未登录',
        '-111': 'CSRF 校验失败',
        '-400': '请求参数错误',
        '12002': '评论区已关闭',
        '12015': '回复过于频繁，请稍后再试',
        '12035': '该评论已不存在'
      }
      const hint = errHints[String(respData.code)]
      return {
        success: false,
        error: `API 错误 code=${respData.code}: ${respData.message}${hint ? ' (' + hint + ')' : ''}`,
        code: respData.code
      }
    }

    const commentId = respData.data?.rpid || respData.data?.reply?.rpid
    log('success, comment_id:', commentId)

    return {
      success: true,
      comment_id: commentId,
      message: message.trim()
    }

  } catch (err) {
    log('error:', err?.message)
    return { success: false, error: String(err?.message || err) }
  }
}
