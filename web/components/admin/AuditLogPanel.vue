<template>
  <section class="admin-setting-block space-y-4">
    <div class="admin-setting-heading">
      <div>
        <h2 class="admin-setting-title">管理员审计</h2>
        <p class="admin-setting-desc">只读的管理员操作记录，按最新记录优先显示。</p>
      </div>
    </div>

    <div v-if="isPrimaryAdmin" class="flex flex-wrap items-center gap-3 rounded-md border p-3 dark:border-gray-700">
      <UCheckbox :model-value="auditEnabled" :disabled="configLoading || configSaving" label="启用管理员审计写入" @update:model-value="saveAuditEnabled($event === true)" />
      <span v-if="configMessage" class="text-sm" :class="configError ? 'text-red-600' : 'text-green-600'">{{ configMessage }}</span>
    </div>

    <div class="grid gap-2 md:grid-cols-2 xl:grid-cols-4">
      <UInput v-model="filters.q" placeholder="搜索安全摘要或目标 ID" @keyup.enter="applyFilters" />
      <UInput v-model="filters.actorUserID" inputmode="numeric" placeholder="操作人 ID" @keyup.enter="applyFilters" />
      <UInput v-model="filters.module" placeholder="模块" @keyup.enter="applyFilters" />
      <UInput v-model="filters.action" placeholder="动作" @keyup.enter="applyFilters" />
      <USelect v-model="filters.result" :options="resultOptions" placeholder="全部结果" />
      <UInput v-model="filters.targetType" placeholder="目标类型" @keyup.enter="applyFilters" />
      <UInput v-model="filters.targetID" placeholder="目标 ID" @keyup.enter="applyFilters" />
      <div class="flex flex-wrap gap-2">
        <UButton :loading="loading" @click="applyFilters">筛选</UButton>
        <UButton :loading="exporting" :disabled="loading || exporting" variant="soft" @click="exportCurrent">导出当前筛选结果</UButton>
        <UButton variant="soft" :disabled="loading || exporting" @click="resetFilters">重置</UButton>
      </div>
      <UInput v-model="filters.start" type="datetime-local" aria-label="开始时间" />
      <UInput v-model="filters.end" type="datetime-local" aria-label="结束时间" />
    </div>

    <p v-if="loadError" class="text-sm text-red-600">{{ loadError }}</p>
    <p v-if="exportMessage" class="text-sm" :class="exportError ? 'text-red-600' : 'text-green-600'">{{ exportMessage }}</p>

    <div class="overflow-x-auto rounded-md border dark:border-gray-700">
      <table class="min-w-full text-sm">
        <thead>
          <tr class="text-left">
            <th class="p-3">时间</th>
            <th class="p-3">操作人</th>
            <th class="p-3">操作说明</th>
            <th class="p-3">结果</th>
            <th class="p-3">模块/动作</th>
            <th class="p-3">摘要</th>
            <th class="p-3">详情</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in items" :key="item.id" class="border-t dark:border-gray-700">
            <td class="p-3 whitespace-nowrap">{{ formatTime(item.created_at) }}</td>
            <td class="p-3">{{ item.actor_username }}</td>
            <td class="p-3 min-w-64 font-medium">{{ item.operation_description || '管理员操作' }}</td>
            <td class="p-3 whitespace-nowrap">
              <UBadge :color="resultColor(item.result)">{{ item.result_description || item.result }}</UBadge>
              <div class="mt-1 text-xs opacity-60">{{ item.result }}</div>
            </td>
            <td class="p-3 whitespace-nowrap">{{ item.module }} / {{ item.action }}</td>
            <td class="p-3 min-w-56">{{ item.summary || '-' }}</td>
            <td class="p-3"><UButton size="xs" variant="soft" @click="loadDetail(item.id)">查看</UButton></td>
          </tr>
          <tr v-if="!items.length && !loading">
            <td colspan="7" class="p-5 text-center opacity-70">暂无审计记录。</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between">
      <span class="text-sm opacity-70">共 {{ total }} 条</span>
      <div class="flex gap-2">
        <UButton size="xs" :disabled="page <= 1 || loading || exporting" @click="page--; load()">上一页</UButton>
        <span class="text-sm leading-7">第 {{ page }} 页</span>
        <UButton size="xs" :disabled="items.length < pageSize || loading || exporting" @click="page++; load()">下一页</UButton>
      </div>
    </div>

    <UModal v-model="detailOpen" :ui="{ width: 'sm:max-w-2xl' }">
      <UCard v-if="detail" :ui="{ ring: '', divide: 'divide-y divide-gray-100 dark:divide-gray-800' }">
        <template #header>
          <div class="flex items-center justify-between">
            <span class="font-semibold">审计记录 #{{ detail.id }}</span>
            <UButton color="gray" variant="ghost" icon="i-heroicons-x-mark" @click="detailOpen = false" />
          </div>
        </template>
        <dl class="grid gap-3 text-sm sm:grid-cols-2">
          <div><dt class="opacity-60">时间</dt><dd>{{ formatTime(detail.created_at) }}</dd></div>
          <div><dt class="opacity-60">认证来源</dt><dd>{{ detail.auth_via || '-' }}</dd></div>
          <div><dt class="opacity-60">操作人</dt><dd>{{ detail.actor_username }} #{{ detail.actor_user_id }}</dd></div>
          <div><dt class="opacity-60">操作说明</dt><dd>{{ detail.operation_description || '管理员操作' }}</dd></div>
          <div><dt class="opacity-60">模块/动作</dt><dd>{{ detail.module }} / {{ detail.action }}</dd></div>
          <div><dt class="opacity-60">能力</dt><dd>{{ detail.capability || '-' }}</dd></div>
          <div><dt class="opacity-60">目标</dt><dd>{{ detail.target_type || '-' }} / {{ detail.target_id || '-' }}</dd></div>
          <div><dt class="opacity-60">结果</dt><dd>{{ detail.result_description || detail.result }}（{{ detail.result }}）</dd></div>
          <div class="sm:col-span-2"><dt class="opacity-60">摘要</dt><dd>{{ detail.summary || '-' }}</dd></div>
          <div v-if="detail.reason || detail.reason_description" class="sm:col-span-2"><dt class="opacity-60">原因</dt><dd>{{ detail.reason_description || detail.reason }}<span v-if="detail.reason_description && detail.reason">（{{ detail.reason }}）</span></dd></div>
          <div v-if="detail.changes_json" class="sm:col-span-2"><dt class="opacity-60">安全变更摘要</dt><dd class="whitespace-pre-wrap break-all">{{ detail.changes_json }}</dd></div>
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

const props = defineProps<{ isPrimaryAdmin?: boolean }>()
const items = ref<Audit[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 30
const loading = ref(false)
const exporting = ref(false)
const detail = ref<Audit | null>(null)
const detailOpen = ref(false)
const auditEnabled = ref(true)
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
    configMessage.value = auditEnabled.value ? '管理员审计写入已启用' : '管理员审计写入已关闭'
  } catch (error) {
    configError.value = true
    configMessage.value = errorMessage(error, '保存审计设置失败')
  } finally {
    configSaving.value = false
  }
}
const formatTime = (value: string) => value ? new Date(value).toLocaleString() : '-'
const resultColor = (result: AuditResult) => result === 'success' ? 'green' : result === 'denied' ? 'yellow' : 'red'
onMounted(() => { load(); loadAuditConfig() })
</script>
