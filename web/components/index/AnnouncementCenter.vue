<template>
  <section class="announcement-center" :class="{ 'is-dark': isDark }">
    <header class="announcement-header">
      <div>
        <h2>{{ announcementPageTitle }}</h2>
        <p>{{ announcementPageDescription }}</p>
      </div>
    </header>

    <div class="announcement-panel">
      <div class="announcement-toolbar">
        <div class="announcement-count">公告（{{ total }}）</div>
        <div class="announcement-actions">
          <span v-if="unreadCount > 0" class="unread-pill">{{ unreadCount }} 未读</span>
          <button type="button" class="icon-action nw-action-btn nw-tooltip-anchor" data-tooltip="刷新" aria-label="刷新公告" :disabled="loading" @click="refresh">
            <UIcon name="i-mdi-refresh" class="w-4 h-4" :class="{ 'animate-spin': refreshing }" />
          </button>
          <button type="button" class="text-action nw-action-btn nw-action-btn--label" :disabled="markingAll || unreadCount === 0" @click="markAllRead">
            全部已读
          </button>
        </div>
      </div>

      <div v-if="loading && !items.length" class="announcement-empty">
        <UIcon name="i-mdi-loading" class="empty-icon animate-spin" />
        <span>正在加载公告</span>
      </div>
      <div v-else-if="loadError && !items.length" class="announcement-empty is-error">
        <UIcon name="i-mdi-alert-circle-outline" class="empty-icon" />
        <span>{{ loadError }}</span>
        <button type="button" class="text-action nw-action-btn nw-action-btn--label" @click="loadAnnouncements">重试</button>
      </div>
      <div v-else-if="!items.length" class="announcement-empty">
        <UIcon name="i-heroicons-megaphone" class="empty-icon" />
        <span>暂无公告</span>
      </div>

      <div v-else class="announcement-list">
        <article v-for="item in items" :key="item.id" class="announcement-item" :class="{ unread: !item.read, expanded: expandedId === item.id }">
          <button type="button" class="announcement-summary" :aria-expanded="expandedId === item.id" @click="toggleAnnouncement(item)">
            <span class="announcement-marker" aria-hidden="true"></span>
            <span class="announcement-summary-main">
              <span class="announcement-title-row">
                <strong>{{ item.title }}</strong>
                <span v-if="!item.read" class="unread-dot">未读</span>
              </span>
              <span class="announcement-meta">
                <time>{{ formatDate(item.published_at || item.created_at) }}</time>
                <span v-if="item.updated_at && item.updated_at !== item.created_at">编辑于 {{ formatDate(item.updated_at) }}</span>
              </span>
              <span v-if="expandedId !== item.id" class="announcement-excerpt">{{ excerpt(item.content) }}</span>
            </span>
            <UIcon :name="expandedId === item.id ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" class="w-5 h-5 announcement-chevron" />
          </button>
          <div v-if="expandedId === item.id" class="announcement-content">
            <MarkdownRenderer :content="item.content" />
          </div>
        </article>
      </div>

      <div v-if="items.length && totalPages > 1" class="announcement-pager">
        <button type="button" class="text-action nw-action-btn nw-action-btn--label" :disabled="page <= 1 || loading" @click="previousPage">上一页</button>
        <span>第 {{ page }} / {{ totalPages }} 页</span>
        <button type="button" class="text-action nw-action-btn nw-action-btn--label" :disabled="page >= totalPages || loading" @click="nextPage">下一页</button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import MarkdownRenderer from '~/components/index/MarkdownRenderer.vue'
import { getRequest, putRequest } from '~/utils/api'

type AnnouncementItem = {
  id: number
  title: string
  content: string
  read: boolean
  published_at?: string
  created_at: string
  updated_at: string
}

type AnnouncementListPayload = {
  items?: AnnouncementItem[]
  page?: number
  page_size?: number
  total?: number
  unread_count?: number
}

const props = defineProps<{ siteConfig?: any }>()
const emit = defineEmits<{ (event: 'unread-change', count: number): void }>()
const announcementPageTitle = computed(() => String(props.siteConfig?.announcementPageTitle || '').trim() || '公告')
const announcementPageDescription = computed(() => String(props.siteConfig?.announcementPageDescription || '').trim() || '查看站点发布的最新公告')
const injectedTheme = inject('contentTheme', ref('light')) as any
const isDark = computed(() => String(injectedTheme?.value ?? injectedTheme ?? 'light') === 'dark')
const items = ref<AnnouncementItem[]>([])
const page = ref(1)
const pageSize = 20
const total = ref(0)
const unreadCount = ref(0)
const loading = ref(false)
const refreshing = ref(false)
const markingAll = ref(false)
const loadError = ref('')
const expandedId = ref<number | null>(null)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

const emitUnread = () => emit('unread-change', unreadCount.value)

const loadAnnouncements = async () => {
  loading.value = true
  loadError.value = ''
  try {
    const response = await getRequest<AnnouncementListPayload>('announcements', { page: page.value, pageSize }, { credentials: 'include', silent: true })
    if (!response || response.code !== 1) throw new Error(response?.msg || '获取公告失败')
    items.value = Array.isArray(response.data?.items) ? response.data.items : []
    total.value = Math.max(0, Number(response.data?.total || 0))
    unreadCount.value = Math.max(0, Number(response.data?.unread_count || 0))
    page.value = Math.max(1, Number(response.data?.page || page.value))
    emitUnread()
  } catch (error: any) {
    loadError.value = error?.message || '公告加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

const refresh = async () => {
  refreshing.value = true
  try { await loadAnnouncements() } finally { refreshing.value = false }
}

const toggleAnnouncement = async (item: AnnouncementItem) => {
  if (expandedId.value === item.id) {
    expandedId.value = null
    return
  }
  expandedId.value = item.id
  if (item.read) return
  const response = await putRequest<any>(`announcements/${item.id}/read`, {}, { credentials: 'include', silent: true })
  if (response?.code === 1) {
    item.read = true
    unreadCount.value = Math.max(0, unreadCount.value - 1)
    emitUnread()
  }
}

const markAllRead = async () => {
  if (markingAll.value || unreadCount.value === 0) return
  markingAll.value = true
  try {
    const response = await putRequest<any>('announcements/read-all', {}, { credentials: 'include', silent: true })
    if (response?.code !== 1) throw new Error(response?.msg || '全部标记已读失败')
    items.value.forEach((item) => { item.read = true })
    unreadCount.value = 0
    emitUnread()
  } catch (error: any) {
    loadError.value = error?.message || '全部标记已读失败'
  } finally {
    markingAll.value = false
  }
}

const excerpt = (content: string) => String(content || '').replace(/\s+/g, ' ').trim().slice(0, 120) || '打开查看公告正文'
const formatDate = (value?: string) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

const previousPage = async () => {
  if (page.value <= 1) return
  page.value -= 1
  expandedId.value = null
  await loadAnnouncements()
}
const nextPage = async () => {
  if (page.value >= totalPages.value) return
  page.value += 1
  expandedId.value = null
  await loadAnnouncements()
}
const goToPage = async (target: string | number) => {
  const parsed = Math.min(totalPages.value, Math.max(1, Number(target) || 1))
  page.value = parsed
  expandedId.value = null
  await loadAnnouncements()
}
const sidebarPagerState = computed(() => ({
  visible: totalPages.value > 1,
  currentPage: page.value,
  totalPages: totalPages.value,
  loading: loading.value,
  canPrevious: page.value > 1,
  canNext: page.value < totalPages.value
}))

defineExpose({ sidebarPagerState, previousPage, nextPage, goToPage, refresh })
onMounted(loadAnnouncements)
</script>

<style scoped>
.announcement-center { color:#172033; }
.announcement-center.is-dark { color:#e5e7eb; }
.announcement-header { padding:4px 2px 18px; text-align:center; }
.announcement-header h2 { margin:0; font-size:1.55rem; font-weight:750; letter-spacing:.02em; }
.announcement-header p { margin:7px 0 0; font-size:.9rem; color:#64748b; }
.is-dark .announcement-header p { color:#94a3b8; }
.announcement-panel { overflow:hidden; border:1px solid rgba(100,116,139,.22); border-radius:18px; background:rgba(255,255,255,.8); box-shadow:0 16px 34px rgba(15,23,42,.08); }
.is-dark .announcement-panel { background:rgba(30,41,59,.76); border-color:rgba(148,163,184,.2); box-shadow:0 18px 36px rgba(0,0,0,.25); }
.announcement-toolbar { display:flex; align-items:center; justify-content:space-between; gap:12px; padding:14px 16px; border-bottom:1px solid rgba(100,116,139,.18); }
.announcement-count { font-size:.95rem; font-weight:700; }
.announcement-actions { display:flex; align-items:center; gap:8px; }
.unread-pill { padding:4px 9px; border-radius:999px; background:#fee2e2; color:#b91c1c; font-size:.75rem; font-weight:700; }
.is-dark .unread-pill { background:rgba(127,29,29,.35); color:#fca5a5; }
.icon-action { width:34px; height:34px; display:inline-flex; align-items:center; justify-content:center; border-radius:9px; }
.text-action { min-height:34px; padding:0 12px; border-radius:9px; font-size:.82rem; }
.announcement-empty { min-height:230px; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:10px; color:#64748b; }
.announcement-empty.is-error { color:#dc2626; }
.empty-icon { width:34px; height:34px; }
.announcement-list { position:relative; padding:8px 16px 12px 28px; }
.announcement-list::before { content:""; position:absolute; left:18px; top:20px; bottom:24px; width:1px; background:rgba(99,102,241,.25); }
.announcement-item { position:relative; margin:8px 0; border:1px solid rgba(100,116,139,.18); border-radius:14px; background:rgba(248,250,252,.82); transition:border-color .16s ease, background .16s ease, transform .16s ease; }
.is-dark .announcement-item { background:rgba(15,23,42,.42); border-color:rgba(148,163,184,.17); }
.announcement-item.unread { border-color:rgba(249,115,22,.4); }
.announcement-item.expanded { border-color:rgba(99,102,241,.42); }
.announcement-summary { width:100%; display:flex; align-items:flex-start; gap:12px; padding:15px 14px; border:0; color:inherit; background:transparent; text-align:left; cursor:pointer; }
.announcement-marker { position:absolute; left:-16px; top:24px; width:9px; height:9px; border-radius:999px; background:#94a3b8; box-shadow:0 0 0 4px rgba(148,163,184,.14); }
.unread .announcement-marker { background:#f97316; box-shadow:0 0 0 4px rgba(249,115,22,.15); }
.announcement-summary-main { min-width:0; flex:1; display:flex; flex-direction:column; gap:6px; }
.announcement-title-row { display:flex; align-items:center; gap:8px; }
.announcement-title-row strong { font-size:1rem; line-height:1.35; }
.unread-dot { padding:2px 6px; border-radius:999px; background:#f97316; color:white; font-size:.65rem; font-weight:750; }
.announcement-meta { display:flex; flex-wrap:wrap; gap:10px; font-size:.72rem; color:#7c889b; font-variant-numeric:tabular-nums; }
.announcement-excerpt { overflow:hidden; white-space:nowrap; text-overflow:ellipsis; color:#64748b; font-size:.84rem; }
.is-dark .announcement-excerpt { color:#a8b3c4; }
.announcement-chevron { margin-top:2px; flex:none; color:#718096; }
.announcement-content { padding:0 18px 18px 44px; border-top:1px dashed rgba(100,116,139,.18); }
.announcement-content :deep(.markdown-body) { margin-top:14px; }
.announcement-pager { display:flex; align-items:center; justify-content:center; gap:12px; padding:13px 16px; border-top:1px solid rgba(100,116,139,.16); color:#64748b; font-size:.8rem; }
@media (max-width:640px) {
  .announcement-toolbar { align-items:flex-start; flex-direction:column; }
  .announcement-actions { width:100%; justify-content:flex-end; flex-wrap:wrap; }
  .announcement-list { padding-left:22px; padding-right:10px; }
  .announcement-list::before { left:13px; }
  .announcement-marker { left:-14px; }
  .announcement-summary { padding:13px 11px; }
  .announcement-content { padding:0 13px 15px; }
  .announcement-meta { flex-direction:column; gap:2px; }
}
@media (prefers-reduced-motion:reduce) { .announcement-item { transition:none; } }
</style>
