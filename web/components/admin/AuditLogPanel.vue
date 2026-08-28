<template>
  <section class="audit-panel" :class="theme?.text || ''">
    <AdminModuleHeader
      title="管理员审计"
      description="查看管理员操作记录、安全结果与变更摘要，记录按时间从新到旧排列。"
      icon="i-heroicons-clipboard-document-check"
      :badge="loading ? '读取中' : `${total} 条记录`"
      accent="slate"
      :theme="theme"
    >
      <template #actions>
        <UButton size="sm" color="gray" variant="soft" icon="i-heroicons-arrow-path" :loading="loading" @click="load">刷新</UButton>
      </template>
    </AdminModuleHeader>

    <div class="audit-body">
      <div v-if="isPrimaryAdmin" class="audit-policy-card" :class="[theme?.border || 'border-slate-200 dark:border-slate-700', theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60']">
        <div class="audit-policy-copy">
          <span class="audit-policy-icon"><UIcon name="i-heroicons-shield-check" class="h-4 w-4" /></span>
          <div>
            <div class="text-sm font-medium">审计写入策略</div>
            <p class="mt-0.5 text-xs leading-5" :class="theme?.mutedText || 'text-slate-500'">关闭后不再写入新的管理员操作记录，已有记录仍可查询。</p>
          </div>
        </div>
        <div class="audit-policy-control">
          <span class="text-xs font-medium" :class="auditEnabled ? 'text-green-600 dark:text-green-400' : (theme?.mutedText || 'text-slate-500')">{{ auditEnabled ? '已启用' : '已关闭' }}</span>
          <UToggle :model-value="auditEnabled" :disabled="configLoading || configSaving" aria-label="启用管理员审计写入" @update:model-value="saveAuditEnabled($event === true)" />
          <USelect v-model="auditRetentionDays" :options="auditRetentionOptions" class="w-36" :disabled="configLoading || configSaving" aria-label="管理员审计保留期限" />
          <UButton size="xs" color="primary" variant="soft" :loading="configSaving" @click="saveAuditRetention">保存期限</UButton>
        </div>
      </div>

      <section class="audit-filter-card" :class="theme?.border || 'border-slate-200 dark:border-slate-700'" aria-labelledby="audit-filter-title">
        <div class="audit-section-heading">
          <div>
            <h3 id="audit-filter-title" class="text-sm font-semibold">筛选审计记录</h3>
            <p class="mt-1 text-xs" :class="theme?.mutedText || 'text-slate-500'">可组合操作人、模块、结果、目标和时间范围缩小记录范围。</p>
          </div>
          <div class="audit-filter-actions">
            <UButton size="xs" color="primary" variant="soft" icon="i-heroicons-funnel" :loading="loading" @click="applyFilters">应用筛选</UButton>
            <UButton size="xs" color="gray" variant="ghost" :disabled="loading || exporting" @click="resetFilters">清空条件</UButton>
          </div>
        </div>

        <div class="audit-filter-grid">
          <label class="audit-filter-field audit-filter-search"><span>摘要或目标 ID</span><UInput v-model="filters.q" placeholder="搜索安全摘要或目标 ID" @keyup.enter="applyFilters" /></label>
          <label class="audit-filter-field"><span>操作人 ID</span><UInput v-model="filters.actorUserID" inputmode="numeric" placeholder="精确匹配" @keyup.enter="applyFilters" /></label>
          <label class="audit-filter-field"><span>模块</span><UInput v-model="filters.module" placeholder="例如 notes" @keyup.enter="applyFilters" /></label>
          <label class="audit-filter-field"><span>动作</span><UInput v-model="filters.action" placeholder="例如 update" @keyup.enter="applyFilters" /></label>
          <label class="audit-filter-field"><span>结果</span><USelect v-model="filters.result" :options="resultOptions" /></label>
          <label class="audit-filter-field"><span>目标类型</span><UInput v-model="filters.targetType" placeholder="例如 note" @keyup.enter="applyFilters" /></label>
          <label class="audit-filter-field"><span>目标 ID</span><UInput v-model="filters.targetID" placeholder="精确匹配" @keyup.enter="applyFilters" /></label>
          <label class="audit-filter-field"><span>开始时间</span><UInput v-model="filters.start" type="datetime-local" /></label>
          <label class="audit-filter-field"><span>结束时间</span><UInput v-model="filters.end" type="datetime-local" /></label>
        </div>

        <div class="audit-export-row" :class="theme?.mutedText || 'text-slate-500'">
          <span>导出会使用当前筛选条件，不受当前页限制。</span>
          <UButton size="xs" color="gray" variant="soft" icon="i-heroicons-arrow-down-tray" :loading="exporting" :disabled="loading || exporting" @click="exportCurrent">导出当前筛选结果</UButton>
        </div>
      </section>

      <div v-if="configMessage || loadError || exportMessage" class="audit-feedback" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
        <p v-if="configMessage" class="text-sm" :class="configError ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'">{{ configMessage }}</p>
        <p v-if="loadError" class="text-sm text-red-600 dark:text-red-400">{{ loadError }}</p>
        <p v-if="exportMessage" class="text-sm" :class="exportError ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'">{{ exportMessage }}</p>
      </div>

      <div class="audit-table-shell" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
        <table class="min-w-[1120px] w-full table-fixed text-sm">
          <colgroup>
            <col class="w-44" />
            <col class="w-28" />
            <col class="w-64" />
            <col class="w-28" />
            <col class="w-44" />
            <col />
            <col class="w-20" />
          </colgroup>
          <thead :class="theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60'">
            <tr class="text-left">
              <th class="px-4 py-3">时间</th>
              <th class="px-3 py-3">操作人</th>
              <th class="px-3 py-3">操作说明</th>
              <th class="px-3 py-3">结果</th>
              <th class="px-3 py-3">模块 / 动作</th>
              <th class="px-3 py-3">摘要</th>
              <th class="px-4 py-3 text-right">详情</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading"><td colspan="7" class="px-3 py-12 text-center" :class="theme?.mutedText || 'text-slate-500'"><UIcon name="i-heroicons-arrow-path" class="mr-2 inline h-4 w-4 animate-spin" />正在读取审计记录…</td></tr>
            <tr v-else-if="!items.length"><td colspan="7" class="px-3 py-12 text-center" :class="theme?.mutedText || 'text-slate-500'"><UIcon name="i-heroicons-clipboard-document-list" class="mx-auto mb-2 h-6 w-6 opacity-60" /><span>当前筛选下暂无审计记录</span></td></tr>
            <tr v-for="item in items" v-else :key="item.id" class="audit-record-row" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
              <td class="px-4 py-3 align-top whitespace-nowrap" :class="theme?.mutedText || 'text-slate-500'">{{ formatTime(item.created_at) }}</td>
              <td class="px-3 py-3 align-top"><span class="font-medium">{{ item.actor_username }}</span><span v-if="item.actor_type !== 'system'" class="mt-0.5 block text-xs" :class="theme?.mutedText || 'text-slate-500'">ID {{ item.actor_user_id }}</span><span v-else class="mt-0.5 block text-xs" :class="theme?.mutedText || 'text-slate-500'">系统任务</span></td>
              <td class="px-3 py-3 align-top font-medium break-words">{{ item.operation_description || '管理员操作' }}</td>
              <td class="px-3 py-3 align-top"><UBadge :color="resultColor(item.result)" size="xs" variant="soft">{{ item.result_description || item.result }}</UBadge><div class="mt-1 text-[11px]" :class="theme?.mutedText || 'text-slate-500'">{{ item.result }}</div></td>
              <td class="px-3 py-3 align-top"><span class="break-all font-medium">{{ item.module }}</span><span class="mt-0.5 block break-all text-xs" :class="theme?.mutedText || 'text-slate-500'">{{ item.action }}</span></td>
              <td class="audit-summary px-3 py-3 align-top">{{ item.summary || '-' }}</td>
              <td class="px-4 py-3 text-right align-top"><UButton size="xs" color="gray" variant="soft" @click="loadDetail(item.id)">查看</UButton></td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="audit-pagination" :class="theme?.mutedText || 'text-slate-500'">
        <span>共 {{ total }} 条 · 每页 {{ pageSize }} 条</span>
        <div class="flex items-center gap-2">
          <UButton size="xs" color="gray" variant="soft" :disabled="page <= 1 || loading || exporting" @click="page--; load()">上一页</UButton>
          <span class="min-w-16 text-center">第 {{ page }} 页</span>
          <UButton size="xs" color="gray" variant="soft" :disabled="items.length < pageSize || loading || exporting" @click="page++; load()">下一页</UButton>
        </div>
      </div>
    </div>

    <UModal v-model="detailOpen" :ui="{ width: 'sm:max-w-2xl' }">
      <UCard v-if="detail" :class="theme?.cardBg" :ui="{ ring: '', divide: 'divide-y divide-gray-100 dark:divide-gray-800' }">
        <template #header>
          <div class="flex items-center justify-between gap-3">
            <div><div class="font-semibold">审计记录 #{{ detail.id }}</div><div class="mt-1 text-xs" :class="theme?.mutedText || 'text-slate-500'">{{ detail.operation_description || '管理员操作' }}</div></div>
            <UButton color="gray" variant="ghost" icon="i-heroicons-x-mark" aria-label="关闭审计详情" @click="detailOpen = false" />
          </div>
        </template>
        <dl class="audit-detail-grid">
          <div><dt>时间</dt><dd>{{ formatTime(detail.created_at) }}</dd></div>
          <div><dt>认证来源</dt><dd>{{ detail.auth_via || '-' }}</dd></div>
          <div><dt>操作人</dt><dd>{{ detail.actor_type === 'system' ? detail.actor_username : `${detail.actor_username} #${detail.actor_user_id}` }}</dd></div>
          <div><dt>结果</dt><dd>{{ detail.result_description || detail.result }}（{{ detail.result }}）</dd></div>
          <div><dt>模块 / 动作</dt><dd>{{ detail.module }} / {{ detail.action }}</dd></div>
          <div><dt>能力</dt><dd class="break-all">{{ detail.capability || '-' }}</dd></div>
          <div class="sm:col-span-2"><dt>目标</dt><dd class="break-all">{{ detail.target_type || '-' }} / {{ detail.target_id || '-' }}</dd></div>
          <div class="sm:col-span-2"><dt>摘要</dt><dd>{{ detail.summary || '-' }}</dd></div>
          <div v-if="detail.reason || detail.reason_description" class="sm:col-span-2"><dt>原因</dt><dd>{{ detail.reason_description || detail.reason }}<span v-if="detail.reason_description && detail.reason">（{{ detail.reason }}）</span></dd></div>
          <div v-if="detail.changes_json" class="sm:col-span-2"><dt>安全变更摘要</dt><dd class="whitespace-pre-wrap break-all">{{ detail.changes_json }}</dd></div>
        </dl>
      </UCard>
    </UModal>
  </section>
</template>

<script setup lang="ts">
type AuditResult = 'success' | 'denied' | 'failure'
type Audit = {
  id: number
  created_at: string
  actor_user_id: number
  actor_username: string
  actor_type?: 'user' | 'system'
  module: string
  action: string
  capability: string
  target_type: string
  target_id: string
  result: AuditResult
  result_description?: string
  operation_description?: string
  reason_description?: string
  summary: string
  reason: string
  changes_json: string
  auth_via: string
}

const props = defineProps<{ isPrimaryAdmin?: boolean; theme?: Record<string, string> }>()
const items = ref<Audit[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 30
const loading = ref(false)
const exporting = ref(false)
const detail = ref<Audit | null>(null)
const detailOpen = ref(false)
const auditEnabled = ref(true)
const auditRetentionDays = ref(730)
const auditRetentionOptions = [
  { label: '永不按时间清理', value: 0 },
  { label: '保留 180 天', value: 180 },
  { label: '保留 365 天', value: 365 },
  { label: '保留 730 天', value: 730 },
  { label: '保留 1095 天', value: 1095 }
]
const configLoading = ref(false)
const configSaving = ref(false)
const configMessage = ref('')
const configError = ref(false)
const loadError = ref('')
const exportMessage = ref('')
const exportError = ref(false)
const filters = reactive({ q: '', result: '', actorUserID: '', module: '', action: '', targetType: '', targetID: '', start: '', end: '' })
const resultOptions = [{ label: '全部结果', value: '' }, { label: '成功', value: 'success' }, { label: '拒绝', value: 'denied' }, { label: '失败', value: 'failure' }]

const requestData = async (response: Response) => {
  const body = await response.json().catch(() => ({}))
  if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '请求失败')
  return body.data
}
const errorMessage = (error: unknown, fallback: string) => error instanceof Error && error.message ? error.message : fallback
const toRFC3339 = (value: string) => value ? new Date(value).toISOString() : ''
const appendFilter = (query: URLSearchParams, key: string, value: string) => {
  const trimmed = value.trim()
  if (trimmed) query.set(key, trimmed)
}
const buildAuditQuery = (includePagination = true) => {
  const query = new URLSearchParams()
  if (includePagination) {
    query.set('page', String(page.value))
    query.set('page_size', String(pageSize))
  }
  appendFilter(query, 'q', filters.q)
  appendFilter(query, 'result', filters.result)
  appendFilter(query, 'actor_user_id', filters.actorUserID)
  appendFilter(query, 'module', filters.module)
  appendFilter(query, 'action', filters.action)
  appendFilter(query, 'target_type', filters.targetType)
  appendFilter(query, 'target_id', filters.targetID)
  const start = toRFC3339(filters.start)
  const end = toRFC3339(filters.end)
  if (start) query.set('start', start)
  if (end) query.set('end', end)
  return query
}
const load = async () => {
  loading.value = true
  loadError.value = ''
  try {
    const data = await fetch(`/api/admin/audit-logs?${buildAuditQuery(true)}`, { credentials: 'include' }).then(requestData)
    items.value = data?.items || []
    total.value = data?.total || 0
  } catch (error) {
    loadError.value = errorMessage(error, '加载审计失败')
  } finally {
    loading.value = false
  }
}
const applyFilters = () => { page.value = 1; load() }
const resetFilters = () => {
  Object.assign(filters, { q: '', result: '', actorUserID: '', module: '', action: '', targetType: '', targetID: '', start: '', end: '' })
  applyFilters()
}
const filenameFromResponse = (response: Response) => {
  const disposition = response.headers.get('Content-Disposition') || ''
  const encoded = disposition.match(/filename\*=UTF-8''([^;]+)/i)?.[1]
  if (encoded) return decodeURIComponent(encoded)
  return disposition.match(/filename="([^"]+)"/i)?.[1] || `admin-audit-${Date.now()}.csv`
}
const exportCurrent = async () => {
  exporting.value = true
  exportMessage.value = ''
  exportError.value = false
  try {
    const response = await fetch(`/api/admin/audit-logs/export?${buildAuditQuery(false)}`, { credentials: 'include' })
    if (!response.ok) {
      const body = await response.json().catch(() => ({}))
      throw new Error(body?.msg || '导出审计失败')
    }
    const link = document.createElement('a')
    const url = URL.createObjectURL(await response.blob())
    link.href = url
    link.download = filenameFromResponse(response)
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
    exportMessage.value = '已开始下载当前筛选结果'
  } catch (error) {
    exportError.value = true
    exportMessage.value = errorMessage(error, '导出审计失败')
  } finally {
    exporting.value = false
  }
}
const loadDetail = async (id: number) => {
  loadError.value = ''
  try {
    detail.value = await fetch(`/api/admin/audit-logs/${id}`, { credentials: 'include' }).then(requestData)
    detailOpen.value = true
  } catch (error) {
    loadError.value = errorMessage(error, '读取审计详情失败')
  }
}
const loadAuditConfig = async () => {
  if (!props.isPrimaryAdmin) return
  configLoading.value = true
  try {
    const data = await fetch('/api/admin/audit-config', { credentials: 'include' }).then(requestData)
    auditEnabled.value = !!data?.enabled
    auditRetentionDays.value = Number(data?.retentionDays ?? 730)
  } catch (error) {
    configError.value = true
    configMessage.value = errorMessage(error, '读取审计设置失败')
  } finally {
    configLoading.value = false
  }
}
const saveAuditEnabled = async (enabled: boolean) => {
  configSaving.value = true
  configMessage.value = ''
  configError.value = false
  try {
    const data = await fetch('/api/admin/audit-config', { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ enabled }) }).then(requestData)
    auditEnabled.value = !!data?.enabled
    auditRetentionDays.value = Number(data?.retentionDays ?? auditRetentionDays.value)
    configMessage.value = auditEnabled.value ? '管理员审计写入已启用' : '管理员审计写入已关闭'
  } catch (error) {
    configError.value = true
    configMessage.value = errorMessage(error, '保存审计设置失败')
  } finally {
    configSaving.value = false
  }
}
const saveAuditRetention = async () => {
  configSaving.value = true
  configMessage.value = ''
  configError.value = false
  try {
    const data = await fetch('/api/admin/audit-config', { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ retentionDays: Number(auditRetentionDays.value) }) }).then(requestData)
    auditEnabled.value = !!data?.enabled
    auditRetentionDays.value = Number(data?.retentionDays ?? 730)
    configMessage.value = auditRetentionDays.value === 0 ? '管理员审计不会按时间自动清理，仍受最大行数安全阀保护' : `管理员审计保留 ${auditRetentionDays.value} 天`
  } catch (error) {
    configError.value = true
    configMessage.value = errorMessage(error, '保存审计保留期限失败')
  } finally {
    configSaving.value = false
  }
}
const formatTime = (value: string) => value ? new Date(value).toLocaleString() : '-'
const resultColor = (result: AuditResult) => result === 'success' ? 'green' : result === 'denied' ? 'yellow' : 'red'
onMounted(() => { load(); loadAuditConfig() })
</script>

<style scoped>
.audit-panel {
  min-width: 0;
  overflow: hidden;
}

.audit-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 0 16px 16px;
}

.audit-policy-card,
.audit-filter-card,
.audit-feedback {
  border-width: 1px;
  border-style: solid;
  border-radius: 10px;
}

.audit-policy-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-top: 16px;
  padding: 12px;
}

.audit-policy-copy,
.audit-policy-control {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.audit-policy-icon {
  display: inline-flex;
  width: 30px;
  height: 30px;
  flex: 0 0 30px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: rgb(71, 85, 105);
  background: rgba(100, 116, 139, 0.13);
}

:global(.dark) .audit-policy-icon {
  color: rgb(203, 213, 225);
}

.audit-filter-card {
  padding: 14px;
}

.audit-section-heading,
.audit-export-row,
.audit-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.audit-section-heading {
  align-items: flex-start;
  margin-bottom: 12px;
}

.audit-filter-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 6px;
}

.audit-filter-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.audit-filter-field {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 5px;
}

.audit-filter-field > span {
  font-size: 11px;
  font-weight: 600;
  line-height: 1.3;
  opacity: 0.72;
}

.audit-filter-search {
  grid-column: span 2;
}

.audit-export-row {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid rgba(148, 163, 184, 0.16);
  font-size: 12px;
}

.audit-feedback {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
}

.audit-table-shell {
  overflow-x: auto;
  border-width: 1px;
  border-style: solid;
  border-radius: 10px;
}

.audit-record-row {
  border-top-width: 1px;
  border-top-style: solid;
  transition: background-color 0.15s ease;
}

.audit-record-row:hover {
  background: rgba(148, 163, 184, 0.06);
}

.audit-summary {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.audit-pagination {
  font-size: 12px;
}

.audit-detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  font-size: 13px;
}

.audit-detail-grid > div {
  min-width: 0;
  padding: 10px;
  border-radius: 8px;
  background: rgba(148, 163, 184, 0.08);
}

.audit-detail-grid dt {
  margin-bottom: 4px;
  font-size: 11px;
  font-weight: 600;
  opacity: 0.62;
}

.audit-detail-grid dd {
  line-height: 1.55;
}

@media (max-width: 1100px) {
  .audit-filter-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 760px) {
  .audit-policy-card,
  .audit-section-heading,
  .audit-export-row,
  .audit-pagination {
    align-items: stretch;
    flex-direction: column;
  }

  .audit-filter-actions {
    justify-content: flex-start;
  }

  .audit-filter-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .audit-filter-search {
    grid-column: span 2;
  }

  .audit-policy-control {
    justify-content: space-between;
  }
}

@media (max-width: 520px) {
  .audit-body {
    padding-right: 14px;
    padding-left: 14px;
  }

  .audit-filter-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .audit-filter-search {
    grid-column: auto;
  }

  .audit-detail-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .audit-detail-grid .sm\:col-span-2 {
    grid-column: auto;
  }
}
</style>
