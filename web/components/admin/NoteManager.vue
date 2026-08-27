<template>
  <section class="note-manager" :class="theme?.text || ''">
    <AdminModuleHeader
      :title="recycleBin ? '笔记回收站' : '笔记管理'"
      :description="recycleBin ? '集中恢复或永久删除已移入回收站的笔记，普通笔记列表不会显示这些内容。' : '筛选、检查并维护全站笔记；可勾选当前页笔记，或主动选择当前筛选结果后执行批量操作。'"
      :icon="recycleBin ? 'i-heroicons-trash' : 'i-heroicons-document-text'"
      :badge="loading ? '读取中' : `${total} 条`"
      :accent="recycleBin ? 'warning' : 'primary'"
      :theme="theme"
    >
      <template #actions>
        <UButton size="sm" color="gray" variant="soft" icon="i-heroicons-arrow-path" :loading="loading" @click="load">刷新</UButton>
      </template>
    </AdminModuleHeader>

    <div class="note-manager-body">
      <div v-if="recycleBin && isPrimaryAdmin" class="note-policy-card" :class="[theme?.border || 'border-slate-200 dark:border-slate-700', theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60']">
        <div class="note-policy-copy">
          <span class="note-policy-icon"><UIcon name="i-heroicons-clock" class="h-4 w-4" /></span>
          <div>
            <div class="text-sm font-medium">自动清理策略</div>
            <p class="mt-0.5 text-xs leading-5" :class="theme?.mutedText || 'text-slate-500'">从移入回收站的时间开始计算；缩短期限会影响现有回收站笔记。</p>
          </div>
        </div>
        <div class="note-policy-controls">
          <USelect v-model="retentionDays" class="w-full sm:w-48" :options="retentionOptions" :disabled="retentionLoading" aria-label="自动清理保留期限" @change="saveRetention" />
          <label class="note-notify-toggle"><span>站长删除时通知作者</span><UToggle v-model="notifyByPrimary" :disabled="retentionLoading" @change="saveRetention" /></label>
        </div>
      </div>

      <section class="note-filter-card" :class="theme?.border || 'border-slate-200 dark:border-slate-700'" aria-labelledby="note-filter-title">
        <div class="note-section-heading">
          <div>
            <h3 id="note-filter-title" class="text-sm font-semibold">筛选与排序</h3>
            <p class="mt-1 text-xs" :class="theme?.mutedText || 'text-slate-500'">组合条件定位笔记，日期和选择项变更后会立即刷新。</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <UButton size="xs" color="primary" variant="soft" icon="i-heroicons-funnel" :loading="loading" @click="applyFilters">应用筛选</UButton>
            <UButton size="xs" color="gray" variant="ghost" :disabled="loading" @click="resetFilters">清空条件</UButton>
          </div>
        </div>
        <div class="note-filter-grid">
          <label class="note-filter-field"><span>正文关键词</span><UInput v-model="filters.keyword" placeholder="输入正文内容" @keyup.enter="applyFilters" /></label>
          <label class="note-filter-field"><span>笔记 ID</span><UInput v-model="filters.id" type="number" placeholder="精确匹配" @keyup.enter="applyFilters" /></label>
          <label class="note-filter-field"><span>作者 ID</span><UInput v-model="filters.authorId" type="number" placeholder="精确匹配" @keyup.enter="applyFilters" /></label>
          <label class="note-filter-field"><span>作者用户名</span><UInput v-model="filters.username" placeholder="输入用户名" @keyup.enter="applyFilters" /></label>
          <label class="note-filter-field"><span>标签</span><UInput v-model="filters.tag" placeholder="不含 #" @keyup.enter="applyFilters" /></label>
          <label class="note-filter-field"><span>可见性</span><USelect v-model="filters.visibility" :options="visibilityOptions" @change="applyFilters" /></label>
          <label class="note-filter-field"><span>创建日期（从）</span><UInput v-model="filters.createdFrom" type="date" @change="applyFilters" /></label>
          <label class="note-filter-field"><span>创建日期（至）</span><UInput v-model="filters.createdTo" type="date" @change="applyFilters" /></label>
          <label class="note-filter-field"><span>全站置顶</span><USelect v-model="filters.pinned" :options="booleanOptions('全站置顶')" @change="applyFilters" /></label>
          <label class="note-filter-field"><span>附件</span><USelect v-model="filters.hasAttachment" :options="booleanOptions('附件')" @change="applyFilters" /></label>
          <label class="note-filter-field"><span>排序方式</span><USelect v-model="filters.sort" :options="sortOptions" @change="applyFilters" /></label>
        </div>
      </section>

      <div class="note-selection-bar" :class="[theme?.border || 'border-slate-200 dark:border-slate-700', theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60', recycleBin ? 'is-recycle-bin' : '']">
        <div class="note-selection-status">
          <span class="note-selection-count">{{ selected.length }}</span>
          <div>
            <div class="text-sm font-medium">{{ selected.length ? `已选择 ${selected.length} 条` : '尚未选择笔记' }}</div>
            <div class="text-xs" :class="theme?.mutedText || 'text-slate-500'">当前筛选共 {{ total }} 条；筛选本身不会触发批量操作。</div>
          </div>
        </div>
        <div class="note-selection-actions">
          <UButton v-if="selected.length" size="xs" color="gray" variant="ghost" @click="clearSelection">清除选择</UButton>
          <UButton v-if="selected.length && !recycleBin && canTrash" size="xs" color="orange" :loading="actionLoading" @click="batchTrash">移入回收站</UButton>
          <UButton v-if="total && !recycleBin && canTrash" size="xs" color="orange" variant="soft" :loading="actionLoading" @click="batchFiltered">全选当前筛选结果并移入回收站</UButton>
          <UButton v-if="selected.length && recycleBin && canRestore" size="xs" color="green" :loading="actionLoading" @click="batchRestore">恢复所选</UButton>
          <UButton v-if="selected.length && recycleBin && canPermanentlyDelete" size="xs" color="red" :loading="actionLoading" @click="batchPermanentDelete">永久删除所选</UButton>
          <UButton v-if="total && recycleBin && canRestore" size="xs" color="green" variant="soft" :loading="actionLoading" @click="batchFilteredRestore">全选当前筛选结果并恢复</UButton>
          <UButton v-if="total && recycleBin && canPermanentlyDelete" size="xs" color="red" variant="soft" :loading="actionLoading" @click="batchFilteredPermanentDelete">全选当前筛选结果并永久删除</UButton>
        </div>
      </div>

      <div class="note-table-shell" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
        <table class="note-table min-w-[1160px] w-full table-fixed text-sm">
          <colgroup>
            <col class="w-12" />
            <col />
            <col class="w-24" />
            <col class="w-28" />
            <col class="w-44" />
            <col class="w-[30rem]" />
          </colgroup>
          <thead :class="theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60'">
          <tr>
            <th class="w-12 px-4 py-3 text-left"><input type="checkbox" :checked="allSelected" aria-label="选择当前页全部笔记" @change="toggleAll" /></th>
            <th class="px-3 py-3 text-left">笔记</th>
            <th class="px-3 py-3 text-left">作者</th>
            <th class="px-3 py-3 text-left">可见性</th>
            <th class="px-3 py-3 text-left">{{ recycleBin ? '删除时间' : '创建时间' }}</th>
            <th class="px-4 py-3 text-right">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading"><td colspan="6" class="px-3 py-12 text-center" :class="theme?.mutedText || 'text-slate-500'"><UIcon name="i-heroicons-arrow-path" class="mr-2 inline h-4 w-4 animate-spin" />正在读取笔记…</td></tr>
          <tr v-else-if="!rows.length"><td colspan="6" class="px-3 py-12 text-center" :class="theme?.mutedText || 'text-slate-500'"><UIcon name="i-heroicons-inbox" class="mx-auto mb-2 h-6 w-6 opacity-60" /><span>当前筛选下暂无笔记</span></td></tr>
          <template v-for="row in rows" v-else :key="row.id">
            <tr class="border-t" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
              <td class="px-4 py-3 align-top"><input v-model="selected" type="checkbox" :value="row.id" :aria-label="`选择笔记 ${row.id}`" /></td>
              <td class="note-cell px-3 py-3 align-top">
                <button class="note-title-button" @click="toggleDetail(row.id)">
                  <span class="note-id">#{{ row.id }}</span>
                  <span class="note-content font-medium">{{ oneLine(row.content) || '（仅附件）' }}</span>
                </button>
                <UBadge v-if="row.is_guestbook" class="mt-1.5" color="indigo" size="xs" variant="soft">规范留言板 · 不可作为普通笔记删除</UBadge>
              </td>
              <td class="px-3 py-3 align-top"><span class="font-medium">{{ row.username || row.user_id }}</span><span v-if="row.username" class="mt-0.5 block text-xs" :class="theme?.mutedText || 'text-slate-500'">ID {{ row.user_id }}</span></td>
              <td class="px-3 py-3 align-top"><UBadge color="gray" size="xs" variant="soft">{{ visibilityLabel(row.visibility) }}</UBadge></td>
              <td class="px-3 py-3 align-top whitespace-nowrap" :class="theme?.mutedText || 'text-slate-500'">{{ formatDate(recycleBin ? row.deleted_at : row.created_at) }}</td>
              <td class="px-4 py-3 text-right align-top">
                <div class="note-row-actions">
                  <UButton size="xs" variant="ghost" color="gray" @click="toggleDetail(row.id)">{{ detailId === row.id ? '收起' : '详情' }}</UButton>
                  <template v-if="!recycleBin && !row.is_guestbook">
                    <UButton v-if="canEdit" size="xs" variant="ghost" color="primary" :loading="actionLoading" @click="editOne(row)">编辑</UButton>
                    <UButton v-if="canChangeVisibility" size="xs" variant="ghost" color="primary" :loading="actionLoading" @click="changeVisibilityOne(row)">可见性</UButton>
                    <UButton v-if="canChangePublishTime" size="xs" variant="ghost" color="primary" :loading="actionLoading" @click="changePublishTimeOne(row)">发布时间</UButton>
                    <UButton v-if="canPinGlobal" size="xs" variant="ghost" color="primary" :loading="actionLoading" @click="toggleGlobalPinOne(row)">{{ row.pinned ? '取消全站置顶' : '全站置顶' }}</UButton>
                  </template>
                  <UButton v-if="!recycleBin && !row.is_guestbook && canTrash" size="xs" variant="ghost" color="orange" :loading="actionLoading" @click="trashOne(row.id)">移入回收站</UButton>
                  <UButton v-if="recycleBin && !row.is_guestbook && canRestore" size="xs" variant="ghost" color="green" :loading="actionLoading" @click="restoreOne(row.id)">恢复</UButton>
                  <UButton v-if="recycleBin && !row.is_guestbook && canPermanentlyDelete" size="xs" variant="ghost" color="red" :loading="actionLoading" @click="permanentDeleteOne(row.id)">永久删除</UButton>
                </div>
              </td>
            </tr>
            <tr v-if="detailId === row.id" class="border-t" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
              <td colspan="6" class="px-4 py-4" :class="theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60'">
                <div class="note-detail-card" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
                  <div class="mb-2 text-xs font-semibold uppercase tracking-wide" :class="theme?.mutedText || 'text-slate-500'">笔记正文</div>
                  <pre class="max-h-64 overflow-auto whitespace-pre-wrap text-xs leading-5" :class="theme?.text || 'text-slate-700 dark:text-slate-200'">{{ row.content || '（无正文）' }}</pre>
                  <div class="mt-3 text-xs" :class="theme?.mutedText || 'text-slate-500'">点赞 {{ row.like_count || 0 }} · 全站置顶 {{ row.pinned ? '是' : '否' }} · 个人置顶 {{ row.personal_pinned ? '是' : '否' }}</div>
                  <div v-if="recycleBin" class="mt-1 text-xs" :class="theme?.mutedText || 'text-slate-500'">删除原因：{{ row.deleted_reason || '未记录' }}</div>
                </div>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
      </div>

      <div class="note-pagination" :class="theme?.mutedText || 'text-slate-500'">
        <span>共 {{ total }} 条 · 每页 {{ pageSize }} 条</span>
        <div class="flex items-center gap-2">
          <UButton size="xs" variant="soft" color="gray" :disabled="page <= 1 || loading" @click="page--; load()">上一页</UButton>
          <span class="min-w-16 text-center">第 {{ page }} 页</span>
          <UButton size="xs" variant="soft" color="gray" :disabled="page * pageSize >= total || loading" @click="page++; load()">下一页</UButton>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { runConfirmedFilteredLifecycle } from '~/utils/note-lifecycle-confirmation'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { deleteRequest, getRequest, postRequest, putRequest } from '~/utils/api'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { createNoteManagerPermissionHandler } from '~/utils/note-manager-permission'

const props = defineProps<{ recycleBin?: boolean; theme?: any }>()
const toast = useToast()
const { can, refreshCapabilities } = useAdminCapabilities()
const canTrash = computed(() => can('notes.trash'))
const canRestore = computed(() => can('notes.restore'))
const canPermanentlyDelete = computed(() => can('notes.delete_permanently'))
const canEdit = computed(() => can('notes.edit'))
const canChangeVisibility = computed(() => can('notes.change_visibility'))
const canChangePublishTime = computed(() => can('notes.change_publish_time'))
const canPinGlobal = computed(() => can('notes.pin_global'))
const isPrimaryAdmin = ref(false)
const retentionDays = ref(0)
const notifyByPrimary = ref(false)
const retentionLoading = ref(false)
const retentionOptions = [
  { label: '永不自动清理', value: 0 },
  { label: '7 天', value: 7 },
  { label: '30 天', value: 30 },
  { label: '90 天', value: 90 },
  { label: '180 天', value: 180 },
  { label: '365 天', value: 365 }
]
const loading = ref(false)
const actionLoading = ref(false)
const rows = ref<any[]>([])
const selected = ref<number[]>([])
const detailId = ref<number | null>(null)
const page = ref(1)
const pageSize = 20
const total = ref(0)
const filters = reactive({ keyword: '', id: '', authorId: '', username: '', tag: '', visibility: '', createdFrom: '', createdTo: '', pinned: '', hasAttachment: '', sort: '' })
const visibilityOptions = [
  { label: '全部可见性', value: '' },
  { label: '公开', value: 'public' },
  { label: '登录用户', value: 'users' },
  { label: '联系人', value: 'contacts' },
  { label: '私密', value: 'private' }
]
const booleanOptions = (label: string) => [{ label: `${label}：全部`, value: '' }, { label: `${label}：是`, value: 'true' }, { label: `${label}：否`, value: 'false' }]
const sortOptions = computed(() => props.recycleBin
  ? [{ label: '默认排序', value: '' }, { label: '最新删除', value: 'deleted_desc' }, { label: '最早删除', value: 'deleted_asc' }, { label: '最新创建', value: 'created_desc' }, { label: '最早创建', value: 'created_asc' }]
  : [{ label: '默认排序', value: '' }, { label: '最新创建', value: 'created_desc' }, { label: '最早创建', value: 'created_asc' }, { label: '置顶优先', value: 'pinned' }])
const endpoint = computed(() => props.recycleBin ? 'admin/recycle-bin' : 'admin/notes')
const allSelected = computed(() => rows.value.length > 0 && rows.value.every((row) => selected.value.includes(row.id)))

const formatDate = (value: any) => {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? String(value).replace('T', ' ').replace('Z', '') : date.toLocaleString('zh-CN', { hour12: false })
}
const oneLine = (value: any) => String(value || '').replace(/\s+/g, ' ').trim().slice(0, 120)
const visibilityLabel = (value: string) => visibilityOptions.find((item) => item.value === value)?.label || value || '公开'
const query = () => ({ page: page.value, pageSize, keyword: filters.keyword, id: filters.id, authorId: filters.authorId, username: filters.username, tag: filters.tag, visibility: filters.visibility, createdFrom: filters.createdFrom, createdTo: filters.createdTo, pinned: filters.pinned, hasAttachment: filters.hasAttachment, sort: filters.sort })
const applyFilters = () => {
  page.value = 1
  clearSelection()
  void load()
}
const resetFilters = () => {
  Object.assign(filters, { keyword: '', id: '', authorId: '', username: '', tag: '', visibility: '', createdFrom: '', createdTo: '', pinned: '', hasAttachment: '', sort: '' })
  applyFilters()
}

const permissionChanged = createNoteManagerPermissionHandler({
  clearState: () => {
    rows.value = []
    total.value = 0
    selected.value = []
    detailId.value = null
  },
  refreshCapabilities,
  notify: () => toast.add({ title: '权限已变化', description: '当前权限已变化，请刷新页面', color: 'orange' })
})
let resetScheduled = false
const resetPermissionGuard = () => {
  if (resetScheduled) return
  resetScheduled = true
  queueMicrotask(() => {
    resetScheduled = false
    permissionChanged.reset()
  })
}
const load = async () => {
  loading.value = true
  try {
    const response = await getRequest<any>(endpoint.value, query(), { silent: true })
    if (response?.status === 403) {
      await permissionChanged()
    } else if (response?.code === 1) {
      rows.value = Array.isArray(response.data?.items) ? response.data.items : []
      total.value = Number(response.data?.total || 0)
      selected.value = selected.value.filter((id) => rows.value.some((row) => row.id === id))
      detailId.value = null
    } else {
      rows.value = []
      total.value = 0
    }
  } finally {
    loading.value = false
  }
}
onMounted(() => {
  window.addEventListener('admin-capabilities-invalidated', resetPermissionGuard)
  void load()
  if (props.recycleBin) void loadRetentionSettings()
})
onBeforeUnmount(() => window.removeEventListener('admin-capabilities-invalidated', resetPermissionGuard))
const clearSelection = () => { selected.value = [] }
const toggleAll = () => {
  selected.value = allSelected.value ? [] : rows.value.map((row) => row.id)
}
const toggleDetail = (id: number) => { detailId.value = detailId.value === id ? null : id }
const confirmPermanent = (count: number) => typeof window === 'undefined' || window.confirm(`将永久删除 ${count} 条已在回收站的笔记，且不可恢复。是否继续？`)
const runAction = async (request: () => Promise<any>) => {
  actionLoading.value = true
  try {
    const response = await request()
    if (response?.status === 403) {
      await permissionChanged()
    } else if (response?.code === 1) {
      const result = response.data || {}
      const succeeded = Number(result.succeeded || 0)
      const failed = Number(result.failed || 0)
      const failures = Array.isArray(result.items) ? result.items.filter((item: any) => !item?.ok) : []
      const detail = failed > 0
        ? `成功 ${succeeded} 项，失败 ${failed} 项${failures.length ? `：${failures.map((item: any) => item.reason || '操作失败').join('；')}` : ''}`
        : (response.msg || '操作完成')
      toast.add({ title: failed > 0 ? '部分操作未完成' : '操作完成', description: detail, color: failed > 0 ? 'orange' : 'green' })
      clearSelection()
      await load()
    } else {
      toast.add({ title: '操作失败', description: response?.msg || '请缩小筛选范围后重试；如问题持续，请稍后再试', color: 'red' })
    }
  } catch (error: any) {
    toast.add({ title: '操作失败', description: error?.data?.msg || error?.message || '请缩小筛选范围后重试；如问题持续，请稍后再试', color: 'red' })
  } finally {
    actionLoading.value = false
  }
}
const trashOne = (id: number) => runAction(() => postRequest(`admin/notes/${id}/trash`, { reason: 'admin request' }, { silent: true }))
const restoreOne = (id: number) => runAction(() => postRequest(`admin/recycle-bin/${id}/restore`, {}, { silent: true }))
const permanentDeleteOne = (id: number) => {
  if (!confirmPermanent(1)) return
  return runAction(() => deleteRequest(`admin/recycle-bin/${id}`, undefined, { silent: true }))
}
const editOne = (row: any) => {
  if (typeof window === 'undefined' || !canEdit.value) return
  const content = window.prompt('编辑笔记正文（留空表示空正文）', String(row.content || ''))
  if (content === null) return
  return runAction(() => putRequest(`admin/notes/${row.id}`, { content }, { silent: true }))
}
const changeVisibilityOne = (row: any) => {
  if (typeof window === 'undefined' || !canChangeVisibility.value) return
  const visibility = window.prompt('输入可见性：public / users / contacts / private', String(row.visibility || 'public'))
  if (!visibility) return
  return runAction(() => putRequest(`admin/notes/${row.id}/visibility`, { visibility, private: visibility === 'private' }, { silent: true }))
}
const changePublishTimeOne = (row: any) => {
  if (typeof window === 'undefined' || !canChangePublishTime.value) return
  const createdAt = window.prompt('输入发布时间（ISO 8601），取消则不修改', String(row.created_at || ''))
  if (!createdAt) return
  return runAction(() => putRequest(`admin/notes/${row.id}/publish-time`, { created_at: createdAt }, { silent: true }))
}
const toggleGlobalPinOne = (row: any) => {
  if (!canPinGlobal.value) return
  return runAction(() => putRequest(`admin/notes/${row.id}/pin/global`, { pinned: !row.pinned }, { silent: true }))
}
const batchTrash = () => runAction(() => postRequest('admin/notes/batch-trash', { ids: selected.value, reason: 'admin batch request' }, { silent: true }))
const batchRestore = () => runAction(() => postRequest('admin/recycle-bin/batch-restore', { ids: selected.value }, { silent: true }))
const batchPermanentDelete = () => {
  if (!confirmPermanent(selected.value.length)) return
  return runAction(() => postRequest('admin/recycle-bin/batch-permanent-delete', { ids: selected.value, reason: 'admin batch request' }, { silent: true }))
}
const confirmInBrowser = (message: string) => typeof window === 'undefined' || window.confirm(message)
const batchFiltered = () => runConfirmedFilteredLifecycle('trash', total.value, confirmInBrowser, () => runAction(() => postRequest('admin/notes/batch-trash-filtered', { filter: query(), reason: 'filtered batch request' }, { silent: true })))
const batchFilteredRestore = () => runConfirmedFilteredLifecycle('restore', total.value, confirmInBrowser, () => runAction(() => postRequest('admin/recycle-bin/batch-restore-filtered', { filter: query() }, { silent: true })))
const batchFilteredPermanentDelete = () => runConfirmedFilteredLifecycle('permanent-delete', total.value, confirmInBrowser, () => runAction(() => postRequest('admin/recycle-bin/batch-permanent-delete-filtered', { filter: query(), reason: 'filtered batch request' }, { silent: true })))
const loadRetentionSettings = async () => {
  const auth = await getRequest<any>('admin/authorization/me', undefined, { silent: true })
  isPrimaryAdmin.value = auth?.code === 1 && auth?.data?.is_primary_admin === true
  if (!isPrimaryAdmin.value) return
  const settings = await getRequest<any>('settings', undefined, { silent: true })
  if (settings?.code === 1) {
    retentionDays.value = Number(settings.data?.recycleBinRetentionDays || 0)
    notifyByPrimary.value = settings.data?.notifyNoteDeletionByPrimary === true
  }
}
const saveRetention = async () => {
  if (!isPrimaryAdmin.value) return
  const next = Number(retentionDays.value || 0)
  const previous = Number((await getRequest<any>('settings', undefined, { silent: true }))?.data?.recycleBinRetentionDays || 0)
  if (next < previous && typeof window !== 'undefined' && !window.confirm('缩短保留期限会影响已有回收站笔记，已到期内容将在下次任务中永久删除。是否继续？')) {
    retentionDays.value = previous
    return
  }
  retentionLoading.value = true
  try {
    const response = await putRequest<any>('settings', { recycleBinRetentionDays: next, notifyNoteDeletionByPrimary: notifyByPrimary.value }, { silent: true })
    if (response?.code === 1) toast.add({ title: '自动清理设置已保存', color: 'green' })
    else toast.add({ title: '自动清理设置保存失败', description: response?.msg || '请稍后重试', color: 'red' })
  } finally {
    retentionLoading.value = false
  }
}
watch(() => props.recycleBin, () => { page.value = 1; clearSelection(); load(); if (props.recycleBin) void loadRetentionSettings() })
</script>

<style scoped>
.note-manager {
  min-width: 0;
  overflow: hidden;
}

.note-policy-copy,
.note-selection-status {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 11px;
}

.note-policy-icon {
  display: inline-flex;
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: rgb(217, 119, 6);
  background: rgba(245, 158, 11, 0.14);
}

.note-manager-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 0 16px 16px;
}

.note-policy-card,
.note-filter-card,
.note-selection-bar,
.note-detail-card {
  border-width: 1px;
  border-style: solid;
}

.note-policy-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-top: 16px;
  padding: 12px;
  border-radius: 10px;
}

.note-policy-controls,
.note-notify-toggle {
  display: flex;
  align-items: center;
  gap: 12px;
}

.note-notify-toggle {
  font-size: 12px;
  white-space: nowrap;
}

.note-filter-card {
  padding: 14px;
  border-radius: 10px;
}

.note-section-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.note-filter-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.note-filter-field {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 5px;
}

.note-filter-field > span {
  font-size: 11px;
  font-weight: 600;
  line-height: 1.3;
  opacity: 0.72;
}

.note-selection-bar {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  overflow: hidden;
  padding: 11px 12px 11px 15px;
  border-radius: 10px;
}

.note-selection-bar::before {
  position: absolute;
  inset: 0 auto 0 0;
  width: 3px;
  content: '';
  background: rgb(99, 102, 241);
}

.note-selection-bar.is-recycle-bin::before {
  background: rgb(245, 158, 11);
}

.note-selection-count {
  display: inline-flex;
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 700;
  color: rgb(79, 70, 229);
  background: rgba(99, 102, 241, 0.12);
}

.is-recycle-bin .note-selection-count {
  color: rgb(180, 83, 9);
  background: rgba(245, 158, 11, 0.14);
}

.note-selection-actions,
.note-row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 4px;
}

.note-row-actions {
  flex-wrap: nowrap;
  white-space: nowrap;
}

.note-table-shell {
  overflow-x: auto;
  border-width: 1px;
  border-style: solid;
  border-radius: 10px;
}

.note-table-shell tbody tr {
  transition: background-color 0.15s ease;
}

.note-table-shell tbody tr:hover:not(:has(.note-detail-card)) {
  background: rgba(148, 163, 184, 0.06);
}

.note-cell {
  min-width: 0;
  overflow: hidden;
}

.note-title-button {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: baseline;
  gap: 7px;
  overflow: hidden;
  text-align: left;
}

.note-content {
  min-width: 0;
  overflow-wrap: anywhere;
  word-break: break-word;
}

.note-title-button:hover .font-medium,
.note-title-button:focus-visible .font-medium {
  text-decoration: underline;
  text-underline-offset: 3px;
}

.note-title-button:focus-visible {
  outline: 2px solid rgba(99, 102, 241, 0.65);
  outline-offset: 3px;
  border-radius: 3px;
}

.note-id {
  flex: 0 0 auto;
  font-size: 11px;
  font-weight: 700;
  opacity: 0.6;
}

.note-detail-card {
  padding: 12px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.32);
}

:global(.dark) .note-detail-card {
  background: rgba(15, 23, 42, 0.18);
}

.note-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
}

@media (max-width: 1100px) {
  .note-filter-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 760px) {
  .note-policy-card,
  .note-section-heading,
  .note-selection-bar,
  .note-pagination {
    align-items: stretch;
    flex-direction: column;
  }

  .note-policy-card > :last-child {
    align-self: stretch;
  }

  .note-filter-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .note-selection-actions {
    justify-content: flex-start;
  }
}

@media (max-width: 520px) {
  .note-manager-body {
    padding-right: 14px;
    padding-left: 14px;
  }

  .note-filter-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
