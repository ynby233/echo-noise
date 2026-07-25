<template>
  <UModal v-model="open" :prevent-close="true" :ui="{ width: 'sm:max-w-2xl', container: 'items-center' }">
    <UCard v-if="current" class="announcement-modal-card" :ui="{ body: { padding: 'p-0' }, header: { padding: 'p-0' }, footer: { padding: 'p-0' } }">
      <template #header>
        <div class="modal-progress-head">
          <div class="progress-copy">
            <span class="progress-label">第 {{ currentIndex + 1 }} 条 / 共 {{ snapshotTotal }} 条</span>
            <time>{{ formatDate(current.published_at || current.created_at) }}</time>
          </div>
          <div class="progress-track" aria-hidden="true">
            <div class="progress-fill" :style="{ width: `${progressPercent}%` }"></div>
          </div>
        </div>
      </template>

      <article class="modal-announcement">
        <div class="modal-title-row">
          <UIcon name="i-heroicons-megaphone" class="w-6 h-6" />
          <h2>{{ current.title }}</h2>
        </div>
        <div class="modal-content-scroll">
          <MarkdownRenderer :content="current.content" />
        </div>
        <p v-if="actionError" class="modal-error" role="alert">{{ actionError }}</p>
      </article>

      <template #footer>
        <div class="modal-actions">
          <button v-if="remainingCount > 1" type="button" class="secondary-action nw-action-btn nw-action-btn--label" :disabled="acting" @click="markAllRead">
            全部已读
          </button>
          <button type="button" class="primary-action" :disabled="acting" @click="advance">
            <UIcon v-if="acting" name="i-mdi-loading" class="w-4 h-4 animate-spin" />
            <span>{{ isLast ? '阅读完毕' : '下一条' }}</span>
            <UIcon v-if="!isLast && !acting" name="i-heroicons-arrow-right" class="w-4 h-4" />
          </button>
        </div>
      </template>
    </UCard>
  </UModal>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import MarkdownRenderer from '~/components/index/MarkdownRenderer.vue'
import { getRequest, putRequest } from '~/utils/api'

type AnnouncementSnapshotItem = {
  id: number
  title: string
  content: string
  published_at?: string
  created_at: string
}

type UnreadAnnouncementPayload = {
  unread_count?: number
  items?: AnnouncementSnapshotItem[]
}

const emit = defineEmits<{ (event: 'unread-change', count: number): void }>()
const open = ref(false)
const items = ref<AnnouncementSnapshotItem[]>([])
const currentIndex = ref(0)
const snapshotTotal = ref(0)
const acting = ref(false)
const actionError = ref('')
const current = computed(() => items.value[currentIndex.value] || null)
const remainingCount = computed(() => Math.max(0, snapshotTotal.value - currentIndex.value))
const isLast = computed(() => currentIndex.value >= snapshotTotal.value - 1)
const progressPercent = computed(() => snapshotTotal.value > 0 ? ((currentIndex.value + 1) / snapshotTotal.value) * 100 : 0)

const loadUnreadAnnouncements = async () => {
  actionError.value = ''
  const response = await getRequest<UnreadAnnouncementPayload>('announcements/unread', {}, { credentials: 'include', silent: true })
  if (!response || response.code !== 1) return
  items.value = Array.isArray(response.data?.items) ? response.data.items : []
  snapshotTotal.value = items.value.length
  currentIndex.value = 0
  emit('unread-change', Math.max(0, Number(response.data?.unread_count ?? items.value.length)))
  open.value = items.value.length > 0
}

const markCurrentRead = async () => {
  if (!current.value) return false
  const response = await putRequest<any>(`announcements/${current.value.id}/read`, {}, { credentials: 'include', silent: true })
  if (!response || response.code !== 1) {
    actionError.value = response?.msg || '标记公告已读失败，请重试'
    return false
  }
  return true
}

const advance = async () => {
  if (acting.value || !current.value) return
  acting.value = true
  actionError.value = ''
  try {
    if (!await markCurrentRead()) return
    const unreadAfter = Math.max(0, snapshotTotal.value - currentIndex.value - 1)
    emit('unread-change', unreadAfter)
    if (isLast.value) {
      open.value = false
      items.value = []
      return
    }
    currentIndex.value += 1
  } finally {
    acting.value = false
  }
}

const markAllRead = async () => {
  if (acting.value || remainingCount.value <= 1) return
  acting.value = true
  actionError.value = ''
  try {
    const response = await putRequest<any>('announcements/read-all', {}, { credentials: 'include', silent: true })
    if (!response || response.code !== 1) {
      actionError.value = response?.msg || '全部标记已读失败，请重试'
      return
    }
    emit('unread-change', 0)
    open.value = false
    items.value = []
  } finally {
    acting.value = false
  }
}

const formatDate = (value?: string) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

defineExpose({ refresh: loadUnreadAnnouncements })
onMounted(loadUnreadAnnouncements)
</script>

<style scoped>
.announcement-modal-card { overflow:hidden; border-radius:20px; }
.modal-progress-head { padding:18px 20px 0; }
.progress-copy { display:flex; align-items:center; justify-content:space-between; gap:12px; color:#64748b; font-size:.75rem; font-variant-numeric:tabular-nums; }
.progress-label { color:#4f46e5; font-weight:800; letter-spacing:.04em; }
.progress-track { height:4px; margin-top:13px; overflow:hidden; border-radius:999px; background:rgba(100,116,139,.16); }
.progress-fill { height:100%; border-radius:inherit; background:linear-gradient(90deg,#6366f1,#f97316); transition:width .2s ease; }
.modal-announcement { padding:20px; }
.modal-title-row { display:flex; align-items:flex-start; gap:10px; color:#f97316; }
.modal-title-row h2 { margin:0; color:#172033; font-size:1.35rem; line-height:1.35; font-weight:780; }
:global(.dark) .modal-title-row h2 { color:#f1f5f9; }
.modal-content-scroll { max-height:min(56vh,560px); margin-top:18px; overflow-y:auto; overscroll-behavior:contain; padding-right:6px; }
.modal-error { margin:14px 0 0; padding:9px 11px; border-radius:10px; background:#fef2f2; color:#b91c1c; font-size:.82rem; }
.modal-actions { display:grid; grid-template-columns:1fr 1fr; gap:10px; padding:16px 20px 20px; border-top:1px solid rgba(100,116,139,.15); }
.modal-actions > :only-child { grid-column:1 / -1; }
.secondary-action,.primary-action { min-height:42px; border-radius:11px; display:inline-flex; align-items:center; justify-content:center; gap:7px; font-weight:700; }
.primary-action { border:1px solid rgba(79,70,229,.45); background:linear-gradient(135deg,#4f46e5,#6366f1); color:white; box-shadow:0 8px 18px rgba(79,70,229,.24); }
.primary-action:hover:not(:disabled) { transform:translateY(-1px); box-shadow:0 10px 22px rgba(79,70,229,.3); }
.primary-action:disabled,.secondary-action:disabled { opacity:.55; cursor:not-allowed; }
@media (max-width:560px) {
  .modal-progress-head,.modal-announcement { padding-left:15px; padding-right:15px; }
  .progress-copy { align-items:flex-start; flex-direction:column; gap:3px; }
  .modal-content-scroll { max-height:50vh; }
  .modal-actions { grid-template-columns:1fr; padding:13px 15px 16px; }
  .modal-actions > * { grid-column:1; }
}
@media (prefers-reduced-motion:reduce) { .progress-fill,.primary-action { transition:none; } }
</style>
