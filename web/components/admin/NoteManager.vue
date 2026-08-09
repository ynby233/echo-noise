<template>
  <div class="space-y-4" :class="theme?.text || ''">
    <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
      <div>
        <h2 class="text-lg font-semibold">{{ recycleBin ? '笔记回收站' : '笔记管理' }}</h2>
        <p class="text-xs mt-1" :class="theme?.mutedText || 'text-slate-500'">
          {{ recycleBin ? '仅在回收站上下文恢复或永久删除；普通列表不会显示回收站笔记。' : '先明确勾选笔记，再移入回收站。不会因为筛选条件自动操作全部结果。' }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <UButton size="xs" color="gray" variant="soft" :loading="loading" @click="load">刷新</UButton>
        <UButton v-if="selected.length" size="xs" color="primary" variant="soft" @click="clearSelection">清除选择</UButton>
        <UButton v-if="selected.length && !recycleBin && canTrash" size="xs" color="orange" :loading="actionLoading" @click="batchTrash">移入回收站</UButton>
        <UButton v-if="selected.length && recycleBin && canRestore" size="xs" color="green" :loading="actionLoading" @click="batchRestore">恢复所选</UButton>
        <UButton v-if="selected.length && recycleBin && canPermanentlyDelete" size="xs" color="red" :loading="actionLoading" @click="batchPermanentDelete">永久删除所选</UButton>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-2 md:grid-cols-4">
      <UInput v-model="filters.keyword" placeholder="正文关键词" @keyup.enter="load" />
      <UInput v-model="filters.id" type="number" placeholder="精确笔记 ID" @keyup.enter="load" />
      <UInput v-model="filters.authorId" type="number" placeholder="作者 ID" @keyup.enter="load" />
      <USelect v-model="filters.visibility" :options="visibilityOptions" @change="load" />
    </div>

    <div class="overflow-x-auto rounded-lg border" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
      <table class="min-w-full text-sm">
        <thead :class="theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60'">
          <tr>
            <th class="w-10 px-3 py-2 text-left"><input type="checkbox" :checked="allSelected" @change="toggleAll" /></th>
            <th class="px-3 py-2 text-left">笔记</th>
            <th class="px-3 py-2 text-left">作者</th>
            <th class="px-3 py-2 text-left">可见性</th>
            <th class="px-3 py-2 text-left">{{ recycleBin ? '删除时间' : '创建时间' }}</th>
            <th class="px-3 py-2 text-right">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading"><td colspan="6" class="px-3 py-8 text-center" :class="theme?.mutedText || 'text-slate-500'">加载中…</td></tr>
          <tr v-else-if="!rows.length"><td colspan="6" class="px-3 py-8 text-center" :class="theme?.mutedText || 'text-slate-500'">暂无笔记</td></tr>
          <template v-for="row in rows" v-else :key="row.id">
            <tr class="border-t" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
              <td class="px-3 py-2 align-top"><input v-model="selected" type="checkbox" :value="row.id" /></td>
              <td class="max-w-[28rem] px-3 py-2 align-top">
                <button class="text-left hover:underline" @click="toggleDetail(row.id)">
                  <span class="font-medium">#{{ row.id }} {{ oneLine(row.content) || '（仅附件）' }}</span>
                </button>
                <div v-if="row.is_guestbook" class="mt-1 text-xs text-indigo-500">规范留言板（不可作为普通笔记删除）</div>
              </td>
              <td class="px-3 py-2 align-top">{{ row.username || row.user_id }}</td>
              <td class="px-3 py-2 align-top">{{ visibilityLabel(row.visibility) }}</td>
              <td class="px-3 py-2 align-top whitespace-nowrap">{{ formatDate(recycleBin ? row.deleted_at : row.created_at) }}</td>
              <td class="px-3 py-2 text-right align-top whitespace-nowrap">
                <UButton size="xs" variant="ghost" color="gray" @click="toggleDetail(row.id)">{{ detailId === row.id ? '收起' : '详情' }}</UButton>
                <UButton v-if="!recycleBin && !row.is_guestbook && canTrash" size="xs" variant="ghost" color="orange" :loading="actionLoading" @click="trashOne(row.id)">移入回收站</UButton>
                <UButton v-if="recycleBin && !row.is_guestbook && canRestore" size="xs" variant="ghost" color="green" :loading="actionLoading" @click="restoreOne(row.id)">恢复</UButton>
                <UButton v-if="recycleBin && !row.is_guestbook && canPermanentlyDelete" size="xs" variant="ghost" color="red" :loading="actionLoading" @click="permanentDeleteOne(row.id)">永久删除</UButton>
              </td>
            </tr>
            <tr v-if="detailId === row.id" class="border-t" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
              <td colspan="6" class="px-3 py-3">
                <pre class="max-h-64 overflow-auto whitespace-pre-wrap text-xs" :class="theme?.mutedText || 'text-slate-600 dark:text-slate-300'">{{ row.content || '（无正文）' }}</pre>
                <div class="mt-2 text-xs" :class="theme?.mutedText || 'text-slate-500'">点赞 {{ row.like_count || 0 }} · 全站置顶 {{ row.pinned ? '是' : '否' }} · 个人置顶 {{ row.personal_pinned ? '是' : '否' }}</div>
                <div v-if="recycleBin" class="mt-1 text-xs" :class="theme?.mutedText || 'text-slate-500'">删除原因：{{ row.deleted_reason || '未记录' }}</div>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between text-xs" :class="theme?.mutedText || 'text-slate-500'">
      <span>共 {{ total }} 条</span>
      <div class="flex items-center gap-2">
        <UButton size="xs" variant="soft" color="gray" :disabled="page <= 1 || loading" @click="page--; load()">上一页</UButton>
        <span>第 {{ page }} 页</span>
        <UButton size="xs" variant="soft" color="gray" :disabled="page * pageSize >= total || loading" @click="page++; load()">下一页</UButton>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { deleteRequest, getRequest, postRequest } from '~/utils/api'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'

const props = defineProps<{ recycleBin?: boolean; theme?: any }>()
const toast = useToast()
const { can } = useAdminCapabilities()
const canTrash = computed(() => can('notes.trash'))
const canRestore = computed(() => can('notes.restore'))
const canPermanentlyDelete = computed(() => can('notes.delete_permanently'))
const loading = ref(false)
const actionLoading = ref(false)
const rows = ref<any[]>([])
const selected = ref<number[]>([])
const detailId = ref<number | null>(null)
const page = ref(1)
const pageSize = 20
const total = ref(0)
const filters = reactive({ keyword: '', id: '', authorId: '', visibility: '' })
const visibilityOptions = [
  { label: '全部可见性', value: '' },
  { label: '公开', value: 'public' },
  { label: '登录用户', value: 'users' },
  { label: '联系人', value: 'contacts' },
  { label: '私密', value: 'private' }
]
const endpoint = computed(() => props.recycleBin ? 'admin/recycle-bin' : 'admin/notes')
const allSelected = computed(() => rows.value.length > 0 && rows.value.every((row) => selected.value.includes(row.id)))

const formatDate = (value: any) => {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? String(value).replace('T', ' ').replace('Z', '') : date.toLocaleString('zh-CN', { hour12: false })
}
const oneLine = (value: any) => String(value || '').replace(/\s+/g, ' ').trim().slice(0, 120)
const visibilityLabel = (value: string) => visibilityOptions.find((item) => item.value === value)?.label || value || '公开'
const query = () => ({ page: page.value, pageSize, keyword: filters.keyword, id: filters.id, authorId: filters.authorId, visibility: filters.visibility })

const load = async () => {
  loading.value = true
  try {
    const response = await getRequest<any>(endpoint.value, query(), { silent: true })
    if (response?.code === 1) {
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
    if (response?.code === 1) {
      toast.add({ title: '操作完成', description: response.msg || '已完成', color: 'green' })
      clearSelection()
      await load()
    }
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
const batchTrash = () => runAction(() => postRequest('admin/notes/batch-trash', { ids: selected.value, reason: 'admin batch request' }, { silent: true }))
const batchRestore = () => runAction(() => postRequest('admin/recycle-bin/batch-restore', { ids: selected.value }, { silent: true }))
const batchPermanentDelete = () => {
  if (!confirmPermanent(selected.value.length)) return
  return runAction(() => postRequest('admin/recycle-bin/batch-permanent-delete', { ids: selected.value, reason: 'admin batch request' }, { silent: true }))
}
watch(() => props.recycleBin, () => { page.value = 1; clearSelection(); load() })
onMounted(load)
</script>
