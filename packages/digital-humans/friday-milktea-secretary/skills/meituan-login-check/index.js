// meituan-login-check: 读取美团外卖 H5 登录态与身份字段
// 纯本地读取（cookie + localStorage），零网络请求，零反爬风险
//
// returns: { success, logged_in, userId?, userName?, uuid?, openh5_uuid?, dfpId?, ... }

async () => {
  try {
    if (!location.host.endsWith('meituan.com')) {
      return {
        success: false,
        error_type: 'wrong_domain',
        error: '当前 Tab 不在 meituan.com 域',
        current_host: location.host,
      }
    }

    const cookies = Object.fromEntries(
      document.cookie.split(';').map((c) => {
        const idx = c.indexOf('=')
        return [c.slice(0, idx).trim(), c.slice(idx + 1)]
      }),
    )

    const userId = cookies.userId || ''
    if (!userId) {
      return { success: true, logged_in: false, reason: 'missing userId cookie' }
    }

    let userName = ''
    try {
      userName = decodeURIComponent(cookies.userName || '')
    } catch {
      userName = cookies.userName || ''
    }

    return {
      success: true,
      logged_in: true,
      userId,
      userName,
      uuid: cookies.iuuid || cookies.uuid || '',
      openh5_uuid: cookies.openh5_uuid || '',
      dfpId: localStorage.getItem('dfpId') || '',
      webdfpid_len: (cookies.WEBDFPID || '').length,
      ci: cookies.ci || '',
      cityname: (() => {
        try { return decodeURIComponent(cookies.cityname || '') } catch { return cookies.cityname || '' }
      })(),
    }
  } catch (e) {
    return { success: false, error_type: 'internal_error', error: e.message }
  }
}
