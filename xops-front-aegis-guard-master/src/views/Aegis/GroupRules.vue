<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  createGroupRule,
  deleteGroupRule,
  executeGroupRules,
  getAllDepartments,
  getGroupRuleStats,
  listGroupRules,
  previewGroupRules,
  reorderGroupRule,
  toggleGroupRule,
  updateGroupRule
} from '@/api/aegis'

interface Condition { operator: string; value: string }
interface GroupRule {
  id: number
  name: string
  department_id: number
  department_name?: string
  enabled: boolean
  sort_order: number
  conditions: Condition[]
  host_count: number
}
interface Department { id: number; name: string }
interface PreviewItem {
  device_id: number
  instance_name: string
  private_ip: string
  current_department_id: number
  department_id: number
  department_name: string
  rule_id: number
  rule_name: string
  match_conditions: Condition[]
}

// 主机名比较区分大小写；保留 equals 的展示名称以兼容已有规则。
const HOSTNAME_OPERATORS = [
  { value: 'contains', label: '包含' },
  { value: 'not_contains', label: '不包含' },
  { value: 'prefix', label: '前缀是' },
  { value: 'not_prefix', label: '前缀不是' },
  { value: 'suffix', label: '后缀是' },
  { value: 'not_suffix', label: '后缀不是' },
  { value: 'regex', label: '正则匹配' }
]
const IP_OPERATORS = [
  { value: 'ip_in', label: 'IP 属于' },
  { value: 'ip_not_in', label: 'IP 不属于' }
]
const OPERATORS = [...HOSTNAME_OPERATORS, ...IP_OPERATORS, { value: 'equals', label: '完全等于' }]
const NEGATIVE_OPERATORS = ['not_contains', 'not_prefix', 'not_suffix', 'ip_not_in']
const operatorLabel = (value: string) => OPERATORS.find((item) => item.value === value)?.label || value
const isRegex = (value: string) => value === 'regex'
const isIpOperator = (value: string) => IP_OPERATORS.some((item) => item.value === value)

const ipVersion = (value: string): 4 | 6 | 0 => {
  const parts = value.split('.')
  if (parts.length === 4 && parts.every((part) => /^(0|[1-9]\d{0,2})$/.test(part) && Number(part) <= 255)) return 4
  if (!value || value.includes('%')) return 0
  try {
    const hostname = new URL(`http://[${value}]/`).hostname
    return hostname.startsWith('[') && hostname.endsWith(']') ? 6 : 0
  } catch {
    return 0
  }
}

const validateIpValue = (value: string) => {
  const parts = value.split('/')
  if (parts.length > 2) return 'IP/CIDR 格式错误'
  const version = ipVersion(parts[0])
  if (!version) return 'IP/CIDR 格式错误'
  if (parts.length === 1) return ''
  if (!/^\d+$/.test(parts[1])) return 'CIDR 前缀长度格式错误'
  const prefix = Number(parts[1])
  const maxPrefix = version === 4 ? 32 : 128
  return prefix <= maxPrefix ? '' : `IPv${version} 的 CIDR 前缀长度必须为 0–${maxPrefix}`
}

// 目标部门标签取一组固定色板，颜色按部门 ID 稳定分配。
const DEPARTMENT_COLORS = ['#f0a020', '#8b5cf6', '#20b2a6', '#e6412e', '#ec4899', '#12b76a', '#3b82f6', '#a16207', '#0ea5e9', '#9333ea']
const departmentColor = (id: number) => DEPARTMENT_COLORS[Math.abs(Number(id) || 0) % DEPARTMENT_COLORS.length]

const loading = ref(false)
const saving = ref(false)
const rules = ref<GroupRule[]>([])
const departments = ref<Department[]>([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const query = reactive({ keyword: '', department_id: '' })
const stats = reactive({ enabled_rules: 0, total_rules: 0, rule_hosts: 0, manual_hosts: 0, unassigned_hosts: 0 })

const dialogVisible = ref(false)
const form = reactive<{ id: number; name: string; department_id: number | undefined; enabled: boolean; conditions: Condition[] }>({
  id: 0,
  name: '',
  department_id: undefined,
  enabled: true,
  conditions: [{ operator: 'contains', value: '' }]
})
const errors = reactive<{ name: string; dept: string; general: string; rows: Record<number, string> }>({
  name: '',
  dept: '',
  general: '',
  rows: {}
})
const editingRule = computed(() => rules.value.find((item) => item.id === form.id) || null)
const dialogTitle = computed(() => {
  if (!form.id) return '新增规则'
  const order = editingRule.value ? editingRule.value.sort_order + 1 : '?'
  return `编辑规则（顺序 ${order}）`
})

const previewVisible = ref(false)
const previewLoading = ref(false)
const executing = ref(false)
const preview = reactive({ total_unassigned: 0, matched_total: 0 })
const previewItems = ref<PreviewItem[]>([])
const previewPage = ref(1)
const previewPageSize = 10

const unwrap = (res: any) => res?.data || res || {}

const load = async () => {
  loading.value = true
  try {
    const params: Record<string, unknown> = { limit: pageSize.value, offset: (page.value - 1) * pageSize.value }
    if (query.keyword.trim()) params.keyword = query.keyword.trim()
    if (query.department_id !== '') params.department_id = query.department_id
    const [ruleRes, statsRes, deptRes] = await Promise.all([listGroupRules(params), getGroupRuleStats(), getAllDepartments()])
    const ruleData = unwrap(ruleRes)
    rules.value = ruleData.items || ruleData.list || []
    total.value = Number(ruleData.total || 0)
    const statsData = unwrap(statsRes)
    stats.enabled_rules = Number(statsData.enabled_rules || 0)
    stats.total_rules = Number(statsData.total_rules || 0)
    stats.rule_hosts = Number(statsData.rule_hosts || 0)
    stats.manual_hosts = Number(statsData.manual_hosts || 0)
    stats.unassigned_hosts = Number(statsData.unassigned_hosts || 0)
    const deptData = unwrap(deptRes)
    departments.value = deptData.items || deptData.list || []
  } finally {
    loading.value = false
  }
}

const search = () => { page.value = 1; load() }
const reset = () => { query.keyword = ''; query.department_id = ''; page.value = 1; load() }

// 条件在列表中的展示：正值多值用「或」，否定类多值用「、」表示都不能出现。
const conditionValues = (condition: Condition) => {
  if (isRegex(condition.operator)) return [condition.value.trim()]
  return condition.value.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
}
const conditionText = (condition: Condition) => {
  const values = conditionValues(condition)
  if (!values.length) return `${operatorLabel(condition.operator)} —`
  const joined = NEGATIVE_OPERATORS.includes(condition.operator) ? values.join('、') : values.join(' 或 ')
  return `${operatorLabel(condition.operator)} ${joined}`
}

const resetForm = () => {
  form.id = 0
  form.name = ''
  form.department_id = undefined
  form.enabled = true
  form.conditions = [{ operator: 'contains', value: '' }]
  errors.name = ''
  errors.dept = ''
  errors.general = ''
  errors.rows = {}
}
const openCreate = () => { resetForm(); dialogVisible.value = true }
const openEdit = (row: GroupRule) => {
  resetForm()
  form.id = row.id
  form.name = row.name
  form.department_id = row.department_id
  form.enabled = row.enabled
  form.conditions = (row.conditions || []).map((item) => ({ operator: item.operator, value: item.value }))
  if (!form.conditions.length) form.conditions = [{ operator: 'contains', value: '' }]
  dialogVisible.value = true
}
const addCondition = () => {
  if (form.conditions.length >= 20) return ElMessage.warning('匹配条件不能超过 20 个')
  form.conditions.push({ operator: 'contains', value: '' })
}
const removeCondition = (index: number) => {
  if (form.conditions.length <= 1) return
  form.conditions.splice(index, 1)
  errors.rows = {}
}
const validateForm = () => {
  const name = form.name.trim()
  errors.name = !name ? '请输入规则名称' : [...name].length > 30 ? '规则名称不能超过 30 个字符' : ''
  errors.dept = form.department_id ? '' : '请选择目标部门'
  errors.general = form.conditions.length ? '' : '至少添加一个匹配条件'
  errors.rows = {}
  form.conditions.forEach((condition, index) => {
    const value = condition.value.trim()
    if (isRegex(condition.operator)) {
      if (!value) {
        errors.rows[index] = '请填写一个正则表达式（不按逗号拆分）'
        return
      }
      try {
        new RegExp(value)
      } catch {
        errors.rows[index] = '正则表达式格式错误'
      }
      return
    }
    const values = value.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
    if (!values.length) {
      errors.rows[index] = '请填写匹配值，多个值用逗号隔开'
      return
    }
    if (isIpOperator(condition.operator)) {
      const invalid = values.map(validateIpValue).find(Boolean)
      if (invalid) errors.rows[index] = invalid
    }
  })
  return !errors.name && !errors.dept && !errors.general && Object.keys(errors.rows).length === 0
}
const save = async () => {
  if (!validateForm()) return
  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      department_id: Number(form.department_id || 0),
      enabled: form.enabled,
      conditions: form.conditions.map((item) => ({ operator: item.operator, value: item.value.trim() }))
    }
    if (form.id) await updateGroupRule(form.id, payload)
    else await createGroupRule(payload)
    ElMessage.success('规则已保存，下次设备同步时生效；需要马上生效请点「立即执行」')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

const toggleEnabled = async (row: GroupRule, enabled: boolean) => {
  try {
    await toggleGroupRule(row.id, enabled)
    row.enabled = enabled
    ElMessage.success(enabled ? '规则已启用' : '规则已停用')
    await load()
  } catch {
    await load()
  }
}
const move = async (row: GroupRule, action: 'top' | 'up' | 'down') => {
  await reorderGroupRule(row.id, action)
  await load()
}
const isFirst = (row: GroupRule) => row.sort_order <= 0
const isLast = (row: GroupRule) => stats.total_rules > 0 && row.sort_order >= stats.total_rules - 1

const remove = async (row: GroupRule) => {
  const hint = row.host_count > 0
    ? `已按此规则分好的 ${row.host_count} 台主机保持现有部门不变。`
    : '删除后该规则不再参与自动分组。'
  try {
    await ElMessageBox.confirm(hint, `删除规则「${row.name}」`, {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch {
    return
  }
  await deleteGroupRule(row.id)
  ElMessage.success('规则已删除')
  if (rules.value.length === 1 && page.value > 1) page.value -= 1
  await load()
}

const openPreview = async () => {
  previewVisible.value = true
  previewPage.value = 1
  await loadPreview()
}
const loadPreview = async () => {
  previewLoading.value = true
  try {
    const res = await previewGroupRules({ limit: previewPageSize, offset: (previewPage.value - 1) * previewPageSize })
    const data = unwrap(res)
    preview.total_unassigned = Number(data.total_unassigned || 0)
    preview.matched_total = Number(data.matched_total || 0)
    previewItems.value = data.items || []
  } finally {
    previewLoading.value = false
  }
}
const changePreviewPage = (value: number) => { previewPage.value = value; loadPreview() }
const confirmExecute = async () => {
  executing.value = true
  try {
    const res = await executeGroupRules()
    const data = unwrap(res)
    const assigned = Number(data.assigned || 0)
    ElMessage.success(assigned > 0 ? `已按规则归属 ${assigned} 台主机` : '没有可归属的未分配主机')
    previewVisible.value = false
    await load()
  } finally {
    executing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="aegis-page">
    <div class="stat-grid">
      <div class="stat-card"><strong>{{ stats.enabled_rules }} / {{ stats.total_rules }}</strong><span>启用规则数 / 规则总数</span></div>
      <div class="stat-card teal"><strong>{{ stats.rule_hosts }}</strong><span>按规则归属的主机</span></div>
      <div class="stat-card warning"><strong>{{ stats.manual_hosts }}</strong><span>手动分配（规则不改动）</span></div>
      <div class="stat-card danger"><strong>{{ stats.unassigned_hosts }}</strong><span>未分配（等待规则）</span></div>
    </div>

    <el-card shadow="never" class="panel">
      <div class="panel-heading">
        <h2>分组规则</h2>
        <div class="heading-actions">
          <el-button :disabled="stats.total_rules === 0" @click="openPreview">立即执行（未分配主机）</el-button>
          <el-button type="primary" @click="openCreate">＋ 新增规则</el-button>
        </div>
      </div>

      <div class="filter-row">
        <el-input v-model="query.keyword" clearable class="keyword" placeholder="🔍 规则名称" @keyup.enter="search" @clear="search" />
        <el-select v-model="query.department_id" clearable filterable placeholder="全部目标部门" @change="search">
          <el-option v-for="item in departments" :key="item.id" :label="item.name" :value="String(item.id)" />
        </el-select>
        <el-button type="primary" @click="search">搜索</el-button>
        <el-button @click="reset">重置</el-button>
      </div>

      <el-table v-loading="loading" :data="rules" stripe empty-text="暂无分组规则">
        <el-table-column label="顺序" width="64" align="center">
          <template #default="{ row }">{{ row.sort_order + 1 }}</template>
        </el-table-column>
        <el-table-column prop="name" label="规则名称" min-width="150" show-overflow-tooltip />
        <el-table-column label="匹配条件" min-width="330">
          <template #default="{ row }">
            <div v-if="row.conditions && row.conditions.length" class="condition-list">
              <template v-for="(condition, index) in row.conditions" :key="index">
                <span v-if="index" class="condition-joiner">且</span>
                <span class="condition-chip">{{ conditionText(condition) }}</span>
              </template>
            </div>
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="目标部门" width="130">
          <template #default="{ row }">
            <el-tag v-if="row.department_name" effect="dark" :color="departmentColor(row.department_id)" class="dept-tag">
              {{ row.department_name }}
            </el-tag>
            <span v-else class="muted">部门已删除</span>
          </template>
        </el-table-column>
        <el-table-column label="当前归属" width="96">
          <template #default="{ row }">{{ row.host_count }} 台</template>
        </el-table-column>
        <el-table-column label="启用" width="72">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" size="small" @change="(value: any) => toggleEnabled(row, !!value)" />
          </template>
        </el-table-column>
        <el-table-column label="调整顺序" width="150">
          <template #default="{ row }">
            <el-button link type="primary" size="small" :disabled="isFirst(row)" @click="move(row, 'top')">置顶</el-button>
            <el-button link type="primary" size="small" :disabled="isFirst(row)" title="上移" @click="move(row, 'up')">↑</el-button>
            <el-button link type="primary" size="small" :disabled="isLast(row)" title="下移" @click="move(row, 'down')">↓</el-button>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="112">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          background
          layout="total, sizes, prev, pager, next"
          :page-sizes="[10, 20, 50, 100]"
          :total="total"
          @current-change="load"
          @size-change="page = 1; load()"
        />
      </div>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="640px" destroy-on-close>
      <el-form label-width="92px">
        <el-form-item label="规则名称" required :error="errors.name">
          <el-input v-model="form.name" maxlength="30" show-word-limit placeholder="最多 30 字，例如：DC 业务" @input="errors.name = ''" />
        </el-form-item>
        <el-form-item label="目标部门" required :error="errors.dept">
          <el-select v-model="form.department_id" filterable placeholder="请选择部门告警群配置里的部门" style="width: 100%" @change="errors.dept = ''">
            <el-option v-for="item in departments" :key="item.id" :label="item.name" :value="item.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="匹配条件">
          <div class="condition-editor">
            <p class="condition-hint">多个条件之间需全部满足；一个条件里用逗号隔开多个值=任一满足（否定类为都不能出现）。正则只匹配主机名，主机名区分大小写；匹配内网 IP 或网段请选“IP 属于”并填写 IP/CIDR。</p>
            <div v-for="(condition, index) in form.conditions" :key="index" class="condition-row-wrap">
              <div class="condition-row">
                <el-select v-model="condition.operator" class="operator-select" @change="errors.rows[index] = ''">
                  <el-option-group label="主机名">
                    <el-option v-for="item in HOSTNAME_OPERATORS" :key="item.value" :label="item.label" :value="item.value" />
                  </el-option-group>
                  <el-option-group label="IP">
                    <el-option v-for="item in IP_OPERATORS" :key="item.value" :label="item.label" :value="item.value" />
                  </el-option-group>
                  <el-option v-if="condition.operator === 'equals'" label="完全等于" value="equals" />
                </el-select>
                <el-input
                  v-model="condition.value"
                  class="condition-value"
                  :class="{ 'is-error': errors.rows[index] }"
                  :placeholder="isRegex(condition.operator) ? '填写一个正则表达式（不按逗号拆分），如 (^|-)ai(-|$)' : isIpOperator(condition.operator) ? '多个 IP 或网段用逗号隔开，如 10.0.0.1, 10.0.0.0/24（IPv4 前缀 0–32，IPv6 前缀 0–128）' : '多个值用中英文逗号隔开，如 -dc-，-tbj-'"
                  @input="errors.rows[index] = ''"
                />
                <el-button link type="danger" :disabled="form.conditions.length <= 1" title="删除该条件" @click="removeCondition(index)">✕</el-button>
              </div>
              <p v-if="errors.rows[index]" class="row-error">{{ errors.rows[index] }}</p>
              <div v-if="isRegex(condition.operator)" class="regex-panel">
                <p class="regex-title">正则只检查主机名，不检查内网 IP。主机名匹配案例：</p>
                <p>① <code>(^|-)ai(-|$)</code>：<b>ai</b> 必须是独立的一段 → 命中 <code>aws-hk-ai-infer-019</code>，不会误中 <code>chainup-prod-node-001</code></p>
                <p>② <code>^(aws|hwc)-hk-</code>：以 <code>aws-hk-</code> 或 <code>hwc-hk-</code> 开头 → 命中 <code>hwc-hk-zf-prod-dc-api-006</code>，不命中 <code>aws-sg-tx-gateway-015</code></p>
                <p>③ <code>-db\d{3}$</code>：以 <code>-db</code> 加三位数字结尾 → 命中 <code>zy-nph-test-ops-jumpserver-db022</code>，不命中 <code>aws-hk-zf-prod-db-mysql-014</code></p>
                <p class="regex-symbols">常用符号：<code>^</code> 开头　<code>$</code> 结尾　<code>|</code> 或　<code>()</code> 括成一段　<code>\d</code> 一位数字　<code>{3}</code> 重复 3 次　<code>[1-5]</code> 任选一个　<code>.*</code> 任意字符；区分大小写。</p>
              </div>
            </div>
            <p v-if="errors.general" class="row-error">{{ errors.general }}</p>
            <el-button link type="primary" @click="addCondition">＋ 添加条件</el-button>
          </div>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="previewVisible" title="立即执行规则 · 预览" width="760px" destroy-on-close>
      <div class="preview-summary">
        按当前规则对未分配主机执行：共 <b>{{ preview.total_unassigned }}</b> 台未分配（已分配的手动 / 规则主机不受影响）。
      </div>
      <div class="preview-result">
        本次可自动归属 <b>{{ preview.matched_total }}</b> 台，其余 <b>{{ preview.total_unassigned - preview.matched_total }}</b> 台仍未命中、保持未分配。
      </div>
      <el-table v-if="preview.matched_total > 0" v-loading="previewLoading" :data="previewItems" stripe size="small">
        <el-table-column prop="instance_name" label="主机名" min-width="210" show-overflow-tooltip />
        <el-table-column prop="private_ip" label="内网 IP" min-width="130" show-overflow-tooltip />
        <el-table-column label="当前" width="110">
          <template #default><el-tag type="info" size="small" effect="plain">未分组</el-tag></template>
        </el-table-column>
        <el-table-column label="归属到" width="150">
          <template #default="{ row }">
            <el-tag size="small" effect="dark" :color="departmentColor(row.department_id)" class="dept-tag">{{ row.department_name }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="命中规则" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <div>规则：{{ row.rule_name }}</div>
            <div class="preview-condition">条件：{{ row.match_conditions.map(conditionText).join(' 且 ') }}</div>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-else v-loading="previewLoading" description="没有可归属的未分配主机" :image-size="80" />
      <div v-if="preview.matched_total > 0" class="pagination preview-pagination">
        <el-pagination
          v-model:current-page="previewPage"
          background
          small
          layout="total, prev, pager, next"
          :page-size="previewPageSize"
          :total="preview.matched_total"
          @current-change="changePreviewPage"
        />
      </div>
      <template #footer>
        <el-button @click="previewVisible = false">取消</el-button>
        <el-button v-if="preview.matched_total > 0" type="primary" :loading="executing" @click="confirmExecute">确认执行</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.aegis-page { padding: 22px; min-height: 100%; color: #3b4451; }
.stat-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; margin-bottom: 16px; }
.stat-card { background: #fff; border: 1px solid #e4e7ed; border-radius: 10px; padding: 14px 16px; }
.stat-card span { display: block; color: #8a94a6; font-size: 13px; }
.stat-card strong { display: block; margin-bottom: 7px; font-size: 28px; color: #3b4451; }
.stat-card.teal strong { color: #1a9c92; }
.stat-card.warning strong { color: #f0a020; }
.stat-card.danger strong { color: #e6412e; }
.panel { border: 1px solid #e4e7ed; border-radius: 10px; }
.panel-heading { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
.panel-heading h2 { margin: 0; font-size: 17px; color: #3b4451; }
.heading-actions { display: flex; align-items: center; gap: 10px; }
.filter-row { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-bottom: 14px; }
.keyword { width: 230px; }
.filter-row .el-select { width: 190px; }
.muted { color: #8a94a6; font-size: 13px; }
.condition-list { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; line-height: 1.9; }
.condition-chip { display: inline-block; padding: 1px 8px; border-radius: 4px; background: #f2f4f7; color: #3b4451; font-size: 12px; }
.condition-joiner { color: #8a94a6; font-size: 12px; }
.dept-tag { border: 0; color: #fff; }
.condition-editor { width: 100%; }
.condition-hint { margin: 0 0 10px; color: #8a94a6; font-size: 12px; line-height: 1.6; }
.condition-row-wrap { margin-bottom: 8px; }
.condition-row { display: flex; align-items: center; gap: 8px; }
.operator-select { width: 130px; flex: 0 0 130px; }
.condition-value { flex: 1; }
.condition-value.is-error :deep(.el-input__wrapper) { box-shadow: 0 0 0 1px #e6412e inset; }
.row-error { margin: 4px 0 0; color: #e6412e; font-size: 12px; }
.regex-panel { margin-top: 8px; padding: 10px 12px; border: 1px dashed #d9dee7; border-radius: 6px; background: #fcfdfe; color: #697586; font-size: 12px; line-height: 1.8; }
.regex-panel p { margin: 0; }
.regex-title { color: #3b4451; font-weight: 600; }
.regex-panel code { padding: 0 4px; border-radius: 3px; background: #eef1f6; color: #2b5a54; }
.regex-symbols { margin-top: 4px !important; }
.pagination { display: flex; justify-content: flex-end; margin-top: 14px; }
.preview-summary { color: #3b4451; margin-bottom: 8px; }
.preview-result { margin-bottom: 12px; padding: 8px 12px; border-radius: 6px; background: #eaf6f4; color: #1a7f74; }
.preview-condition { overflow: hidden; color: #8a94a6; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.preview-pagination { margin-top: 10px; }
</style>
