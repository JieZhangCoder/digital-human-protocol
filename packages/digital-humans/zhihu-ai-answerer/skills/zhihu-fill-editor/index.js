// zhihu-fill-editor: 通过合成 ClipboardEvent 更新 Draft.js editorState
// 必须在 zhihu-open-editor 成功后调用
// params: { text: string }
// returns: { success, char_count?, error? }

async (params) => {
  const text = params?.text;
  if (!text) return { success: false, error: 'params.text 不能为空' };

  const editor = document.querySelector('.public-DraftEditor-content');
  if (!editor) return { success: false, error: '编辑器未找到，请先调用 zhihu-open-editor' };

  const dt = new DataTransfer();
  dt.setData('text/plain', text);
  editor.dispatchEvent(new ClipboardEvent('paste', {
    clipboardData: dt,
    bubbles: true,
    cancelable: true
  }));

  await new Promise(r => setTimeout(r, 1000));

  const charCount = document.querySelector('.public-DraftEditor-content')
    ?.innerText?.replace(/\n/g, '').length || 0;

  if (charCount === 0) {
    return { success: false, error: '写入后编辑器内容为空，ClipboardEvent 可能未生效' };
  }

  return { success: true, char_count: charCount };
}
