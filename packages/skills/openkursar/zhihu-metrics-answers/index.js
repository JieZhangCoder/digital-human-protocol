// zhihu-metrics-answers: 从内容管理-回答页读取近期回答的阅读/赞同/收藏数据，输出策略信号
// 调用前需已导航到: https://www.zhihu.com/creator/manage/creation/answer
// params: { recent_count?: number (default 10) }
// returns: { success, answers[], strategy_signal, signal_reason, error? }
// strategy_signal: "keep" | "adjust" | "urgent_adjust"

async (params) => {
  const recentCount = params?.recent_count || 10;
  await new Promise(r => setTimeout(r, 1500));

  const cards = document.querySelectorAll('.CreationManage-CreationCard');
  if (!cards.length) {
    return { success: false, error: 'No CreationManage-CreationCard elements found', answers: [] };
  }

  function parseNum(text) {
    return parseInt((text || '0').replace(/[,，\s]/g, '')) || 0;
  }

  const answers = [];
  for (const card of Array.from(cards).slice(0, recentCount)) {
    const titleEl = card.querySelector('.CreationCardTitle-wrapper span');
    const linkEl  = card.querySelector('a[href*="/answer/"]');
    const metrics = card.querySelector('.css-1uostix');
    if (!titleEl || !linkEl || !metrics) continue;

    const raw = metrics.textContent || '';
    const reads     = parseNum(raw.match(/([\d,]+)\s*阅读/)?.[1]);
    const upvotes   = parseNum(raw.match(/([\d,]+)\s*赞同/)?.[1]);
    const comments  = parseNum(raw.match(/([\d,]+)\s*评论/)?.[1]);
    const favorites = parseNum(raw.match(/([\d,]+)\s*收藏/)?.[1]);

    // 折叠检测：赞同为负数不可能，但若 upvotes=0 且 reads>200 是强信号
    const likely_collapsed = upvotes === 0 && reads > 200;

    answers.push({
      title:           titleEl.textContent.trim(),
      url:             linkEl.href,
      reads, upvotes, comments, favorites,
      likely_collapsed,
    });
  }

  if (!answers.length) {
    return { success: false, error: 'No answer data extracted from cards', answers: [] };
  }

  // ── 策略信号计算 ──────────────────────────────────────────────
  const collapsed_count = answers.filter(a => a.likely_collapsed).length;
  const zero_upvote_count = answers.filter(a => a.upvotes === 0).length;
  const total_reads   = answers.reduce((s, a) => s + a.reads, 0);
  const total_upvotes = answers.reduce((s, a) => s + a.upvotes, 0);
  const upvote_ratio  = total_reads > 0 ? total_upvotes / total_reads : 0; // 赞同/阅读比

  let strategy_signal = 'keep';
  let signal_reason   = '近期回答表现正常，延续当前策略';

  if (collapsed_count >= 2 || zero_upvote_count >= Math.ceil(answers.length * 0.7)) {
    strategy_signal = 'urgent_adjust';
    signal_reason = `${collapsed_count > 0 ? collapsed_count + '篇疑似被折叠，' : ''}近${answers.length}篇中${zero_upvote_count}篇赞同为0，平台可能识别为低质量内容，需立即调整风格`;
  } else if (upvote_ratio < 0.003 || zero_upvote_count >= Math.ceil(answers.length * 0.4)) {
    strategy_signal = 'adjust';
    signal_reason = `赞同/阅读比 ${(upvote_ratio * 100).toFixed(2)}%，近期互动偏低，建议调整内容策略`;
  } else {
    signal_reason = `赞同/阅读比 ${(upvote_ratio * 100).toFixed(2)}%，互动正常`;
  }

  return {
    success:         true,
    answers,
    strategy_signal,
    signal_reason,
    stats: {
      total_reads,
      total_upvotes,
      upvote_ratio:    parseFloat((upvote_ratio * 100).toFixed(2)),
      zero_upvote_count,
      collapsed_count,
    },
  };
}
