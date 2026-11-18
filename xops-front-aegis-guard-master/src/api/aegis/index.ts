import request from '@/axios'
import { SPECIAL_API_BASE_URL } from '@/axios/service'

export const listAlerts = (params: Record<string, unknown> = {}) =>
  request.get({ url: '/v1/alertlist', params })

export const getAllAlerts = () => request.get({ url: '/v1/alerts' })

export const getAlertStats = () => request.get({ url: '/v1/alertstats' })

export const getAlert = (id: number) => request.get({ url: `/v1/alert/${id}` })

export const getAlertHistory = (id: number) => request.get({ url: `/v1/alert/${id}/history` })

export const updateAlert = (id: number, data: Record<string, unknown>) =>
  request.put({ url: '/v1/alert', data: { ...data, id } })

export const confirmAlert = (id: number, note: string, occurrenceId?: number) =>
  request.post({ url: '/v1/alert-confirm', data: { id, note, occurrence_id: occurrenceId } })

export const confirmAlerts = (ids: number[], note: string) =>
  request.post({ url: '/v1/alerts-confirm', data: { ids, note } })

export const listDevices = (params: Record<string, unknown> = {}) =>
  request.get({ url: '/v1/devicelist', params })

export const getDeviceStats = () =>
  request.get({ url: '/v1/devicestats' })

export const getAllDevices = () => request.get({ url: '/v1/devices' })

export const getSimpleDevices = () => request.get({ url: '/v1/devicesimple' })

export const getRobotList = () =>
  request.get({ url: '/api/robot/v1/ims', baseURL: SPECIAL_API_BASE_URL })

export const syncDevices = () => request.post({ url: '/v1/device-sync', data: {} })

export const assignDeviceDepartment = (id: number, department_id: number) =>
  request.put({ url: '/v1/device-department', data: { id, department_id } })

export const batchAssignDeviceDepartment = (ids: number[], department_id: number) =>
  request.put({ url: '/v1/device-departments', data: { ids, department_id } })

export const listDepartments = (params: Record<string, unknown> = {}) =>
  request.get({ url: '/v1/departmentlist', params })

export const getAllDepartments = () => request.get({ url: '/v1/departments' })

export const createDepartment = (data: Record<string, unknown>) =>
  request.post({ url: '/v1/department', data })

export const updateDepartment = (id: number, data: Record<string, unknown>) =>
  request.put({ url: '/v1/department', data: { ...data, id } })

export const deleteDepartment = (id: number) =>
  request.delete({ url: '/v1/department', data: { id } })

export const getDepartmentFallback = () =>
  request.get({ url: '/v1/department-fallback' })

export const saveDepartmentFallback = (robot_id: number) =>
  request.put({ url: '/v1/department-fallback', data: { robot_id } })

export const listSimpleUsers = (params: Record<string, unknown> = {}) =>
  request.get({ url: '/api/sso/v2/simple_users', params, baseURL: SPECIAL_API_BASE_URL })

// ── 二期：主机自动分组规则 ──────────────────────────────────────────────
export const listGroupRules = (params: Record<string, unknown> = {}) =>
  request.get({ url: '/v1/group-rules', params })

export const createGroupRule = (data: Record<string, unknown>) =>
  request.post({ url: '/v1/group-rule', data })

export const updateGroupRule = (id: number, data: Record<string, unknown>) =>
  request.put({ url: '/v1/group-rule', data: { ...data, id } })

export const deleteGroupRule = (id: number) =>
  request.delete({ url: '/v1/group-rule', data: { id } })

export const toggleGroupRule = (id: number, enabled: boolean) =>
  request.put({ url: '/v1/group-rule-enabled', data: { id, enabled } })

export const reorderGroupRule = (id: number, action: 'top' | 'up' | 'down') =>
  request.put({ url: '/v1/group-rule-order', data: { id, action } })

export const getGroupRuleStats = () => request.get({ url: '/v1/group-rulestats' })

export const previewGroupRules = (params: Record<string, unknown> = {}) =>
  request.get({ url: '/v1/group-rule-preview', params })

export const executeGroupRules = () => request.post({ url: '/v1/group-rule-execute', data: {} })
