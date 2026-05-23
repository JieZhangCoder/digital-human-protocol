// zhihu-creator-invited: 从知乎创作者中心获取被邀请回答的问题列表
// 调用前需已导航到: https://www.zhihu.com/creator/featured-question/invited
// params: { max_results?: number }
// returns: { success, questions[], count, error? }
// questions[]: { question_id, url, title, answer_count, follow_count, source }

async (params) => {
  const maxResults = params?.max_results || 20;
  await new Promise(r => setTimeout(r, 1000));

  const rows = document.querySelectorAll('.css-n9ov20');
  if (rows.length === 0) {
    return { success: false, error: 'No invite rows found (.css-n9ov20)', questions: [] };
  }

  const questions = [];
  for (const row of Array.from(rows).slice(0, maxResults)) {
    const link = row.querySelector('a[href*="/question/"]:not([href*="write"])');
    if (!link) continue;

    const url = link.href.split('?')[0];
    const question_id = url.match(/\/question\/(\d+)/)?.[1];
    const title = link.textContent.trim();
    if (!question_id || !title) continue;

    const infoText = row.querySelector('.css-1rs9lm3')?.textContent || '';
    const answer_count = parseInt(infoText.match(/(\d+)\s*回答/)?.[1] || '0');
    const follow_count = parseInt(infoText.match(/(\d+)\s*关注/)?.[1] || '0');

    questions.push({
      question_id,
      url,
      title,
      answer_count,
      follow_count,
      source: 'invited',
    });
  }

  return { success: true, questions, count: questions.length };
}
