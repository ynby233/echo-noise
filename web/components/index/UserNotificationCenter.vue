<template>
  <section class="notification-center">
    <div class="notification-header">
      <div>
        <h2 class="notification-title">空间消息</h2>
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

    <div v-else class="notification-feed-panel">
      <div v-if="loading && !items.length" class="empty-state compact">
        <UIcon name="i-mdi-loading" class="empty-icon spin" />
        <div class="empty-title">正在加载</div>
      </div>

      <div v-else-if="!items.length" class="empty-state compact">
        <UIcon name="i-mdi-bell-outline" class="empty-icon" />
        <div class="empty-title">暂无通知</div>
      </div>

      <div v-else ref="feedRef" class="notification-feed">
        <article
          v-for="item in items"
          :key="item.id"
          class="notification-feed-item"
          :class="{ unread: !item.read, highlighted: highlightedId === item.id }"
          :data-notification-id="item.id"
        >
          <img :src="actorAvatar(item)" class="notification-avatar" :alt="actorName(item)" loading="lazy" @error="onAvatarError" />
          <div class="notification-item-body">
            <div class="notification-item-head">
              <div class="notification-actor-block">
                <div class="notification-actor-line">
                  <span class="notification-actor-name">{{ actorName(item) }}</span>
                  <span v-if="item.type === 'like'" class="like-action-inline">赞了</span>
                </div>
                <div class="notification-time">{{ formatTime(item.created_at) }}</div>
              </div>
              <button
                v-if="canReply(item)"
                type="button"
                class="reply-toggle"
                @click="toggleReply(item)"
              >
                回复
              </button>
              <UIcon v-else-if="item.type === 'like'" name="i-mdi-hand-heart-outline" class="like-corner-icon" />
            </div>

            <p v-if="actorContent(item)" class="notification-actor-content">{{ actorContent(item) }}</p>

            <button type="button" class="notification-target-card" @click="jumpToTarget(item)">
              <img v-if="targetImage(item)" :src="targetImage(item)" class="notification-target-image" alt="通知关联图片" loading="lazy" />
              <div class="notification-target-text">
                <span v-if="targetOwner(item)" class="target-owner">{{ targetOwner(item) }}：</span>{{ targetText(item) || targetFallbackText(item) }}
              </div>
            </button>

            <div v-if="replyOpenId === item.id" class="inline-reply-box">
              <textarea
                v-model="replyDrafts[item.id]"
                class="inline-reply-input"
                :placeholder="`回复${actorName(item)}：`"
                rows="2"
                @keydown.ctrl.enter.prevent="submitInlineReply(item)"
                @keydown.meta.enter.prevent="submitInlineReply(item)"
              ></textarea>
              <div class="inline-reply-actions">
                <span class="inline-reply-hint">Ctrl / ⌘ + Enter 发送</span>
                <button type="button" class="inline-reply-cancel" :disabled="replySubmitting[item.id]" @click="replyOpenId = null">取消</button>
                <button type="button" class="inline-reply-submit" :disabled="replySubmitting[item.id]" @click="submitInlineReply(item)">
                  {{ replySubmitting[item.id] ? '发送中' : '发送' }}
                </button>
              </div>
            </div>
          </div>
        </article>
      </div>

      <div v-if="items.length < total" class="load-more-row">
        <button type="button" class="text-action" :disabled="loadingMore" @click="loadMore">
          {{ loadingMore ? '加载中' : '加载更多' }}
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useUserStore } from '~/store/user'
import { getRequest, postRequest, putRequest } from '~/utils/api'
import { resolveMediaURL } from '~/utils/media-url'

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
  username?: string
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

const props = defineProps<{ siteConfig?: any, initialMessageId?: number | null, initialCommentId?: number | null }>()
const emit = defineEmits<{
  (event: 'unread-change', count: number): void
  (event: 'jump', item: UserNotification): void
}>()

const user = useUserStore()
const items = ref<UserNotification[]>([])
const feedRef = ref<HTMLElement | null>(null)
const loading = ref(false)
const loadingMore = ref(false)
const markingAll = ref(false)
const page = ref(1)
const pageSize = 20
const total = ref(0)
const unreadCount = ref(0)
const resolvingInitialTarget = ref(false)
const highlightedId = ref<number | null>(null)
const replyOpenId = ref<number | null>(null)
const replyDrafts = ref<Record<number, string>>({})
const replySubmitting = ref<Record<number, boolean>>({})

const runtimeConfig = useRuntimeConfig()
const baseApi = computed(() => runtimeConfig.public.baseApi || '/api')

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

const truncate = (value?: string | null, limit = 120) => {
  const text = normalizeText(value)
  if (text.length <= limit) return text
  return `${text.slice(0, limit)}...`
}

const actorName = (item: UserNotification) => {
  const name = String(item.actor?.username || '').trim()
  return name || '有用户'
}

const messageOwnerName = (item: UserNotification) => {
  const name = String(item.message?.username || '').trim()
  return name || ''
}

const actorContent = (item: UserNotification) => {
  if (item.type === 'like' || item.type === 'guestbook') return ''
  return truncate(item.comment?.content || '', 160)
}

const targetText = (item: UserNotification) => {
  if (item.type === 'guestbook') return truncate(item.comment?.content || '', 160)
  if (item.type === 'reply') return truncate(item.parent_comment?.content || item.message?.content || '', 160)
  return truncate(item.message?.content || '', 160)
}

const targetOwner = (item: UserNotification) => {
  if (item.type === 'reply') return item.parent_comment?.user?.username || ''
  if (item.type === 'guestbook') return actorName(item)
  return messageOwnerName(item)
}

const targetFallbackText = (item: UserNotification) => {
  if (item.type === 'like') return '查看被点赞的笔记'
  if (item.type === 'reply') return '查看被回复的内容'
  if (item.type === 'guestbook') return '查看留言'
  return '查看被评论的笔记'
}

const targetImage = (item: UserNotification) => {
  const raw = String(item.message?.image_url || '').trim()
  return raw ? resolveMediaURL(baseApi.value, raw) : ''
}

const canReply = (item: UserNotification) => {
  return item.type !== 'like' && Number(item.comment_id || 0) > 0 && Number(item.message_id || 0) > 0
}

const formatTime = (value?: string) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

const genericAvatar = () => {
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 96 96"><rect width="96" height="96" rx="48" fill="#94a3b8"/><circle cx="48" cy="36" r="18" fill="#e2e8f0"/><path d="M22 80c4-18 18-30 26-30s22 12 26 30" fill="#e2e8f0"/></svg>'
  return `data:image/svg+xml;charset=UTF-8,${encodeURIComponent(svg)}`
}

const actorAvatar = (item: UserNotification) => {
  const raw = String(item.actor?.avatar_url || '').trim()
  if (raw) return resolveMediaURL(baseApi.value, raw)
  const siteAvatar = String(props.siteConfig?.avatarURL || '').trim()
  return siteAvatar ? resolveMediaURL(baseApi.value, siteAvatar) : genericAvatar()
}

const onAvatarError = (event: Event) => {
  const img = event.target as HTMLImageElement | null
  if (img) img.src = genericAvatar()
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

const focusNotificationItem = async (item: UserNotification) => {
  highlightedId.value = item.id
  await markRead(item)
  await nextTick()
  const el = feedRef.value?.querySelector(`[data-notification-id="${item.id}"]`) as HTMLElement | null
  el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  window.setTimeout(() => {
    if (highlightedId.value === item.id) highlightedId.value = null
  }, 2200)
}

const selectInitialNotification = async () => {
  if (!hasInitialTarget()) return false
  const matched = items.value.find(matchesInitialTarget)
  if (!matched) return false
  await focusNotificationItem(matched)
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

const jumpToTarget = async (item: UserNotification) => {
  await markRead(item)
  highlightedId.value = item.id
  emit('jump', item)
}

const toggleReply = async (item: UserNotification) => {
  if (!canReply(item)) return
  await markRead(item)
  replyOpenId.value = replyOpenId.value === item.id ? null : item.id
  if (replyOpenId.value === item.id && replyDrafts.value[item.id] === undefined) replyDrafts.value[item.id] = ''
}

const submitInlineReply = async (item: UserNotification) => {
  if (!canReply(item) || replySubmitting.value[item.id]) return
  const content = String(replyDrafts.value[item.id] || '').trim()
  if (!content) {
    useToast().add({ title: '回复内容不能为空', color: 'orange' })
    return
  }
  const messageId = Number(item.message_id || 0)
  const commentId = Number(item.comment_id || 0)
  replySubmitting.value = { ...replySubmitting.value, [item.id]: true }
  try {
    const res = await postRequest<any>(`messages/${messageId}/comments`, { content, parent_id: commentId }, { credentials: 'include' })
    if (res?.code === 1) {
      replyDrafts.value[item.id] = ''
      replyOpenId.value = null
      useToast().add({ title: '已回复', color: 'green' })
      await loadNotifications(true)
    } else {
      useToast().add({ title: '回复失败', description: res?.msg, color: 'red' })
    }
  } catch {
    useToast().add({ title: '回复失败', color: 'red' })
  } finally {
    replySubmitting.value = { ...replySubmitting.value, [item.id]: false }
  }
}

watch(() => user.isLogin, (loggedIn) => {
  if (loggedIn) loadNotifications(true)
  else {
    items.value = []
    total.value = 0
    replyOpenId.value = null
    setUnreadCount(0)
  }
})

watch(() => [props.initialMessageId, props.initialCommentId], () => resolveInitialTargetAcrossPages())

onMounted(() => loadNotifications(true))

defineExpose({ refresh: () => loadNotifications(true) })
</script>

<style scoped>
.notification-center { width:100%; }
.notification-header { display:flex; align-items:center; justify-content:space-between; gap:16px; margin-bottom:14px; }
.notification-title { margin:0; font-size:20px; line-height:1.3; font-weight:700; }
.notification-subtitle { margin:4px 0 0; font-size:13px; opacity:.68; }
.notification-actions { display:flex; align-items:center; gap:10px; flex-wrap:wrap; justify-content:flex-end; }
.unread-pill { display:inline-flex; align-items:center; min-height:28px; padding:0 10px; border-radius:999px; font-size:12px; font-weight:600; color:#fff; background:#2563eb; }
.icon-action, .text-action { border:1px solid rgba(148,163,184,.36); background:rgba(255,255,255,.72); color:inherit; }
.icon-action { width:32px; height:32px; display:inline-flex; align-items:center; justify-content:center; border-radius:8px; }
.text-action { min-height:32px; padding:0 12px; border-radius:8px; font-size:13px; }
.icon-action:disabled, .text-action:disabled { opacity:.5; cursor:not-allowed; }
.notification-feed-panel, .empty-state { border:1px solid rgba(148,163,184,.22); background:rgba(255,255,255,.72); backdrop-filter:blur(12px); border-radius:10px; }
.notification-feed-panel { padding:0; overflow:hidden; }
.notification-feed { display:flex; flex-direction:column; }
.notification-feed-item { display:flex; gap:14px; padding:18px 18px 20px; border-bottom:1px solid rgba(148,163,184,.18); transition:background .16s ease, box-shadow .16s ease; }
.notification-feed-item:last-child { border-bottom:0; }
.notification-feed-item.unread { background:rgba(239,246,255,.66); }
.notification-feed-item.highlighted { box-shadow:inset 4px 0 0 rgba(37,99,235,.72); background:rgba(219,234,254,.72); }
.notification-avatar { width:44px; height:44px; flex:0 0 44px; border-radius:999px; object-fit:cover; background:#e2e8f0; }
.notification-item-body { min-width:0; flex:1; }
.notification-item-head { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; min-height:44px; }
.notification-actor-block { min-width:0; }
.notification-actor-line { display:flex; align-items:center; gap:8px; min-width:0; }
.notification-actor-name { font-size:17px; line-height:1.25; font-weight:700; color:#0f172a; word-break:break-word; }
.like-action-inline { font-size:14px; color:#64748b; }
.notification-time { margin-top:3px; font-size:13px; line-height:1.2; color:#8a94a6; }
.reply-toggle { flex:0 0 auto; border:0; background:transparent; color:#0f3f75; font-size:15px; line-height:1.4; font-weight:700; padding:2px 0 2px 10px; cursor:pointer; }
.reply-toggle:hover { color:#2563eb; }
.like-corner-icon { width:24px; height:24px; color:#64748b; }
.notification-actor-content { margin:12px 0 0; font-size:15px; line-height:1.72; white-space:pre-wrap; word-break:break-word; color:#0f172a; }
.notification-target-card { width:100%; margin-top:14px; padding:12px 14px; border:0; border-radius:4px; background:#f1f5f9; color:#0f172a; display:flex; align-items:center; gap:12px; text-align:left; cursor:pointer; transition:background .16s ease, transform .16s ease; }
.notification-target-card:hover { background:#e9eef5; transform:translateY(-1px); }
.notification-target-image { width:64px; height:64px; flex:0 0 64px; object-fit:cover; border-radius:2px; background:#e2e8f0; }
.notification-target-text { min-width:0; font-size:15px; line-height:1.6; word-break:break-word; }
.target-owner { color:#0f3f75; font-weight:700; }
.inline-reply-box { margin-top:14px; }
.inline-reply-input { width:100%; min-height:42px; resize:vertical; border:1px solid rgba(148,163,184,.24); border-radius:5px; background:#f3f6fb; color:#0f172a; padding:10px 12px; line-height:1.5; outline:none; }
.inline-reply-input:focus { border-color:rgba(37,99,235,.42); box-shadow:0 0 0 3px rgba(37,99,235,.1); }
.inline-reply-actions { display:flex; align-items:center; justify-content:flex-end; gap:10px; margin-top:8px; }
.inline-reply-hint { margin-right:auto; font-size:12px; color:#94a3b8; }
.inline-reply-cancel, .inline-reply-submit { min-height:30px; border-radius:7px; padding:0 12px; font-size:13px; }
.inline-reply-cancel { border:1px solid rgba(148,163,184,.3); background:rgba(255,255,255,.72); color:inherit; }
.inline-reply-submit { border:1px solid rgba(37,99,235,.44); background:#2563eb; color:#fff; }
.inline-reply-cancel:disabled, .inline-reply-submit:disabled { opacity:.55; cursor:not-allowed; }
.empty-state { min-height:220px; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:8px; text-align:center; padding:24px; }
.empty-state.compact { min-height:220px; border:0; background:transparent; }
.empty-icon { width:28px; height:28px; opacity:.62; }
.empty-title { font-size:14px; font-weight:600; opacity:.72; }
.load-more-row { display:flex; justify-content:center; padding:14px 0 16px; border-top:1px solid rgba(148,163,184,.16); }
.spin { animation:notification-spin 1s linear infinite; }
@keyframes notification-spin { to { transform:rotate(360deg); } }
:global(.dark) .icon-action,
:global(.dark) .text-action,
:global(.dark) .inline-reply-cancel { background:rgba(15,23,42,.72); border-color:rgba(148,163,184,.28); }
:global(.dark) .notification-feed-panel,
:global(.dark) .empty-state { background:rgba(15,23,42,.64); border-color:rgba(148,163,184,.22); }
:global(.dark) .notification-feed-item { border-color:rgba(148,163,184,.16); }
:global(.dark) .notification-feed-item.unread { background:rgba(30,58,138,.24); }
:global(.dark) .notification-feed-item.highlighted { background:rgba(30,58,138,.34); box-shadow:inset 4px 0 0 rgba(96,165,250,.72); }
:global(.dark) .notification-actor-name,
:global(.dark) .notification-actor-content,
:global(.dark) .notification-target-card { color:#e5e7eb; }
:global(.dark) .notification-time,
:global(.dark) .like-action-inline { color:#94a3b8; }
:global(.dark) .reply-toggle,
:global(.dark) .target-owner { color:#93c5fd; }
:global(.dark) .notification-target-card { background:rgba(30,41,59,.86); }
:global(.dark) .notification-target-card:hover { background:rgba(51,65,85,.88); }
:global(.dark) .inline-reply-input { background:rgba(15,23,42,.72); border-color:rgba(148,163,184,.26); color:#e5e7eb; }
@media (max-width: 720px) {
  .notification-header { align-items:flex-start; flex-direction:column; }
  .notification-actions { justify-content:flex-start; }
  .notification-feed-item { gap:10px; padding:15px 12px 18px; }
  .notification-avatar { width:40px; height:40px; flex-basis:40px; }
  .notification-actor-name { font-size:16px; }
  .notification-target-card { padding:10px 12px; }
  .notification-target-image { width:56px; height:56px; flex-basis:56px; }
}
</style>
