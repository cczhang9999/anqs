<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElAlert, ElIcon, ElMessage, ElMessageBox } from 'element-plus'
import { Clock, RefreshRight, Search } from '@element-plus/icons-vue'
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
const listLoadFailed = ref(false)
let listRequest = 0
const alerts = ref<Alert[]>([])
const total = ref(0)
const selected = ref<Alert[]>([])
const departments = ref<Department[]>([])
const page = ref(1)
const pageSize = ref(10)
const detailVisible = ref(false)
const detailLoading = ref(false)
const detailLoadFailed = ref(false)
const confirmVisible = ref(false)
const historyVisible = ref(false)
const historyLoading = ref(false)
const historyLoadFailed = ref(false)
const historyRows = ref<AlertOccurrence[]>([])
const historyAlert = ref<Alert | null>(null)
const current = ref<Alert | null>(null)
const confirmTarget = ref<Alert | null>(null)
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

const emptyQuery = {
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
}
const query = reactive({ ...emptyQuery })
let appliedQuery = { ...emptyQuery }

const unwrap = (res: any) => res?.data || res || {}
const timeFormatter = new Intl.DateTimeFormat('sv-SE', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit'
})
const formatTime = (value?: string) => {
  if (!value) return '—'
  if (!/(?:Z|[+-]\d{2}:\d{2})$/i.test(value)) return value.replace('T', ' ')
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : timeFormatter.format(date)
}
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
    !/AQS|核心文件告警|告警名称[：:]|告警编号[：:]|告警等级[：:]|告警时间[：:]|告警来源[：:]|首次发生时间[：:]|最新发生时间[：:]|攻击阶段[：:]|事件状态[：:]|^(?:告警名|日志时间|事件 ID|等级标签|资产名称|私网 IP|告警时间)[：:]|^【[^】]+】$|^🏷\s*内容[：:]?$/i.test(line)
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
  const request = ++listRequest
  loading.value = true
  listLoadFailed.value = false
  try {
    const params = {
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value,
      ...Object.fromEntries(Object.entries(appliedQuery).filter(([, value]) => value))
    }
    const [alertRes, statsRes, departmentRes] = await Promise.all([
      listAlerts(params),
      getAlertStats(),
      getAllDepartments()
    ])
    if (request !== listRequest) return
    if (!alertRes || !statsRes || !departmentRes) throw new Error('Incomplete alert list response')
    const alertData = unwrap(alertRes)
    const statsData = unwrap(statsRes)
    const departmentData = unwrap(departmentRes)
    const rows: Alert[] = alertData.items || alertData.list || []
    alerts.value = rows.map((row) => ({ ...row, message_preview: buildAlertPreview(row) }))
    total.value = Number(alertData.total || 0)
    globalStats.total = Number(statsData.total || 0)
    globalStats.pending = Number(statsData.pending || 0)
    globalStats.confirmed = Number(statsData.confirmed || 0)
    globalStats.today = Number(statsData.today || 0)
    globalStats.severity = statsData.severity || {}
    departments.value = departmentData.items || departmentData.list || []
  } catch {
    if (request !== listRequest) return
    listLoadFailed.value = true
    alerts.value = []
    selected.value = []
  } finally {
    if (request === listRequest) loading.value = false
  }
}

const search = () => {
  if (query.from && query.to && query.from > query.to) {
    ElMessage.warning('最新发生结束日期须在起始日期当天或之后')
    return
  }
  appliedQuery = { ...query, host: query.host.trim(), ip: query.ip.trim(), keyword: query.keyword.trim() }
  page.value = 1
  return load()
}
const reset = () => {
  Object.assign(query, emptyQuery)
  if (route.query.device_id) router.replace({ name: 'aegis-alerts' })
  return search()
}
const clearDeviceFilter = () => {
  query.device_id = ''
  appliedQuery = { ...appliedQuery, device_id: '' }
  page.value = 1
  router.replace({ name: 'aegis-alerts' })
  return load()
}
let detailRequest = 0
const openDetail = async (row: Alert) => {
  const request = ++detailRequest
  detailVisible.value = true
  detailLoading.value = true
  detailLoadFailed.value = false
  current.value = row
  try {
    const result = await getAlert(row.id)
    if (!result) throw new Error('Empty alert detail response')
    if (request === detailRequest) current.value = { ...row, ...unwrap(result) }
  } catch {
    if (request === detailRequest) detailLoadFailed.value = true
  } finally {
    if (request === detailRequest) detailLoading.value = false
  }
}
const openConfirm = (row: Alert, occurrenceId?: number) => {
  confirmTarget.value = row
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
  historyLoadFailed.value = false
  try {
    const result = await getAlertHistory(row.id)
    if (!result) throw new Error('Empty alert history response')
    const data = unwrap(result)
    historyRows.value = data.items || data.list || []
  } catch {
    historyLoadFailed.value = true
  } finally {
    historyLoading.value = false
  }
}
const refreshAfterConfirm = async () => {
  await load()
  if (historyVisible.value && historyAlert.value) {
    const id = historyAlert.value.id
    const updated = alerts.value.find((row) => row.id === id)
    await openHistory(updated || historyAlert.value)
  }
}
const confirmSubmit = async () => {
  if (confirming.value) return
  const target = confirmTarget.value
  if (!target) return
  const note = confirmNote.value.trim()
  const noteLength = Array.from(note).length
  if (noteLength < 5 || noteLength > 1000) return ElMessage.warning('备注须为 5 至 1000 个字符')
  confirming.value = true
  try {
    const result = await confirmAlert(target.id, note, confirmOccurrenceId.value)
    if (!result) return
    const notification = unwrap(result)
    if (notification.notification_error) {
      ElMessage.warning(`告警已确认，群消息未发送：${notification.notification_error}`)
    } else if (notification.notification_sent === true) {
      ElMessage.success('告警已确认，群消息已提交')
    } else {
      ElMessage.info('告警确认完成')
    }
    if (current.value?.id === target.id) {
      current.value = { ...current.value, ...unwrap(result) }
    }
    confirmVisible.value = false
    await refreshAfterConfirm()
  } catch {
    // The request interceptor displays API errors.
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
    const response = await confirmAlerts(ids, result.value.trim())
    if (!response) return
    const results: Array<{ notification_sent?: boolean; notification_error?: string }> = unwrap(response).items || []
    const failedNotifications = results.filter((item) => item.notification_error)
    const sentCount = results.filter((item) => item.notification_sent === true).length
    selected.value = []
    if (failedNotifications.length) {
      const errors = [...new Set(failedNotifications.map((item) => item.notification_error))]
      ElMessage.warning(`批量确认完成，${failedNotifications.length} 条群消息未发送：${errors.join('；')}`)
    } else if (sentCount > 0) {
      ElMessage.success(`批量确认完成，已提交 ${sentCount} 条群消息`)
    } else {
      ElMessage.info('批量确认完成')
    }
    await refreshAfterConfirm()
  } catch {
    // The request interceptor displays API errors.
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
  appliedQuery = { ...query }
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
      <div class="panel-heading">
        <h2>告警列表</h2>
        <el-button :loading="loading" @click="load">刷新数据</el-button>
      </div>
      <el-tag v-if="query.device_id" closable type="primary" effect="light" class="device-filter" @close="clearDeviceFilter">
        主机：{{ filteredDeviceName }}
      </el-tag>
      <form class="alert-filters" aria-label="告警筛选" @submit.prevent="search">
        <div class="filter-grid">
          <el-input v-model="query.host" :prefix-icon="Search" clearable placeholder="实例名称" aria-label="实例名称" />
          <el-input v-model="query.ip" :prefix-icon="Search" clearable placeholder="内网 IP" aria-label="内网 IP" />
          <el-date-picker v-model="query.from" type="date" value-format="YYYY-MM-DD" placeholder="最新发生起始" aria-label="最新发生起始" />
          <el-date-picker v-model="query.to" type="date" value-format="YYYY-MM-DD" placeholder="最新发生结束" aria-label="最新发生结束" />
          <el-select v-model="query.alert_type" clearable placeholder="告警类型" aria-label="告警类型">
            <el-option label="可疑事件" value="suspicious_event" />
            <el-option label="Cloud SIEM 事件" value="custom_incident" />
            <el-option label="核心文件" value="core_file" />
            <el-option label="蜜罐" value="honeypot" />
          </el-select>
          <el-select v-model="query.severity" clearable placeholder="告警等级" aria-label="告警等级">
            <el-option label="严重" value="serious" /><el-option label="高危" value="high" /><el-option label="可疑" value="suspicious" /><el-option label="中危" value="medium" /><el-option label="低危" value="low" /><el-option label="提示" value="info" />
          </el-select>
          <el-select v-model="query.status" clearable placeholder="状态" aria-label="状态">
            <el-option label="待确认" value="firing" /><el-option label="已确认" value="confirmed" /><el-option label="已过期" value="stale" />
          </el-select>
          <el-select v-model="query.department_id" clearable filterable placeholder="所属部门" aria-label="所属部门">
            <el-option v-for="item in departments" :key="item.id" :label="item.name" :value="String(item.id)" />
          </el-select>
        </div>
        <div class="filter-actions">
          <el-input v-model="query.keyword" clearable placeholder="告警名称 / ID" aria-label="告警名称 / ID" />
          <el-button native-type="submit" type="primary" :loading="loading">搜索</el-button>
          <el-button @click="reset">重置</el-button>
        </div>
      </form>
      <div class="bulk-row" v-if="selected.length">
        <span>已选 {{ selected.length }} 条</span><el-button v-hasPermi="'aegis-alerts-confirm'" size="small" type="primary" :loading="confirming" @click="confirmSelected">批量确认</el-button>
      </div>
      <el-table
        v-loading="loading" :data="alerts" row-key="id" stripe class="alert-table"
        :empty-text="listLoadFailed ? '告警加载失败，请刷新重试' : '暂无符合条件的告警'"
        @selection-change="selected = $event"
      >
        <el-table-column type="selection" width="40" align="center" :selectable="selectableAlert" />
        <el-table-column label="序号" width="60" align="center"><template #default="{ $index }">{{ (page - 1) * pageSize + $index + 1 }}</template></el-table-column>
        <el-table-column label="等级" width="70"><template #default="{ row }"><el-tag type="info" effect="plain" :class="`severity-${row.severity || 'unknown'}`" size="small">{{ severityText(row.severity) }}</el-tag></template></el-table-column>
        <el-table-column prop="title" label="告警名称" min-width="145" show-overflow-tooltip />
        <el-table-column label="告警内容" min-width="285">
          <template #default="{ row }">
            <div class="alert-content" :title="alertContent(row)">{{ alertContent(row) }}</div>
          </template>
        </el-table-column>
        <el-table-column label="实例名称 / 内网 IP" min-width="175" show-overflow-tooltip><template #default="{ row }">{{ row.device_name || '未关联设备' }}<br /><span class="sub-text">{{ row.device_private_ip || '—' }}</span></template></el-table-column>
        <el-table-column label="所属组" min-width="100" show-overflow-tooltip><template #default="{ row }">{{ row.department_name || '未分组' }}</template></el-table-column>
        <el-table-column label="聚合" width="70"><template #default="{ row }"><el-button link type="primary" @click="openHistory(row)">×{{ row.occurrence_count || 1 }}</el-button></template></el-table-column>
        <el-table-column label="发生时间" width="210">
          <template #default="{ row }">
            <div class="time-cell">
              <span class="time-first"><el-icon><Clock /></el-icon>首次：{{ formatTime(row.first_provider_time || row.first_seen_at) }}</span>
              <span class="time-latest"><el-icon><RefreshRight /></el-icon>最近：{{ formatTime(row.last_provider_time || row.last_seen_at) }}</span>
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
                  <el-tag v-if="row.status === 'stale'" type="info" size="small">已过期</el-tag>
                  <el-tag v-else type="danger" size="small">{{ pendingLabel(row) }}</el-tag>
                  <el-button v-hasPermi="'aegis-alerts-confirm'" link type="success" @click="openConfirm(row)">确认</el-button>
                </div>
                <span v-if="row.confirmed_count">{{ row.confirmed_count }} 条已确认</span>
              </template>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="65" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="openDetail(row)">详情</el-button></template></el-table-column>
      </el-table>
      <div class="pagination"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" background layout="total, sizes, prev, pager, next" :total="total" :page-sizes="[10, 20, 50, 100]" @current-change="load" @size-change="page = 1; load()" /></div>
    </el-card>

    <el-dialog v-model="detailVisible" title="告警详情" width="min(760px, 95vw)">
      <div v-if="current" v-loading="detailLoading">
        <div class="detail-heading">
          <h3>{{ current.title || '—' }}</h3>
          <el-tag size="small" :class="`severity-${current.severity || 'unknown'}`">{{ severityText(current.severity) }}</el-tag>
          <el-tag size="small" :type="current.status === 'confirmed' ? 'success' : 'info'">{{ statusText(current.status) }}</el-tag>
        </div>
        <div class="detail-grid">
          <div><span>告警 ID</span><b>{{ current.provider_alert_id || '—' }}</b></div>
          <div><span>发生次数</span><b>{{ current.occurrence_count || 1 }}</b></div>
          <div><span>首次发生时间</span><b>{{ formatTime(current.first_provider_time || current.first_seen_at) }}</b></div>
          <div><span>最近发生时间</span><b class="detail-latest-time">{{ formatTime(current.last_provider_time || current.last_seen_at) }}</b></div>
          <div v-if="current.confirmed_at"><span>最近确认</span><b>{{ current.confirmed_by || '—' }} · {{ formatTime(current.confirmed_at) }}</b></div>
          <div v-if="current.confirm_note" class="detail-note"><span>确认备注</span><b>{{ current.confirm_note }}</b></div>
        </div>
        <h4 class="detail-body-heading">告警内容</h4>
        <pre class="detail-message">{{ current.last_message || '暂无告警内容' }}</pre>
        <el-alert v-if="detailLoadFailed" title="原始数据加载失败，可重新打开详情重试" type="error" :closable="false" show-icon />
        <details v-else-if="current.last_payload" :key="current.id" class="raw-payload">
          <summary>原始告警数据</summary>
          <pre class="payload">{{ jsonText(current.last_payload) }}</pre>
        </details>
      </div>
      <template #footer>
        <el-button @click="detailVisible = false">关闭</el-button>
        <el-button v-if="current && selectableAlert(current)" v-hasPermi="'aegis-alerts-confirm'" type="primary" @click="openConfirm(current)">确认告警</el-button>
      </template>
    </el-dialog>
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
      <el-alert v-if="historyLoadFailed" title="聚合明细加载失败，请重新打开重试" type="error" :closable="false" show-icon />
      <div v-if="!historyLoading && historyRows.some((item) => item.snapshot)" class="history-notice">该告警的历史明细在本版本启用前未保存，当前展示的是最近一次聚合快照；后续每次告警变化都会记录明细。</div>
      <div v-if="!historyLoading && !historyLoadFailed && !historyRows.length" class="history-notice">暂无已保存的历史明细；历史明细从本次版本启用后开始记录。</div>
      <el-table v-else-if="!historyLoadFailed" v-loading="historyLoading" :data="historyPageRows" class="history-table">
        <el-table-column type="index" :index="(index: number) => (historyPage - 1) * historyPageSize + index + 1" label="#" width="50" />
        <el-table-column prop="occurred_at" label="发生时间" min-width="170"><template #default="{ row }">{{ formatTime(row.occurred_at || row.captured_at) }}</template></el-table-column>
        <el-table-column label="确认状态" min-width="100"><template #default="{ row }"><el-tag :type="row.status === 'confirmed' ? 'success' : 'danger'" size="small">{{ statusText(row.status) }}</el-tag></template></el-table-column>
        <el-table-column label="确认人" min-width="120"><template #default="{ row }">{{ row.confirmed_by || '—' }}</template></el-table-column>
        <el-table-column label="确认时间" min-width="150"><template #default="{ row }">{{ formatConfirmTime(row.confirmed_at) }}</template></el-table-column>
        <el-table-column label="备注" min-width="160"><template #default="{ row }"><div class="confirmation-note">{{ row.confirm_note || '—' }}</div></template></el-table-column>
        <el-table-column label="操作" width="80" fixed="right"><template #default="{ row }"><el-button v-if="row.status !== 'confirmed' && historyAlert" v-hasPermi="'aegis-alerts-confirm'" link type="success" @click="openConfirm(historyAlert, row.id)">确认</el-button><span v-else>—</span></template></el-table-column>
      </el-table>
      <div v-if="historyRows.length" class="pagination history-pagination"><el-pagination v-model:current-page="historyPage" v-model:page-size="historyPageSize" background layout="sizes, total, prev, pager, next" :total="historyRows.length" :page-sizes="[10, 20, 50]" @size-change="historyPage = 1" /></div>
      <template #footer><el-button @click="historyVisible = false">关闭</el-button></template>
    </el-dialog>
    <el-dialog v-model="confirmVisible" title="确认告警" width="min(520px, 95vw)" append-to-body><el-form label-width="90px"><el-form-item label="告警名称">{{ confirmTarget?.title }}</el-form-item><el-form-item label="确认备注" required><el-input v-model="confirmNote" type="textarea" :rows="4" minlength="5" maxlength="1000" show-word-limit placeholder="请输入至少 5 个字符的处理结论或确认原因" /></el-form-item></el-form><template #footer><el-button @click="confirmVisible = false">取消</el-button><el-button type="primary" :loading="confirming" @click="confirmSubmit">确认</el-button></template></el-dialog>
  </div>
</template>

<style scoped>
.aegis-page {
  min-height: 100%;
  padding: 18px 22px 24px;
  color: #394250;
}
.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
  margin-bottom: 16px;
}
.stat-card {
  min-height: 105px;
  padding: 14px 16px;
  background: #fff;
  border: 1px solid #e0e4eb;
  border-radius: 10px;
}
.stat-card span, .stat-card small {
  display: block;
  color: #647288;
  font-size: 13px;
  line-height: 1.45;
}
.stat-card strong {
  display: block;
  margin-bottom: 5px;
  color: #394250;
  font-size: 29px;
  line-height: 1.35;
  font-variant-numeric: tabular-nums;
}
.stat-card.danger strong { color: #e6412e; }
.stat-card.teal strong { color: #178f85; }
.panel { border: 1px solid #e0e4eb; border-radius: 10px; }
.panel-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}
.panel-heading h2 { margin: 0; font-size: 18px; color: #394250; }
.panel-heading .el-button { height: 26px; font-size: 12px; }
.device-filter { margin-bottom: 10px; }
.alert-filters { container-type: inline-size; margin-bottom: 14px; }
.filter-grid {
  display: grid;
  max-width: 1360px;
  grid-template-columns: minmax(120px, 1.18fr) minmax(110px, 1fr) repeat(2, minmax(150px, 1.62fr)) repeat(4, minmax(110px, 1fr));
  gap: 10px;
}
.filter-grid :deep(.el-date-editor) { width: 100%; }
.filter-actions {
  display: grid;
  grid-template-columns: minmax(0, 170px) auto auto;
  gap: 10px;
  justify-content: start;
  margin-top: 10px;
}
.filter-actions .el-button { margin-left: 0; }
@container (max-width: 1030px) {
  .filter-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); }
}
@container (max-width: 570px) {
  .filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .filter-actions { grid-template-columns: minmax(0, 1fr) auto auto; }
}
.bulk-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
  padding: 9px 12px;
  color: #167d73;
  background: #eaf6f4;
  border: 1px solid #bfe6e0;
  border-radius: 7px;
}
.alert-table { --el-table-border-color: #e8ecf2; }
.alert-table :deep(th.el-table__cell) { padding-block: 9px; color: #667284; }
.alert-table :deep(td.el-table__cell) { padding-block: 10px; }
.alert-table :deep(.el-table__row--striped td.el-table__cell) { --el-fill-color-lighter: #f3f6fa; }
.alert-table :deep(.el-scrollbar__bar.is-horizontal) { height: 8px; }
.sub-text { color: #647288; font-size: 12px; }
.pagination { display: flex; justify-content: flex-end; margin-top: 16px; overflow-x: auto; }
.detail-heading { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-bottom: 12px; }
.detail-heading h3 { margin: 0; color: #394250; font-size: 16px; overflow-wrap: anywhere; }
.detail-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-bottom: 16px; }
.detail-grid div { display: flex; flex-direction: column; gap: 4px; padding: 10px 12px; background: #f7f9fb; border-radius: 6px; }
.detail-grid .detail-note { grid-column: 1 / -1; }
.detail-grid span { color: #647288; font-size: 12px; }
.detail-grid b { overflow-wrap: anywhere; white-space: pre-wrap; }
.detail-body-heading { margin: 0 0 8px; }
.detail-message, .payload {
  max-height: 360px;
  margin: 0 0 12px;
  padding: 14px;
  overflow: auto;
  color: #394250;
  background: #f7f9fb;
  border: 1px solid #e0e4eb;
  border-radius: 8px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.detail-message { font: inherit; line-height: 1.65; }
.payload { margin-block: 8px 0; font-size: 12px; }
.raw-payload summary { width: fit-content; color: var(--el-color-primary-dark-2); cursor: pointer; }
.history-title { font-size: 16px; font-weight: 600; }.history-title span { color: #8a94a6; font-size: 13px; font-weight: 400; }.history-notice { margin: 8px 0; padding: 9px 12px; border: 1px solid #b8d8f5; border-radius: 5px; color: #337ecc; background: #f0f7ff; font-size: 13px; }
.history-summary { display: flex; justify-content: space-between; align-items: center; gap: 16px; margin-bottom: 16px; font-size: 13px; }.history-summary-fields { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; min-width: 0; }.history-summary-item { display: inline-flex; align-items: center; gap: 9px; min-width: 0; overflow-wrap: anywhere; }.history-summary-item strong { white-space: nowrap; }.history-summary-count { flex: none; white-space: nowrap; }.history-summary-count strong { color: #d93025; }
.history-detail-box { display: flex; gap: 12px; min-height: 135px; max-height: 170px; padding: 12px 15px; margin-bottom: 14px; overflow: auto; border: 1px dashed #e56b44; border-radius: 8px; background: #fffaf8; font-size: 13px; line-height: 1.65; }.history-detail-heading { display: flex; align-items: flex-start; flex: none; gap: 7px; white-space: nowrap; }.history-detail-content { min-width: 0; color: #374151; overflow-wrap: anywhere; }.history-detail-content > div:first-child { color: #7a8494; }.history-pagination { margin-top: 10px; }.history-table :deep(.el-table__header-wrapper th) { background: #f7f8fa; }
:global(.history-dialog .el-dialog__header) { padding: 17px 20px; margin-right: 0; border-bottom: 1px solid #e4e7ed; }:global(.history-dialog .el-dialog__body) { padding: 18px 20px 16px; }:global(.history-dialog .el-dialog__footer) { padding: 14px 20px; border-top: 1px solid #e4e7ed; }
.status-cell { display: flex; flex-direction: column; align-items: flex-start; gap: 5px; }.status-cell > span:not(.el-tag) { max-width: 100%; color: #7a8494; font-size: 12px; white-space: pre-wrap; overflow-wrap: anywhere; }
.confirmation-note { white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.5; }.pending-actions { display: flex; align-items: center; gap: 10px; }.history-count { margin-right: auto; color: #8a94a6; font-size: 12px; }
.time-cell { display: flex; flex-direction: column; gap: 5px; font-size: 12px; line-height: 1.35; white-space: nowrap; }.time-cell > span { display: flex; align-items: center; gap: 4px; }
.time-first { color: #647288; }
.time-latest, .detail-latest-time { color: #2677c8; }
.alert-content {
  display: -webkit-box;
  overflow: hidden;
  line-height: 1.55;
  white-space: pre-line;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
:deep(.severity-serious) { color: #fff; border-color: #d93025; background: #d93025; }.severity-high, :deep(.severity-high) { color: #fff; border-color: #f08c00; background: #f08c00; }.severity-suspicious, :deep(.severity-suspicious) { color: #fff; border-color: #c46b00; background: #c46b00; }.severity-medium, :deep(.severity-medium) { color: #fff; border-color: #7c3aed; background: #7c3aed; }.severity-low, :deep(.severity-low) { color: #fff; border-color: #2563eb; background: #2563eb; }.severity-info, :deep(.severity-info) { color: #fff; border-color: #16a34a; background: #16a34a; }.severity-unknown, :deep(.severity-unknown) { color: #fff; border-color: #909399; background: #909399; }
@media (max-width: 1100px) {
  .stat-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 700px) {
  .aegis-page { padding: 12px; }
  .stat-grid { gap: 8px; }
  .stat-card { padding: 10px; }
  .panel :deep(.el-card__body) { padding: 12px; }
  .pagination { justify-content: safe flex-end; }
}
@media (max-width: 700px) { .history-summary { align-items: flex-start; flex-direction: column; }.history-summary-count { white-space: normal; }.history-detail-box { display: block; }.history-detail-heading { margin-bottom: 7px; } }
</style>
