import axios, { AxiosError } from 'axios'
import { defaultRequestInterceptors, defaultResponseInterceptors } from './config'

import { AxiosInstance, InternalAxiosRequestConfig, RequestConfig, AxiosResponse } from './types'
import { ElMessage } from 'element-plus'
import { REQUEST_TIMEOUT } from '@/constants'

import { getTenant } from '@/utils/index'

const configuredApiBaseURL = import.meta.env.VITE_API_BASE_PATH?.trim().replace(/\/+$/, '')

// 普通安骑士接口优先使用环境配置，例如 UAT 的 /api/aegisguard。
// 未配置时保留原有逻辑：开发环境走 Vite 相对路径代理，其他环境走租户域名。
export const PATH_URL = configuredApiBaseURL || (import.meta.env.VITE_NODE_ENV === 'development' ? '' : getTenant())

// SSO 和 robot 不属于安骑士服务，必须绕过 /api/aegisguard 前缀，直连同域 /api/...。
export const SPECIAL_API_BASE_URL = configuredApiBaseURL
  ? new URL(configuredApiBaseURL, window.location.origin).origin
  : PATH_URL



const abortControllerMap: Map<string, AbortController> = new Map()

const axiosInstance: AxiosInstance = axios.create({
  timeout: REQUEST_TIMEOUT,
  baseURL: PATH_URL
})

axiosInstance.interceptors.request.use((res: InternalAxiosRequestConfig) => {
  const controller = new AbortController()
  const url = res.url || ''
  res.signal = controller.signal
  abortControllerMap.set(
    import.meta.env.VITE_USE_MOCK === 'true' ? url.replace('/mock', '') : url,
    controller
  )
  return res
})

axiosInstance.interceptors.response.use(
  (res: AxiosResponse) => {
    const url = res.config.url || ''
    abortControllerMap.delete(url)
    // 这里不能做任何处理，否则后面的 interceptors 拿不到完整的上下文了
    return res
  },
  (error: AxiosError) => {
    console.log('err： ' + error) // for debug
    let msg =  error?.response?.data?.message
    ElMessage.error( msg || error.message)
    return Promise.reject(error)
  }
)

axiosInstance.interceptors.request.use(defaultRequestInterceptors)
axiosInstance.interceptors.response.use(defaultResponseInterceptors)

const service = {
  request: (config: RequestConfig) => {
    return new Promise((resolve, reject) => {
      if (config.interceptors?.requestInterceptors) {
        config = config.interceptors.requestInterceptors(config as any)
      }

      axiosInstance
        .request(config)
        .then((res) => {
          resolve(res)
        })
        .catch((err: any) => {
          reject(err)
        })
    })
  },
  cancelRequest: (url: string | string[]) => {
    const urlList = Array.isArray(url) ? url : [url]
    for (const _url of urlList) {
      abortControllerMap.get(_url)?.abort()
      abortControllerMap.delete(_url)
    }
  },
  cancelAllRequest() {
    for (const [_, controller] of abortControllerMap) {
      controller.abort()
    }
    abortControllerMap.clear()
  }
}

export default service
