import request from '@/axios'
// 查询日志
export const userLogs = (params: any) => {
  return request.get({ url: '/v1/user-logs', params })
}

export const ssoLogs = (params: any) => {
  return request.get({ url: '/v1/sso-logs', params,responseType: 'blob' })
}
export const sasLogs = (params: any) => {
  return request.get({ url: '/v1/sas-logs', params,responseType: 'blob' })
}

