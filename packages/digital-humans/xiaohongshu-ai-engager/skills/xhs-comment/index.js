/**
 * XHS Comment — browser_run 脚本
 *
 * 在小红书帖子详情页上下文中执行。
 * 通过 execCommand + input 事件注入评论文字，触发 Vue 响应式，
 * 点击发送按钮，并拦截 XHR 确认评论已发布。
 *
 * 契约：
 *   - 单个 async 箭头函数，由 browser_run 通过 evaluateScript 调用
 *   - 接收 params 对象作为第一个参数
 *   - 返回 JSON 可序列化的结果
 *   - 页面必须已导航到帖子详情页（explore/{note_id}）
 *   - 用户 session/cookies 自动可用
 *   - 错误以 { success: false, error: "..." } 形式返回，不抛出异常
 */
async (params) => {
  const { content } = params
  const log = (...a) => console.log('[xhs-comment]', ...a)
  const sleep = (ms) => new Promise(r => setTimeout(r, ms))

  if (!content?.trim()) {
    return { success: false, error: '评论内容不能为空' }
  }

  log('开始，内容长度:', content.length)

  // ------------------------------------
  // 1. 安装 XHR 拦截器，监听 comment/post 响应
  // ------------------------------------
  window.__xhs_commentResp = null
  const origOpen = XMLHttpRequest.prototype.open
  const origSend = XMLHttpRequest.prototype.send

  XMLHttpRequest.prototype.open = function (method, url, ...rest) {
    this.__xhs_url = typeof url === 'string' ? url : String(url || '')
    return origOpen.call(this, method, url, ...rest)
  }

  XMLHttpRequest.prototype.send = function (body) {
    if (this.__xhs_url?.includes('comment/post')) {
      log('XHR 匹配: comment/post')
      this.addEventListener('load', function () {
        log('XHR load, status:', this.status)
        try {
          window.__xhs_commentResp = { status: this.status, body: JSON.parse(this.responseText) }
        } catch (_) {
          window.__xhs_commentResp = { status: this.status, parseError: true, raw: this.responseText?.slice(0, 100) }
        }
      })
    }
    return origSend.call(this, body)
  }

  try {
    // ------------------------------------
    // 2. 找到评论输入框
    // ------------------------------------
    const input = document.querySelector('.content-input')
    if (!input) {
      return { success: false, error: '未找到评论输入框（.content-input）' }
    }
    log('找到输入框:', input.tagName, input.className)

    // ------------------------------------
    // 3. 注入文字（execCommand 适配 contenteditable）
    //    + 派发 input 事件触发 Vue 响应式
    // ------------------------------------
    input.focus()
    document.execCommand('insertText', false, content)
    input.dispatchEvent(new Event('input', { bubbles: true }))
    log('文字已注入:', input.innerText?.slice(0, 30))

    // ------------------------------------
    // 4. 等待 Vue nextTick 激活发送按钮（约 400ms）
    // ------------------------------------
    await sleep(400)

    // ------------------------------------
    // 5. 找到发送按钮并点击
    // ------------------------------------
    const sendBtn = [...document.querySelectorAll('button')].find(b => b.innerText.trim() === '发送')
    if (!sendBtn) {
      return { success: false, error: '未找到发送按钮', inputText: input.innerText }
    }
    if (sendBtn.disabled) {
      return { success: false, error: '发送按钮仍处于禁用状态（Vue 未激活）', inputText: input.innerText }
    }

    log('点击发送...')
    sendBtn.click()

    // ------------------------------------
    // 6. 等待 API 响应（最多 6 秒）
    // ------------------------------------
    for (let i = 0; i < 20; i++) {
      await sleep(300)
      if (window.__xhs_commentResp) break
    }

    const resp = window.__xhs_commentResp
    if (!resp) {
      return { success: false, error: 'API 超时：comment/post 未在 6 秒内响应' }
    }

    if (!resp.body?.success) {
      return {
        success: false,
        error: resp.body?.msg || 'API 返回失败',
        code: resp.body?.code
      }
    }

    log('成功:', resp.body.data?.toast)
    return {
      success: true,
      comment_id: resp.body.data?.comment?.id,
      content: resp.body.data?.comment?.content,
      toast: resp.body.data?.toast
    }
  } finally {
    // ------------------------------------
    // 7. 清理拦截器
    // ------------------------------------
    XMLHttpRequest.prototype.open = origOpen
    XMLHttpRequest.prototype.send = origSend
    delete window.__xhs_commentResp
  }
}
