<template>
  <section class="notification-center">
    <div class="notification-header">
      <div>
        <h2 class="notification-title">通知</h2>
        <p class="notification-subtitle">评论、回复、留言和点赞</p>
      </div>
      <div v-if="user.isLogin" class="notification-actions">
        <span v-if="unreadCount > 0" class="unread-pill">{{ unreadCount }} 未读</span>
        <button type="button" class="icon-action nw-tooltip-anchor" data-tooltip="刷新" aria-label="刷新" :disabled="loading" @click="loadNotifications(true)">
          <UIcon name="i-mdi-refresh" class="w-4 h-4" />
        </button>
        <button type="button" class="text-action" :disabled="markingAll || unreadCount === 0" @click="markAllRead">全部已读</button>
      </div>
    </div>

    <div v-if="!user.isLogin" class="empty-state">
      <UIcon name="i-mdi-bell-off-outline" class="empty-icon" />
      <div class="empty-title">登录后查看通知</div>
    </div>

    <div v-else class="notification-shell">
      <div class="notification-list-panel">
        <div v-if="loading && !items.length" class="empty-state compact">
          <UIcon name="i-mdi-loading" class="empty-icon spin" />
          <div class="empty-title">正在加载</div>
        </div>

        <div v-else-if="!items.length" class="empty-state compact">
          <UIcon name="i-mdi-bell-outline" class="empty-icon" />
          <div class="empty-title">暂无通知</div>
        </div>

        <div v-else class="notification-masonry">
          <article
            v-for="item in items"
            :key="item.id"
            class="notification-card"
            :class="{ unread: !item.read, active: selected?.id === item.id }"
            @click="openNotification(item)"
          >
            <div class="notification-card-top">
              <div class="notification-icon-wrap" :class="item.type">
                <UIcon :name="typeIcon(item.type)" class="w-4 h-4" />
              </div>
              <div class="notification-card-title">
                <div class="notification-title-line">{{ titleFor(item) }}</div>
                <div class="notification-time">{{ formatTime(item.created_at) }}</div>
              </div>
              <span v-if="!item.read" class="unread-dot" aria-label="未读"></span>
            </div>

            <p v-if="primaryText(item)" class="notification-primary">{{ primaryText(item) }}</p>
            <p v-if="messageText(item)" class="notification-message">{{ messageText(item) }}</p>

            <div class="notification-card-actions">
              <button type="button" class="inline-action" @click.stop="openNotification(item)">查看</button>
              <button v-if="canReply(item)" type="button" class="inline-action strong" @click.stop="replyNotification(item)">回复</button>
            </div>
          </article>
        </div>

        <div v-if="items.length < total" class="load-more-row">
          <button type="button" class="text-action" :disabled="loadingMore" @click="loadMore">
            {{ loadingMore ? '加载中' : '加载更多' }}
          </button>
        </div>
      </div>

      <aside class="notification-detail-panel">
        <div v-if="selected" class="notification-detail-card">
          <div class="detail-head">
            <div class="notification-icon-wrap" :class="selected.type">
              <UIcon :name="typeIcon(selected.type)" class="w-4 h-4" />
            </div>
            <div class="min-w-0">
              <div class="detail-title">{{ titleFor(selected) }}</div>
              <div class="detail-meta">{{ formatTime(selected.created_at) }}</div>
            </div>
          </div>
          <div v-if="selected.message" class="detail-message">
            <div class="detail-label">原内容</div>
            <MarkdownRenderer :content="messageText(selected) || '无内容'" />
          </div>
          <BuiltinComments
            v-if="selected.message_id"
            :key="selected.message_id"
            ref="commentThreadRef"
            :message-id="selected.message_id"
            :site-config="siteConfig"
            :show-input="true"
            :auto-scroll-input="false"
            context-label="评论"
          />
        </div>

        <div v-else class="empty-state detail-empty">
          <UIcon name="i-mdi-bell-ring-outline" class="empty-icon" />
          <div class="empty-title">选择一条通知</div>
        </div>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import BuiltinComments from '~/components/comments/BuiltinComments.vue'
import MarkdownRenderer from '~/components/index/MarkdownRenderer.vue'
import { useUserStore } from '~/store/user'
import { getRequest, putRequest } from '~/utils/api'
type NotificationActor = {
  id: number
  username?: string
  avatar_url?: string
}

type NotificationMessage = {
  id: number
  content?: string
  image_url?: string
  visibility?: string
  user_id?: number
  is_guestbook?: boolean
}

type NotificationComment = {
  id: number
  message_id: number
  content?: string
  user?: NotificationActor
  parent_id?: number | null
}

type UserNotification = {
  id: number
  type: 'comment' | 'reply' | 'guestbook' | 'like' | string
  actor?: NotificationActor | null
  actor_user_id?: number | null
  message_id?: number | null
  comment_id?: number | null
  parent_comment_id?: number | null
  message?: NotificationMessage | null
  comment?: NotificationComment | null
  parent_comment?: NotificationComment | null
  target_tab?: string
  target_url?: string
  read: boolean
  created_at: string
}

type NotificationListPayload = {
  items?: UserNotification[]
  total?: number
  unread_count?: number
  unreadCount?: number
  page?: number
  pageSize?: number
}

type CommentThreadExpose = {
  focusCommentById: (commentId: number) => Promise<boolean>
  replyToCommentById: (commentId: number) => Promise<boolean>
}

const props = defineProps<{ siteConfig?: any, initialMessageId?: number | null, initialCommentId?: number | null }>()
const emit = defineEmits<{ (event: 'unread-change', count: number): void }>()

const user = useUserStore()
const items = ref<UserNotification[]>([])
const selected = ref<UserNotification | null>(null)
const commentThreadRef = ref<CommentThreadExpose | null>(null)
const loading = ref(false)
const loadingMore = ref(false)
const markingAll = ref(false)
const page = ref(1)
const pageSize = 20
const total = ref(0)
const unreadCount = ref(0)
const resolvingInitialTarget = ref(false)

const siteConfig = computed(() => props.siteConfig || {})

const setUnreadCount = (count: number) => {
  unreadCount.value = Math.max(0, Number(count || 0))
  emit('unread-change', unreadCount.value)
}

const normalizeText = (value?: string | null) => {
  return String(value || '')
    .replace(/<[^>]+>/g, '')
    .replace(/[#*_>`\[\]()]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
}

const truncate = (value?: string | null, limit = 96) => {
  const text = normalizeText(value)
  if (text.length <= limit) return text
  return `${text.slice(0, limit)}...`
}

const actorName = (item: UserNotification) => {
  const name = String(item.actor?.username || '').trim()
  return name || '有用户'
}

const typeIcon = (type: string) => {
  if (type === 'reply') return 'i-mdi-reply-outline'
  if (type === 'guestbook') return 'i-mdi-message-badge-outline'
  if (type === 'like') return 'i-mdi-heart-outline'
  return 'i-mdi-comment-text-outline'
}

const titleFor = (item: UserNotification) => {
  if (item.type === 'reply') return `${actorName(item)} 回复了你`
  if (item.type === 'guestbook') return `${actorName(item)} 留言了`
  if (item.type === 'like') return `${actorName(item)} 点赞了你的内容`
  return `${actorName(item)} 评论了你的内容`
}

const primaryText = (item: UserNotification) => {
  if (item.type === 'like') return ''
  return truncate(item.comment?.content || '', 120)
}

const messageText = (item: UserNotification) => truncate(item.message?.content || '', 120)

const canReply = (item: UserNotification) => {
  return item.type !== 'like' && Number(item.comment_id || 0) > 0 && Number(item.message_id || 0) > 0
}

const formatTime = (value?: string) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

const initialMessageId = () => Number(props.initialMessageId || 0)
const initialCommentId = () => Number(props.initialCommentId || 0)
const hasInitialTarget = () => initialMessageId() > 0 || initialCommentId() > 0
const matchesInitialTarget = (item: UserNotification) => {
  const messageId = initialMessageId()
  const commentId = initialCommentId()
  if (commentId > 0) return Number(item.comment_id || 0) === commentId
  return messageId > 0 && Number(item.message_id || 0) === messageId
}

const selectInitialNotification = async () => {
  if (!hasInitialTarget()) return false
  if (selected.value && matchesInitialTarget(selected.value)) return true
  const matched = items.value.find(matchesInitialTarget)
  if (!matched) return false
  await openNotification(matched)
  return true
}

const resolveInitialTargetAcrossPages = async () => {
  if (!hasInitialTarget() || resolvingInitialTarget.value) return
  resolvingInitialTarget.value = true
  try {
    let found = await selectInitialNotification()
    while (!found && items.value.length < total.value) {
      const beforeCount = items.value.length
      page.value += 1
      await loadNotifications(false, { skipInitialResolve: true })
      if (items.value.length <= beforeCount) break
      found = await selectInitialNotification()
    }
  } finally {
    resolvingInitialTarget.value = false
  }
}

const loadNotifications = async (reset = false, options: { skipInitialResolve?: boolean } = {}) => {
  if (!user.isLogin) return
  if (reset) page.value = 1
  loading.value = reset || !items.value.length
  try {
    const res = await getRequest<NotificationListPayload>('notifications', { page: page.value, pageSize }, { credentials: 'include', silent: true })
    if (res?.code === 1 && res.data) {
      const payload = res.data
      const nextItems = Array.isArray(payload.items) ? payload.items : []
      items.value = page.value === 1 ? nextItems : [...items.value, ...nextItems]
      total.value = Number(payload.total ?? items.value.length)
      setUnreadCount(Number(payload.unread_count ?? payload.unreadCount ?? 0))
      if (!options.skipInitialResolve) await resolveInitialTargetAcrossPages()
    }
  } finally {
    loading.value = false
  }
}

const loadMore = async () => {
  if (loadingMore.value || items.value.length >= total.value) return
  loadingMore.value = true
  try {
    page.value += 1
    await loadNotifications(false)
  } finally {
    loadingMore.value = false
  }
}

const markRead = async (item: UserNotification) => {
  if (item.read) return
  const res = await putRequest<any>(`notifications/read/${item.id}`, {}, { credentials: 'include', silent: true })
  if (res?.code === 1) {
    item.read = true
    setUnreadCount(unreadCount.value - 1)
  }
}

const markAllRead = async () => {
  if (markingAll.value || unreadCount.value === 0) return
  markingAll.value = true
  try {
    const res = await putRequest<any>('notifications/read-all', {}, { credentials: 'include', silent: true })
    if (res?.code === 1) {
      items.value = items.value.map((item) => ({ ...item, read: true }))
      setUnreadCount(0)
    }
  } finally {
    markingAll.value = false
  }
}

const openNotification = async (item: UserNotification) => {
  selected.value = item
  await markRead(item)
  await nextTick()
  const commentId = Number(item.comment_id || 0)
  if (commentId > 0) await commentThreadRef.value?.focusCommentById(commentId)
}

const replyNotification = async (item: UserNotification) => {
  selected.value = item
  await markRead(item)
  await nextTick()
  const commentId = Number(item.comment_id || 0)
  if (commentId > 0) await commentThreadRef.value?.replyToCommentById(commentId)
}

watch(() => user.isLogin, (loggedIn) => {
  if (loggedIn) loadNotifications(true)
  else {
    items.value = []
    selected.value = null
    total.value = 0
    setUnreadCount(0)
  }
})

watch(() => [props.initialMessageId, props.initialCommentId], () => resolveInitialTargetAcrossPages())

onMounted(() => loadNotifications(true))

defineExpose({ refresh: () => loadNotifications(true) })
</script>

<style scoped>
.notification-center { width: 100%; }
.notification-header { display:flex; align-items:center; justify-content:space-between; gap:16px; margin-bottom:16px; }
.notification-title { margin:0; font-size:20px; line-height:1.3; font-weight:700; }
.notification-subtitle { margin:4px 0 0; font-size:13px; opacity:.68; }
.notification-actions { display:flex; align-items:center; gap:10px; flex-wrap:wrap; justify-content:flex-end; }
.unread-pill { display:inline-flex; align-items:center; min-height:28px; padding:0 10px; border-radius:999px; font-size:12px; font-weight:600; color:#fff; background:#2563eb; }
.icon-action, .text-action, .inline-action { border:1px solid rgba(148,163,184,.36); background:rgba(255,255,255,.72); color:inherit; }
.icon-action { width:32px; height:32px; display:inline-flex; align-items:center; justify-content:center; border-radius:8px; }
.text-action { min-height:32px; padding:0 12px; border-radius:8px; font-size:13px; }
.inline-action { min-height:28px; padding:0 10px; border-radius:7px; font-size:12px; }
.inline-action.strong { border-color:rgba(37,99,235,.42); color:#1d4ed8; }
.icon-action:disabled, .text-action:disabled { opacity:.5; cursor:not-allowed; }
.notification-shell { display:grid; grid-template-columns:minmax(0, 1fr) minmax(320px, .86fr); gap:16px; align-items:start; }
.notification-list-panel, .notification-detail-card, .empty-state { border:1px solid rgba(148,163,184,.24); background:rgba(255,255,255,.66); backdrop-filter:blur(12px); border-radius:8px; }
.notification-list-panel { padding:12px; min-height:260px; }
.notification-masonry { column-count:2; column-gap:12px; }
.notification-card { break-inside:avoid; margin:0 0 12px; padding:12px; border:1px solid rgba(148,163,184,.26); border-radius:8px; background:rgba(255,255,255,.86); cursor:pointer; transition:border-color .16s ease, box-shadow .16s ease, transform .16s ease; }
.notification-card:hover { border-color:rgba(37,99,235,.38); box-shadow:0 8px 24px rgba(15,23,42,.08); transform:translateY(-1px); }
.notification-card.unread { border-color:rgba(37,99,235,.42); background:rgba(239,246,255,.92); }
.notification-card.active { box-shadow:0 0 0 2px rgba(37,99,235,.22); }
.notification-card-top { display:flex; align-items:flex-start; gap:10px; }
.notification-icon-wrap { width:32px; height:32px; flex:0 0 32px; display:inline-flex; align-items:center; justify-content:center; border-radius:8px; background:#e0f2fe; color:#0369a1; }
.notification-icon-wrap.reply { background:#ecfdf5; color:#047857; }
.notification-icon-wrap.guestbook { background:#fef3c7; color:#92400e; }
.notification-icon-wrap.like { background:#ffe4e6; color:#be123c; }
.notification-card-title { min-width:0; flex:1; }
.notification-title-line { font-size:14px; font-weight:700; line-height:1.4; word-break:break-word; }
.notification-time, .detail-meta { font-size:12px; opacity:.62; margin-top:2px; }
.unread-dot { width:8px; height:8px; border-radius:999px; background:#2563eb; margin-top:4px; }
.notification-primary { margin:10px 0 0; font-size:14px; line-height:1.55; word-break:break-word; }
.notification-message { margin:8px 0 0; padding:8px; border-left:3px solid rgba(148,163,184,.42); background:rgba(248,250,252,.8); border-radius:6px; font-size:12px; line-height:1.5; opacity:.82; word-break:break-word; }
.notification-card-actions { display:flex; justify-content:flex-end; gap:8px; margin-top:12px; }
.notification-detail-panel { min-width:0; }
.notification-detail-card { padding:14px; }
.detail-head { display:flex; align-items:flex-start; gap:10px; margin-bottom:12px; }
.detail-title { font-size:15px; line-height:1.4; font-weight:700; word-break:break-word; }
.detail-message { margin-bottom:12px; padding:10px; border:1px solid rgba(148,163,184,.24); border-radius:8px; background:rgba(248,250,252,.68); font-size:13px; line-height:1.55; }
.detail-label { font-size:12px; opacity:.62; margin-bottom:4px; }
.empty-state { min-height:220px; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:8px; text-align:center; padding:24px; }
.empty-state.compact { min-height:180px; border:0; background:transparent; }
.empty-state.detail-empty { min-height:260px; }
.empty-icon { width:28px; height:28px; opacity:.62; }
.empty-title { font-size:14px; font-weight:600; opacity:.72; }
.load-more-row { display:flex; justify-content:center; padding:8px 0 2px; }
.spin { animation:notification-spin 1s linear infinite; }
@keyframes notification-spin { to { transform:rotate(360deg); } }
:global(.dark) .icon-action,
:global(.dark) .text-action,
:global(.dark) .inline-action { background:rgba(15,23,42,.72); border-color:rgba(148,163,184,.28); }
:global(.dark) .notification-list-panel,
:global(.dark) .notification-detail-card,
:global(.dark) .empty-state { background:rgba(15,23,42,.64); border-color:rgba(148,163,184,.22); }
:global(.dark) .notification-card { background:rgba(15,23,42,.82); border-color:rgba(148,163,184,.22); }
:global(.dark) .notification-card.unread { background:rgba(30,58,138,.32); border-color:rgba(96,165,250,.4); }
:global(.dark) .notification-message,
:global(.dark) .detail-message { background:rgba(15,23,42,.54); border-color:rgba(148,163,184,.22); }
@media (max-width: 1100px) {
  .notification-shell { grid-template-columns:1fr; }
  .notification-detail-panel { order:-1; }
}
@media (max-width: 720px) {
  .notification-header { align-items:flex-start; flex-direction:column; }
  .notification-actions { justify-content:flex-start; }
  .notification-masonry { column-count:1; }
}
</style>