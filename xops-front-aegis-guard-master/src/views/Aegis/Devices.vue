<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'
import { assignDeviceDepartment, batchAssignDeviceDepartment, getAllDepartments, getDeviceStats, listDevices, syncDevices } from '@/api/aegis'
import { Icon } from '@/components/Icon'

interface Department { id: number; name: string }
interface Device {
  id: number
  instance_name?: string
  private_ip?: string
  os_name?: string
  account_type?: string
  department_id?: number
  assign_source?: string
  assign_rule_id?: number
  assign_rule_name?: string
  assign_rule_deleted?: boolean
  assign_operator?: string
}

const loading = ref(false)
const syncing = ref(false)
const devices = ref<Device[]>([])
const departments = ref<Department[]>([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const deviceStats = reactive({ total: 0, assigned: 0, unassigned: 0 })
const departmentNameMap = computed(() => new Map(departments.value.map((item) => [item.id, item.name])))
const query = reactive({ keyword: '', ip: '', department_id: '', source: '' })
const selectedIds = ref<number[]>([])
const bulkDepartmentId = ref('')
const assignmentVisible = ref(false)
const assignmentRow = ref<Device | null>(null)
const assignmentDepartmentId = ref('')
const router = useRouter()

const unwrap = (res: any) => res?.data || res || {}
const accountEdition = (accountType?: string) => ({ AQS: '企', ULT: '旗' } as Record<string, string>)[String(accountType || '').toUpperCase()] || ''
const accountEditionTitle = (accountType?: string) => ({ AQS: '企业版', ULT: '旗舰版' } as Record<string, string>)[String(accountType || '').toUpperCase()] || ''
const accountEditionClass = (accountType?: string) => `account-edition-${String(accountType || '').toLowerCase()}`
const deviceOSIcon = (osName?: string) => {
  const value = String(osName || '').toLowerCase()
  if (value.includes('windows')) return 'vi-logos:microsoft-windows-icon'
  if (value.includes('centos')) return 'vi-logos:centos-icon'
  if (value.includes('debian')) return 'vi-logos:debian'
  if (value.includes('ubuntu')) return 'vi-logos:ubuntu'
  if (value.includes('red hat') || value.includes('rhel')) return 'vi-logos:redhat-icon'
  if (value.includes('rocky')) return 'vi-logos:rocky-linux-icon'
  if (value.includes('alma')) return 'vi-devicon:almalinux'
  if (value.includes('fedora')) return 'vi-logos:fedora'
  if (value.includes('suse')) return 'vi-logos:suse'
  if (value.includes('arch')) return 'vi-logos:archlinux'
  if (value.includes('mint')) return 'vi-logos:linux-mint'
  if (value.includes('alpine')) return 'vi-simple-icons:alpinelinux'
  if (value.includes('amazon linux')) return 'vi-logos:aws'
  if (value.includes('alibaba cloud linux') || value.includes('anolis')) return 'vi-logos:alibabacloud'
  if (value.includes('oracle linux')) return 'vi-logos:oracle'
  if (value.includes('mac') || value.includes('darwin') || value.includes('os x')) return 'vi-logos:apple'
  if (value.includes('linux')) return 'vi-logos:linux-tux'
  return 'vi-mdi:server'
}
const load = async () => {
  loading.value = true
  try {
    const params: Record<string, unknown> = { limit: pageSize.value, offset: (page.value - 1) * pageSize.value }
    ;['keyword', 'ip', 'department_id', 'source'].forEach((key) => {
      const value = query[key as keyof typeof query]
      if (value !== '') params[key] = value
    })
    const [deviceRes, statsRes, deptRes] = await Promise.all([listDevices(params), getDeviceStats(), getAllDepartments()])
    const deviceData = unwrap(deviceRes)
    const statsData = unwrap(statsRes)
    devices.value = deviceData.items || deviceData.list || []
    total.value = Number(deviceData.total || 0)
    deviceStats.total = Number(statsData.total || 0)
    deviceStats.assigned = Number(statsData.assigned || 0)
    deviceStats.unassigned = Number(statsData.unassigned || 0)
    const deptData = unwrap(deptRes)
    departments.value = deptData.items || deptData.list || []
  } finally {
    loading.value = false
  }
}
// A new filter creates a different result set, so selections from the old
// result set must not remain eligible for a bulk assignment.
const applyFilters = () => {
  page.value = 1
  selectedIds.value = []
  load()
}
const reset = () => { Object.assign(query, { keyword: '', ip: '', department_id: '', source: '' }); applyFilters() }
// 归属来源：规则 / 手动 / 未分配；规则被删除后仍显示规则名并标注（已删除）。
const sourceLabel = (row: Device) => {
  if (row.assign_source === 'rule') return row.assign_rule_deleted ? `规则：${row.assign_rule_name || ''}（已删除）` : `规则：${row.assign_rule_name || ''}`
  if (row.assign_source === 'manual') return row.assign_operator ? `手动 ${row.assign_operator}` : '手动'
  return '未分配'
}
const sourceTagType = (row: Device) => (row.assign_source === 'rule' ? '' : row.assign_source === 'manual' ? 'warning' : 'info')
const sourceTagEffect = (row: Device) => (row.assign_source === 'rule' ? 'plain' : 'plain')
const pageSelected = computed(() => devices.value.length > 0 && devices.value.every((row) => selectedIds.value.includes(row.id)))
const toggleSelected = (row: Device, checked: boolean) => {
  selectedIds.value = checked ? [...new Set([...selectedIds.value, row.id])] : selectedIds.value.filter((id) => id !== row.id)
}
const togglePage = (checked: boolean) => {
  const ids = devices.value.map((row) => row.id)
  selectedIds.value = checked ? [...new Set([...selectedIds.value, ...ids])] : selectedIds.value.filter((id) => !ids.includes(id))
}
const changeDepartment = async (row: Device, departmentId: number) => {
  await assignDeviceDepartment(row.id, departmentId)
  ElMessage.success('设备部门已更新')
  await load()
}
const openAssignment = (row: Device) => {
  assignmentRow.value = row
  assignmentDepartmentId.value = row.department_id ? String(row.department_id) : ''
  assignmentVisible.value = true
}
const submitAssignment = async () => {
  if (!assignmentRow.value) return
  if (assignmentDepartmentId.value === '') return ElMessage.warning('请选择目标部门')
  await changeDepartment(assignmentRow.value, Number(assignmentDepartmentId.value || 0))
  assignmentVisible.value = false
}
const bulkAssign = async () => {
  if (!selectedIds.value.length || bulkDepartmentId.value === '') return ElMessage.warning('请选择设备和目标部门')
  await batchAssignDeviceDepartment(selectedIds.value, Number(bulkDepartmentId.value))
  ElMessage.success(`已分配 ${selectedIds.value.length} 台主机`)
  selectedIds.value = []
  bulkDepartmentId.value = ''
  await load()
}
const sync = async () => {
  syncing.value = true
  try { await syncDevices(); ElMessage.success('设备同步完成'); await load() } finally { syncing.value = false }
}
const openDeviceAlerts = (row: Device) => {
  router.push({ name: 'aegis-alerts', query: { device_id: String(row.id) } })
}
onMounted(load)
</script>

<template>
  <div class="aegis-page">
    <div class="stat-grid">
      <div class="stat-card"><strong>{{ deviceStats.total }}</strong><span>主机总数</span></div>
      <div class="stat-card teal"><strong>{{ deviceStats.assigned }}</strong><span>已分配</span></div>
      <div class="stat-card danger"><strong>{{ deviceStats.unassigned }}</strong><span>未分组（待处理）</span></div>
      <div class="stat-card"><strong>{{ departments.length }}</strong><span>部门数</span></div>
    </div>
    <el-card shadow="never" class="panel">
      <div class="panel-heading"><h2>主机分组</h2><el-button v-hasPermi="'aegis-devices-sync'" size="small" :loading="syncing" @click="sync">同步设备</el-button></div>
      <div class="filter-row">
        <el-input v-model="query.keyword" clearable class="keyword" placeholder="🔍 主机名 / 实例名称" @keyup.enter="applyFilters" />
        <el-input v-model="query.ip" clearable class="ip-keyword" placeholder="🔍 IP" @keyup.enter="applyFilters" />
        <el-select v-model="query.department_id" clearable filterable placeholder="全部部门" @change="applyFilters"><el-option label="未分组" value="0" /><el-option v-for="item in departments" :key="item.id" :label="item.name" :value="String(item.id)" /></el-select>
        <el-select v-model="query.source" clearable placeholder="全部来源" @change="applyFilters"><el-option label="规则" value="rule" /><el-option label="手动" value="manual" /><el-option label="未分配" value="unassigned" /></el-select>
        <el-button type="primary" @click="applyFilters">搜索</el-button><el-button @click="reset">重置</el-button>
        <span class="hint">勾选多台可<b>批量分配到部门</b>；选「未分配」可把主机交还给规则</span>
      </div>
      <div v-if="selectedIds.length" class="bulk-row"><span>已选 <b>{{ selectedIds.length }}</b> 台　批量分配到部门：</span><el-select v-model="bulkDepartmentId" size="small" placeholder="请选择部门" style="width: 180px"><el-option label="未分组" value="0" /><el-option v-for="item in departments" :key="item.id" :label="item.name" :value="String(item.id)" /></el-select><el-button v-hasPermi="'aegis-devices-assign'" size="small" type="primary" @click="bulkAssign">分配</el-button><el-button link size="small" @click="selectedIds = []">取消选择</el-button></div>
      <el-table v-loading="loading" :data="devices" stripe>
        <el-table-column width="52"><template #header><el-checkbox :model-value="pageSelected" @change="togglePage" /></template><template #default="{ row }"><el-checkbox :model-value="selectedIds.includes(row.id)" @change="(checked: boolean) => toggleSelected(row, checked)" /></template></el-table-column>
        <el-table-column label="序号" width="70" align="center"><template #default="{ $index }">{{ (page - 1) * pageSize + $index + 1 }}</template></el-table-column>
        <el-table-column prop="instance_name" label="主机名（影响资产备注名称）" min-width="360" class-name="device-name-column" show-overflow-tooltip><template #default="{ row }"><div class="device-name-cell"><button type="button" class="device-name device-alert-link" :title="`查看 ${row.instance_name || '该主机'} 的告警`" @click="openDeviceAlerts(row)"><Icon class="device-os-icon" :icon="deviceOSIcon(row.os_name)" :size="19" /><span class="device-name-text">{{ row.instance_name || '—' }}</span><el-tag v-if="accountEdition(row.account_type)" size="small" effect="light" class="account-edition-tag" :class="accountEditionClass(row.account_type)" :title="accountEditionTitle(row.account_type)">{{ accountEdition(row.account_type) }}</el-tag></button></div></template></el-table-column>
        <el-table-column label="IP" min-width="180"><template #default="{ row }">{{ row.private_ip || '—' }}</template></el-table-column>
        <el-table-column label="所属部门" min-width="150"><template #default="{ row }"><el-tag v-if="row.department_id" type="success" size="small">{{ departmentNameMap.get(row.department_id) || '已分配' }}</el-tag><el-tag v-else type="info" size="small">未分组</el-tag></template></el-table-column>
        <el-table-column label="归属来源" min-width="230" show-overflow-tooltip><template #default="{ row }"><el-tag :type="sourceTagType(row)" :effect="sourceTagEffect(row)" size="small">{{ sourceLabel(row) }}</el-tag></template></el-table-column>
        <el-table-column label="操作" width="140"><template #default="{ row }"><el-button v-hasPermi="'aegis-devices-assign'" link type="primary" @click="openAssignment(row)">{{ row.department_id ? '编辑告警组' : '分配告警组' }}</el-button></template></el-table-column>
      </el-table>
      <div class="pagination"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" background layout="total, sizes, prev, pager, next" :page-sizes="[10, 20, 50, 100]" :total="total" @current-change="load" @size-change="page = 1; load()" /></div>
    </el-card>
    <el-dialog v-model="assignmentVisible" title="分配主机到部门" width="520px">
      <template v-if="assignmentRow">
        <div class="assignment-host"><strong class="device-name" :title="assignmentRow.os_name || '未知操作系统'"><Icon class="device-os-icon" :icon="deviceOSIcon(assignmentRow.os_name)" :size="19" />{{ assignmentRow.instance_name || '未命名主机' }}</strong><span>{{ assignmentRow.private_ip || '—' }}</span><el-tag size="small" :type="sourceTagType(assignmentRow)" effect="plain">{{ sourceLabel(assignmentRow) }}</el-tag></div>
        <p class="assignment-hint">选择该主机归属的部门（一台主机只归一个部门）。选部门 = 手动分配，规则不再改动；选「未分配」= 交给规则，下次执行规则时自动归属。</p>
        <el-form label-width="86px"><el-form-item label="所属部门"><el-select v-model="assignmentDepartmentId" style="width: 100%" placeholder="请选择部门"><el-option label="未分配（交给规则）" value="0" /><el-option v-for="item in departments" :key="item.id" :label="item.name" :value="String(item.id)" /></el-select></el-form-item></el-form>
      </template>
      <template #footer><el-button @click="assignmentVisible = false">取消</el-button><el-button type="primary" @click="submitAssignment">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.aegis-page { padding: 22px; min-height: 100%; color: #3b4451; }.stat-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; margin-bottom: 16px; }.stat-card { background: #fff; border: 1px solid #e4e7ed; border-radius: 10px; padding: 14px 16px; }.stat-card span { display: block; color: #8a94a6; font-size: 13px; }.stat-card strong { display: block; margin-bottom: 7px; font-size: 28px; color: #3b4451; }.stat-card.teal strong { color: #1a9c92; }.stat-card.danger strong { color: #e6412e; }.panel { border: 1px solid #e4e7ed; border-radius: 10px; }.panel-heading { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }.panel-heading h2 { margin: 0; font-size: 17px; color: #3b4451; }.filter-row { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-bottom: 14px; }.keyword { width: 230px; }.ip-keyword { width: 155px; }.filter-row .el-select { width: 160px; }.hint { color: #8a94a6; font-size: 12px; }.bulk-row { display: flex; align-items: center; gap: 10px; padding: 9px 12px; margin-bottom: 12px; border: 1px solid #bfe6e0; border-radius: 6px; color: #2b5a54; background: #eaf6f4; }.sub-text { color: #8a94a6; font-size: 12px; }.device-name { display: inline-flex; align-items: center; min-width: 0; }.device-name-column :deep(.cell) { overflow: visible; }.device-name-cell { min-width: 0; overflow: visible; }.device-alert-link { width: 100%; max-width: 100%; min-width: 0; padding: 0; overflow: visible; border: 0; color: #409eff; background: transparent; cursor: pointer; font: inherit; white-space: nowrap; }.device-name-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.device-alert-link:hover { color: #66b1ff; text-decoration: underline; }.device-alert-link:focus-visible { border-radius: 3px; outline: 2px solid #a0cfff; outline-offset: 2px; }.device-os-icon { flex: 0 0 auto; margin-right: 8px; }.account-edition-tag { position: relative; top: -6px; flex: 0 0 auto; width: 20px; height: 20px; margin-left: 4px; padding: 0; line-height: 18px; font-size: 11px; font-weight: 600; border-radius: 50%; }.account-edition-aqs { color: #a45b00; border-color: #f2cf99; background: #fff5e6; }.account-edition-ult { color: #7b3ba4; border-color: #d5b6e9; background: #f8effd; }.assignment-host { display: flex; justify-content: space-between; gap: 18px; padding: 12px; border-radius: 6px; background: #f7f9fb; }.assignment-host span, .assignment-hint { color: #8a94a6; font-size: 13px; }.assignment-hint { margin: 12px 0 18px; } .pagination { display: flex; justify-content: flex-end; margin-top: 16px; } @media (max-width: 1100px) { .stat-grid { grid-template-columns: repeat(2, 1fr); } }
</style>
