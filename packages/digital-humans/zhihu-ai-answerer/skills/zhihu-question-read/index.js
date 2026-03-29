// zhihu-question-read: 读取当前问题页的标题、描述、回答数和TOP回答
// 调用前需已导航到: https://www.zhihu.com/question/<id>
// params: { top_answers?: number (default 3) }
// returns: { success, question, description, answer_count, follower_count, top_answers[] }

async (params) => {
  const topN = params?.top_answers ?? 3;

  await new Promise(r => setTimeout(r, 1200));

  // 问题标题
  const title = document.querySelector('h1.QuestionHeader-title')?.textContent?.trim() || '';

  // 问题描述（展开全部）
  const moreBtn = document.querySelector('.QuestionRichText-more');
  if (moreBtn) {
    moreBtn.click();
    await new Promise(r => setTimeout(r, 500));
  }
  const description = document.querySelector('.QuestionRichText-content, .QuestionRichText')
    ?.textContent?.trim()?.slice(0, 800) || '';

  // 回答数
  const metaText = Array.from(document.querySelectorAll('h4, .List-headerText'))
    .find(el => el.textContent.includes('回答'))?.textContent || '';
  const answerCountMatch = metaText.match(/([\d,]+)\s*个回答/);
  const answer_count = answerCountMatch ? parseInt(answerCountMatch[1].replace(',', '')) : 0;

  // 关注者数
  const followerEl = document.querySelector('.NumberBoard-itemValue');
  const follower_count = followerEl ? parseInt(followerEl.textContent.replace(',', '')) : 0;

  // TOP回答
  const top_answers = [];
  const answerItems = document.querySelectorAll('.AnswerItem');
  for (let i = 0; i < Math.min(topN, answerItems.length); i++) {
    const item = answerItems[i];
    const author = item.querySelector('.AuthorInfo-name')?.textContent?.trim() || '匿名用户';
    const voteEl = item.querySelector('.VoteButton--up');
    const votes = voteEl?.textContent?.trim() || '0';
    // 展开回答全文
    const expandBtn = item.querySelector('.ContentItem-expandButton, [class*="expand"]');
    if (expandBtn) {
      expandBtn.click();
      await new Promise(r => setTimeout(r, 300));
    }
    const content = item.querySelector('.RichContent-inner, .RichText')
      ?.textContent?.trim()?.slice(0, 600) || '';
    top_answers.push({ author, votes, content });
  }

  return {
    success: !!title,
    question: title,
    description,
    answer_count,
    follower_count,
    top_answers,
    error: !title ? 'Could not find question title' : null,
  };
}
