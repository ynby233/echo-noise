<template>
  <div class="admin-card" :class="[theme?.cardBg, theme?.border]">
    <AdminModuleHeader title="公告管理" description="创建草稿，确认内容后发布；已发布公告需先撤回才能删除。" icon="i-heroicons-megaphone" :badge="`共 ${total} 条`" :theme="theme">
      <template #actions>
        <UButton size="sm" class="admin-action" :loading="loading" color="gray" variant="soft" icon="i-heroicons-arrow-path" @click="loadAnnouncements">刷新</UButton>
      </template>
    </AdminModuleHeader>

    <div class="px-4 pb-4">
      <div class="rounded-lg p-3 mb-3" :class="theme?.subtleBg">
        <div class="text-sm mb-2" :class="theme?.mutedText">新建公告草稿</div>
        <UInput v-model="draft.title" maxlength="100" placeholder="公告标题" class="admin-input mb-2" />
        <UTextarea v-model="draft.content" :rows="5" placeholder="公告正文，支持 Markdown" class="admin-textarea w-full mb-2" />
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-xs" :class="theme?.mutedText">{{ draft.title.trim().length }}/100</span>
          <UButton size="sm" color="primary" class="admin-action" :loading="creating" :disabled="!canCreate" @click="createDraft">保存草稿</UButton>
        </div>
      </div>

      <div class="announcement-batch-toolbar rounded-lg border px-3 py-2 mb-3" :class="[theme?.border, theme?.subtleBg]">
        <div class="flex items-center gap-2 flex-wrap">
          <USelect v-model="statusFilter" :options="statusOptions" class="admin-select w-32" @change="changeFilter" />
          <span class="text-xs" :class="theme?.mutedText">已选择 {{ selectedIds.length }} 条</span>
        </div>
        <div class="announcement-batch-actions">
          <UButton class="admin-action" size="sm" color="gray" variant="soft" icon="i-heroicons-check-circle" :disabled="deletableItems.length === 0" @click="selectAllDeletable">
            {{ allDeletableSelected ? '取消全选' : '全选可删除项' }}
          </UButton>
          <UButton class="admin-action" size="sm" color="red" variant="soft" icon="i-heroicons-trash" :loading="deletingBatch" :disabled="selectedIds.length === 0" @click="batchDelete">
            批量删除（{{ selectedIds.length }}）
          </UButton>
        </div>
      </div>

      <div v-if="loading && !items.length" class="text-sm" :class="theme?.mutedText">正在加载公告…</div>
      <div v-else-if="!items.length" class="text-sm" :class="theme?.mutedText">当前筛选下暂无公告，可先创建草稿。</div>
      <div v-else class="announcement-admin-list">
        <div v-for="item in items" :key="item.id" class="announcement-item-card rounded-lg border p-3" :class="[theme?.border, selectedIds.includes(item.id) ? 'announcement-item-selected' : '']">
          <label class="announcement-select-check" :class="{ 'is-disabled': !isDeletable(item) }">
            <input
              type="checkbox"
              :checked="selectedIds.includes(item.id)"
              :disabled="!isDeletable(item)"
              :aria-label="`选择公告 ${item.title}`"
              @change="toggleSelected(item.id)"
            />
            <span class="text-xs" :class="theme?.mutedText">选择</span>
          </label>
          <div class="announcement-card-main">
            <div class="announcement-card-head">
              <div class="flex items-center gap-2 flex-wrap min-w-0">
                <div class="announcement-card-title text-sm font-semibold" :class="theme?.text">{{ item.title }}</div>
                <UBadge class="admin-badge" :color="statusColor(item.status)" size="xs" variant="soft">{{ statusLabel(item.status) }}</UBadge>
                <UBadge class="admin-badge" v-if="item.revision > 1" color="blue" size="xs" variant="soft">修订 {{ item.revision }}</UBadge>
              </div>
              <div class="text-xs whitespace-nowrap" :class="theme?.mutedText">{{ formatDate(item.updated_at) }}</div>
            </div>
            <p class="text-xs mt-2 leading-relaxed" :class="theme?.mutedText">{{ excerpt(item.content) }}</p>

            <div v-if="item.push_enabled" class="announcement-push-summary rounded p-2 mt-2" :class="theme?.subtleBg">
              <span class="announcement-push-label text-xs" :class="theme?.text">
                <UIcon name="i-heroicons-paper-airplane" class="w-4 h-4" />VoceChat 投递
              </span>
              <span class="text-xs" :class="theme?.mutedText">待发送 {{ item.push_summary?.pending || 0 }}</span>
              <span class="text-xs" :class="theme?.mutedText">发送中 {{ item.push_summary?.processing || 0 }}</span>
              <span class="text-xs text-emerald-500">成功 {{ item.push_summary?.sent || 0 }}</span>
              <span class="text-xs" :class="theme?.mutedText">跳过 {{ item.push_summary?.skipped || 0 }}</span>
              <span class="text-xs" :class="(item.push_summary?.failed || 0) > 0 ? 'text-red-500 font-semibold' : theme?.mutedText">失败 {{ item.push_summary?.failed || 0 }}</span>
              <UButton class="admin-action" v-if="item.status === 'published' && (item.push_summary?.failed || 0) > 0" size="sm" color="red" variant="soft" @click="retryFailedPush(item)">重试失败项</UButton>
            </div>

            <div class="announcement-card-actions mt-2">
              <UButton class="admin-action" size="sm" color="gray" variant="soft" icon="i-heroicons-pencil-square" @click="openEdit(item)">编辑</UButton>
              <UButton class="admin-action" v-if="item.status === 'draft'" size="sm" color="primary" variant="solid" icon="i-heroicons-paper-airplane" @click="openPublish(item)">发布</UButton>
              <UButton class="admin-action" v-else-if="item.status === 'published'" size="sm" color="orange" variant="soft" icon="i-heroicons-arrow-uturn-left" @click="withdraw(item)">撤回</UButton>
              <UButton class="admin-action" v-else-if="item.status === 'withdrawn'" size="sm" color="primary" variant="solid" icon="i-heroicons-paper-airplane" @click="openPublish(item)">恢复发布</UButton>
              <UButton class="admin-action" v-if="isDeletable(item)" size="sm" color="red" variant="soft" icon="i-heroicons-trash" @click="deleteOne(item)">删除</UButton>
            </div>
          </div>
        </div>
      </div>

      <div v-if="totalPages > 1" class="flex items-center justify-center gap-3 mt-3">
        <UButton class="admin-action" size="sm" color="gray" variant="soft" :disabled="page <= 1 || loading" @click="goPage(page - 1)">上一页</UButton>
        <span class="text-xs" :class="theme?.mutedText">第 {{ page }} / {{ totalPages }} 页</span>
        <UButton class="admin-action" size="sm" color="gray" variant="soft" :disabled="page >= totalPages || loading" @click="goPage(page + 1)">下一页</UButton>
      </div>
    </div>

    <UModal v-model="editOpen" :ui="{ width: 'sm:max-w-2xl' }">
      <UCard class="admin-dialog" :class="theme?.cardBg">
        <template #header>
          <div class="font-semibold" :class="theme?.text">编辑公告</div>
        </template>
        <div class="flex flex-col gap-3">
          <UInput class="admin-input" v-model="editForm.title" maxlength="100" placeholder="公告标题" />
          <UTextarea class="admin-textarea" v-model="editForm.content" :rows="10" placeholder="公告正文，支持 Markdown" />
          <label v-if="editingItem?.status === 'published'" class="announcement-toggle-control rounded-lg border p-3" :class="[theme?.border, theme?.subtleBg]">
            <UToggle v-model="editForm.renotify" />
            <span>
              <strong class="text-sm" :class="theme?.text">重新通知</strong>
              <small class="text-xs" :class="theme?.mutedText">开启后，该公告会在站内重新标记为未读；VoceChat 不会重复推送。</small>
            </span>
          </label>
        </div>
        <template #footer>
          <div class="flex items-center justify-end gap-2">
            <UButton size="sm" class="admin-action" variant="soft" color="gray" @click="editOpen=false">取消</UButton>
            <UButton size="sm" class="admin-action" color="primary" :loading="savingEdit" @click="saveEdit">保存修改</UButton>
          </div>
        </template>
      </UCard>
    </UModal>

    <UModal v-model="publishOpen" :ui="{ width: 'sm:max-w-lg' }">
      <UCard class="admin-dialog" :class="theme?.cardBg">
        <template #header>
          <div class="font-semibold" :class="theme?.text">{{ publishingItem?.status === 'withdrawn' ? '恢复发布公告' : '发布公告' }}</div>
        </template>
        <div class="flex flex-col gap-3">
          <p class="text-sm" :class="theme?.text">{{ publishingItem?.status === 'withdrawn'
            ? `确认恢复发布“${publishingItem?.title}”吗？恢复后仅重新公开，不改变原有已读状态。`
            : `确认发布“${publishingItem?.title}”吗？发布后会立即进入前台未读公告。`
          }}</p>
          <label v-if="publishingItem?.status === 'draft'" class="announcement-toggle-control rounded-lg border p-3" :class="[theme?.border, theme?.subtleBg]">
            <UToggle v-model="publishPushEnabled" />
            <span>
              <strong class="text-sm" :class="theme?.text">公告推送</strong>
              <small class="text-xs" :class="theme?.mutedText">向所有已开启“接收 VoceChat 推送”的用户建立持久化投递任务。</small>
            </span>
          </label>
          <p v-else class="text-xs" :class="theme?.mutedText">恢复发布不会重置已读状态，也不会重复发送 VoceChat。</p>
        </div>
        <template #footer>
          <div class="flex items-center justify-end gap-2">
            <UButton size="sm" class="admin-action" variant="soft" color="gray" @click="publishOpen=false">取消</UButton>
            <UButton size="sm" class="admin-action" color="primary" :loading="publishing" @click="publish">确认发布</UButton>
          </div>
        </template>
      </UCard>
    </UModal>
  </div>
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

defineProps<{ theme?: Record<string, string> }>()
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
type StatusBadgeColor = 'gray' | 'green' | 'orange'
const statusColor = (status: string): StatusBadgeColor => {
  if (status === 'published') return 'green'
  if (status === 'withdrawn') return 'orange'
  return 'gray'
}
const excerpt = (content: string) => String(content || '').replace(/\s+/g, ' ').trim().slice(0, 160)
const formatDate = (value?: string) => {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}
onMounted(loadAnnouncements)
</script>

<style scoped>
.announcement-batch-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.announcement-batch-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.announcement-admin-list { display:flex; flex-direction:column; gap:8px; }

.announcement-item-card {
  position: relative;
  min-width: 0;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 10px;
  transition: border-color 0.16s ease, box-shadow 0.16s ease, background-color 0.16s ease;
}

.announcement-item-selected {
  border-color: rgba(99, 102, 241, 0.9) !important;
  box-shadow: 0 0 0 1px rgba(99, 102, 241, 0.45);
}

.announcement-select-check { display:inline-flex; align-items:center; gap:5px; flex-direction:column; padding-top:2px; cursor:pointer; }
.announcement-select-check.is-disabled { opacity:.5; cursor:not-allowed; }
.announcement-card-main { min-width:0; }
.announcement-card-head { display:flex; align-items:flex-start; justify-content:space-between; gap:10px; }
.announcement-card-title { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.announcement-card-actions { display:flex; align-items:center; justify-content:flex-end; flex-wrap:wrap; gap:8px; }
.announcement-push-summary { display:flex; align-items:center; flex-wrap:wrap; gap:9px; }
.announcement-push-label { display:inline-flex; align-items:center; gap:5px; font-weight:600; }
.announcement-toggle-control { display:flex; align-items:flex-start; gap:11px; }
.announcement-toggle-control span { display:flex; flex-direction:column; gap:3px; }
@media (max-width: 520px) {
  .announcement-batch-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }

  .announcement-batch-actions {
    justify-content: flex-start;
  }

  .announcement-card-head {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }

  .announcement-card-title { white-space:normal; }
  .announcement-card-actions { justify-content:flex-start; }
}
</style>
