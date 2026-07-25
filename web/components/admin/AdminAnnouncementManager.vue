<template>
  <section class="admin-announcement-manager" :class="{ 'is-dark': isDark }">
    <div class="manager-heading">
      <div>
        <h3>公告管理</h3>
        <p>创建草稿，确认内容后发布；已发布公告需先撤回才能删除。</p>
      </div>
      <button type="button" class="icon-action nw-action-btn nw-tooltip-anchor" data-tooltip="刷新" aria-label="刷新公告管理列表" :disabled="loading" @click="loadAnnouncements">
        <UIcon name="i-mdi-refresh" class="w-4 h-4" :class="{ 'animate-spin': loading }" />
      </button>
    </div>

    <div class="draft-composer">
      <div class="composer-title">新建公告草稿</div>
      <UInput v-model="draft.title" maxlength="100" placeholder="公告标题" />
      <UTextarea v-model="draft.content" :rows="5" placeholder="公告正文，支持 Markdown" />
      <div class="composer-footer">
        <span>{{ draft.title.trim().length }}/100</span>
        <UButton color="primary" :loading="creating" :disabled="!canCreate" @click="createDraft">保存草稿</UButton>
      </div>
    </div>

    <div class="list-toolbar">
      <div class="filter-row">
        <USelect v-model="statusFilter" :options="statusOptions" class="w-36" @change="changeFilter" />
        <span class="total-copy">共 {{ total }} 条</span>
      </div>
      <div class="bulk-row">
        <label class="select-all-control" :class="{ disabled: deletableItems.length === 0 }">
          <input type="checkbox" :checked="allDeletableSelected" :disabled="deletableItems.length === 0" @change="selectAllDeletable" />
          <span>全选可删除项</span>
        </label>
        <UButton color="red" variant="soft" size="sm" :loading="deletingBatch" :disabled="selectedIds.length === 0" @click="batchDelete">
          批量删除（{{ selectedIds.length }}）
        </UButton>
      </div>
    </div>

    <div v-if="loading && !items.length" class="manager-empty">正在加载公告…</div>
    <div v-else-if="!items.length" class="manager-empty">当前筛选下暂无公告，可先创建草稿。</div>
    <div v-else class="announcement-admin-list">
      <article v-for="item in items" :key="item.id" class="admin-announcement-card">
        <div class="selection-column">
          <input
            type="checkbox"
            :checked="selectedIds.includes(item.id)"
            :disabled="!isDeletable(item)"
            :aria-label="`选择公告 ${item.title}`"
            @change="toggleSelected(item.id)"
          />
        </div>
        <div class="announcement-card-main">
          <div class="card-title-row">
            <div class="title-and-status">
              <h4>{{ item.title }}</h4>
              <span class="status-badge" :class="`status-${item.status}`">{{ statusLabel(item.status) }}</span>
              <span v-if="item.revision > 1" class="revision-badge">修订 {{ item.revision }}</span>
            </div>
            <time>{{ formatDate(item.updated_at) }}</time>
          </div>
          <p class="content-preview">{{ excerpt(item.content) }}</p>

          <div v-if="item.push_enabled" class="push-summary">
            <span class="push-summary-label"><UIcon name="i-mdi-message-fast-outline" class="w-4 h-4" />VoceChat 投递</span>
            <span>待发送 {{ item.push_summary?.pending || 0 }}</span>
            <span>发送中 {{ item.push_summary?.processing || 0 }}</span>
            <span class="push-success">成功 {{ item.push_summary?.sent || 0 }}</span>
            <span>跳过 {{ item.push_summary?.skipped || 0 }}</span>
            <span :class="{ 'push-failed': (item.push_summary?.failed || 0) > 0 }">失败 {{ item.push_summary?.failed || 0 }}</span>
            <button v-if="item.status === 'published' && (item.push_summary?.failed || 0) > 0" type="button" class="retry-button" @click="retryFailedPush(item)">重试失败项</button>
          </div>

          <div class="card-actions">
            <UButton size="xs" color="indigo" variant="soft" @click="openEdit(item)">编辑</UButton>
            <UButton v-if="item.status === 'draft'" size="xs" color="green" variant="soft" @click="openPublish(item)">发布</UButton>
            <UButton v-else-if="item.status === 'published'" size="xs" color="orange" variant="soft" @click="withdraw(item)">撤回</UButton>
            <UButton v-else-if="item.status === 'withdrawn'" size="xs" color="green" variant="soft" @click="openPublish(item)">恢复发布</UButton>
            <UButton v-if="isDeletable(item)" size="xs" color="red" variant="soft" @click="deleteOne(item)">删除</UButton>
          </div>
        </div>
      </article>
    </div>

    <div v-if="totalPages > 1" class="manager-pager">
      <UButton size="xs" variant="soft" :disabled="page <= 1 || loading" @click="goPage(page - 1)">上一页</UButton>
      <span>第 {{ page }} / {{ totalPages }} 页</span>
      <UButton size="xs" variant="soft" :disabled="page >= totalPages || loading" @click="goPage(page + 1)">下一页</UButton>
    </div>

    <UModal v-model="editOpen" :ui="{ width: 'sm:max-w-2xl' }">
      <UCard>
        <template #header><h3 class="modal-heading">编辑公告</h3></template>
        <div class="modal-form">
          <UInput v-model="editForm.title" maxlength="100" placeholder="公告标题" />
          <UTextarea v-model="editForm.content" :rows="10" placeholder="公告正文，支持 Markdown" />
          <label v-if="editingItem?.status === 'published'" class="renotify-control">
            <UToggle v-model="editForm.renotify" />
            <span>
              <strong>重新通知</strong>
              <small>开启后，该公告会在站内重新标记为未读；VoceChat 不会重复推送。</small>
            </span>
          </label>
        </div>
        <template #footer>
          <div class="modal-footer-actions">
            <UButton variant="soft" color="gray" @click="editOpen=false">取消</UButton>
            <UButton color="primary" :loading="savingEdit" @click="saveEdit">保存修改</UButton>
          </div>
        </template>
      </UCard>
    </UModal>

    <UModal v-model="publishOpen" :ui="{ width: 'sm:max-w-lg' }">
      <UCard>
        <template #header><h3 class="modal-heading">{{ publishingItem?.status === 'withdrawn' ? '恢复发布公告' : '发布公告' }}</h3></template>
        <div class="publish-confirmation">
          <p>{{ publishingItem?.status === 'withdrawn'
            ? `确认恢复发布“${publishingItem?.title}”吗？恢复后仅重新公开，不改变原有已读状态。`
            : `确认发布“${publishingItem?.title}”吗？发布后会立即进入前台未读公告。`
          }}</p>
          <label v-if="publishingItem?.status === 'draft'" class="renotify-control">
            <UToggle v-model="publishPushEnabled" />
            <span>
              <strong>公告推送</strong>
              <small>向所有已开启“接收 VoceChat 推送”的用户建立持久化投递任务。</small>
            </span>
          </label>
          <p v-else class="restore-note">恢复发布不会重置已读状态，也不会重复发送 VoceChat。</p>
        </div>
        <template #footer>
          <div class="modal-footer-actions">
            <UButton variant="soft" color="gray" @click="publishOpen=false">取消</UButton>
            <UButton color="green" :loading="publishing" @click="publish">确认发布</UButton>
          </div>
        </template>
      </UCard>
    </UModal>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { deleteRequest, getRequest, postRequest, putRequest } from '~/utils/api'
import { useToast } from '#ui/composables/useToast'

type PushSummary = { total?: number; pending?: number; processing?: number; sent?: number; failed?: number; skipped?: number }
type AdminAnnouncement = {
  id: number
  title: string
  content: string
  status: 'draft' | 'published' | 'withdrawn'
  revision: number
  push_enabled: boolean
  published_at?: string
  withdrawn_at?: string
  created_at: string
  updated_at: string
  push_summary?: PushSummary
}
type AdminAnnouncementListPayload = { items?: AdminAnnouncement[]; page?: number; page_size?: number; total?: number }

defineProps<{ isDark?: boolean }>()
const toast = useToast()
const items = ref<AdminAnnouncement[]>([])
const page = ref(1)
const pageSize = 20
const total = ref(0)
const loading = ref(false)
const creating = ref(false)
const deletingBatch = ref(false)
const savingEdit = ref(false)
const publishing = ref(false)
const statusFilter = ref('all')
const selectedIds = ref<number[]>([])
const draft = reactive({ title: '', content: '' })
const editOpen = ref(false)
const editingItem = ref<AdminAnnouncement | null>(null)
const editForm = reactive({ title: '', content: '', renotify: false })
const publishOpen = ref(false)
const publishingItem = ref<AdminAnnouncement | null>(null)
const publishPushEnabled = ref(false)
const statusOptions = [
  { label: '全部状态', value: 'all' },
  { label: '草稿', value: 'draft' },
  { label: '已发布', value: 'published' },
  { label: '已撤回', value: 'withdrawn' }
]
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
const canCreate = computed(() => draft.title.trim().length > 0 && draft.content.trim().length > 0)
const isDeletable = (item: AdminAnnouncement) => item.status === 'draft' || item.status === 'withdrawn'
const deletableItems = computed(() => items.value.filter(isDeletable))
const allDeletableSelected = computed(() => deletableItems.value.length > 0 && deletableItems.value.every((item) => selectedIds.value.includes(item.id)))

const notify = (title: string, description: string, color: 'green' | 'red' | 'orange' = 'green') => toast.add({ title, description, color, timeout: 2200 })
const loadAnnouncements = async () => {
  loading.value = true
  try {
    const response = await getRequest<AdminAnnouncementListPayload>('admin/announcements', { status: statusFilter.value, page: page.value, pageSize }, { credentials: 'include', silent: true })
    if (!response || response.code !== 1) throw new Error(response?.msg || '获取公告管理列表失败')
    items.value = Array.isArray(response.data?.items) ? response.data.items : []
    total.value = Math.max(0, Number(response.data?.total || 0))
    page.value = Math.max(1, Number(response.data?.page || page.value))
    selectedIds.value = selectedIds.value.filter((id) => items.value.some((item) => item.id === id && isDeletable(item)))
  } catch (error: any) {
    notify('公告加载失败', error?.message || '请稍后重试', 'red')
  } finally {
    loading.value = false
  }
}
const changeFilter = async () => { page.value = 1; selectedIds.value = []; await loadAnnouncements() }
const goPage = async (target: number) => { page.value = Math.min(totalPages.value, Math.max(1, target)); selectedIds.value = []; await loadAnnouncements() }

const createDraft = async () => {
  if (!canCreate.value || creating.value) return
  creating.value = true
  try {
    const response = await postRequest<any>('admin/announcements', { title: draft.title.trim(), content: draft.content.trim() }, { credentials: 'include', silent: true })
    if (!response || response.code !== 1) throw new Error(response?.msg || '创建公告草稿失败')
    draft.title = ''
    draft.content = ''
    statusFilter.value = 'all'
    page.value = 1
    await loadAnnouncements()
    notify('草稿已保存', '确认内容后可从列表发布。')
  } catch (error: any) {
    notify('保存失败', error?.message || '公告草稿未保存', 'red')
  } finally { creating.value = false }
}

const toggleSelected = (id: number) => {
  selectedIds.value = selectedIds.value.includes(id) ? selectedIds.value.filter((item) => item !== id) : [...selectedIds.value, id]
}
const selectAllDeletable = () => {
  selectedIds.value = allDeletableSelected.value ? [] : deletableItems.value.map((item) => item.id)
}
const batchDelete = async () => {
  if (!selectedIds.value.length || deletingBatch.value) return
  if (!window.confirm(`确定删除选中的 ${selectedIds.value.length} 条公告吗？此操作不可恢复。`)) return
  deletingBatch.value = true
  try {
    const response = await postRequest<any>('admin/announcements/batch-delete', { ids: selectedIds.value }, { credentials: 'include', silent: true })
    if (!response || response.code !== 1) throw new Error(response?.msg || '批量删除失败')
    const deleted = Number(response.data?.deleted_count || 0)
    const skipped = Array.isArray(response.data?.skipped_ids) ? response.data.skipped_ids.length : 0
    selectedIds.value = []
    await loadAnnouncements()
    notify('批量删除完成', `已删除 ${deleted} 条${skipped ? `，跳过 ${skipped} 条不可删除公告` : ''}`)
  } catch (error: any) {
    notify('批量删除失败', error?.message || '请稍后重试', 'red')
  } finally { deletingBatch.value = false }
}
const deleteOne = async (item: AdminAnnouncement) => {
  if (!window.confirm(`确定删除“${item.title}”吗？`)) return
  try {
    const response = await deleteRequest<any>(`admin/announcements/${item.id}`, undefined, { credentials: 'include', silent: true })
    if (!response || response.code !== 1) throw new Error(response?.msg || '删除失败')
    await loadAnnouncements()
    notify('公告已删除', item.title)
  } catch (error: any) { notify('删除失败', error?.message || '请稍后重试', 'red') }
}

const openEdit = (item: AdminAnnouncement) => {
  editingItem.value = item
  editForm.title = item.title
  editForm.content = item.content
  editForm.renotify = false
  editOpen.value = true
}
const saveEdit = async () => {
  if (!editingItem.value || savingEdit.value) return
  savingEdit.value = true
  try {
    const response = await putRequest<any>(`admin/announcements/${editingItem.value.id}`, {
      title: editForm.title.trim(), content: editForm.content.trim(), renotify: editForm.renotify
    }, { credentials: 'include', silent: true })
    if (!response || response.code !== 1) throw new Error(response?.msg || '保存公告失败')
    editOpen.value = false
    await loadAnnouncements()
    notify('公告已更新', editForm.renotify ? '已在站内重新标记为未读，VoceChat 未重复推送。' : '已保留原有已读状态。')
  } catch (error: any) { notify('保存失败', error?.message || '请稍后重试', 'red') }
  finally { savingEdit.value = false }
}

const openPublish = (item: AdminAnnouncement) => {
  publishingItem.value = item
  publishPushEnabled.value = false
  publishOpen.value = true
}
const publish = async () => {
  if (!publishingItem.value || publishing.value) return
  const restoring = publishingItem.value.status === 'withdrawn'
  publishing.value = true
  try {
    const response = await postRequest<any>(`admin/announcements/${publishingItem.value.id}/publish`, { push_enabled: publishPushEnabled.value }, { credentials: 'include', silent: true })
    if (!response || response.code !== 1) throw new Error(response?.msg || '发布公告失败')
    publishOpen.value = false
    await loadAnnouncements()
    notify(
      restoring ? '公告已恢复发布' : '公告已发布',
      restoring
        ? '已保留原有已读状态，VoceChat 未重复推送。'
        : (publishPushEnabled.value ? 'VoceChat 持久化投递任务已建立。' : '公告已进入前台未读列表。')
    )
  } catch (error: any) { notify('发布失败', error?.message || '请稍后重试', 'red') }
  finally { publishing.value = false }
}
const withdraw = async (item: AdminAnnouncement) => {
  if (!window.confirm(`确定撤回“${item.title}”吗？`)) return
  const response = await postRequest<any>(`admin/announcements/${item.id}/withdraw`, {}, { credentials: 'include', silent: true })
  if (!response || response.code !== 1) { notify('撤回失败', response?.msg || '请稍后重试', 'red'); return }
  await loadAnnouncements()
  notify('公告已撤回', '前台不再显示该公告。', 'orange')
}
const retryFailedPush = async (item: AdminAnnouncement) => {
  const response = await postRequest<any>(`admin/announcements/${item.id}/retry-push`, {}, { credentials: 'include', silent: true })
  if (!response || response.code !== 1) { notify('重试失败', response?.msg || '请稍后重试', 'red'); return }
  await loadAnnouncements()
  notify('失败项已重新排队', `共 ${Number(response.data?.retried_count || 0)} 条。`)
}

const statusLabel = (status: string) => ({ draft: '草稿', published: '已发布', withdrawn: '已撤回' }[status] || status)
const excerpt = (content: string) => String(content || '').replace(/\s+/g, ' ').trim().slice(0, 160)
const formatDate = (value?: string) => {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}
onMounted(loadAnnouncements)
</script>

<style scoped>
.admin-announcement-manager { display:flex; flex-direction:column; gap:14px; color:#1e293b; }
.admin-announcement-manager.is-dark { color:#e2e8f0; }
.manager-heading,.list-toolbar,.composer-footer,.card-title-row,.card-actions,.modal-footer-actions { display:flex; align-items:center; justify-content:space-between; gap:12px; }
.manager-heading h3,.modal-heading { margin:0; font-size:1.05rem; font-weight:750; }
.manager-heading p { margin:4px 0 0; color:#64748b; font-size:.78rem; }
.is-dark .manager-heading p { color:#94a3b8; }
.icon-action { width:34px; height:34px; display:inline-flex; align-items:center; justify-content:center; border-radius:9px; }
.draft-composer { display:flex; flex-direction:column; gap:10px; padding:14px; border:1px solid rgba(99,102,241,.22); border-radius:14px; background:rgba(99,102,241,.055); }
.composer-title { font-size:.88rem; font-weight:720; color:#4f46e5; }
.composer-footer { color:#64748b; font-size:.72rem; }
.list-toolbar { padding:11px 12px; border:1px solid rgba(100,116,139,.18); border-radius:12px; background:rgba(248,250,252,.72); }
.is-dark .list-toolbar { background:rgba(15,23,42,.3); }
.filter-row,.bulk-row,.select-all-control,.title-and-status,.push-summary { display:flex; align-items:center; gap:9px; }
.total-copy,.select-all-control { color:#64748b; font-size:.76rem; }
.select-all-control { cursor:pointer; }
.select-all-control.disabled { opacity:.55; cursor:not-allowed; }
.announcement-admin-list { display:flex; flex-direction:column; gap:9px; }
.admin-announcement-card { display:grid; grid-template-columns:28px minmax(0,1fr); gap:8px; padding:14px; border:1px solid rgba(100,116,139,.18); border-radius:14px; background:rgba(255,255,255,.7); }
.is-dark .admin-announcement-card { background:rgba(15,23,42,.35); border-color:rgba(148,163,184,.18); }
.selection-column { padding-top:3px; }
.announcement-card-main { min-width:0; }
.card-title-row h4 { margin:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:.95rem; }
.card-title-row time { flex:none; color:#7c889b; font-size:.7rem; font-variant-numeric:tabular-nums; }
.status-badge,.revision-badge { padding:2px 7px; border-radius:999px; font-size:.65rem; font-weight:750; }
.status-draft { background:#e2e8f0; color:#475569; }
.status-published { background:#dcfce7; color:#15803d; }
.status-withdrawn { background:#ffedd5; color:#c2410c; }
.revision-badge { background:#e0e7ff; color:#4338ca; }
.content-preview { margin:9px 0; color:#64748b; font-size:.8rem; line-height:1.55; }
.is-dark .content-preview { color:#a6b2c3; }
.push-summary { flex-wrap:wrap; margin:9px 0; padding:8px 10px; border-radius:10px; background:rgba(15,23,42,.045); color:#64748b; font-size:.69rem; }
.is-dark .push-summary { background:rgba(2,6,23,.35); color:#a8b3c4; }
.push-summary-label { display:inline-flex; align-items:center; gap:5px; color:#4f46e5; font-weight:750; }
.push-success { color:#15803d; }
.push-failed { color:#dc2626; font-weight:750; }
.retry-button { border:0; padding:2px 6px; border-radius:6px; background:#fee2e2; color:#b91c1c; font-weight:700; }
.card-actions { justify-content:flex-end; flex-wrap:wrap; }
.manager-empty { padding:40px 18px; border:1px dashed rgba(100,116,139,.25); border-radius:14px; text-align:center; color:#64748b; font-size:.85rem; }
.manager-pager { display:flex; align-items:center; justify-content:center; gap:12px; color:#64748b; font-size:.75rem; }
.modal-form,.publish-confirmation { display:flex; flex-direction:column; gap:12px; }
.renotify-control { display:flex; align-items:flex-start; gap:11px; padding:12px; border:1px solid rgba(99,102,241,.2); border-radius:12px; background:rgba(99,102,241,.05); }
.renotify-control span { display:flex; flex-direction:column; gap:3px; }
.renotify-control strong { font-size:.84rem; }
.renotify-control small,.restore-note { color:#64748b; font-size:.74rem; line-height:1.45; }
.modal-footer-actions { justify-content:flex-end; }
@media (max-width:680px) {
  .manager-heading,.list-toolbar,.card-title-row { align-items:flex-start; flex-direction:column; }
  .manager-heading .icon-action { align-self:flex-end; margin-top:-40px; }
  .list-toolbar,.filter-row,.bulk-row { width:100%; }
  .bulk-row { justify-content:space-between; flex-wrap:wrap; }
  .card-title-row time { margin-top:3px; }
  .title-and-status { flex-wrap:wrap; }
  .admin-announcement-card { grid-template-columns:24px minmax(0,1fr); padding:12px 10px; }
}
</style>
