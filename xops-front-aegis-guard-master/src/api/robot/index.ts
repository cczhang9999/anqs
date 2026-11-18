import request from '@/axios'
// 获取机器人列表
export const getImList = (params: any) => {
    return request.get({ url: '/v1/imlist', params })
}
// 删除机器人
export const deleteIm = (data: any) => {
    return request.delete({ url: '/v1/im', data })
}
// 新增机器人
export const addIm = (data: any) => {
    return request.post({ url: '/v1/im', data })
}
// 修改机器人
export const putIm = (data: any) => {
    return request.put({ url: '/v1/im', data })
}
// 获取全部机器人
export const getImAll = (params: any) => {
    return request.get({ url: '/v1/robot-ims', params })
}