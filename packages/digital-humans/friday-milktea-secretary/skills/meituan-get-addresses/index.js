// meituan-get-addresses: 读取美团外卖账户的全部收货地址
// 数据源: localStorage.addstore（美团 H5 自家把 /openh5/address/list 缓存进来）
//
// params: { keyword?: string, phone?: string, limit?: number }
// returns: { success, addresses[], total, total_stored, error? }

async (params) => {
  try {
    const { keyword = '', phone = '', limit = 50 } = params || {}

    const raw = localStorage.getItem('addstore')
    if (!raw) {
      return {
        success: false,
        error_type: 'not_initialized',
        error: 'addstore 未写入，需先访问美团外卖首页让其缓存',
      }
    }

    let list
    try {
      list = JSON.parse(raw)
    } catch (e) {
      return { success: false, error_type: 'parse_error', error: e.message }
    }

    if (!Array.isArray(list)) {
      return { success: false, error_type: 'parse_error', error: 'addstore 不是数组' }
    }

    const total_stored = list.length
    const kw = (keyword || '').toLowerCase()

    const filtered = list.filter((a) => {
      if (phone && a.phone !== phone) return false
      if (kw) {
        const hay = ((a.poi || '') + ' ' + (a.address || '') + ' ' + (a.houseNumber || '')).toLowerCase()
        if (!hay.includes(kw)) return false
      }
      return true
    })

    const addresses = filtered.slice(0, limit).map((a) => ({
      addressId: a.addressId,
      name: a.name,
      gender: a.gender,
      phone: a.phone,
      poi: a.poi,
      address: a.address || '',
      houseNumber: a.houseNumber || '',
      // localStorage 存的是整数（×1e6），转回十进制度方便对比
      lat: typeof a.lat === 'number' ? a.lat / 1e6 : a.lat,
      lng: typeof a.lng === 'number' ? a.lng / 1e6 : a.lng,
      bindType: a.bindType,
      addressType: a.addressType,
      isDefault: a.isDefault,
      isOutOfRange: a.isOutOfRange,
    }))

    return {
      success: true,
      addresses,
      total: addresses.length,
      total_stored,
    }
  } catch (e) {
    return { success: false, error_type: 'internal_error', error: e.message }
  }
}
