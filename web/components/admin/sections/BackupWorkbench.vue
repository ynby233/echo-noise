<template>
  <section v-if="allowed('view')" :class="adminShellCardClass">
    <AdminModuleHeader :title="label" icon="i-heroicons-archive-box" :description="backupScopes[kind]" :theme="theme" />
    <div class="px-4 pb-4 space-y-4">
      <ConfigLoadState :loading="loading" :error="error" @retry="load" />
      <p class="text-sm break-all">服务器备份目录：{{ directory || '读取后显示' }}</p>
      <p class="text-sm" :class="theme.mutedText">备份包可能包含私密内容，仅站长可下载或恢复。</p>
      <p v-if="pendingRestart" role="status" class="text-amber-600">恢复已暂存，请由站长重启服务后生效。当前运行中的数据尚未切换。</p>
      <p v-for="warning in warnings" :key="warning" role="status" class="text-amber-600">{{ warning }}</p>
      <p v-if="notice" role="status" class="text-sm whitespace-pre-line">{{ notice }}</p>
      <p v-if="busy" role="status" aria-live="polite" class="text-sm">操作进行中，请等待服务器处理完成。大附件可能耗时较长；若请求中断，请刷新记录确认结果。</p>
      <fieldset v-if="ready" :disabled="busy || loading" class="space-y-4 min-w-0">
        <section v-if="allowed('create')" class="admin-form-section">
          <div class="admin-section-heading"><h3>手动生成备份</h3></div>
          <p class="text-sm" :class="theme.mutedText">备份保存在服务器目录或站长配置的云通道；生成后可在记录中查看结果。云端操作由服务端检查主通道是否可用。</p>
          <label v-if="kind === 'attachments'" class="admin-labeled-field mt-2"><span>备份范围</span><select v-model="scope" class="admin-field"><option value="local">仅本地附件</option><option value="all">本地和云端附件</option></select></label>
          <p v-if="kind === 'attachments'" class="text-sm" :class="theme.mutedText">仅本地备份不包含云端文件；选择本地和云端会下载本项目管理的云附件，外部图片或视频链接不在范围内。</p>
          <div class="admin-row-actions mt-3"><label>保存位置 <select v-model="target" class="admin-field"><option value="local">本地</option><option value="cloud">云端</option><option value="both">本地和云端</option></select></label><UButton class="admin-action" :loading="busy" @click="createBackup">生成备份</UButton></div>
        </section>
        <section v-if="allowed('plan') && plan" class="admin-form-section">
          <div class="admin-section-heading"><h3>自动备份计划</h3></div>
          <label class="admin-toggle-row"><input v-model="plan.enabled" type="checkbox" />启用此类备份计划</label>
          <div class="admin-fields-grid">
            <label class="admin-labeled-field min-w-0"><span>执行时间</span><input v-model="plan.time" type="time" class="admin-field w-full min-w-0" /></label>
            <label class="admin-labeled-field min-w-0"><span>时区</span><input v-model="plan.timezone" placeholder="Asia/Shanghai" class="admin-field w-full min-w-0" /></label>
            <label class="admin-labeled-field min-w-0"><span>本地保留数量</span><input v-model.number="plan.keepLocal" type="number" min="1" max="1000" class="admin-field w-full min-w-0" /></label>
            <label class="admin-labeled-field min-w-0"><span>云端保留数量</span><input v-model.number="plan.keepCloud" type="number" min="1" max="1000" class="admin-field w-full min-w-0" /></label>
            <label class="admin-labeled-field min-w-0"><span>保存位置</span><select v-model="plan.target" class="admin-field w-full min-w-0"><option value="local">本地</option><option value="cloud">云端</option><option value="both">本地和云端</option></select></label>
            <label v-if="kind === 'attachments'" class="admin-labeled-field min-w-0"><span>备份范围</span><select v-model="plan.scope" class="admin-field w-full min-w-0"><option value="local">仅本地附件</option><option value="all">本地和云端附件</option></select></label>
          </div>
          <p class="text-sm mt-2">上次执行：{{ formatTime(plan.lastRun) }} · 下次执行：{{ formatTime(plan.nextRun) }}</p>
          <p class="text-sm" :class="theme.mutedText">同类备份在每个保存位置分别保留最近指定份数，手动与自动共同计数；新备份成功后才清理旧文件。</p>
          <div class="admin-form-actions"><UButton class="admin-action" :loading="busy" @click="savePlan">保存计划</UButton></div>
        </section>
        <section v-if="allowed('history')" class="admin-form-section">
          <div class="admin-section-heading admin-inline-toolbar"><h3>备份记录</h3><UButton class="admin-action" size="sm" color="gray" @click="load">刷新</UButton></div>
          <p v-if="!records.length" class="text-sm" :class="theme.mutedText">暂无备份记录。手动与自动备份会合并显示。</p>
          <div v-for="record in records" :key="record.id" class="admin-subcard p-3 space-y-2 break-words" :class="theme.subtleBg">
            <p>{{ formatTime(record.createdAt) }} · {{ record.trigger === 'scheduled' ? '自动' : record.trigger === 'discovered' ? (record.local ? '本地已有备份' : '云端已有备份') : '手动' }} · {{ backupRecordStatus(record.status) }} · {{ formatSize(record.size) }}</p>
            <p v-if="record.error" role="alert" class="text-red-600">{{ record.error }}</p>
            <div v-if="record.local || record.cloud" class="admin-row-actions">
              <template v-for="source in recordSources(record)" :key="source">
                <UButton v-if="allowed('download')" class="admin-action" size="sm" color="gray" @click="downloadRecord(record, source)">下载{{ source === 'local' ? '本地' : '云端' }}包</UButton>
                <UButton v-if="allowed('restore')" class="admin-action" size="sm" color="orange" @click="restoreRecord(record, source)">恢复{{ source === 'local' ? '本地' : '云端' }}包</UButton>
              </template>
            </div>
          </div>
        </section>
        <section v-if="allowed('restore')" class="admin-form-section">
          <div class="admin-section-heading"><h3>检查与恢复备份包</h3></div>
          <label v-if="kind === 'attachments' || (inspection?.legacy && inspectionHasAttachments(inspection))" class="admin-labeled-field"><span>附件恢复位置</span><select v-model="attachmentMode" class="admin-field"><option value="original">保留原本地／云端分布</option><option value="local">全部恢复到本地</option></select></label>
          <p class="text-sm my-2" :class="theme.mutedText">选择 ZIP 后检查内容，不会立即恢复。独立包请在对应模块恢复；旧完整包由站长在数据备份模块恢复，其中包含的附件也会一起恢复。检查通过后仍需人工确认。</p>
          <input ref="fileInput" type="file" accept=".zip,application/zip" aria-label="选择要检查的备份 ZIP 包" class="block w-full min-w-0 max-w-full text-sm" @change="selectFile" />
          <div v-if="inspection" class="mt-3 space-y-2">
            <p>{{ inspection.legacy ? '旧完整备份包' : '独立备份包' }} · {{ backupLabels[inspection.kind] }}<span v-if="kind === 'attachments' || inspection.legacy"> · 附件 {{ inspection.attachmentCount }} 个</span></p>
            <p>包含：{{ inspection.components.join('、') || '以检查结果为准' }}</p>
            <p v-for="warning in inspection.warnings" :key="warning" class="text-amber-600">{{ warning }}</p>
            <p v-if="inspection.fallbackRequired" class="text-amber-600">原云存储不可用；恢复前请选择本地并确认附件迁移。</p>
            <div class="admin-row-actions"><UButton class="admin-action" color="orange" :loading="busy" @click="restoreUpload">确认恢复此包</UButton><UButton class="admin-action" color="gray" @click="clearUpload">取消</UButton></div>
          </div>
        </section>
      </fieldset>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onActivated, onDeactivated, onMounted, onUnmounted, ref } from 'vue'
import { useRuntimeConfig } from '#imports'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import ConfigLoadState from './ConfigLoadState.vue'
import { backupCapability, backupLabels, backupScopes, backupRecordStatus, backupPlanInput, backupResultMessage, confirmBackupRestore, type AttachmentMode, type BackupKind, type BackupPlan, type BackupRecord, type BackupTarget } from './backup-contract'
import { BackupFallbackRequired, inspectionHasAttachments, parseBackupEnvelope, parseStatus, parseHistory, parseRestore, parseInspection, parseMessage, type Inspection, type RestoreResult } from './backup-payload'
const props = defineProps<{ kind: BackupKind, theme: Record<string, string>, adminShellCardClass: string | string[] | Record<string, boolean> }>()
const { can, isPrimaryAdmin } = useAdminCapabilities()
const label = computed(() => backupLabels[props.kind])
const allowed = (action: string) => (props.kind !== 'server-config' || isPrimaryAdmin.value) && (!['download', 'restore'].includes(action) || isPrimaryAdmin.value) && can(backupCapability(props.kind, 'view')) && can(backupCapability(props.kind, action))
const base = `${useRuntimeConfig().public.baseApi || '/api'}/admin/backups`
const loading = ref(false), busy = ref(false), ready = ref(false), error = ref(''), notice = ref(''), directory = ref(''), pendingRestart = ref(false)
const warnings = ref<string[]>([]), records = ref<BackupRecord[]>([]), plan = ref<BackupPlan | null>(null), target = ref<BackupTarget>('local'), attachmentMode = ref<AttachmentMode>('original')
const scope = ref<'local' | 'all'>('local')
const inspection = ref<Inspection | null>(null), fileInput = ref<HTMLInputElement | null>(null)
let selectedFile: File | null = null
let active = true, generation = 0, lifecycle = 0
const controllers = new Set<AbortController>()
const request = async <T,>(path: string, parse: (value: unknown) => T, init: RequestInit = {}): Promise<T> => {
  const current = lifecycle
  const controller = new AbortController()
  controllers.add(controller)
  const timer = setTimeout(() => controller.abort(), init.method === 'POST' ? 1800000 : 120000)
  try {
    const response = await fetch(`${base}${path}`, { ...init, credentials: 'include', signal: controller.signal })
    const body: unknown = await response.json()
    if (!active || current !== lifecycle) throw new Error('操作已中止')
    return parse(parseBackupEnvelope(body, response.ok))
  } finally { clearTimeout(timer); controllers.delete(controller) }
}
const json = (value: object): RequestInit => ({ method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(value) })
const fail = (cause: unknown) => { if (active) error.value = cause instanceof Error ? (cause.name === 'AbortError' || cause.name === 'TimeoutError' ? '请求超时或已中止，请刷新记录确认服务器结果后重试。' : cause.message) : '操作失败，请重试' }
const load = async () => {
  if (!active || !allowed('view') || loading.value || busy.value) return
  const current = ++generation
  loading.value = true; error.value = ''; ready.value = false
  try {
    const status = await request('/status', parseStatus)
    const history = allowed('history') ? await request(`/${props.kind}/history`, parseHistory) : null
    if (!active || current !== generation || !allowed('view')) return
    plan.value = status.plans.find(item => item.kind === props.kind) || null
    scope.value = plan.value?.scope === 'all' ? 'all' : 'local'
    target.value = plan.value?.target || 'local'; directory.value = status.directories[props.kind] || ''; pendingRestart.value = status.pendingRestart || !!history?.pendingRestart; warnings.value = [...new Set([...status.warnings, ...(history?.warnings || [])])]
    records.value = (history?.records || []).sort((a, b) => b.createdAt.localeCompare(a.createdAt)); ready.value = true
  } catch (cause) { if (current === generation) fail(cause) } finally { if (current === generation) loading.value = false }
}
const run = async (action: string, operation: () => Promise<void>) => {
  if (!active || !ready.value || busy.value || loading.value || !allowed(action)) return
  const current = lifecycle
  busy.value = true; error.value = ''; notice.value = ''
  try { await operation() } catch (cause) { if (current === lifecycle) fail(cause) } finally { if (current === lifecycle) busy.value = false }
  if (active && current === lifecycle && ['create', 'plan'].includes(action)) {
    const actionError = error.value
    await load()
    if (active && current === lifecycle && actionError) error.value = [actionError, error.value].filter(Boolean).join('；')
  }
}
const createBackup = () => run('create', async () => {
  const result = await request(`/${props.kind}/create`, parseMessage, json({ target: target.value, ...(props.kind === 'attachments' ? { scope: scope.value } : {}) }))
  if (active) notice.value = result.message || '备份已生成。'
})
const savePlan = () => run('plan', async () => {
  if (!plan.value) return
  await request(`/plans/${props.kind}`, () => undefined, { ...json(backupPlanInput(plan.value)), method: 'PUT' }); if (active) notice.value = '自动备份计划已保存。'
})
const showRestore = (result: RestoreResult) => {
  if (!active) return
  pendingRestart.value = !!result.pendingRestart || !!result.restartRequired
  warnings.value = result.warnings || []
  notice.value = `${pendingRestart.value ? '恢复已暂存：' : ''}${backupResultMessage(result)}`
}
const confirmRestore = (name: string, includesAttachments = props.kind === 'attachments') => includesAttachments
  ? confirmBackupRestore(message => window.confirm(message), name, attachmentMode.value)
  : window.confirm(`确认恢复“${name}”？当前内容将被替换，并在重启后生效。请先备份当前内容。`)
const restoreWithFallback = async (operation: (mode: AttachmentMode) => Promise<RestoreResult>, includesAttachments = props.kind === 'attachments') => {
  try { showRestore(await operation(includesAttachments ? attachmentMode.value : 'original')) } catch (cause) {
    if (!includesAttachments || !(cause instanceof BackupFallbackRequired) || attachmentMode.value !== 'original') throw cause
    if (!active || !allowed('restore') || !window.confirm('原云存储当前不可用。确认将附件迁移到本地后恢复？取消将不发送后续恢复请求。')) return false
    attachmentMode.value = 'local'
    showRestore(await operation('local'))
  }
  return true
}
const restoreRecord = (record: BackupRecord, source: 'local' | 'cloud') => {
  if (!active || !ready.value || loading.value || !allowed('restore') || busy.value || record.kind !== props.kind || !recordSources(record).includes(source) || !confirmRestore(`${label.value} ${formatTime(record.createdAt)}`)) return
  return run('restore', async () => {
    await restoreWithFallback(mode => request(`/${props.kind}/history/${encodeURIComponent(record.id)}/restore`, parseRestore, json({ confirm: true, source, attachmentMode: mode, ...(mode === 'local' ? { confirmLocalMigration: true } : {}) })))
  })
}
const downloadRecord = (record: BackupRecord, source: 'local' | 'cloud') => run('download', async () => {
  if (record.kind !== props.kind || !recordSources(record).includes(source)) return
  const current = lifecycle, controller = new AbortController()
  controllers.add(controller)
  const timer = setTimeout(() => controller.abort(), 1800000)
  try {
    const response = await fetch(`${base}/${props.kind}/history/${encodeURIComponent(record.id)}/download?source=${source}`, { credentials: 'include', signal: controller.signal })
    if (!response.ok || response.headers.get('content-type')?.includes('application/json')) throw new Error('下载失败或无下载权限')
    const blob = await response.blob()
    if (!active || current !== lifecycle || !allowed('download')) return
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a'); anchor.href = url; anchor.download = `${props.kind}-${record.id}.zip`; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
  } finally { clearTimeout(timer); controllers.delete(controller) }
})
const clearUpload = () => { selectedFile = null; inspection.value = null; if (fileInput.value) fileInput.value.value = '' }
const selectFile = (event: Event) => {
  const file = (event.target as HTMLInputElement).files?.[0]
  clearUpload()
  if (!file || !allowed('restore')) return
  return run('restore', async () => {
    const form = new FormData(); form.append('file', file)
    const result = await request('/inspect', parseInspection, { method: 'POST', body: form })
    if (!active) return
    if ((!result.legacy && result.kind !== props.kind) || !allowed('restore') || (result.legacy && (!isPrimaryAdmin.value || props.kind !== 'data'))) throw new Error('此备份包不属于当前模块或没有恢复权限；旧完整包请由站长在数据备份模块检查。')
    attachmentMode.value = 'original'; selectedFile = file; inspection.value = result
  })
}
const restoreUpload = () => {
  if (!active || !ready.value || loading.value || !allowed('restore') || busy.value || !selectedFile || !inspection.value || !confirmRestore(selectedFile.name, props.kind === 'attachments' || (inspection.value.legacy && inspectionHasAttachments(inspection.value)))) return
  return run('restore', async () => {
    if (!selectedFile) return
    const file = selectedFile
    const includesAttachments = props.kind === 'attachments' || !!(inspection.value?.legacy && inspectionHasAttachments(inspection.value))
    const completed = await restoreWithFallback(mode => {
      const restoreMode = includesAttachments ? mode : 'original'
      const form = new FormData(); form.append('file', file); form.append('confirm', 'true'); form.append('attachmentMode', restoreMode)
      if (restoreMode === 'local') form.append('confirmLocalMigration', 'true')
      return request('/restore', parseRestore, { method: 'POST', body: form })
    }, includesAttachments)
    if (completed) clearUpload()
  })
}
const recordSources = (record: BackupRecord): ('local' | 'cloud')[] => [...(record.local ? ['local' as const] : []), ...(record.cloud ? ['cloud' as const] : [])]
const formatTime = (value?: string) => value ? new Date(value).toLocaleString('zh-CN') : '暂无'
const formatSize = (size: number) => `${(size / 1024 / 1024).toFixed(2)} MB`
const stop = () => { active = false; ++generation; ++lifecycle; for (const controller of controllers) controller.abort(); controllers.clear(); loading.value = false; busy.value = false; ready.value = false }
onMounted(load)
onActivated(() => { active = true; if (!ready.value) void load() })
onDeactivated(stop)
onUnmounted(stop)
</script>
