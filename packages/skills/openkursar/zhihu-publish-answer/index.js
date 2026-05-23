// zhihu-publish-answer: 发布当前已填写内容的回答
// 调用前需已用 zhihu-fill-editor 将内容写入编辑器
// params: {}
// returns: { success, char_count?, answer_url?, error? }

async (params) => {
  // 验证编辑器有内容
  const editorEl = document.querySelector('.public-DraftEditor-content');
  const text = editorEl?.innerText || '';
  const charCount = text.replace(/\n/g, '').length;
  if (charCount < 10) {
    return { success: false, error: `编辑器内容为空（${charCount} 字），请先调用 zhihu-fill-editor` };
  }

  // 从 URL 提取问题 ID
  const questionMatch = location.href.match(/question\/(\d+)/);
  if (!questionMatch) {
    return { success: false, error: '无法从 URL 提取问题 ID，当前 URL: ' + location.href };
  }
  const questionId = questionMatch[1];

  // 获取 CSRF token
  const getCookie = (name) => {
    const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
    return match ? decodeURIComponent(match[2]) : null;
  };
  const xsrf = getCookie('_xsrf');
  if (!xsrf) {
    return { success: false, error: '未找到 _xsrf cookie，可能未登录' };
  }

  // DOM 点击方案（优先测试）
  const btn = [...document.querySelectorAll('button')].find(
    b => b.textContent.trim() === '发布回答' || b.textContent.trim() === '提交修改'
  );

  if (!btn) {
    return { success: false, error: '未找到发布按钮（发布回答/提交修改）' };
  }

  const isDisabled = btn.disabled
    || btn.getAttribute('disabled') !== null
    || btn.getAttribute('aria-disabled') === 'true';
  if (isDisabled) {
    return { success: false, error: '发布按钮处于 disabled 状态，不正常，请检查编辑器内容是否正确写入' };
  }

  btn.click();

  // 轮询等待页面跳转（最多 10s）
  const startUrl = location.href;
  for (let i = 0; i < 20; i++) {
    await new Promise(r => setTimeout(r, 500));
    const cur = location.href;
    if (cur !== startUrl && cur.includes('/answer/')) {
      return { success: true, char_count: charCount, answer_url: cur };
    }
  }
  if (location.href.includes('/answer/')) {
    return { success: true, char_count: charCount, answer_url: location.href };
  }
  return { success: false, error: '点击发布后等待超时（10s），未检测到页面跳转' };

  // ---- 旧方案：直接调用知乎发布 API ----
  // const html = '<p>' + text.trim().replace(/\n\n+/g, '</p><p>').replace(/\n/g, '<br>') + '</p>';
  // if (!xsrf) return { success: false, error: '未找到 _xsrf cookie，可能未登录' };
  // try {
  //   const resp = await fetch(`https://www.zhihu.com/api/v4/questions/${questionId}/answers`, {
  //     method: 'POST',
  //     headers: {
  //       'Content-Type': 'application/json',
  //       'x-xsrftoken': xsrf,
  //       'x-requested-with': 'fetch',
  //       'Referer': location.href,
  //     },
  //     body: JSON.stringify({
  //       content: html,
  //       reward_setting: { can_reward: false, tagline: '' },
  //       reshipment_settings: 'allowed',
  //       comment_permission: 'all',
  //       mark_infos: [],
  //       publish_token: '',
  //     }),
  //   });
  //   if (!resp.ok) {
  //     const errBody = await resp.text().catch(() => '');
  //     return { success: false, error: `HTTP ${resp.status}: ${errBody.slice(0, 200)}` };
  //   }
  //   const data = await resp.json();
  //   const answerId = data.id;
  //   const answerUrl = answerId
  //     ? `https://www.zhihu.com/question/${questionId}/answer/${answerId}`
  //     : location.href;
  //   return { success: true, char_count: charCount, answer_url: answerUrl };
  // } catch (e) {
  //   return { success: false, error: e.message };
  // }
  // ---- 旧方案结束 ----
}
