import request from '@/axios'
// 查询用户统计
export const userCount = (params: any) => {
  return request.get({ url: '/v1/user-count', params })
}

// 查询告警统计
export const alertCount = (params: any) => {
  return request.get({ url: '/v1/alert-count', params })
}

// 按天查询告警统计
export const alertCountDay = (params: any) => {
  return request.get({ url: '/v1/alert-count-day', params })
}
