<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRoute, useRouter } from 'vue-router'
import { listAlerts, getAlert, getAlertStats, getAllDepartments, confirmAlert, confirmAlerts, getAlertHistory } from '@/api/aegis'

interface Department { id: number; name: string; code?: string }
interface Alert {
  id: number
  alert_type: string
  product_type: string
  provider_alert_id: string
  title: string
  severity: string
  status: string
  confirm_note?: string
  confirmed_by?: string
  confirmed_at?: string
  device_id: number
  device_name?: string
  device_private_ip?: string
  department_id?: number
  department_name?: string
  last_message?: string
  message_preview?: string
  first_seen_at?: string
  first_provider_time?: string
  last_seen_at?: string
  last_provider_time?: string
  confirmation_count?: number
  confirmed_count?: number
  pending_count?: number
  occurrence_count?: number
  last_payload?: string
}
interface AlertOccurrence {
  id: number
  alert_id: number
  provider_alert_id?: string
  title: string
  device_id: number
  severity: string
  occurred_at?: string
  captured_at?: string
  payload?: string
  message?: string
  snapshot?: boolean
  status: string
  confirmed_by?: string
  confirmed_at?: string
  confirm_note?: string
}

const loading = ref(false)
const alerts = ref<Alert[]>([])
const total = ref(0)
const selected = ref<Alert[]>([])
const departments = ref<Department[]>([])
const page = ref(1)
const pageSize = ref(10)
const detailVisible = ref(false)
const confirmVisible = ref(false)
const historyVisible = ref(false)
const historyLoading = ref(false)
const historyRows = ref<AlertOccurrence[]>([])
const historyAlert = ref<Alert | null>(null)
const current = ref<Alert | null>(null)
const confirmNote = ref('')
const confirming = ref(false)
const confirmOccurrenceId = ref<number>()
const historyPage = ref(1)
const historyPageSize = ref(10)
const historyPageRows = computed(() => historyRows.value.slice((historyPage.value - 1) * historyPageSize.value, historyPage.value * historyPageSize.value))
const historyPending = computed(() => historyRows.value.filter((row) => row.status !== 'confirmed').length)
const historyDetailLines = computed(() => {
  const row = historyAlert.value
  if (!row) return []
  const message = row.last_message || historyRows.value[0]?.message || ''
  const details = String(message).split(/\r?\n/).map(cleanAlertLine).filter((line) =>
    line && !/^(告警名称|首次发生时间|最新发生时间)[：:]/.test(line)
  )
  return [
    `告警名称：${row.title || '—'}`,
    `首次发生时间：${formatTime(row.first_provider_time || row.first_seen_at)}`,
    `最新发生时间：${formatTime(row.last_provider_time || row.last_seen_at)}`,
    ...details
  ]
})
const selectableAlert = (row: Alert) => row.status !== 'confirmed'
const formatConfirmTime = (value?: string) => value ? formatTime(value).slice(0, 16) : '—'
const pendingLabel = (row: Alert) => `待确认 ${row.pending_count ?? row.occurrence_count ?? 1}/${row.occurrence_count || 1}`
const globalStats = reactive({ total: 0, pending: 0, confirmed: 0, today: 0, severity: {} as Record<string, number> })
const route = useRoute()
const router = useRouter()

const query = reactive({
  keyword: '',
  host: '',
  ip: '',
  from: '',
  to: '',
  alert_type: '',
  status: '',
  severity: '',
  department_id: '',
  device_id: ''
})

const unwrap = (res: any) => res?.data || res || {}
const formatTime = (value?: string) => value ? String(value).replace('T', ' ').replace(/\+08:00$/, '') : '—'
const severityText = (value?: string) => ({ serious: '严重', high: '高危', suspicious: '可疑', medium: '中危', low: '低危', info: '提示' } as Record<string, string>)[value || ''] || '未标识'
const statusText = (value?: string) => value === 'confirmed' ? '已确认' : value === 'stale' ? '已过期' : '待确认'
const cleanAlertLine = (value: string) => value.replace(/\*\*/g, '').replace(/\u00a0/g, ' ').trim()
const alertMessageLines = (row: Alert) => String(row.last_message || '').split(/\r?\n/).map(cleanAlertLine).filter(Boolean)
const buildAlertPreview = (row: Alert) => {
  const lines = alertMessageLines(row)
  const excluded = /内网\s*IP[：:]|实例名称[：:]|告警ip[：:]|告警主机[：:]/i
  const preferred = lines.filter((line) =>
    /提示[：:]|告警原因[：:]|文件路径[：:]|命令(?:行)?[：:]|描述[：:]/i.test(line) && !excluded.test(line)
  )
  const fallback = lines.filter((line) =>
    !excluded.test(line) &&
    !/AQS|核心文件告警|告警名称[：:]|告警编号[：:]|告警等级[：:]|告警时间[：:]|告警来源[：:]|首次发生时间[：:]|最新发生时间[：:]|攻击阶段[：:]|事件状态[：:]|^🏷\s*内容[：:]?$/i.test(line)
  )
  return (preferred.length ? preferred : fallback).slice(0, 2).join('\n') || row.title || '—'
}
const alertContent = (row: Alert) => row.message_preview || row.title || '—'
const globalSeveritySummary = computed(() => {
  const counts = globalStats.severity
  return Object.entries(counts).map(([key, count]) => `${severityText(key)} ${count}`).join(' / ') || '暂无'
})
const filteredDeviceName = computed(() => {
  const deviceId = Number(query.device_id)
  return alerts.value.find((item) => item.device_id === deviceId)?.device_name || `设备 #${query.device_id}`
})

const load = async () => {
  loading.value = true
  try {
    const params: Record<string, unknown> = { limit: pageSize.value, offset: (page.value - 1) * pageSize.value }
    ;['keyword', 'host', 'ip', 'from', 'to', 'alert_type', 'status', 'severity', 'department_id', 'device_id'].forEach((key) => {
      const value = query[key as keyof typeof query]
      if (value) params[key] = value
    })
    const [alertRes, statsRes, departmentRes] = await Promise.all([
      listAlerts(params),
      getAlertStats(),
      getAllDepartments()
    ])
    const alertData = unwrap(alertRes)
    const statsData = unwrap(statsRes)
    const departmentData = unwrap(departmentRes)
    const rows: Alert[] = alertData.items || alertData.list || []
    // 告警摘要只在本页数据加载时计算一次，避免选择行时反复解析长文本。
    alerts.value = rows.map((row) => ({ ...row, message_preview: buildAlertPreview(row) }))
    total.value = Number(alertData.total || 0)
    globalStats.total = Number(statsData.total || 0)
    globalStats.pending = Number(statsData.pending || 0)
    globalStats.confirmed = Number(statsData.confirmed || 0)
    globalStats.today = Number(statsData.today || 0)
    globalStats.severity = statsData.severity || {}
    departments.value = departmentData.items || departmentData.list || []
  } finally {
    loading.value = false
  }
}

const refresh = () => {
  load()
}

const reset = () => {
  Object.assign(query, { keyword: '', host: '', ip: '', from: '', to: '', alert_type: '', status: '', severity: '', department_id: '', device_id: '' })
  page.value = 1
  if (route.query.device_id) router.replace({ name: 'aegis-alerts' })
  load()
}
const clearDeviceFilter = () => {
  query.device_id = ''
  page.value = 1
  router.replace({ name: 'aegis-alerts' })
  load()
}
const openDetail = async (row: Alert) => {
  detailVisible.value = true
  current.value = row
  const data = unwrap(await getAlert(row.id))
  current.value = { ...row, ...data }
}
const openConfirm = (row: Alert, occurrenceId?: number) => {
  current.value = row
  confirmOccurrenceId.value = occurrenceId
  confirmNote.value = ''
  confirmVisible.value = true
}
const openHistory = async (row: Alert) => {
  historyAlert.value = row
  historyPage.value = 1
  historyRows.value = []
  historyVisible.value = true
  historyLoading.value = true
  try {
    const data = unwrap(await getAlertHistory(row.id))
    historyRows.value = data.items || data.list || []
  } finally {
    historyLoading.value = false
  }
}
const refreshAfterConfirm = async () => {
  await load()
  if (historyVisible.value && historyAlert.value) {
    const id = historyAlert.value.id
    const data = unwrap(await getAlertHistory(id))
    historyRows.value = data.items || data.list || []
    historyAlert.value = alerts.value.find((row) => row.id === id) || { ...historyAlert.value, ...unwrap(await getAlert(id)) }
  }
}
const confirmSubmit = async () => {
  if (confirming.value) return
  const note = confirmNote.value.trim()
  if (!current.value || Array.from(note).length < 5) return ElMessage.warning('确认备注至少 5 个字符')
  confirming.value = true
  try {
    await confirmAlert(current.value.id, note, confirmOccurrenceId.value)
    ElMessage.success('告警已确认')
    confirmVisible.value = false
    await refreshAfterConfirm()
  } finally {
    confirming.value = false
  }
}
const confirmSelected = async () => {
  if (confirming.value) return
  const ids = selected.value.filter(selectableAlert).map((row) => row.id)
  if (!ids.length) return ElMessage.warning('请先选择待确认告警')
  const result = await ElMessageBox.prompt('请输入批量确认备注', '批量确认', {
    inputValidator: (value) => { const n = Array.from((value || '').trim()).length; return n >= 5 && n <= 1000 },
    inputErrorMessage: '备注须为 5 至 1000 个字符'
  }).catch(() => null)
  if (!result) return
  confirming.value = true
  try {
    await confirmAlerts(ids, result.value.trim())
    selected.value = []
    ElMessage.success('批量确认完成')
    await refreshAfterConfirm()
  } finally {
    confirming.value = false
  }
}
const jsonText = (value?: string) => {
  if (!value) return '暂无详情'
  try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value }
}

onMounted(() => {
  const deviceId = route.query.device_id
  query.device_id = Array.isArray(deviceId) ? String(deviceId[0] || '') : String(deviceId || '')
  load()
})
</script>

<template>
  <div class="aegis-page">
    <div class="stat-grid">
      <div class="stat-card danger"><strong>{{ globalStats.pending }}</strong><span>待确认</span><small>{{ globalSeveritySummary }}</small></div>
      <div class="stat-card teal"><strong>{{ globalStats.confirmed }}</strong><span>已确认</span><small>全局聚合告警</small></div>
      <div class="stat-card"><strong>{{ globalStats.today }}</strong><span>今日告警（聚合后）</span><small>全局数据</small></div>
      <div class="stat-card"><strong>{{ globalStats.total }}</strong><span>告警总数</span><small>全局聚合告警</small></div>
    </div>

    <el-card shadow="never" class="panel">
      <div class="panel-heading"><h2>告警列表</h2><el-button size="small" @click="refresh">刷新数据</el-button></div>
      <div class="filter-row">
        <el-tag v-if="query.device_id" closable type="primary" effect="light" @close="clearDeviceFilter">主机：{{ filteredDeviceName }}</el-tag>
        <el-input v-model="query.host" clearable placeholder="🔍 实例名称" class="host-keyword" @keyup.enter="load" />
        <el-input v-model="query.ip" clearable placeholder="🔍 内网 IP" class="ip-keyword" @keyup.enter="load" />
        <el-date-picker v-model="query.from" type="date" value-format="YYYY-MM-DD" placeholder="最新发生起始" class="date-input" />
        <el-date-picker v-model="query.to" type="date" value-format="YYYY-MM-DD" placeholder="最新发生结束" class="date-input" />
        <el-select v-model="query.alert_type" clearable placeholder="告警类型" @change="page = 1; load()">
          <el-option label="可疑事件" value="suspicious_event" />
          <el-option label="Cloud SIEM 事件" value="custom_incident" />
          <el-option label="核心文件" value="core_file" />
        </el-select>
        <el-select v-model="query.severity" clearable placeholder="告警等级" @change="page = 1; load()">
          <el-option label="严重" value="serious" /><el-option label="高危" value="high" /><el-option label="可疑" value="suspicious" /><el-option label="中危" value="medium" /><el-option label="低危" value="low" /><el-option label="提示" value="info" />
        </el-select>
        <el-select v-model="query.status" clearable placeholder="状态" @change="page = 1; load()">
          <el-option label="待确认" value="firing" /><el-option label="已确认" value="confirmed" /><el-option label="已过期" value="stale" />
        </el-select>
        <el-select v-model="query.department_id" clearable filterable placeholder="所属部门" @change="page = 1; load()">
          <el-option v-for="item in departments" :key="item.id" :label="item.name" :value="String(item.id)" />
        </el-select>
        <el-input v-model="query.keyword" clearable placeholder="告警名称 / ID" class="keyword" @keyup.enter="load" />
        <el-button type="primary" @click="page = 1; load()">搜索</el-button>
        <el-button @click="reset">重置</el-button>
      </div>
      <div class="bulk-row" v-if="selected.length">
        <span>已选 {{ selected.length }} 条</span><el-button v-hasPermi="'aegis-alerts-confirm'" size="small" type="primary" :loading="confirming" @click="confirmSelected">批量确认</el-button>
      </div>
      <el-table v-loading="loading" :data="alerts" row-key="id" stripe @selection-change="selected = $event">
        <el-table-column type="selection" width="30" :selectable="selectableAlert" />
        <el-table-column label="序号" width="60" align="center"><template #default="{ $index }">{{ (page - 1) * pageSize + $index + 1 }}</template></el-table-column>
        <el-table-column label="等级" width="70"><template #default="{ row }"><el-tag type="info" effect="plain" :class="`severity-${row.severity || 'unknown'}`" size="small">{{ severityText(row.severity) }}</el-tag></template></el-table-column>
        <el-table-column prop="title" label="告警名称" width="145" show-overflow-tooltip />
        <el-table-column label="告警内容" min-width="150">
          <template #default="{ row }">
            <div class="alert-content" :title="row.last_message || '暂无告警全文'">{{ alertContent(row) }}</div>
          </template>
        </el-table-column>
        <el-table-column label="实例名称 / 内网 IP" min-width="130" show-overflow-tooltip><template #default="{ row }">{{ row.device_name || '未关联设备' }}<br /><span class="sub-text">{{ row.device_private_ip || '—' }}</span></template></el-table-column>
        <el-table-column label="所属组" width="120" show-overflow-tooltip><template #default="{ row }">{{ row.department_name || '未分组' }}</template></el-table-column>
        <el-table-column label="聚合" width="70"><template #default="{ row }"><el-button link type="primary" @click="openHistory(row)">×{{ row.occurrence_count || 1 }}</el-button></template></el-table-column>
        <el-table-column label="发生时间" width="210">
          <template #default="{ row }">
            <div class="time-cell">
              <span class="time-first">🕐 首次：{{ formatTime(row.first_provider_time || row.first_seen_at) }}</span>
              <span class="time-latest">🔄 最近：{{ formatTime(row.last_provider_time || row.last_seen_at) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="状态 / 确认" width="200">
          <template #default="{ row }">
            <div class="status-cell">
              <template v-if="row.status === 'confirmed'">
                <el-tag type="success" size="small">已确认</el-tag>
                <template v-if="row.confirmation_count >= 2">
                  <span>分 {{ row.confirmation_count }} 次确认</span>
                  <span>最近：{{ row.confirmed_by || '—' }} · {{ formatConfirmTime(row.confirmed_at) }}</span>
                </template>
                <template v-else>
                  <span>{{ row.confirmed_by || '—' }} · {{ formatConfirmTime(row.confirmed_at) }}</span>
                  <span class="confirmation-note">备注：{{ row.confirm_note || '—' }}</span>
                </template>
              </template>
              <template v-else>
                <div class="pending-actions">
                  <el-tag type="danger" size="small">{{ pendingLabel(row) }}</el-tag>
                  <el-button v-hasPermi="'aegis-alerts-confirm'" link type="success" @click="openConfirm(row)">确认</el-button>
                </div>
                <span v-if="row.confirmed_count">{{ row.confirmed_count }} 条已确认</span>
              </template>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="65"><template #default="{ row }"><el-button link type="primary" @click="openDetail(row)">详情</el-button></template></el-table-column>
      </el-table>
      <div class="pagination"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" background layout="total, sizes, prev, pager, next" :total="total" :page-sizes="[10, 20, 50, 100]" @current-change="load" @size-change="page = 1; load()" /></div>
    </el-card>

    <el-dialog v-model="detailVisible" title="告警详情" width="760px"><template v-if="current"><div class="detail-grid"><div><span>告警 ID</span><b>{{ current.provider_alert_id || '—' }}</b></div><div><span>🕐 首次发生时间</span><b>{{ formatTime(current.first_provider_time || current.first_seen_at) }}</b></div><div><span>🔄 最近发生时间</span><b class="detail-latest-time">{{ formatTime(current.last_provider_time || current.last_seen_at) }}</b></div><div><span>发生次数</span><b>{{ current.occurrence_count || 1 }}</b></div><div v-if="current.confirm_note" class="detail-note"><span>📝 确认备注</span><b>{{ current.confirm_note }}</b></div></div><pre class="payload">{{ jsonText(current.last_payload) }}</pre></template></el-dialog>
    <el-dialog v-model="historyVisible" class="history-dialog" title="聚合明细" width="min(1100px, 95vw)">
      <template #header><div class="history-title">聚合明细 <span>（×{{ historyAlert?.occurrence_count || 1 }}，同一告警 ID）</span></div></template>
      <div v-if="historyAlert" class="history-summary">
        <div class="history-summary-fields">
          <span class="history-summary-item"><strong>告警名称</strong>{{ historyAlert.title || '—' }}</span>
          <el-tag size="small" effect="dark" :class="`severity-${historyAlert.severity || 'unknown'}`">{{ severityText(historyAlert.severity) }}</el-tag>
          <span class="history-summary-item"><strong>告警ID</strong>{{ historyAlert.provider_alert_id || '—' }}</span>
          <span class="history-summary-item"><strong>实例名称</strong>{{ historyAlert.device_name || '未关联设备' }}</span>
        </div>
        <span class="history-summary-count">共 {{ historyRows.length || historyAlert.occurrence_count || 0 }} 条&nbsp;&nbsp; 未确认 <strong>{{ historyPending }}</strong> 条</span>
      </div>
      <div v-if="historyAlert" class="history-detail-box">
        <div class="history-detail-heading"><strong>告警详情</strong><el-tag size="small" effect="dark" :class="`severity-${historyAlert.severity || 'unknown'}`">本次</el-tag></div>
        <div class="history-detail-content">
          <div v-for="(line, index) in historyDetailLines" :key="index">{{ line }}</div>
        </div>
      </div>
      <div v-if="!historyLoading && historyRows.some((item) => item.snapshot)" class="history-notice">该告警的历史明细在本版本启用前未保存，当前展示的是最近一次聚合快照；后续每次告警变化都会记录明细。</div>
      <div v-if="!historyLoading && !historyRows.length" class="history-notice">暂无已保存的历史明细；历史明细从本次版本启用后开始记录。</div>
      <el-table v-else v-loading="historyLoading" :data="historyPageRows" class="history-table">
        <el-table-column type="index" :index="(index: number) => (historyPage - 1) * historyPageSize + index + 1" label="#" width="50" />
        <el-table-column prop="occurred_at" label="发生时间" min-width="170"><template #default="{ row }">{{ formatTime(row.occurred_at || row.captured_at) }}</template></el-table-column>
        <el-table-column label="确认状态" min-width="100"><template #default="{ row }"><el-tag :type="row.status === 'confirmed' ? 'success' : 'danger'" size="small">{{ statusText(row.status) }}</el-tag></template></el-table-column>
        <el-table-column label="确认人" min-width="120"><template #default="{ row }">{{ row.confirmed_by || '—' }}</template></el-table-column>
        <el-table-column label="确认时间" min-width="150"><template #default="{ row }">{{ formatConfirmTime(row.confirmed_at) }}</template></el-table-column>
        <el-table-column label="备注" min-width="160"><template #default="{ row }"><div class="confirmation-note">{{ row.confirm_note || '—' }}</div></template></el-table-column>
        <el-table-column label="操作" width="80" fixed="right"><template #default="{ row }"><el-button v-if="row.status !== 'confirmed' && historyAlert" v-hasPermi="'aegis-alerts-confirm'" link type="success" @click="openConfirm(historyAlert, row.id)">确认</el-button><span v-else>—</span></template></el-table-column>
      </el-table>
      <div class="pagination history-pagination"><el-pagination v-model:current-page="historyPage" v-model:page-size="historyPageSize" background layout="sizes, total, prev, pager, next" :total="historyRows.length" :page-sizes="[10, 20, 50]" @size-change="historyPage = 1" /></div>
      <template #footer><el-button @click="historyVisible = false">关闭</el-button></template>
    </el-dialog>
    <el-dialog v-model="confirmVisible" title="确认告警" width="520px"><el-form label-width="90px"><el-form-item label="告警名称">{{ current?.title }}</el-form-item><el-form-item label="确认备注" required><el-input v-model="confirmNote" type="textarea" :rows="4" minlength="5" maxlength="1000" show-word-limit placeholder="请输入至少 5 个字符的处理结论或确认原因" /></el-form-item></el-form><template #footer><el-button @click="confirmVisible = false">取消</el-button><el-button type="primary" :loading="confirming" @click="confirmSubmit">确认</el-button></template></el-dialog>
  </div>
</template>

<style scoped>
.aegis-page { padding: 22px; min-height: 100%; color: #3b4451; }.stat-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; margin-bottom: 16px; }.stat-card { background: #fff; border: 1px solid #e4e7ed; border-radius: 10px; padding: 14px 16px; }.stat-card span, .stat-card small { display: block; color: #8a94a6; font-size: 13px; }.stat-card strong { display: block; margin-bottom: 7px; font-size: 28px; color: #3b4451; }.stat-card.danger strong { color: #e6412e; }.stat-card.teal strong { color: #1a9c92; }.panel { border: 1px solid #e4e7ed; border-radius: 10px; }.panel-heading { display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; }.panel-heading h2 { margin: 0; font-size: 17px; color: #3b4451; }.filter-row { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-bottom: 14px; }.keyword { width: 170px; }.host-keyword { width: 160px; }.ip-keyword { width: 135px; }.date-input { width: 165px !important; }.filter-row .el-select { width: 135px; }.bulk-row { display: flex; align-items: center; gap: 12px; background: #eaf6f4; color: #167d73; border: 1px solid #bfe6e0; padding: 9px 12px; border-radius: 7px; margin-bottom: 12px; }.sub-text { color: #8a94a6; font-size: 12px; }.pagination { display: flex; justify-content: flex-end; margin-top: 16px; }.detail-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 12px; margin-bottom: 16px; }.detail-grid div { display: flex; flex-direction: column; gap: 4px; padding: 10px 12px; background: #f7f9fb; border-radius: 6px; }.detail-grid .detail-note { grid-column: 1 / -1; }.detail-grid span { color: #8a96a4; font-size: 12px; }.detail-grid b { word-break: break-all; white-space: pre-wrap; }.payload { max-height: 420px; overflow: auto; padding: 14px; background: #18232e; color: #d8f4ed; border-radius: 8px; font-size: 12px; white-space: pre-wrap; word-break: break-all; }
.history-title { font-size: 16px; font-weight: 600; }.history-title span { color: #8a94a6; font-size: 13px; font-weight: 400; }.history-notice { margin: 8px 0; padding: 9px 12px; border: 1px solid #b8d8f5; border-radius: 5px; color: #337ecc; background: #f0f7ff; font-size: 13px; }
.history-summary { display: flex; justify-content: space-between; align-items: center; gap: 16px; margin-bottom: 16px; font-size: 13px; }.history-summary-fields { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; min-width: 0; }.history-summary-item { display: inline-flex; align-items: center; gap: 9px; min-width: 0; overflow-wrap: anywhere; }.history-summary-item strong { white-space: nowrap; }.history-summary-count { flex: none; white-space: nowrap; }.history-summary-count strong { color: #d93025; }
.history-detail-box { display: flex; gap: 12px; min-height: 135px; max-height: 170px; padding: 12px 15px; margin-bottom: 14px; overflow: auto; border: 1px dashed #e56b44; border-radius: 8px; background: #fffaf8; font-size: 13px; line-height: 1.65; }.history-detail-heading { display: flex; align-items: flex-start; flex: none; gap: 7px; white-space: nowrap; }.history-detail-content { min-width: 0; color: #374151; overflow-wrap: anywhere; }.history-detail-content > div:first-child { color: #7a8494; }.history-pagination { margin-top: 10px; }.history-table :deep(.el-table__header-wrapper th) { background: #f7f8fa; }
:global(.history-dialog .el-dialog__header) { padding: 17px 20px; margin-right: 0; border-bottom: 1px solid #e4e7ed; }:global(.history-dialog .el-dialog__body) { padding: 18px 20px 16px; }:global(.history-dialog .el-dialog__footer) { padding: 14px 20px; border-top: 1px solid #e4e7ed; }
.status-cell { display: flex; flex-direction: column; align-items: flex-start; gap: 5px; }.status-cell > span:not(.el-tag) { max-width: 100%; color: #7a8494; font-size: 12px; white-space: pre-wrap; overflow-wrap: anywhere; }
.confirmation-note { white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.5; }.pending-actions { display: flex; align-items: center; gap: 10px; }.history-count { margin-right: auto; color: #8a94a6; font-size: 12px; }
.time-cell { display: flex; flex-direction: column; gap: 5px; font-size: 12px; line-height: 1.35; white-space: nowrap; }.time-first { color: #8a94a6; }.time-latest, .detail-latest-time { color: #409eff; }
.alert-content { max-width: 100%; overflow: hidden; line-height: 1.55; text-overflow: ellipsis; white-space: pre; }
:deep(.severity-serious) { color: #fff; border-color: #d93025; background: #d93025; }.severity-high, :deep(.severity-high) { color: #fff; border-color: #f08c00; background: #f08c00; }.severity-suspicious, :deep(.severity-suspicious) { color: #fff; border-color: #c46b00; background: #c46b00; }.severity-medium, :deep(.severity-medium) { color: #fff; border-color: #7c3aed; background: #7c3aed; }.severity-low, :deep(.severity-low) { color: #fff; border-color: #2563eb; background: #2563eb; }.severity-info, :deep(.severity-info) { color: #fff; border-color: #16a34a; background: #16a34a; }.severity-unknown, :deep(.severity-unknown) { color: #fff; border-color: #909399; background: #909399; }
@media (max-width: 1100px) { .stat-grid { grid-template-columns: repeat(2, 1fr); } }
@media (max-width: 700px) { .history-summary { align-items: flex-start; flex-direction: column; }.history-summary-count { white-space: normal; }.history-detail-box { display: block; }.history-detail-heading { margin-bottom: 7px; } }
</style>
