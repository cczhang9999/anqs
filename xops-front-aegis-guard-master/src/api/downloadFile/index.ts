import axios, { AxiosResponse } from 'axios'

interface DownloadFileOptions {
  url: string // 下载文件的 URL
  method?: 'GET' | 'POST' // 请求方法，默认为 'GET'
  params?: Record<string, any> // GET 请求时的 URL 查询参数
  data?: Record<string, any> // POST 请求时的请求体参数
  headers?: Record<string, string> // 请求头部
  fileName?: string // 下载文件的名称（可选）
}



const downloadFile = async ({
  url,
  method = 'GET',
  params = {},
  data = {},
  headers = {},
  fileName = 'downloaded-file'
}: DownloadFileOptions): Promise<void> => {
  try {
    // 发起 axios 请求，设置 responseType 为 'blob' 来接收二进制流数据
    const response: AxiosResponse<Blob> = await axios({
      url,
      method,
      params, // GET 请求的查询参数
      data, // POST 请求的请求体数据
      headers, // 请求头
      responseType: 'blob' // 关键：告诉 axios 返回二进制数据
    })

    // 确认响应成功
    if (response.status === 200) {
      // 通过 Content-Disposition 获取文件名（如果有）
      let downloadFileName = fileName
      const contentDisposition = response.headers['content-disposition']
      // 如果 Content-Disposition 存在，进行处理
      if (contentDisposition) {
        // 正则提取 `attachment; filename=...` 或 `filename="..."` 中的文件名
        const matches = contentDisposition.split(';')
        if (matches[1]) {
          let arr = matches[1].split('=')
          if (arr[1]) {
            downloadFileName = arr[1]
          }
        }
      }

      // 创建一个 Blob 对象
      const blob = new Blob([response.data], { type: response.headers['content-type'] })

      // 创建一个临时 URL
      const url = window.URL.createObjectURL(blob)

      // 创建一个 <a> 元素，模拟点击下载
      const a = document.createElement('a')
      a.href = url

      // 设置下载文件的名称
      a.download = downloadFileName

      // 触发点击事件，开始下载
      a.click()

      // 清理 URL 对象
      window.URL.revokeObjectURL(url)
    } else {
      console.error('文件下载失败，响应状态码：', response.status)
    }
  } catch (error) {
    console.error('下载文件时发生错误:', error)
  }
}

export default downloadFile
