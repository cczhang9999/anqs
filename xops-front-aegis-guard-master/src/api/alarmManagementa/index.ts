import request from '@/axios'
// 查询告警列表
export const alertlist = (params: any) => {
  return request.get({ url: '/v1/alertlist', params })
}
// 删除告警
export const delDelete = (data: any) => {
  return request.delete({ url: '/v1/alert', data })
}
// 修改告警
export const editalert = (data: any) => {
  return request.put({ url: '/v1/alert', data })
}
// 查询用户全部
export const users = (params: any) => {
  return request.get({ url: '/v1/users', params })
}
