<template>
  <UModal
    v-model="open"
    :prevent-close="true"
    :ui="{ width: 'sm:max-w-2xl', container: 'items-center', base: 'backdrop-blur-sm', background: 'bg-transparent dark:bg-transparent', shadow: 'shadow-none', rounded: 'rounded-none' }"
  >
    <UCard v-if="current" class="nw-modal-card" :ui="{ rounded: 'rounded-none', ring: 'ring-0', shadow: 'shadow-none' }">
      <template #header>
        <div class="flex items-center justify-between gap-3">
          <h3 class="modal-heading">
            <UIcon name="i-heroicons-megaphone" class="modal-heading-icon" />
            <span>{{ current.title }}</span>
          </h3>
          <span class="modal-progress-pill">第 {{ currentIndex + 1 }} 条 / 共 {{ snapshotTotal }} 条</span>
        </div>
        <div class="modal-time">{{ formatTime(current.published_at || current.created_at) }}</div>
      </template>

      <div class="modal-content-scroll">
        <MarkdownRenderer :content="current.content" />
      </div>
      <p v-if="actionError" class="modal-error" role="alert">{{ actionError }}</p>

      <template #footer>
        <div class="modal-footer-actions">
          <UButton v-if="remainingCount > 1" variant="soft" color="gray" :disabled="acting" @click="markAllRead">全部已读</UButton>
          <UButton color="orange" :loading="acting" :trailing-icon="isLast ? undefined : 'i-heroicons-arrow-right'" @click="advance">
            {{ isLast ? '阅读完毕' : '下一条' }}
          </UButton>
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

const formatTime = (value?: string) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  return `${month}月${day}日${hour}:${minute}`
}

defineExpose({ refresh: loadUnreadAnnouncements })
onMounted(loadUnreadAnnouncements)
</script>

<style scoped>
.modal-heading { display:flex; align-items:center; gap:8px; min-width:0; margin:0; font-size:1.25rem; font-weight:600; line-height:1.4; word-break:break-word; }
.modal-heading-icon { width:1.25rem; height:1.25rem; flex:none; color:#f97316; }
.modal-progress-pill { flex:none; padding:2px 10px; border-radius:999px; background:rgba(15,23,42,.06); font-size:12px; font-weight:650; font-variant-numeric:tabular-nums; opacity:.9; }
:global(.dark) .modal-progress-pill { background:rgba(255,255,255,.10); }
.modal-time { margin-top:6px; font-size:12px; line-height:1.25; opacity:.7; }
.modal-content-scroll { max-height:min(56vh,560px); overflow-y:auto; overscroll-behavior:contain; padding-right:6px; }
.modal-error { margin:14px 0 0; padding:9px 11px; border-radius:10px; background:rgba(254,242,242,.9); color:#b91c1c; font-size:13px; }
:global(.dark) .modal-error { background:rgba(127,29,29,.24); color:#fca5a5; }
.modal-footer-actions { display:flex; align-items:center; justify-content:flex-end; gap:8px; flex-wrap:wrap; }
@media (max-width:560px) {
  .modal-heading { font-size:1.1rem; }
  .modal-content-scroll { max-height:50vh; }
  .modal-footer-actions { justify-content:stretch; }
  .modal-footer-actions > * { flex:1 1 auto; justify-content:center; }
}
</style>
