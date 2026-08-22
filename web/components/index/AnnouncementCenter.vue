<template>
  <section class="announcement-center" :class="{ 'announcement-theme-dark': isDark }">
    <div class="announcement-header">
      <div class="announcement-heading">
        <div class="announcement-title-row">
          <h2 class="announcement-title">{{ announcementPageTitle }}</h2>
        </div>
        <p class="announcement-subtitle">{{ announcementPageDescription }}</p>
      </div>
    </div>

    <div class="announcement-feed-panel announcement-board-wrap">
      <div class="announcement-board-head">
        <div class="announcement-count-title">公告 ({{ total }})</div>
        <div class="announcement-actions">
          <span v-if="unreadCount > 0" class="unread-pill">{{ unreadCount }} 未读</span>
          <button type="button" class="announcement-refresh-button nw-action-btn nw-tooltip-anchor" data-tooltip="刷新" aria-label="刷新" :disabled="refreshing || loading" @click="refresh">
            <UIcon name="i-mdi-refresh" class="w-4 h-4" :class="{ 'animate-spin': refreshing }" />
          </button>
          <button type="button" class="announcement-text-button nw-action-btn nw-action-btn--label" :disabled="markingAll || unreadCount === 0" @click="markAllRead">全部已读</button>
        </div>
      </div>

      <div v-if="loading && !items.length" class="empty-state compact">
        <UIcon name="i-mdi-loading" class="empty-icon spin" />
        <div class="empty-title">正在加载</div>
      </div>

      <div v-else-if="loadError && !items.length" class="empty-state compact error-state">
        <UIcon name="i-mdi-alert-circle-outline" class="empty-icon" />
        <div class="empty-title">{{ loadError }}</div>
        <button type="button" class="announcement-text-button nw-action-btn nw-action-btn--label" @click="loadAnnouncements">重试</button>
      </div>

      <div v-else-if="!items.length" class="empty-state compact">
        <UIcon name="i-heroicons-megaphone" class="empty-icon" />
        <div class="empty-title">暂无公告</div>
      </div>

      <div v-else class="announcement-feed">
        <article
          v-for="item in items"
          :key="item.id"
          class="announcement-feed-item"
          :class="{ unread: !item.read, expanded: expandedId === item.id }"
        >
          <span class="announcement-badge" aria-hidden="true">
            <UIcon name="i-heroicons-megaphone" class="announcement-badge-icon" />
          </span>
          <div class="announcement-item-body">
            <div class="announcement-item-head">
              <div class="announcement-actor-block">
                <div class="announcement-actor-line">
                  <span class="announcement-actor-name">{{ item.title }}</span>
                </div>
                <div class="announcement-time">
                  <span>{{ formatTime(item.published_at || item.created_at) }}</span>
                  <span v-if="item.updated_at && item.updated_at !== item.created_at" class="announcement-time-edited">编辑于 {{ formatTime(item.updated_at) }}</span>
                </div>
              </div>
              <button
                type="button"
                class="announcement-toggle nw-action-btn nw-action-btn--label"
                :aria-expanded="expandedId === item.id"
                :aria-controls="`announcement-content-${item.id}`"
                @click="toggleAnnouncement(item)"
              >
                {{ expandedId === item.id ? '收起' : '展开' }}
              </button>
            </div>

            <button
              v-if="expandedId !== item.id"
              type="button"
              class="announcement-target-card"
              :aria-label="`展开公告 ${item.title}`"
              @click="toggleAnnouncement(item)"
            >
              <div class="announcement-target-text">{{ excerpt(item.content) }}</div>
            </button>

            <div v-else :id="`announcement-content-${item.id}`" class="announcement-content">
              <MarkdownRenderer :content="item.content" />
            </div>
          </div>
        </article>
      </div>

      <div v-if="items.length" class="pager-shell" :class="{ 'is-dark': isDark }">
        <div class="pager-nav-group">
          <button
            v-if="page > 1"
            type="button"
            class="pager-btn nw-action-btn nw-action-btn--label"
            :disabled="loading"
            @click="previousPage"
          >
            <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-left" class="w-4 h-4 pager-icon" /></span>
            <span>上一页</span>
          </button>
          <button
            v-if="page < totalPages"
            type="button"
            class="pager-btn nw-action-btn nw-action-btn--label"
            :disabled="loading"
            @click="nextPage"
          >
            <span>下一页</span>
            <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-right" class="w-4 h-4 pager-icon" /></span>
          </button>
          <span v-if="loading" class="pager-status-text">加载中...</span>
        </div>
        <div class="pager-jump-group">
          <span class="pager-page-text">第</span>
          <div class="pager-number-control">
            <input
              v-model="targetPage"
              type="text"
              inputmode="numeric"
              pattern="[0-9]*"
              class="pager-page-input"
              placeholder="#"
              aria-label="跳转页码"
              @keyup.enter="jumpToPage"
            />
            <div class="pager-stepper" aria-label="页码增减">
              <button
                type="button"
                class="pager-stepper-btn nw-action-btn"
                aria-label="页码加一"
                :disabled="loading"
                @click="adjustTargetPage(1)"
              >
                <UIcon name="i-heroicons-chevron-up-20-solid" class="w-3 h-3" />
              </button>
              <button
                type="button"
                class="pager-stepper-btn nw-action-btn"
                aria-label="页码减一"
                :disabled="loading"
                @click="adjustTargetPage(-1)"
              >
                <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
              </button>
            </div>
          </div>
          <span class="pager-page-text">页 / 共 {{ totalPages }} 页</span>
          <button
            type="button"
            class="pager-jump-btn nw-action-btn nw-action-btn--label"
            :disabled="loading"
            @click="jumpToPage"
          >
            跳转
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref, watch } from 'vue'
import MarkdownRenderer from '~/components/index/MarkdownRenderer.vue'
import { getRequest, putRequest } from '~/utils/api'
import { useToast } from '#ui/composables/useToast'

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
const targetPage = ref('1')
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

const syncTargetPageToCurrent = () => {
  const next = Math.min(Math.max(Number(page.value) || 1, 1), totalPages.value)
  targetPage.value = String(next)
}

const adjustTargetPage = (delta: number) => {
  const parsed = Number.parseInt(targetPage.value.trim() || '', 10)
  const base = Number.isFinite(parsed) && parsed > 0 ? parsed : page.value
  const next = Math.min(Math.max(base + delta, 1), totalPages.value)
  targetPage.value = String(next)
}

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
    syncTargetPageToCurrent()
    emitUnread()
  } catch (error: any) {
    loadError.value = error?.message || '公告加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

const refresh = async () => {
  if (refreshing.value || loading.value) return
  refreshing.value = true
  try {
    await loadAnnouncements()
  } finally {
    setTimeout(() => {
      refreshing.value = false
    }, 300)
  }
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

const excerpt = (content: string) => {
  const text = String(content || '')
    .replace(/<[^>]+>/g, '')
    .replace(/[#*_>`\[\]()]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  if (!text) return '展开查看公告正文'
  return text.length <= 120 ? text : `${text.slice(0, 120)}...`
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
const jumpToPage = async () => {
  const next = Number.parseInt(targetPage.value.trim() || '', 10)
  if (!next || next < 1 || next > totalPages.value || loading.value) {
    useToast().add({
      title: '页码无效',
      description: `请输入 1-${totalPages.value} 之间的数字`,
      color: 'orange',
      timeout: 2000
    })
    return
  }
  await goToPage(next)
}
const sidebarPagerState = computed(() => ({
  visible: items.value.length > 0,
  currentPage: page.value,
  totalPages: totalPages.value,
  loading: loading.value,
  canPrevious: !loading.value && page.value > 1,
  canNext: !loading.value && page.value < totalPages.value
}))

watch([page, totalPages], syncTargetPageToCurrent)

defineExpose({ sidebarPagerState, previousPage, nextPage, goToPage, refresh })
onMounted(loadAnnouncements)
</script>

<style scoped>
.announcement-center {
  position:relative;
  width:100%;
  box-sizing:border-box;
  margin:0;
  padding:0;
  border:0;
  border-radius:0;
  background:transparent;
  color:var(--notice-text);
  box-shadow:none;
  --notice-border: rgba(15,23,42,.10);
  --notice-card: #ffffff;
  --notice-card-hover: #f8fafc;
  --notice-input: #f8fafc;
  --notice-card-shadow: 0 14px 30px rgba(15,23,42,.12);
  --notice-text: #374151;
  --notice-strong: #111827;
  --notice-heading: #000000;
  --notice-count: #000000;
  --notice-muted: #6b7280;
  --nw-floating-hover-bg: rgba(249, 115, 22, .12);
  --nw-floating-hover-border: rgba(249, 115, 22, .36);
}
:global(.dark) .announcement-center,
.announcement-center.announcement-theme-dark {
  --notice-border: rgba(255,255,255,.14);
  --notice-card: rgba(15,23,42,.52);
  --notice-card-hover: rgba(30,41,59,.68);
  --notice-input: rgba(15,23,42,.36);
  --notice-card-shadow: 0 16px 32px rgba(2,6,23,.52);
  --notice-text: #cbd5e1;
  --notice-strong: #f8fafc;
  --notice-heading: #ffffff;
  --notice-count: #e5e7eb;
  --notice-muted: #94a3b8;
  --nw-floating-hover-bg: rgba(249, 115, 22, .18);
  --nw-floating-hover-border: rgba(249, 115, 22, .42);
}
.announcement-header { display:block; margin-bottom:0; padding:0; text-align:center; }
.announcement-heading { width:100%; }
.announcement-title-row { position:relative; display:block; width:100%; margin:0; padding:0; }
.announcement-title { display:block; margin:0 0 14px; padding:0; border-radius:0; color:var(--notice-heading); font-size:18px; font-weight:700; line-height:1.5; }
.announcement-subtitle { max-width:42rem; margin:2px auto 20px; color:var(--notice-heading); font-size:13px; line-height:1.7; text-align:center; opacity:.8; font-weight:400; }
.announcement-feed-panel { padding:0; overflow:visible; border:0; background:transparent; border-radius:0; }
.announcement-board-wrap { box-sizing:border-box; max-width:48rem; margin:0 auto 8px; padding:8px; }
.announcement-board-head { display:flex; align-items:center; justify-content:space-between; gap:8px; min-height:28px; margin:0 0 8px; }
.announcement-count-title { min-width:0; margin:0; color:var(--notice-count); font-size:14px; font-weight:400; line-height:20px; }
.announcement-actions { display:flex; align-items:center; gap:8px; flex:0 0 auto; flex-wrap:wrap; justify-content:flex-end; max-width:70%; }
.unread-pill { display:inline-flex; align-items:center; min-height:28px; padding:0 10px; border-radius:999px; font-size:12px; font-weight:650; color:#fff; background:#3b82f6; }
.announcement-refresh-button,
.announcement-text-button,
.announcement-toggle {
  height:28px;
  min-height:28px;
  border-radius:8px;
  font-size:12px;
  font-weight:650;
  line-height:1;
  --nw-action-bg:rgba(15,23,42,.06);
  --nw-action-text:var(--notice-text);
  --nw-action-border:var(--notice-border);
}
.announcement-refresh-button { width:28px; min-width:28px; }
.announcement-text-button,
.announcement-toggle { min-width:max-content; padding:0 8px; }
:global(.dark) .announcement-center .announcement-refresh-button,
:global(.dark) .announcement-center .announcement-text-button,
:global(.dark) .announcement-center .announcement-toggle,
.announcement-center.announcement-theme-dark .announcement-refresh-button,
.announcement-center.announcement-theme-dark .announcement-text-button,
.announcement-center.announcement-theme-dark .announcement-toggle {
  --nw-action-bg:rgba(51,65,85,.96);
  --nw-action-text:#cbd5e1;
  --nw-action-border:rgba(148,163,184,.28);
}
.empty-state { min-height:220px; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:8px; text-align:center; padding:24px; border:1px solid var(--notice-border); background:var(--notice-card); color:var(--notice-text); border-radius:12px; }
.empty-state.compact { min-height:220px; border:0; background:transparent; }
.error-state .empty-icon { color:#ef4444; opacity:.82; }
.empty-icon { width:28px; height:28px; opacity:.62; }
.empty-title { font-size:14px; font-weight:600; color:var(--notice-muted); }
.announcement-feed { display:flex; flex-direction:column; gap:12px; }
.announcement-feed-item { position:relative; display:flex; gap:12px; padding:12px; border:1px solid var(--notice-border); border-radius:12px; background:var(--notice-card); color:var(--notice-text); box-shadow:var(--notice-card-shadow); transition:background-color .16s ease, border-color .16s ease, transform .16s ease; }
.announcement-feed-item:hover { background:var(--notice-card-hover); }
.announcement-feed-item.unread { border-color:rgba(59,130,246,.36); background:linear-gradient(0deg, rgba(59,130,246,.10), rgba(59,130,246,.10)), var(--notice-card); }
.announcement-feed-item.unread::before { content:''; position:absolute; left:8px; top:23px; width:6px; height:6px; border-radius:999px; background:#3b82f6; }
.announcement-badge { width:36px; height:36px; flex:0 0 36px; display:inline-flex; align-items:center; justify-content:center; border-radius:999px; background:var(--notice-input); border:1px solid var(--notice-border); color:#f97316; }
.announcement-badge-icon { width:20px; height:20px; }
.announcement-item-body { min-width:0; flex:1; }
.announcement-item-head { display:flex; align-items:flex-start; justify-content:space-between; gap:10px; min-height:36px; }
.announcement-actor-block { min-width:0; }
.announcement-actor-line { display:flex; align-items:center; gap:8px; min-width:0; }
.announcement-actor-name { font-size:14px; line-height:1.35; font-weight:700; color:var(--notice-strong); word-break:break-word; }
.announcement-time { display:flex; flex-wrap:wrap; gap:8px; margin-top:3px; font-size:12px; line-height:1.25; color:var(--notice-muted); }
.announcement-time-edited { opacity:.8; }
.announcement-toggle { flex:0 0 auto; }
.announcement-target-card { width:100%; margin-top:10px; padding:10px 12px; border:1px solid var(--notice-border); border-radius:12px; background:var(--notice-input); color:var(--notice-text); display:flex; align-items:center; flex-wrap:wrap; gap:10px; text-align:left; cursor:pointer; transition:background-color .16s ease, border-color .16s ease, opacity .16s ease; }
.announcement-target-card:hover { border-color:var(--nw-floating-hover-border); background:var(--nw-floating-hover-bg); }
.announcement-target-text { min-width:0; flex:1 1 160px; font-size:14px; line-height:1.58; word-break:break-word; }
.announcement-content { margin-top:12px; padding-top:12px; border-top:1px solid var(--notice-border); }
.announcement-content :deep(.markdown-body) { margin-top:0; }
.spin { animation:announcement-spin 1s linear infinite; }
@keyframes announcement-spin { to { transform:rotate(360deg); } }
@media (max-width: 720px) {
  .announcement-header { text-align:center; margin-bottom:0; padding:0; }
  .announcement-title-row { display:block; min-height:0; margin:0; }
  .announcement-title { line-height:1.5; }
  .announcement-subtitle { margin:2px auto 20px; }
  .announcement-board-head { align-items:center; flex-wrap:wrap; }
  .announcement-actions { justify-content:flex-end; max-width:100%; }
  .announcement-feed-item { gap:10px; padding:14px 12px; }
  .announcement-feed-item.unread::before { left:7px; top:24px; }
  .announcement-badge { width:40px; height:40px; flex-basis:40px; }
  .announcement-actor-name { font-size:16px; }
  .announcement-target-card { padding:10px; }
  .announcement-target-text { font-size:15px; line-height:1.68; }
  .pager-shell { border-radius:18px; gap:10px; }
  .pager-nav-group,
  .pager-jump-group { width:100%; }
}
@media (prefers-reduced-motion:reduce) { .announcement-feed-item, .announcement-target-card { transition:none; } }
</style>
