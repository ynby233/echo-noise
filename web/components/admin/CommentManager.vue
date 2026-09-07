<template>
  <section class="comment-manager" :class="theme?.text || ''">
    <AdminModuleHeader
      :title="recycleBin ? '互动回收站' : '互动管理'"
      :description="recycleBin ? '恢复或永久删除评论、回复与留言；被作者从个人回收站彻底删除的内容只能清理，不能恢复。' : '按互动类型和内容检索全站互动，查看笔记与上级互动语境后再执行管理操作。'"
      :icon="recycleBin ? 'i-heroicons-archive-box' : 'i-heroicons-chat-bubble-left-right'"
      :badge="loading ? '读取中' : `${total} 条`"
      :accent="recycleBin ? 'warning' : 'primary'"
      :theme="theme"
    >
      <template #actions><UButton class="admin-action" size="sm" color="gray" variant="soft" icon="i-heroicons-arrow-path" :loading="loading" @click="load">刷新</UButton></template>
    </AdminModuleHeader>

    <div class="manager-body">
      <div v-if="recycleBin && isPrimaryAdmin" class="policy-card" :class="[borderClass, subtleClass]">
        <div>
          <div class="text-sm font-semibold">互动回收站策略</div>
          <p class="mt-1 text-xs leading-5" :class="mutedClass">期限按每条互动移入回收站的时间实时计算；缩短期限会影响已有内容。</p>
        </div>
        <div class="policy-controls">
          <USelect v-model="retentionDays" :options="retentionOptions" class="admin-select w-44" :disabled="savingPolicy" @change="savePolicy" />
          <label class="notify-toggle"><span>站长删除时通知作者</span><UToggle v-model="notifyByPrimary" :disabled="savingPolicy" @change="savePolicy" /></label>
        </div>
      </div>

      <div class="filter-card" :class="borderClass">
        <label class="filter-field"><span>互动内容</span><UInput class="admin-input" v-model="filters.q" placeholder="搜索互动内容" @keyup.enter="applyFilters" /></label>
        <label class="filter-field"><span>互动类型</span><USelect class="admin-select" v-model="filters.kind" :options="kindOptions" @change="applyFilters" /></label>
        <label class="filter-field"><span>作者 ID</span><UInput class="admin-input" v-model="filters.authorId" type="number" placeholder="作者 ID" @keyup.enter="applyFilters" /></label>
        <label v-if="recycleBin" class="filter-field"><span>删除原因</span><USelect class="admin-select" v-model="filters.reason" :options="reasonOptions" @change="applyFilters" /></label>
        <div class="filter-actions">
          <UButton size="sm" class="admin-action" color="primary" variant="solid" @click="applyFilters">应用筛选</UButton>
          <UButton size="sm" class="admin-action" color="gray" variant="soft" @click="resetFilters">清空条件</UButton>
        </div>
      </div>

      <AdminSelectionBar v-if="canSelectForBatch" :selected="selected.length" :total="rows.length" :all-selected="allSelected" :disabled="loading || actionLoading" scope-label="全选当前页" @select-all="toggleAll" @clear="selected = []">
        <template #summary>已选 {{ selected.length }} 条互动</template>
        <UButton class="admin-action" v-if="!recycleBin && canTrash" size="sm" color="orange" variant="soft" :disabled="!selected.length" :loading="actionLoading" @click="batchTrash">批量移入回收站</UButton>
        <UButton class="admin-action" v-if="recycleBin && canRestore" size="sm" color="primary" variant="soft" :disabled="!selected.length" :loading="actionLoading" @click="batchRestore">批量恢复</UButton>
        <UButton class="admin-action" v-if="recycleBin && canDeletePermanently" size="sm" color="red" variant="soft" :disabled="!selected.length" :loading="actionLoading" @click="batchPermanentDelete">批量永久删除</UButton>
      </AdminSelectionBar>

      <div v-if="loading" class="empty"><UIcon name="i-heroicons-arrow-path" class="animate-spin" />正在读取互动…</div>
      <div v-else-if="!rows.length" class="empty"><UIcon name="i-heroicons-inbox" />当前筛选下没有互动</div>
      <div v-else class="interaction-list">
        <article v-for="row in rows" :key="row.id" class="interaction-card" :class="borderClass">
          <div class="interaction-head">
            <div class="interaction-identity">
              <input v-if="canSelectForBatch" v-model="selected" type="checkbox" :value="row.id" :aria-label="`选择互动 ${row.id}`" />
              <UBadge class="admin-badge" size="xs" :color="kindColor(row.kind)" variant="soft">{{ kindLabel(row.kind) }}</UBadge>
              <strong>#{{ row.id }}</strong>
              <span :class="mutedClass">{{ row.username || `用户 ${row.user_id || '—'}` }}</span>
            </div>
            <span class="text-xs" :class="mutedClass">{{ formatDate(recycleBin ? row.deleted_at : row.created_at) }}</span>
          </div>
          <p class="interaction-content">{{ row.content || '（无正文）' }}</p>
          <div class="thread-trail" :class="subtleClass">
            <div class="trail-node"><span>笔记 #{{ row.message_context?.id || row.message_id }}</span><span>{{ contextText(row.message_context) }}</span></div>
            <div v-for="node in row.context || []" :key="`${node.kind}-${node.id}`" class="trail-node is-child"><span>{{ kindLabel(node.kind) }} #{{ node.id }}</span><span>{{ contextText(node) }}</span></div>
          </div>
          <div class="interaction-meta">
            <span>可见性：{{ visibilityLabel(row.effective_visibility) }}</span>
            <span v-if="row.limited_by_ancestor">受上级可见性限制</span>
            <span v-if="recycleBin">原因：{{ reasonLabel(row.deletion_reason_code) }}</span>
            <span v-if="row.user_purged" class="purged-label">作者已从个人回收站彻底删除</span>
            <span v-if="recycleBin" :class="row.recycle_deadline?.auto_cleanup_enabled ? 'deadline' : mutedClass">{{ deadlineText(row.recycle_deadline) }}</span>
          </div>
          <div class="interaction-actions">
            <UButton class="admin-action" v-if="!recycleBin && canEdit && row.can_edit" size="sm" color="primary" variant="soft" :loading="actionLoading" @click="editBody(row)">编辑正文</UButton>
            <UButton class="admin-action" v-if="!recycleBin && canChangeVisibility && row.can_change_visibility" size="sm" color="primary" variant="soft" :loading="actionLoading" @click="changeVisibility(row)">调整可见性</UButton>
            <UButton class="admin-action" v-if="!recycleBin && canTrash && row.can_trash" size="sm" color="orange" variant="soft" :loading="actionLoading" @click="trash(row)">移入回收站</UButton>
            <UButton class="admin-action" v-if="recycleBin && canRestore && row.can_restore && !row.user_purged" size="sm" color="primary" variant="soft" :loading="actionLoading" @click="restore(row)">恢复</UButton>
            <UButton class="admin-action" v-if="recycleBin && canDeletePermanently && row.can_permanently_delete" size="sm" color="red" variant="soft" :loading="actionLoading" @click="removePermanently(row)">永久删除</UButton>
            <span v-if="recycleBin && !row.can_restore && !row.user_purged" class="text-xs" :class="mutedClass">需先恢复仍在回收站中的所有上级内容</span>
          </div>
        </article>
      </div>

      <div class="pagination" :class="mutedClass">
        <span>共 {{ total }} 条</span>
        <div><UButton class="admin-action" size="sm" color="gray" variant="soft" :disabled="page <= 1 || loading" @click="page--; load()">上一页</UButton><span>第 {{ page }} 页</span><UButton class="admin-action" size="sm" color="gray" variant="soft" :disabled="page * pageSize >= total || loading" @click="page++; load()">下一页</UButton></div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import AdminSelectionBar from '~/components/admin/AdminSelectionBar.vue'
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { deleteRequest, getRequest, postRequest, putRequest } from '~/utils/api'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'

const props = defineProps<{ recycleBin?: boolean; theme?: any }>()
const toast = useToast()
const { can } = useAdminCapabilities()
const loading = ref(false)
const actionLoading = ref(false)
const rows = ref<any[]>([])
const selected = ref<number[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const filters = reactive({ q: '', kind: '', authorId: '', reason: '' })
const isPrimaryAdmin = ref(false)
const retentionDays = ref(0)
const notifyByPrimary = ref(false)
const savingPolicy = ref(false)
const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | null = null
const canTrash = computed(() => can('comments.trash'))
const canEdit = computed(() => can('comments.edit'))
const canChangeVisibility = computed(() => can('comments.change_visibility'))
const canRestore = computed(() => can('comments.restore'))
const canDeletePermanently = computed(() => can('comments.delete_permanently'))
const canSelectForBatch = computed(() => props.recycleBin ? (canRestore.value || canDeletePermanently.value) : canTrash.value)
const borderClass = computed(() => props.theme?.border || 'border-slate-200 dark:border-slate-700')
const subtleClass = computed(() => props.theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60')
const mutedClass = computed(() => props.theme?.mutedText || 'text-slate-500 dark:text-slate-400')
const endpoint = computed(() => props.recycleBin ? 'admin/comment-recycle-bin' : 'admin/comments')
const allSelected = computed(() => rows.value.length > 0 && rows.value.every(row => selected.value.includes(Number(row.id))))
const retentionOptions = [0, 7, 30, 90, 180, 365].map(value => ({ label: value ? `${value} 天` : '永不自动清理', value }))
const kindOptions = [{ label: '全部类型', value: '' }, { label: '评论', value: 'comment' }, { label: '回复', value: 'reply' }, { label: '留言', value: 'guestbook' }]
const reasonOptions = [{ label: '全部原因', value: '' }, { label: '本人删除', value: 'self' }, { label: '内容管理', value: 'moderation' }, { label: '内容所有者清理', value: 'owner_cleanup' }, { label: '上级内容牵连', value: 'ancestor' }]
const query = () => ({ page: page.value, pageSize, q: filters.q, kind: filters.kind, authorId: filters.authorId, reason: filters.reason })
const kindLabel = (value: string) => ({ comment: '评论', reply: '回复', guestbook: '留言', note: '笔记' } as Record<string, string>)[value] || '互动'
const kindColor = (value: string) => value === 'reply' ? 'blue' : value === 'guestbook' ? 'violet' : 'indigo'
const reasonLabel = (value: string) => ({ self: '本人删除', moderation: '内容管理员处理', owner_cleanup: '内容所有者清理', ancestor: '上级内容删除时一并处理', system: '系统定时清理' } as Record<string, string>)[value] || '未记录'
const visibilityLabel = (value: string) => ({ public: '公开', users: '成员', contacts: '联系人', private: '私密' } as Record<string, string>)[value] || value || '公开'
const contextText = (node: any) => node?.placeholder || String(node?.content || '（无正文）').replace(/\s+/g, ' ').slice(0, 100)
const formatDate = (value: any) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
const deadlineText = (deadline: any) => {
  void now.value
  if (!deadline?.auto_cleanup_enabled) return '当前不会自动清理'
  const seconds = Math.max(0, Math.ceil((new Date(deadline.scheduled_deletion_at).getTime() - Date.now()) / 1000))
  return seconds <= 0 ? '已到清理时间' : `距自动清理还有 ${Math.floor(seconds / 86400)}天${Math.floor((seconds % 86400) / 3600)}小时`
}
const load = async () => {
  loading.value = true
  try {
    const response = await getRequest<any>(endpoint.value, query(), { silent: true })
    rows.value = response?.code === 1 && Array.isArray(response.data?.items) ? response.data.items : []
    total.value = response?.code === 1 ? Number(response.data?.total || 0) : 0
    const visible = new Set(rows.value.map(row => Number(row.id)))
    selected.value = selected.value.filter(id => visible.has(Number(id)))
  } finally { loading.value = false }
}
const applyFilters = () => { page.value = 1; void load() }
const resetFilters = () => { Object.assign(filters, { q: '', kind: '', authorId: '', reason: '' }); applyFilters() }
const toggleAll = () => {
  selected.value = allSelected.value ? [] : rows.value.map(row => Number(row.id))
}
const run = async (request: () => Promise<any>) => {
  actionLoading.value = true
  try {
    const response = await request()
    const failed = Number(response?.data?.failed || 0)
    const succeeded = Number(response?.data?.succeeded || 0)
    const isBatch = response?.data && typeof response.data.succeeded === 'number'
    const description = isBatch ? `成功 ${succeeded} 项，失败 ${failed} 项` : response?.msg
    toast.add({ title: response?.code !== 1 ? '操作失败' : failed > 0 ? '部分操作未完成' : '操作完成', description, color: response?.code !== 1 ? 'red' : failed > 0 ? 'orange' : 'green' })
    if (response?.code === 1) await load()
    return response
  } finally { actionLoading.value = false }
}
const trash = (row: any) => run(() => deleteRequest(`messages/${row.message_id}/comments/${row.id}`, undefined, { silent: true }))
const editBody = (row: any) => {
  if (typeof window === 'undefined') return
  const content = window.prompt('编辑互动正文', String(row.content || ''))
  if (content === null || !content.trim()) return
  return run(() => putRequest(`messages/${row.message_id}/comments/${row.id}`, { content: content.trim() }, { silent: true }))
}
const changeVisibility = (row: any) => {
  if (typeof window === 'undefined') return
  const visibility = window.prompt('输入可见范围：public / users / contacts / private', String(row.stored_visibility || 'public'))
  if (!visibility) return
  return run(() => putRequest(`messages/${row.message_id}/comments/${row.id}`, { content: row.content, visibility: visibility.trim() }, { silent: true }))
}
const restore = (row: any) => run(() => postRequest(`admin/comment-recycle-bin/${row.id}/restore`, {}, { silent: true }))
const removePermanently = (row: any) => {
  if (typeof window !== 'undefined' && !window.confirm(`永久删除${kindLabel(row.kind)} #${row.id}？若仍有后代，将保留最小墓碑结构。`)) return
  return run(() => deleteRequest(`admin/comment-recycle-bin/${row.id}`, undefined, { silent: true }))
}
const batchTrash = () => {
  if (typeof window !== 'undefined' && !window.confirm(`将所选 ${selected.value.length} 条互动移入回收站？其后代会按生命周期规则一并处理。`)) return
  return runSelectedBatch('admin/comments/batch-trash')
}
const runSelectedBatch = async (endpoint: string) => {
  const response = await run(() => postRequest(endpoint, { ids: selected.value }, { silent: true }))
  if (response?.code === 1) selected.value = []
  return response
}
const batchRestore = () => runSelectedBatch('admin/comment-recycle-bin/batch-restore')
const batchPermanentDelete = () => {
  if (typeof window !== 'undefined' && !window.confirm(`永久删除所选 ${selected.value.length} 条互动？仍有后代的互动会保留最小墓碑。`)) return
  return runSelectedBatch('admin/comment-recycle-bin/batch-permanent-delete')
}
const loadPolicy = async () => {
  const auth = await getRequest<any>('admin/authorization/me', undefined, { silent: true })
  isPrimaryAdmin.value = auth?.code === 1 && auth?.data?.is_primary_admin === true
  if (!isPrimaryAdmin.value) return
  const response = await getRequest<any>('settings', undefined, { silent: true })
  retentionDays.value = Number(response?.data?.commentRecycleBinRetentionDays || 0)
  notifyByPrimary.value = response?.data?.notifyCommentDeletionByPrimary === true
}
const savePolicy = async () => {
  if (!isPrimaryAdmin.value) return
  savingPolicy.value = true
  try {
    const response = await putRequest<any>('settings', { commentRecycleBinRetentionDays: Number(retentionDays.value), notifyCommentDeletionByPrimary: notifyByPrimary.value }, { silent: true })
    toast.add({ title: response?.code === 1 ? '互动回收站策略已保存' : '策略保存失败', description: response?.msg, color: response?.code === 1 ? 'green' : 'red' })
  } finally { savingPolicy.value = false }
}
onMounted(() => { clock = setInterval(() => { now.value = Date.now() }, 60000); void load(); if (props.recycleBin) void loadPolicy() })
watch(() => props.recycleBin, () => { page.value = 1; selected.value = []; void load(); if (props.recycleBin) void loadPolicy() })
onUnmounted(() => { if (clock) clearInterval(clock) })
</script>

<style scoped>
.comment-manager { min-width: 0; overflow: hidden; container-type: inline-size; }
.manager-body { display: flex; flex-direction: column; gap: var(--admin-gap, 12px); padding: 0 var(--admin-space, 16px) var(--admin-space, 16px); }
.comment-manager .policy-card, .comment-manager .filter-card { border: 1px solid var(--admin-line); border-radius: var(--admin-radius, 8px); padding: var(--admin-space, 16px); }
.policy-card { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--admin-gap, 12px); margin-top: 16px; }
.policy-controls, .notify-toggle { display: flex; align-items: center; gap: var(--admin-gap, 12px); }
.policy-controls { flex-wrap: wrap; }
.notify-toggle { font-size: 12px; }
.filter-card { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 160px), 1fr)); gap: var(--admin-gap, 12px); align-items: end; }
.filter-field { display: flex; min-width: 0; flex-direction: column; gap: 6px; font-size: 12px; color: var(--admin-muted); }
.filter-actions { display: flex; flex-wrap: wrap; gap: var(--admin-gap, 12px); }
.interaction-list { border: 1px solid var(--admin-line); border-radius: var(--admin-radius, 8px); overflow: hidden; }
.interaction-list > .interaction-card { border-radius: 0; display: grid; grid-template-columns: minmax(0, 1fr) minmax(180px, 26%); gap: var(--admin-gap, 12px); padding: var(--admin-space, 16px); }
.interaction-card + .interaction-card { border-top: 1px solid var(--admin-line); }
.interaction-head, .interaction-identity, .interaction-actions, .interaction-meta, .pagination, .pagination > div { display: flex; align-items: center; gap: var(--admin-gap, 12px); }
.interaction-head { grid-column: 1 / -1; justify-content: space-between; flex-wrap: wrap; }
.interaction-identity { min-width: 0; flex-wrap: wrap; font-size: 13px; overflow-wrap: anywhere; }
.interaction-identity input { accent-color: var(--admin-accent); }
.interaction-content { grid-column: 1; margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 14px; line-height: 1.65; }
.thread-trail { grid-column: 1; padding: var(--admin-space, 16px); border-radius: var(--admin-radius, 8px); }
.trail-node { display: grid; grid-template-columns: 92px minmax(0, 1fr); gap: 12px; font-size: 12px; line-height: 1.5; }
.trail-node + .trail-node { margin-top: 6px; }
.trail-node.is-child { position: relative; padding-left: 14px; }
.trail-node.is-child::before { content: '↳'; position: absolute; left: 0; color: var(--admin-muted); }
.trail-node span:last-child { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.interaction-meta { grid-column: 1; flex-wrap: wrap; font-size: 12px; color: var(--admin-muted); }
.purged-label { color: #dc2626; }
.deadline { color: #d97706; font-weight: 650; }
.interaction-actions { grid-column: 2; grid-row: 2 / 5; align-content: start; align-items: flex-start; justify-content: flex-end; flex-wrap: wrap; }
.empty { display: flex; min-height: 160px; align-items: center; justify-content: center; gap: 12px; color: var(--admin-muted); }
.pagination { justify-content: space-between; flex-wrap: wrap; font-size: 12px; }
.pagination > div span { min-width: 62px; text-align: center; }
@container (max-width: 760px) {
  .interaction-list > .interaction-card { grid-template-columns: minmax(0, 1fr); }
  .interaction-actions { grid-column: 1; grid-row: auto; justify-content: flex-start; }
}
@container (max-width: 420px) {
  .trail-node { grid-template-columns: minmax(0, 1fr); gap: 4px; }
  .trail-node span:last-child { white-space: normal; overflow-wrap: anywhere; }
}
</style>
