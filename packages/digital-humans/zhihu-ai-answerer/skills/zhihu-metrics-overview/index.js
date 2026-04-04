// zhihu-metrics-overview: 从创作者内容分析页读取近期汇总指标
// 调用前需已导航到: https://www.zhihu.com/creator/analytics
// params: 无
// returns: { success, reads_total, reads_today, upvotes_total, upvotes_today, favorites_total, favorites_today, error? }

async (params) => {
  await new Promise(r => setTimeout(r, 1500));

  const cards = document.querySelectorAll('.StatisticCard');
  if (!cards.length) {
    return { success: false, error: 'No StatisticCard elements found, page may not have loaded' };
  }

  function parseNum(text) {
    return parseInt((text || '0').replace(/[,，\s]/g, '')) || 0;
  }

  function readCard(label) {
    const card = Array.from(cards).find(c =>
      c.querySelector('.css-1fu8ne5')?.textContent?.includes(label)
    );
    if (!card) return { total: 0, today: 0 };
    const nums = Array.from(card.querySelectorAll('*'))
      .filter(el => el.children.length === 0 && /^[\d,]+$/.test(el.textContent.trim()))
      .map(el => parseNum(el.textContent));
    return { total: nums[0] || 0, today: nums[1] || 0 };
  }

  const reads     = readCard('阅读总量');
  const upvotes   = readCard('赞同总量');
  const favorites = readCard('收藏总量');

  return {
    success: true,
    reads_total:      reads.total,
    reads_today:      reads.today,
    upvotes_total:    upvotes.total,
    upvotes_today:    upvotes.today,
    favorites_total:  favorites.total,
    favorites_today:  favorites.today,
  };
}
