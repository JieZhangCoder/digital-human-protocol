// zhihu-publish-answer: 发布当前已填写内容的回答
// 调用前需已用 zhihu-fill-editor 将内容写入编辑器
// params: {}
// returns: { success, char_count?, error? }

async (params) => {
  // 验证编辑器有内容（直接读 innerText）
  const charCount = document.querySelector('.public-DraftEditor-content')?.innerText?.replace(/\n/g, '').length || 0;
  if (charCount < 10) {
    return { success: false, error: `编辑器内容为空（${charCount} 字），请先调用 zhihu-fill-editor` };
  }

  // 找到并点击"发布回答"按钮
  const publishBtn = Array.from(document.querySelectorAll('button'))
    .find(el => el.textContent.trim() === '发布回答');
  if (!publishBtn) return { success: false, error: '未找到发布回答按钮' };
  if (publishBtn.disabled) return { success: false, error: '发布按钮被禁用' };

  publishBtn.click();

  // 等待 1.5 秒后检查编辑器是否消失（页面跳转说明发布成功）
  await new Promise(r => setTimeout(r, 1500));

  const editorStillVisible = !!document.querySelector('.public-DraftEditor-content');
  if (editorStillVisible) {
    return { success: false, error: '点击发布后编辑器仍然存在，发布可能失败（按钮点击未触发提交）' };
  }

  return { success: true, char_count: charCount };
}
