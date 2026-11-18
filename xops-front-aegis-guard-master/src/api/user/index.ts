import request from '@/axios'
import { SPECIAL_API_BASE_URL } from '@/axios/service'

export const schedulelist = (params: any) => {
  return request.get({ url: '/v1/schedulelist', params })
}

export const authApi = (): Promise<IResponse> => {
  return request.get({ url: '/api/sso/v2/auth', baseURL: SPECIAL_API_BASE_URL })
}

export const schedules = (data: any) => {
  return request.post({ url: '/v1/schedules', data })
}

export const loginOutApi = (data: any) => {
  return request.post({ url: '/api/sso/v2/logout/', data, baseURL: SPECIAL_API_BASE_URL })
}

export const editSchedules = (data: any) => {
  return request.put({ url: '/v1/schedules', data })
}

export const editUser = (data: any) => {
  return request.put({ url: '/v1/user', data })
}
export const addSchedules = (data: any) => {
  return request.post({ url: '/v1/schedules', data })
}

export const importSchedules = (data: any) => {
  return request.post({ url: '/v1/import-schedules', data })
}

export const tokenCheck = (): Promise<IResponse> => {
  return request.get({ url: '/v1/user-token-check' })
}

export const userMap = (data: any) => {
  return request.post({ url: '/v1/user-map', data })
}

export const getUserMap = (params: any) => {
  return request.get({ url: '/v1/user-map', params })
}

export const delUserMap = (data: any) => {
  return request.delete({ url: '/v1/user-map', data })
}

