import Mock from 'mockjs'
import { SUCCESS_CODE } from '@/constants'

const timeout = 1000

export default [
  // 列表接口
  {
    url: '/mock/menu/list',
    method: 'get',
    timeout,
    response: () => {
      return {
        code: SUCCESS_CODE,
        data: {
          list: []
        }
      }
    }
  }
]
