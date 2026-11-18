import request from '@/axios'
import { DepartmentListResponse, DepartmentUserParams, DepartmentUserResponse } from './types'

export const getDepartmentApi = () => {
  return request.get<DepartmentListResponse>({ url: '/v1/users' })
}

export const getUserByIdApi = (params: any) => {
  return request.get<DepartmentUserResponse>({ url: '/v1/userlist', params })
}

export const deleteUserByIdApi = (ids: string[] | number[]) => {
  return request.post({ url: '/mock/department/user/delete', data: { ids } })
}

export const saveUserApi = (data: any) => {
  return request.post({ url: '/mock/department/user/save', data })
}

export const saveDepartmentApi = (data: any) => {
  return request.post({ url: '/v1/user-group', data })
}

export const putDepartmentApi = (data: any) => {
  return request.put({ url: '/v1/user-group', data })
}

export const deleteDepartmentApi = (ids: string[] | number[]) => {
  return request.delete({ url: '/v1/user-group', data: { ids } })
}

export const getDepartmentTableApi = (params: any) => {
  return request.get({ url: '/v1/user-grouplist', params })
}

export const getUserList = (params: any) => {
  return request.get({ url: '/v1/users', params })
}

export const groups = (params: any) => {
  return request.get({ url: '/v1/user-groups', params })
}

export const systemConfigs = (params: any) => {
  return request.get({ url: '/v1/system-configs', params })
}
