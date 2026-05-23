// zhihu-creator-search: 从知乎创作者搜索页提取问题列表（含实时增量数据）
// 调用前需已导航到: https://www.zhihu.com/creator/search-question/day?q=<关键词>&sort=
// params: { max_results?: number }
// returns: { success, questions[], count, error? }
// questions[]: { question_id, url, title, view_increment, follow_increment, answer_increment, upvote_increment }

async (params) => {
  const maxResults = params?.max_results || 20;
  await new Promise(r => setTimeout(r, 1000));

  const rows = document.querySelectorAll('.css-1fd22oq');
  if (rows.length === 0) {
    return { success: false, error: 'No question rows found (.css-1fd22oq)', questions: [] };
  }

  function parseIncrement(text) {
    // 取增量部分（"28 万共 32.3 万" → 取 "28 万"；"155 共 155" → 取 "155"）
    const raw = (text || '').split('共')[0].trim();
    if (raw.includes('万')) return Math.round(parseFloat(raw.replace('万', '')) * 10000);
    return parseInt(raw.replace(/[^0-9]/g, '')) || 0;
  }

  const questions = [];
  for (const row of Array.from(rows).slice(0, maxResults)) {
    const link = row.querySelector('a[href*="/question/"]:not([href*="write"])');
    if (!link) continue;

    const url = link.href.split('?')[0];
    const question_id = url.match(/\/question\/(\d+)/)?.[1];
    const title = link.textContent.trim();
    if (!question_id || !title) continue;

    const metrics = Array.from(row.querySelectorAll('.css-1aeb3xt'));
    questions.push({
      question_id,
      url,
      title,
      view_increment: parseIncrement(metrics[0]?.textContent),
      follow_increment: parseIncrement(metrics[1]?.textContent),
      answer_increment: parseIncrement(metrics[2]?.textContent),
      upvote_increment: parseIncrement(metrics[3]?.textContent),
      source: 'creator_search',
    });
  }

  return { success: true, questions, count: questions.length };
}
