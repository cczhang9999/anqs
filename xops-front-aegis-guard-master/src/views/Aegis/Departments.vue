<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { createDepartment, listDepartments, getSimpleDevices, getDepartmentFallback, getRobotList, listSimpleUsers, saveDepartmentFallback, updateDepartment } from '@/api/aegis'

interface Department { id: number; name: string; code?: string; robot_id: number; enabled: boolean; duty_user_id?: string; member_ids?: number[] | string }
interface SimpleUser { id?: number; user_id?: number; username?: string; account?: string; name?: string; email?: string }
interface Robot { id: number; name: string; user_id?: string; im_type?: number }

// SSO 不可用时的兜底测试数据，正常情况下以 simple_users 的实际返回为准。
const mockSimpleUsers: SimpleUser[] = [
  { id: 1, name: '张伟', username: 'zhangwei', account: 'zhangwei' },
  { id: 2, name: '李娜', username: 'lina', account: 'lina' },
  { id: 3, name: '王强', username: 'wangqiang', account: 'wangqiang' },
  { id: 4, name: '赵敏', username: 'zhaomin', account: 'zhaomin' },
  { id: 5, name: '陈晨', username: 'chenchen', account: 'chenchen' },
  { id: 6, name: '刘洋', username: 'liuyang', account: 'liuyang' },
]

const loading = ref(false)
const saving = ref(false)
const departments = ref<Department[]>([])
const users = ref<SimpleUser[]>([])
const robots = ref<Robot[]>([])
const devices = ref<Array<{ department_id?: number }>>([])
const fallbackRobotId = ref(0)
const fallbackDialogVisible = ref(false)
const fallbackRobotForm = ref(0)
const fallbackSaving = ref(false)
const dialogVisible = ref(false)
const membersVisible = ref(false)
const viewingDepartment = ref<Department | null>(null)
const editing = ref(false)
const formRef = ref()
const form = reactive<Department>({ id: 0, name: '', robot_id: 0, enabled: true, duty_user_id: '', member_ids: [] })
const rules = { name: [{ required: true, message: '请输入部门名称', trigger: 'blur' }] }

const unwrap = (res: any) => res?.data || res || {}
const simpleUserItems = (res: any): SimpleUser[] => {
  const data = unwrap(res)
  const candidates = [data.items, data.list, data.users, data.data, data]
  return candidates.find(Array.isArray) || []
}
const userId = (user: SimpleUser) => Number(user.id ?? user.user_id ?? 0)
const userName = (user: SimpleUser) => user.name || user.username || user.account || user.email || String(userId(user))
const dutyUserLabel = (id?: string) => String(id || '').trim() || '未设置'
const robotLabel = (id?: number) => robots.value.find((item) => item.id === Number(id))?.name || `机器人 #${id}`
const hostCount = (departmentId: number) => devices.value.filter((item) => item.department_id === departmentId).length
const memberIds = (row: Department): number[] => {
  if (Array.isArray(row.member_ids)) return row.member_ids.map(Number).filter((id) => id > 0)
  if (typeof row.member_ids === 'string') {
    try { const value = JSON.parse(row.member_ids); return Array.isArray(value) ? value.map(Number).filter((id) => id > 0) : [] } catch { return [] }
  }
  return []
}
const memberLabel = (id: number) => {
  const user = users.value.find((item) => userId(item) === Number(id))
  return user ? userName(user) : `用户 ID：${id}`
}
const currentMemberIDs = computed(() => memberIds(form))
const selectedMemberUsers = computed(() => {
  const selected = new Set(currentMemberIDs.value)
  return users.value.filter((user) => selected.has(userId(user)))
})
const unselectedMemberUsers = computed(() => {
  const selected = new Set(currentMemberIDs.value)
  return users.value.filter((user) => !selected.has(userId(user)))
})
const missingSelectedMemberIDs = computed(() => {
  const known = new Set(users.value.map(userId))
  return currentMemberIDs.value.filter((id) => !known.has(id))
})
const viewingMemberIDs = computed(() => viewingDepartment.value ? memberIds(viewingDepartment.value) : [])
const loadSimpleUsers = async (): Promise<SimpleUser[]> => {
	const response = await listSimpleUsers()
	return simpleUserItems(response).filter((item) => userId(item) > 0)
}
const load = async () => {
  loading.value = true
  try {
    const fallbackPromise = getDepartmentFallback().catch(() => ({ data: { robot_id: 0 } }))
    const robotPromise = getRobotList().catch(() => ({ data: { ims: [] } }))
    const userPromise = loadSimpleUsers().catch(() => [])
    const [departmentRes, deviceRes, fallbackRes, robotRes, userRes] = await Promise.all([listDepartments({ limit: 200, offset: 0 }), getSimpleDevices(), fallbackPromise, robotPromise, userPromise])
    const departmentData = unwrap(departmentRes)
    const fallbackData = unwrap(fallbackRes)
    departments.value = departmentData.items || departmentData.list || []
    const ssoUsers = simpleUserItems(userRes)
    users.value = ssoUsers.length ? ssoUsers : mockSimpleUsers
    const robotData = unwrap(robotRes)
    robots.value = robotData.ims || robotData.items || []
    devices.value = unwrap(deviceRes).items || unwrap(deviceRes).list || []
    fallbackRobotId.value = Number(fallbackData.robot_id || 0)
  } finally { loading.value = false }
}
const resetForm = () => { Object.assign(form, { id: 0, name: '', robot_id: 0, enabled: true, duty_user_id: '', member_ids: [] }); formRef.value?.clearValidate() }
const openCreate = () => { editing.value = false; resetForm(); dialogVisible.value = true }
const openEdit = (row: Department) => { editing.value = true; Object.assign(form, { ...row, member_ids: memberIds(row) }); dialogVisible.value = true }
const openMembers = (row: Department) => { viewingDepartment.value = row; membersVisible.value = true }
const openFallbackEditor = () => { fallbackRobotForm.value = fallbackRobotId.value; fallbackDialogVisible.value = true }
const saveFallback = async () => {
  fallbackSaving.value = true
  try {
    await saveDepartmentFallback(Number(fallbackRobotForm.value || 0))
    fallbackRobotId.value = Number(fallbackRobotForm.value || 0)
    fallbackDialogVisible.value = false
    ElMessage.success('未分组告警群已更新')
  } finally { fallbackSaving.value = false }
}
const save = async () => {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const payload = { name: form.name.trim(), robot_id: Number(form.robot_id || 0), enabled: form.enabled, duty_user_id: String(form.duty_user_id || '').trim(), member_ids: memberIds(form) }
    if (editing.value) await updateDepartment(form.id, payload)
    else await createDepartment(payload)
    ElMessage.success(editing.value ? '部门已更新' : '部门已新增')
    dialogVisible.value = false
    await load()
  } finally { saving.value = false }
}
onMounted(load)
</script>

<template>
  <div class="aegis-page">
    <el-card shadow="never" class="panel">
      <div class="panel-heading"><h2>部门 ↔ 告警群</h2><el-button v-hasPermi="'aegis-departments-add'" type="primary" size="small" @click="openCreate">＋ 新增部门</el-button></div>
      <el-table v-loading="loading" :data="departments" stripe>
        <el-table-column type="index" label="#" width="55" />
        <el-table-column label="业务组 / 部门" min-width="170"><template #default="{ row }"><el-tag type="success" effect="plain">{{ row.name }}</el-tag></template></el-table-column>
        <el-table-column label="值班人员" min-width="170"><template #default="{ row }"><el-tag v-if="row.duty_user_id" size="small" effect="plain">{{ dutyUserLabel(row.duty_user_id) }}</el-tag><span v-else class="muted">未设置</span></template></el-table-column>
        <el-table-column label="绑定告警群 / 机器人" min-width="190"><template #default="{ row }"><el-tag v-if="row.robot_id" type="warning" size="small">{{ robotLabel(row.robot_id) }}</el-tag><span v-else class="muted">未绑定</span></template></el-table-column>
        <el-table-column label="可见成员（SSO账号）" min-width="240"><template #default="{ row }"><div v-if="memberIds(row).length" class="member-tags"><el-tag v-for="id in memberIds(row).slice(0, 4)" :key="String(id)" size="small" effect="plain">{{ memberLabel(id) }}</el-tag><el-button v-if="memberIds(row).length > 4" link type="primary" size="small" @click="openMembers(row)">+{{ memberIds(row).length - 4 }} 更多</el-button></div><span v-else class="muted">未绑定成员</span></template></el-table-column>
        <el-table-column label="关联主机" width="110"><template #default="{ row }">{{ hostCount(row.id) }} 台</template></el-table-column>
        <el-table-column label="操作" width="100"><template #default="{ row }"><el-button v-hasPermi="'aegis-departments-edit'" link type="primary" @click="openEdit(row)">编辑</el-button></template></el-table-column>
      </el-table>
      <div class="fallback-row"><div><el-tag type="info" effect="plain">未分组（兜底）</el-tag><span>未绑定部门的主机和告警将归入此处</span></div><div class="fallback-actions"><el-tag v-if="fallbackRobotId" type="warning" size="small">机器人 #{{ fallbackRobotId }}</el-tag><span v-else class="muted">未绑定告警群</span><span>{{ devices.filter((item) => !item.department_id).length }} 台</span><el-button v-hasPermi="'aegis-departments-fallback'" link type="primary" size="small" @click="openFallbackEditor">设置告警群</el-button></div></div>
    </el-card>
    <el-dialog v-model="dialogVisible" :title="editing ? '编辑部门' : '新增部门'" width="580px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item label="部门名称" prop="name"><el-input v-model="form.name" maxlength="128" /></el-form-item>
        <el-form-item label="告警群 / 机器人"><el-select v-model="form.robot_id" clearable filterable style="width: 100%" placeholder="请选择 robot-new 机器人"><el-option v-for="robot in robots" :key="robot.id" :label="`${robot.name} (#${robot.id})`" :value="robot.id" /></el-select></el-form-item>
        <el-form-item label="值班人员"><el-input v-model="form.duty_user_id" clearable maxlength="128" placeholder="请输入值班人员名称或联系方式" /></el-form-item>
        <el-form-item label="可见成员"><el-select v-model="form.member_ids" multiple collapse-tags collapse-tags-tooltip filterable style="width: 100%" placeholder="请选择可查看本部门告警的成员"><el-option-group v-if="selectedMemberUsers.length || missingSelectedMemberIDs.length" label="已选择"><el-option v-for="user in selectedMemberUsers" :key="userId(user)" :label="userName(user)" :value="userId(user)" /><el-option v-for="id in missingSelectedMemberIDs" :key="id" :label="`用户 ID：${id}（SSO 未返回）`" :value="id" /></el-option-group><el-option-group v-if="unselectedMemberUsers.length" label="其他可选人员"><el-option v-for="user in unselectedMemberUsers" :key="userId(user)" :label="userName(user)" :value="userId(user)" /></el-option-group></el-select></el-form-item>
        <el-form-item label="状态"><el-switch v-model="form.enabled" active-text="启用" inactive-text="停用" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="dialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>
    <el-dialog v-model="membersVisible" :title="`${viewingDepartment?.name || '部门'}的全部可见成员`" width="620px">
      <div v-if="viewingMemberIDs.length" class="all-member-tags"><el-tag v-for="id in viewingMemberIDs" :key="String(id)" effect="plain">{{ memberLabel(id) }}</el-tag></div>
      <el-empty v-else description="未绑定成员" :image-size="90" />
    </el-dialog>
    <el-dialog v-model="fallbackDialogVisible" title="设置未分组兜底告警群" width="460px">
      <div class="fallback-hint">未分组设备和告警不会归入具体部门，仅将通知发送到这里配置的机器人。</div>
      <el-form label-width="110px"><el-form-item label="告警群 / 机器人"><el-select v-model="fallbackRobotForm" clearable filterable style="width: 100%" placeholder="请选择 robot-new 机器人"><el-option v-for="robot in robots" :key="robot.id" :label="`${robot.name} (#${robot.id})`" :value="robot.id" /></el-select></el-form-item></el-form>
      <template #footer><el-button @click="fallbackDialogVisible = false">取消</el-button><el-button type="primary" :loading="fallbackSaving" @click="saveFallback">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.aegis-page { padding: 22px; min-height: 100%; color: #3b4451; }.panel { border: 1px solid #e4e7ed; border-radius: 10px; }.panel-heading { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }.panel-heading h2 { margin: 0; font-size: 17px; color: #3b4451; }.muted { color: #8a94a6; font-size: 13px; }.member-tags, .all-member-tags { display: flex; flex-wrap: wrap; gap: 5px; align-items: center; }.all-member-tags { max-height: 360px; padding: 4px; overflow: auto; }.fallback-row { display: flex; align-items: center; justify-content: space-between; margin-top: 12px; padding: 12px 14px; border: 1px dashed #d9dee7; border-radius: 7px; background: #fafbfd; color: #697586; font-size: 13px; }.fallback-row > div { display: flex; align-items: center; gap: 10px; }.fallback-actions { display: flex; align-items: center; gap: 10px; }.fallback-hint { margin-bottom: 18px; color: #697586; line-height: 1.7; }
</style>
