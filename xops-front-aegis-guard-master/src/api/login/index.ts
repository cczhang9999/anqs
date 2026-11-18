import request from '@/axios'
import { SPECIAL_API_BASE_URL } from '@/axios/service'
import type { UserType } from './types'

interface RoleParams {
  roleName: string
}

export const loginApi = (): Promise<IResponse> => {
  return request.get({ url: '/api/sso/v2/auth/', baseURL: SPECIAL_API_BASE_URL })
}

export const loginOutApi = (): Promise<IResponse> => {
  return request.get({ url: '/mock/user/loginOut' })
}

export const getUserListApi = ({ params }: AxiosConfig) => {
  return request.get<{
    code: string
    data: {
      list: UserType[]
      total: number
    }
  }>({ url: '/mock/user/list', params })
}

export const getAdminRoleApi = (
  params: RoleParams
): Promise<IResponse<AppCustomRouteRecordRaw[]>> => {
  return request.get({ url: '/mock/role/list', params })
}

export const getTestRoleApi = (params: RoleParams): Promise<IResponse<string[]>> => {
  return request.get({ url: '/mock/role/list2', params })
}
