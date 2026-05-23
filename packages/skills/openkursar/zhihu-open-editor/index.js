// zhihu-open-editor: 在当前问题页打开回答编辑器
// 调用前需已导航到: https://www.zhihu.com/question/<id>
// params: {}
// returns: { success, already_open?, error? }

async (params) => {
  await new Promise(r => setTimeout(r, 1000));

  // 编辑器已打开则直接返回
  if (document.querySelector('.public-DraftEditor-content')) {
    const editor = document.querySelector('.public-DraftEditor-content');
    editor.focus();
    await new Promise(r => setTimeout(r, 1000));
    return { success: true, already_open: true };
  }

  // 已回答过此题（页面显示"编辑回答"而非"写回答"）
  const editBtn = Array.from(document.querySelectorAll('button'))
    .find(el => el.textContent.trim() === '编辑回答');
  if (editBtn) return { success: false, error: '已回答过此题，跳过', already_answered: true };

  // 点击"写回答"
  const group = document.querySelector('.QuestionButtonGroup');
  const btnsFound = group
    ? Array.from(group.querySelectorAll('button')).map(b => b.textContent.trim().replace(/\u200B/g, ''))
    : [];
  const writeBtn = btnsFound.length
    ? Array.from(group.querySelectorAll('button')).find(b => b.textContent.includes('写回答'))
    : Array.from(document.querySelectorAll('button')).find(b => b.textContent.includes('写回答'));
  if (!writeBtn) return {
    success: false,
    buttons_found: btnsFound,
    error: `未找到写回答按钮（当前按钮：${btnsFound.join('、') || '无'}），可能该题已回答或回答已删除，请直接跳过此题`,
  };
  writeBtn.click();
  await new Promise(r => setTimeout(r, 1500));

  const editor = document.querySelector('.public-DraftEditor-content');
  if (!editor) return { success: false, error: '编辑器未出现' };

  // 关闭创作助手（如果存在，避免遮挡）
  const closeBtn = Array.from(document.querySelectorAll('button'))
    .find(el => el.textContent.trim() === '关闭创作助手' || el.getAttribute('aria-label') === '关闭创作助手');
  if (closeBtn) {
    closeBtn.click();
    await new Promise(r => setTimeout(r, 400));
  }

  editor.focus();
  // 额外等待 Draft.js 内部事件系统完成绑定
  await new Promise(r => setTimeout(r, 1000));
  return { success: true };
}
