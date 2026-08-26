<template>
  <section class="personal-content" :class="theme?.text || ''">
    <div class="personal-head">
      <div><h3>我的内容与回收站</h3><p :class="mutedClass">即使上级内容后来变为不可见，你仍可在这里查看自己的互动；上级语境会以中性占位说明。</p></div>
      <UButton size="sm" color="gray" variant="soft" icon="i-heroicons-arrow-path" :loading="loading" @click="load">刷新</UButton>
    </div>
    <div class="personal-tabs" role="tablist">
      <button v-for="item in tabs" :key="item.value" type="button" :class="{ active: tab === item.value }" @click="tab = item.value">{{ item.label }}</button>
    </div>
    <div v-if="loading" class="personal-empty"><UIcon name="i-heroicons-arrow-path" class="animate-spin" />正在读取…</div>
    <div v-else-if="!rows.length" class="personal-empty">这里暂时没有内容</div>
    <div v-else class="personal-list">
      <article v-for="row in rows" :key="`${tab}-${row.id}`" class="personal-card" :class="borderClass">
        <div class="personal-card-head"><strong>{{ itemTitle(row) }}</strong><span :class="mutedClass">{{ formatDate(row.deleted_at || row.created_at) }}</span></div>
        <div v-if="tab !== 'notes'" class="context-strip" :class="subtleClass">
          <span>{{ contextText(row.message_context) }}</span>
          <span v-for="node in row.context || []" :key="node.id">→ {{ contextText(node) }}</span>
        </div>
        <p class="personal-body">{{ row.content || '（无正文）' }}</p>
        <div v-if="tab !== 'active'" class="personal-status">
          <span>原因：{{ reasonLabel(row.deletion_reason_code || row.deleted_reason) }}</span>
          <span :class="deadlineEnabled(row) ? 'deadline' : mutedClass">{{ deadlineText(row.recycle_deadline) }}</span>
        </div>
        <div v-if="tab !== 'active'" class="personal-actions">
          <UButton v-if="row.can_restore !== false" size="xs" color="green" variant="soft" :loading="acting" @click="restore(row)">恢复</UButton>
          <UButton v-if="tab === 'interactions'" size="xs" color="red" variant="soft" :loading="acting" @click="purge(row)">从我的回收站彻底删除</UButton>
          <span v-if="row.can_restore === false && !row.user_purged" class="text-xs" :class="mutedClass">需先恢复仍在回收站中的上级内容</span>
        </div>
        <div v-else-if="row.can_open_thread" class="personal-actions"><UButton size="xs" color="primary" variant="soft" :to="{ path: '/', query: { tab: 'latest', message_id: row.message_id, comment_id: row.id } }">查看所在互动串</UButton></div>
      </article>
    </div>
    <div class="personal-pager" :class="mutedClass"><span>共 {{ total }} 条</span><div><UButton size="xs" color="gray" variant="soft" :disabled="page <= 1" @click="page--; load()">上一页</UButton><span>第 {{ page }} 页</span><UButton size="xs" color="gray" variant="soft" :disabled="page * pageSize >= total" @click="page++; load()">下一页</UButton></div></div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { deleteRequest, getRequest, postRequest } from '~/utils/api'

const props = defineProps<{ theme?: any }>()
const toast = useToast()
const tab = ref<'active' | 'notes' | 'interactions'>('active')
const tabs = [{ label: '我的互动', value: 'active' }, { label: '笔记回收站', value: 'notes' }, { label: '互动回收站', value: 'interactions' }] as const
const rows = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 10
const loading = ref(false)
const acting = ref(false)
const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | null = null
const borderClass = computed(() => props.theme?.border || 'border-slate-200 dark:border-slate-700')
const subtleClass = computed(() => props.theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60')
const mutedClass = computed(() => props.theme?.mutedText || 'text-slate-500 dark:text-slate-400')
const endpoint = computed(() => tab.value === 'active' ? 'user/interactions' : tab.value === 'notes' ? 'user/recycle-bin/notes' : 'user/recycle-bin/comments')
const load = async () => {
  loading.value = true
  try {
    const response = await getRequest<any>(endpoint.value, { page: page.value, pageSize }, { silent: true })
    rows.value = response?.code === 1 && Array.isArray(response.data?.items) ? response.data.items : []
    total.value = response?.code === 1 ? Number(response.data?.total || 0) : 0
  } finally { loading.value = false }
}
const itemTitle = (row: any) => tab.value === 'notes' ? `笔记 #${row.id}` : `${({ comment: '评论', reply: '回复', guestbook: '留言' } as Record<string, string>)[row.kind] || '互动'} #${row.id}`
const contextText = (node: any) => node?.placeholder || String(node?.content || '上级内容').replace(/\s+/g, ' ').slice(0, 80)
const formatDate = (value: any) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
const reasonLabel = (value: string) => ({ self: '由你本人删除', moderation: '由内容管理员处理', owner_cleanup: '内容所有者清理互动', ancestor: '上级内容被删除时一并处理', system: '系统定时清理' } as Record<string, string>)[String(value || '')] || String(value || '未记录')
const deadlineEnabled = (row: any) => row?.recycle_deadline?.auto_cleanup_enabled === true
const deadlineText = (deadline: any) => {
  void now.value
  if (!deadline?.auto_cleanup_enabled) return '当前不会自动清理'
  const seconds = Math.max(0, Math.ceil((new Date(deadline.scheduled_deletion_at).getTime() - Date.now()) / 1000))
  if (seconds <= 0) return '已到清理时间'
  return `距自动清理还有 ${Math.floor(seconds / 86400)}天${Math.floor((seconds % 86400) / 3600)}小时`
}
const run = async (request: () => Promise<any>) => {
  acting.value = true
  try {
    const response = await request()
    toast.add({ title: response?.code === 1 ? '操作完成' : '操作失败', description: response?.msg, color: response?.code === 1 ? 'green' : 'red' })
    if (response?.code === 1) await load()
  } finally { acting.value = false }
}
const restore = (row: any) => run(() => postRequest(tab.value === 'notes' ? `user/recycle-bin/notes/${row.id}/restore` : `user/recycle-bin/comments/${row.id}/restore`, {}, { silent: true }))
const purge = (row: any) => {
  if (typeof window !== 'undefined' && !window.confirm('从你的个人回收站彻底删除后，你将无法再查看或恢复这条互动。是否继续？')) return
  return run(() => deleteRequest(`user/recycle-bin/comments/${row.id}`, undefined, { silent: true }))
}
watch(tab, () => { page.value = 1; void load() })
onMounted(() => { clock = setInterval(() => { now.value = Date.now() }, 60000); void load() })
onUnmounted(() => { if (clock) clearInterval(clock) })
</script>

<style scoped>
.personal-content{margin:16px 16px 0;padding:16px;border-top:1px solid rgba(148,163,184,.25)}.personal-head,.personal-card-head,.personal-status,.personal-actions,.personal-pager,.personal-pager>div{display:flex;align-items:center;gap:10px}.personal-head,.personal-card-head,.personal-pager{justify-content:space-between}.personal-head h3{margin:0;font-size:15px;font-weight:700}.personal-head p{margin:4px 0 0;font-size:12px}.personal-tabs{display:flex;gap:4px;margin:14px 0;padding:4px;border-radius:10px;background:rgba(148,163,184,.12)}.personal-tabs button{flex:1;padding:7px 10px;border-radius:7px;font-size:12px;font-weight:650;color:#64748b}.personal-tabs button.active{background:rgba(255,255,255,.9);color:#4f46e5;box-shadow:0 1px 4px rgba(15,23,42,.1)}:global(.dark) .personal-tabs button.active{background:rgba(51,65,85,.9);color:#a5b4fc}.personal-list{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.personal-card{min-width:0;padding:12px;border-width:1px;border-style:solid;border-radius:10px}.personal-card-head span{font-size:11px}.context-strip{display:flex;gap:5px;margin-top:9px;padding:7px 9px;overflow:hidden;border-radius:8px;color:#64748b;font-size:11px;white-space:nowrap}.context-strip span{overflow:hidden;text-overflow:ellipsis}.personal-body{margin:9px 0 0;display:-webkit-box;overflow:hidden;-webkit-box-orient:vertical;-webkit-line-clamp:3;font-size:13px;line-height:1.6;white-space:pre-wrap}.personal-status{margin-top:9px;justify-content:space-between;flex-wrap:wrap;font-size:11px}.deadline{color:#d97706;font-weight:700}.personal-actions{margin-top:10px;justify-content:flex-end;flex-wrap:wrap}.personal-empty{min-height:130px;display:flex;align-items:center;justify-content:center;gap:8px;color:#64748b;font-size:13px}.personal-pager{margin-top:12px;font-size:11px}.personal-pager>div span{min-width:62px;text-align:center}@media(max-width:800px){.personal-list{grid-template-columns:1fr}}@media(max-width:600px){.personal-content{margin:12px 0 0;padding:12px}.personal-head{align-items:flex-start}.personal-tabs{overflow:auto}.personal-tabs button{min-width:96px}.personal-pager{align-items:stretch;flex-direction:column}.personal-pager>div{justify-content:space-between}}
</style>
