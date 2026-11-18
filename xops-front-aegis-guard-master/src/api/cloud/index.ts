import request from '@/axios'

export const devList = (params: any) => {
  return request.get({ url: '/v1/devlist', params })
}

export const configlist = (params: any) => {
  return request.get({ url: '/v1/system-configlist', params })
}

export const saveDepartmentApi = (data: any) => {
  return request.post({ url: '/v1/system-config', data })
}

export const putDepartmentApi = (data: any) => {
  return request.put({ url: '/v1/system-config', data })
}

export const deleteDepartmentApi = (ids: string[] | number[]) => {
  return request.delete({ url: '/v1/system-config', data: { ids } })
}
export const editDev = (data: any) => {
  return request.put({ url: '/v1/dev', data })
}

