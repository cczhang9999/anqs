import request from '@/axios'
import { SPECIAL_API_BASE_URL } from '@/axios/service'

export const getDomain = (params: any) => {
    return request.get({ url: '/api/sso/v1/project/all', params, baseURL: SPECIAL_API_BASE_URL })
}

export const addDomians = (data: any) => {
    return request.post({ url: '/v1/domain_alerts', data })
}

export const getDomain_alertlist = (params: any) => {
    return request.get({ url: '/v1/domain_alertlist', params })
}


export const editDomain_alert = (data: any) => {
  return request.put({ url: '/v1/domain_alert', data })
}

export const delDomain = (data: any) => {
  return request.delete({ url: '/v1/domain_alert', data })
}
