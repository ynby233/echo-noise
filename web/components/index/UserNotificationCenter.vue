<template>
  <section class="notification-center" :class="{ 'notification-theme-dark': isDark }">
    <div class="notification-header">
      <div class="notification-heading">
        <div class="notification-title-row">
          <h2 class="notification-title">{{ notificationTitle }}</h2>
        </div>
        <p class="notification-subtitle">{{ notificationDescription }}</p>
      </div>
    </div>

    <div v-if="!user.isLogin" class="empty-state">
      <UIcon name="i-mdi-bell-off-outline" class="empty-icon" />
      <div class="empty-title">登录后查看通知</div>
    </div>

    <div v-else class="notification-feed-panel notification-board-wrap">
      <div class="notification-board-head">
        <div class="notification-count-title">通知 ({{ total }})</div>
        <div class="notification-actions">
          <span v-if="unreadCount > 0" class="unread-pill">{{ unreadCount }} 未读</span>
          <button type="button" class="notification-refresh-button nw-action-btn nw-tooltip-anchor" data-tooltip="刷新" aria-label="刷新" :disabled="notificationRefreshing || loading" @click="refreshNotifications">
            <UIcon name="i-mdi-refresh" class="w-4 h-4" :class="{ 'animate-spin': notificationRefreshing }" />
          </button>
          <button type="button" class="notification-text-button nw-action-btn nw-action-btn--label" :disabled="markingAll || unreadCount === 0" @click="markAllRead">全部已读</button>
        </div>
      </div>
      <div v-if="loading && !items.length" class="empty-state compact">
        <UIcon name="i-mdi-loading" class="empty-icon spin" />
        <div class="empty-title">正在加载</div>
      </div>

      <div v-else-if="loadError && !items.length" class="empty-state compact error-state">
        <UIcon name="i-mdi-alert-circle-outline" class="empty-icon" />
        <div class="empty-title">{{ loadError }}</div>
        <button type="button" class="notification-text-button nw-action-btn nw-action-btn--label" @click="loadNotifications(true)">重试</button>
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
                class="reply-toggle nw-action-btn nw-action-btn--label"
                :aria-expanded="replyOpenId === item.id"
                :aria-controls="`notification-reply-${item.id}`"
                @click="toggleReply(item)"
              >
                {{ replyOpenId === item.id ? '收起' : '回复' }}
              </button>
            </div>

            <p v-if="actorContent(item)" class="notification-actor-content">{{ actorContent(item) }}</p>

            <button
              type="button"
              class="notification-target-card"
              :class="{ jumping: jumpingId === item.id }"
              :aria-label="targetAriaLabel(item)"
              :disabled="jumpingId === item.id"
              @click="jumpToTarget(item)"
            >
              <img v-if="targetImage(item)" :src="targetImage(item)" class="notification-target-image" alt="通知关联图片" loading="lazy" />
              <div class="notification-target-text">
                <span v-if="targetOwner(item)" class="target-owner">{{ targetOwner(item) }}：</span>{{ targetText(item) || targetFallbackText(item) }}
              </div>
              <span v-if="jumpingId === item.id" class="notification-target-jumping" aria-live="polite">
                <UIcon name="i-mdi-loading" class="notification-target-jumping-icon" />
                正在跳转
              </span>
            </button>

            <div v-if="replyOpenId === item.id" :id="`notification-reply-${item.id}`" class="inline-reply-box">
              <BuiltinComments
                :key="`notification-reply-thread-${item.id}-${replyCommentId(item) || 0}`"
                :message-id="targetMessageId(item)"
                :message-visibility="item.message?.visibility"
                :site-config="props.siteConfig"
                :show-input="true"
                :reply-input-only="true"
                :reply-comment-id="replyCommentId(item)"
                :reply-comment-author="replyCommentAuthor(item)"
                :context-label="item.type === 'guestbook' ? '留言' : '评论'"
                auto-scroll-input
                @cancel="replyOpenId = null"
              />
            </div>
          </div>
        </article>
      </div>

      <div v-if="loadError && items.length" class="notification-feed-error" role="status">
        <span>{{ loadError }}</span>
        <button type="button" class="notification-text-button nw-action-btn nw-action-btn--label" @click="loadNotifications(false)">重试</button>
      </div>

      <div v-if="items.length" class="pager-shell" :class="{ 'is-dark': isDark }">
        <div class="pager-nav-group">
          <button
            v-if="page > 1"
            type="button"
            class="pager-btn nw-action-btn nw-action-btn--label"
            :disabled="loading"
            @click="loadPreviousPage"
          >
            <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-left" class="w-4 h-4 pager-icon" /></span>
            <span>上一页</span>
          </button>
          <button
            v-if="page < totalPages"
            type="button"
            class="pager-btn nw-action-btn nw-action-btn--label"
            :disabled="loading"
            @click="loadNextPage"
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
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useUserStore } from '~/store/user'
import { getRequest, putRequest } from '~/utils/api'
import { resolveMediaURL } from '~/utils/media-url'
import BuiltinComments from '~/components/comments/BuiltinComments.vue'
import { useToast } from '#ui/composables/useToast'

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
  message_id?: number | null
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

type NotificationJumpPayload = UserNotification & {
  target_message_id?: number | null
  target_comment_id?: number | null
}

type NotificationListPayload = {
  items?: UserNotification[]
  total?: number
  unread_count?: number
  unreadCount?: number
  page?: number
  pageSize?: number
}

const props = defineProps<{ siteConfig?: any, initialMessageId?: number | null, initialCommentId?: number | null, restoreFocusId?: number | null }>()
const emit = defineEmits<{
  (event: 'unread-change', count: number): void
  (event: 'jump', item: NotificationJumpPayload): void
  (event: 'restore-consumed'): void
}>()

const notificationTitle = computed(() => String(props.siteConfig?.notificationPageTitle || '').trim() || '通知')
const notificationDescription = computed(() => String(props.siteConfig?.notificationPageDescription || '').trim() || '欢迎彼此间互相交流')

const user = useUserStore()
const injectedTheme = inject('contentTheme', ref('light')) as any
const isDark = computed(() => {
  const value = injectedTheme && typeof injectedTheme.value !== 'undefined' ? injectedTheme.value : injectedTheme
  return String(value || 'light') === 'dark'
})
const items = ref<UserNotification[]>([])
const feedRef = ref<HTMLElement | null>(null)
const loading = ref(false)
const notificationRefreshing = ref(false)
const markingAll = ref(false)
const loadError = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
const targetPage = ref('1')
const unreadCount = ref(0)
const resolvingInitialTarget = ref(false)
const resolvingRestoreFocus = ref(false)
const highlightedId = ref<number | null>(null)
const replyOpenId = ref<number | null>(null)
const jumpingId = ref<number | null>(null)
let jumpFeedbackTimer: ReturnType<typeof setTimeout> | null = null

const runtimeConfig = useRuntimeConfig()
const baseApi = computed(() => runtimeConfig.public.baseApi || '/api')

const setUnreadCount = (count: number) => {
  unreadCount.value = Math.max(0, Number(count || 0))
  emit('unread-change', unreadCount.value)
}

const syncTargetPageToCurrent = () => {
  const next = Math.min(Math.max(Number(page.value) || 1, 1), totalPages.value)
  targetPage.value = String(next)
}

const normalizeTargetPage = (fallback = page.value) => {
  const parsed = Number.parseInt(targetPage.value.trim() || '', 10)
  const next = Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
  return Math.min(Math.max(next, 1), totalPages.value)
}

const adjustTargetPage = (delta: number) => {
  targetPage.value = String(normalizeTargetPage(page.value) + delta)
  targetPage.value = String(normalizeTargetPage(page.value))
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
  if (item.type === 'reply') return '查看对应回复'
  if (item.type === 'guestbook') return '查看留言'
  return '查看被评论的笔记'
}

const targetAriaLabel = (item: UserNotification) => {
  if (item.type === 'guestbook') return '查看对应留言位置'
  if (item.type === 'reply') return '查看对应回复位置'
  if (item.type === 'like') return '查看被点赞笔记位置'
  return '查看被评论笔记位置'
}

const targetImage = (item: UserNotification) => {
  const raw = String(item.message?.image_url || '').trim()
  return raw ? resolveMediaURL(baseApi.value, raw) : ''
}

const parseTargetUrlNumber = (item: UserNotification, key: 'message_id' | 'comment_id') => {
  const raw = String(item.target_url || '').trim()
  if (!raw) return 0
  try {
    const url = new URL(raw, typeof window !== 'undefined' ? window.location.origin : 'http://localhost')
    const n = Number(url.searchParams.get(key) || 0)
    return Number.isFinite(n) && n > 0 ? n : 0
  } catch {
    const match = raw.match(new RegExp(`[?&]${key}=([0-9]+)`))
    const n = Number(match?.[1] || 0)
    return Number.isFinite(n) && n > 0 ? n : 0
  }
}

const targetMessageId = (item: UserNotification) => Number(item.message_id || item.comment?.message_id || item.parent_comment?.message_id || item.message?.id || parseTargetUrlNumber(item, 'message_id') || 0)
const targetCommentId = (item: UserNotification) => {
  if (item.type === 'like') return 0
  return Number(item.comment_id || item.comment?.id || parseTargetUrlNumber(item, 'comment_id') || 0)
}
const replyCommentId = (item: UserNotification) => {
  if (item.type === 'reply') {
    return Number(item.parent_comment_id || item.parent_comment?.id || item.comment?.parent_id || item.comment_id || item.comment?.id || 0)
  }
  return targetCommentId(item)
}
const replyCommentAuthor = (item: UserNotification) => {
  if (item.type === 'reply') return item.parent_comment?.user?.username || item.comment?.user?.username || actorName(item)
  return item.comment?.user?.username || actorName(item)
}

const canReply = (item: UserNotification) => {
  return item.type !== 'like' && replyCommentId(item) > 0 && targetMessageId(item) > 0
}
const jumpPayload = (item: UserNotification): NotificationJumpPayload => ({
  ...item,
  target_message_id: targetMessageId(item),
  target_comment_id: targetCommentId(item) || null
})

const scrollInlineReplyIntoView = async (itemId: number) => {
  await nextTick()
  const box = feedRef.value?.querySelector(`#notification-reply-${itemId}`) as HTMLElement | null
  if (box) scrollElementToAppCenter(box)
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
const restoreFocusId = () => Number(props.restoreFocusId || 0)
const hasInitialTarget = () => initialMessageId() > 0 || initialCommentId() > 0
const matchesInitialTarget = (item: UserNotification) => {
  const messageId = initialMessageId()
  const commentId = initialCommentId()
  if (commentId > 0) {
    return targetCommentId(item) === commentId
  }
  return messageId > 0 && targetMessageId(item) === messageId
}

const scrollElementToAppCenter = (el: HTMLElement) => {
  if (typeof document === 'undefined') return
  const isScrollableY = (target: HTMLElement | null) => {
    if (!target || typeof window === 'undefined') return false
    const style = window.getComputedStyle(target)
    return /(auto|scroll|overlay)/.test(`${style.overflowY || ''} ${style.overflow || ''}`) && target.scrollHeight > target.clientHeight
  }
  const candidates = [
    el.closest('.center-col') as HTMLElement | null,
    el.closest('.content-wrapper') as HTMLElement | null,
    document.querySelector('.content-wrapper') as HTMLElement | null,
    document.querySelector('.center-col') as HTMLElement | null,
  ]
  const wrapper = candidates.find(isScrollableY) || candidates.find(Boolean) || null
  if (!wrapper) {
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    return
  }
  const wrapperRect = wrapper.getBoundingClientRect()
  const elRect = el.getBoundingClientRect()
  const targetTop = wrapper.scrollTop + elRect.top - wrapperRect.top - Math.max(24, (wrapper.clientHeight - elRect.height) / 2)
  wrapper.scrollTo({ top: Math.max(0, targetTop), behavior: 'smooth' })
}

const focusNotificationItem = async (item: UserNotification) => {
  highlightedId.value = item.id
  await markRead(item)
  await nextTick()
  const el = feedRef.value?.querySelector(`[data-notification-id="${item.id}"]`) as HTMLElement | null
  if (el) scrollElementToAppCenter(el)
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

const selectRestoreNotification = async () => {
  const id = restoreFocusId()
  if (!id || hasInitialTarget()) return false
  const matched = items.value.find((item) => Number(item.id) === id)
  if (!matched) return false
  await focusNotificationItem(matched)
  emit('restore-consumed')
  return true
}

const firstNotificationItem = () => {
  return feedRef.value?.querySelector<HTMLElement>('.notification-feed-item') || null
}

const scrollNotificationFirstBlockToTop = async () => {
  await nextTick()
  const target = firstNotificationItem() || feedRef.value
  if (!target) return
  scrollElementTopToAppTop(target)
}

const scrollElementTopToAppTop = (el: HTMLElement) => {
  if (typeof document === 'undefined') return
  const isScrollableTarget = (target: HTMLElement | null) => {
    if (!target || typeof window === 'undefined') return false
    const style = window.getComputedStyle(target)
    return /(auto|scroll|overlay)/.test(`${style.overflowY || ''} ${style.overflow || ''}`) && target.scrollHeight > target.clientHeight
  }
  const candidates = [
    el.closest('.center-col') as HTMLElement | null,
    el.closest('.content-wrapper') as HTMLElement | null,
    document.querySelector('.content-wrapper') as HTMLElement | null,
    document.querySelector('.center-col') as HTMLElement | null,
  ]
  const wrapper = candidates.find(isScrollableTarget) || candidates.find(Boolean) || null
  const rect = el.getBoundingClientRect()
  if (wrapper) {
    const wrapperRect = wrapper.getBoundingClientRect()
    wrapper.scrollTo({ top: Math.max(0, wrapper.scrollTop + rect.top - wrapperRect.top), behavior: 'instant' })
    return
  }
  window.scrollTo({
    top: Math.max(0, (window.scrollY || window.pageYOffset || 0) + rect.top),
    left: window.scrollX || window.pageXOffset || 0,
    behavior: 'instant',
  })
}

const resolveInitialTargetAcrossPages = async () => {
  if (!hasInitialTarget() || resolvingInitialTarget.value) return
  resolvingInitialTarget.value = true
  const startPage = page.value
  try {
    let found = await selectInitialNotification()
    while (!found && page.value < totalPages.value) {
      page.value += 1
      await loadNotifications(false, { skipInitialResolve: true })
      found = await selectInitialNotification()
    }
    if (!found && page.value !== startPage) {
      page.value = startPage
      await loadNotifications(false, { skipInitialResolve: true, skipRestoreResolve: true })
    }
  } finally {
    resolvingInitialTarget.value = false
  }
}

const resolveRestoreFocusAcrossPages = async () => {
  if (!restoreFocusId() || resolvingRestoreFocus.value || hasInitialTarget()) return
  resolvingRestoreFocus.value = true
  const startPage = page.value
  try {
    let found = await selectRestoreNotification()
    while (!found && page.value < totalPages.value) {
      page.value += 1
      await loadNotifications(false, { skipInitialResolve: true, skipRestoreResolve: true })
      found = await selectRestoreNotification()
    }
    if (!found && page.value !== startPage) {
      page.value = startPage
      await loadNotifications(false, { skipInitialResolve: true, skipRestoreResolve: true })
    }
  } finally {
    resolvingRestoreFocus.value = false
  }
}

const loadNotifications = async (reset = false, options: { skipInitialResolve?: boolean, skipRestoreResolve?: boolean } = {}) => {
  if (!user.isLogin) return
  if (reset) page.value = 1
  loading.value = true
  try {
    loadError.value = ''
    const res = await getRequest<NotificationListPayload>('notifications', { page: page.value, pageSize }, { credentials: 'include', silent: true })
    if (res?.code === 1 && res.data) {
      const payload = res.data
      const nextItems = Array.isArray(payload.items) ? payload.items : []
      items.value = nextItems
      total.value = Number(payload.total ?? items.value.length)
      page.value = Math.min(Math.max(Number(payload.page || page.value || 1), 1), totalPages.value)
      syncTargetPageToCurrent()
      setUnreadCount(Number(payload.unread_count ?? payload.unreadCount ?? 0))
      if (!options.skipInitialResolve) await resolveInitialTargetAcrossPages()
      if (!options.skipRestoreResolve) await resolveRestoreFocusAcrossPages()
    } else {
      loadError.value = String(res?.msg || '通知加载失败')
    }
  } catch {
    loadError.value = '通知加载失败'
  } finally {
    loading.value = false
  }
}

const refreshNotifications = async () => {
  if (notificationRefreshing.value || loading.value) return
  notificationRefreshing.value = true
  try {
    await loadNotifications(true)
  } finally {
    setTimeout(() => {
      notificationRefreshing.value = false
    }, 300)
  }
}

const loadPage = async (nextPage: number) => {
  if (loading.value) return
  const bounded = Math.min(Math.max(Number(nextPage) || 1, 1), totalPages.value)
  if (bounded === page.value && items.value.length) {
    syncTargetPageToCurrent()
    return
  }
  page.value = bounded
  await loadNotifications(false, { skipInitialResolve: true, skipRestoreResolve: true })
  await scrollNotificationFirstBlockToTop()
}

const loadPreviousPage = () => {
  if (page.value <= 1) return
  void loadPage(page.value - 1)
}

const loadNextPage = () => {
  if (page.value >= totalPages.value) return
  void loadPage(page.value + 1)
}

const jumpToPage = () => {
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
  void loadPage(next)
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
  if (jumpingId.value === item.id) return
  clearJumpFeedback()
  jumpingId.value = item.id
  await markRead(item)
  highlightedId.value = item.id
  emit('jump', jumpPayload(item))
  jumpFeedbackTimer = setTimeout(() => {
    if (jumpingId.value === item.id) jumpingId.value = null
    jumpFeedbackTimer = null
  }, 1600)
}

const clearJumpFeedback = () => {
  if (jumpFeedbackTimer) clearTimeout(jumpFeedbackTimer)
  jumpFeedbackTimer = null
  jumpingId.value = null
}

const toggleReply = async (item: UserNotification) => {
  if (!canReply(item)) return
  await markRead(item)
  const isOpening = replyOpenId.value !== item.id
  replyOpenId.value = isOpening ? item.id : null
  if (isOpening) await scrollInlineReplyIntoView(item.id)
}

watch(() => user.isLogin, (loggedIn) => {
  if (loggedIn) loadNotifications(true)
  else {
    items.value = []
    total.value = 0
    loadError.value = ''
    replyOpenId.value = null
    page.value = 1
    syncTargetPageToCurrent()
    clearJumpFeedback()
    setUnreadCount(0)
  }
})

watch(() => [props.initialMessageId, props.initialCommentId], () => resolveInitialTargetAcrossPages())
watch(page, syncTargetPageToCurrent, { immediate: true })
watch(totalPages, syncTargetPageToCurrent)

onMounted(() => loadNotifications(true))

onBeforeUnmount(() => {
  clearJumpFeedback()
})

defineExpose({ refresh: () => loadNotifications(true) })
</script>

<style scoped>
.notification-center {
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
  --notice-surface: var(--home-surface-light);
  --notice-card: #ffffff;
  --notice-card-hover: #f8fafc;
  --notice-input: #f8fafc;
  --notice-card-shadow: 0 14px 30px rgba(15,23,42,.12);
  --notice-text: #374151;
  --notice-strong: #111827;
  --notice-heading: #000000;
  --notice-count: #000000;
  --notice-muted: #6b7280;
  --notice-link: #2563eb;
  --comment-toolbar-bg: rgba(255, 255, 255, .86);
  --comment-toolbar-control-bg: rgba(15, 23, 42, .06);
  --comment-toolbar-control-hover-bg: rgba(15, 23, 42, .12);
  --comment-toolbar-border: rgba(15, 23, 42, .10);
  --comment-toolbar-text: #374151;
  --comment-toolbar-preview-border: rgba(15, 23, 42, .12);
  --comment-toolbar-preview-bg: rgba(15, 23, 42, .04);
  --nw-floating-bg: rgba(255, 255, 255, .98);
  --nw-floating-text: #111827;
  --nw-floating-border: rgba(15, 23, 42, .12);
  --nw-floating-shadow: 0 18px 42px rgba(15, 23, 42, .16);
  --nw-floating-hover-bg: rgba(249, 115, 22, .12);
  --nw-floating-hover-border: rgba(249, 115, 22, .36);
  --nw-floating-selected-bg: rgba(249, 115, 22, .18);
  --nw-floating-selected-border: rgba(249, 115, 22, .48);
}
:global(.dark) .notification-center,
.notification-center.notification-theme-dark {
  --notice-border: rgba(255,255,255,.14);
  --notice-surface: linear-gradient(180deg, rgba(30,41,59,.48) 0%, rgba(15,23,42,.82) 100%);
  --notice-card: rgba(15,23,42,.52);
  --notice-card-hover: rgba(30,41,59,.68);
  --notice-input: rgba(15,23,42,.36);
  --notice-card-shadow: 0 16px 32px rgba(2,6,23,.52);
  --notice-text: #cbd5e1;
  --notice-strong: #f8fafc;
  --notice-heading: #ffffff;
  --notice-count: #e5e7eb;
  --notice-muted: #94a3b8;
  --notice-link: #93c5fd;
  --comment-toolbar-bg: rgba(39, 50, 66, .68);
  --comment-toolbar-control-bg: rgba(255, 255, 255, .06);
  --comment-toolbar-control-hover-bg: rgba(255, 255, 255, .12);
  --comment-toolbar-border: rgba(255, 255, 255, .12);
  --comment-toolbar-text: #cbd5e1;
  --comment-toolbar-preview-border: rgba(255, 255, 255, .16);
  --comment-toolbar-preview-bg: rgba(255, 255, 255, .06);
  --nw-floating-bg: rgba(15, 23, 42, .98);
  --nw-floating-text: #f8fafc;
  --nw-floating-border: rgba(255, 255, 255, .18);
  --nw-floating-shadow: 0 18px 42px rgba(0, 0, 0, .42);
  --nw-floating-hover-bg: rgba(249, 115, 22, .18);
  --nw-floating-hover-border: rgba(249, 115, 22, .42);
  --nw-floating-selected-bg: rgba(249, 115, 22, .30);
  --nw-floating-selected-border: rgba(251, 146, 60, .58);
}
.notification-header { display:block; margin-bottom:0; padding:0; text-align:center; }
.notification-heading { width:100%; }
.notification-title-row { position:relative; display:block; width:100%; margin:0; padding:0; }
.notification-title { display:block; margin:0 0 14px; padding:0; border-radius:0; color:var(--notice-heading); font-size:18px; font-weight:700; line-height:1.5; }
.notification-subtitle { max-width:42rem; margin:2px auto 20px; color:var(--notice-heading); font-size:13px; line-height:1.7; text-align:center; opacity:.8; font-weight:400; }
.notification-board-head { display:flex; align-items:center; justify-content:space-between; gap:8px; min-height:28px; margin:0 0 8px; }
.notification-count-title { min-width:0; margin:0; color:var(--notice-count); font-size:14px; font-weight:400; line-height:20px; }
.notification-actions { display:flex; align-items:center; gap:8px; flex:0 0 auto; flex-wrap:wrap; justify-content:flex-end; max-width:70%; }
.unread-pill { display:inline-flex; align-items:center; min-height:28px; padding:0 10px; border-radius:999px; font-size:12px; font-weight:650; color:#fff; background:#3b82f6; }
.notification-refresh-button,
.notification-text-button,
.reply-toggle {
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
.notification-refresh-button { width:28px; min-width:28px; }
.notification-text-button,
.reply-toggle { min-width:max-content; padding:0 8px; }
:global(.dark) .notification-center .notification-refresh-button,
:global(.dark) .notification-center .notification-text-button,
:global(.dark) .notification-center .reply-toggle,
.notification-center.notification-theme-dark .notification-refresh-button,
.notification-center.notification-theme-dark .notification-text-button,
.notification-center.notification-theme-dark .reply-toggle {
  --nw-action-bg:rgba(51,65,85,.96);
  --nw-action-text:#cbd5e1;
  --nw-action-border:rgba(148,163,184,.28);
}
.notification-feed-panel { padding:0; overflow:visible; border:0; background:transparent; border-radius:0; }
.notification-board-wrap { box-sizing:border-box; max-width:48rem; margin:0 auto 8px; padding:8px; }
.notification-feed { display:flex; flex-direction:column; gap:12px; }
.notification-feed-item { position:relative; display:flex; gap:12px; padding:12px; border:1px solid var(--notice-border); border-radius:12px; background:var(--notice-card); color:var(--notice-text); box-shadow:var(--notice-card-shadow); transition:background-color .16s ease, border-color .16s ease, transform .16s ease; }
.notification-feed-item:hover { background:var(--notice-card-hover); }
.notification-feed-item.unread { border-color:rgba(59,130,246,.36); background:linear-gradient(0deg, rgba(59,130,246,.10), rgba(59,130,246,.10)), var(--notice-card); }
.notification-feed-item.unread::before { content:''; position:absolute; left:8px; top:23px; width:6px; height:6px; border-radius:999px; background:#3b82f6; }
.notification-feed-item.highlighted { border-color:rgba(59,130,246,.56); background:linear-gradient(0deg, rgba(59,130,246,.14), rgba(59,130,246,.14)), var(--notice-card); box-shadow:inset 3px 0 0 rgba(59,130,246,.72), var(--notice-card-shadow); }
.notification-avatar { width:36px; height:36px; flex:0 0 36px; border-radius:999px; object-fit:cover; background:var(--notice-input); border:1px solid var(--notice-border); }
.notification-item-body { min-width:0; flex:1; }
.notification-item-head { display:flex; align-items:flex-start; justify-content:space-between; gap:10px; min-height:36px; }
.notification-actor-block { min-width:0; }
.notification-actor-line { display:flex; align-items:center; gap:8px; min-width:0; }
.notification-actor-name { font-size:14px; line-height:1.35; font-weight:700; color:var(--notice-strong); word-break:break-word; }
.like-action-inline { font-size:13px; color:var(--notice-muted); }
.notification-time { margin-top:3px; font-size:12px; line-height:1.25; color:var(--notice-muted); }
.reply-toggle { flex:0 0 auto; }
.notification-actor-content { margin:8px 0 0; font-size:14px; line-height:1.68; white-space:pre-wrap; word-break:break-word; color:var(--notice-strong); }
.notification-target-card { width:100%; margin-top:10px; padding:10px 12px; border:1px solid var(--notice-border); border-radius:12px; background:var(--notice-input); color:var(--notice-text); display:flex; align-items:center; flex-wrap:wrap; gap:10px; text-align:left; cursor:pointer; transition:background-color .16s ease, border-color .16s ease, opacity .16s ease; }
.notification-target-card:hover { border-color:var(--nw-floating-hover-border); background:var(--nw-floating-hover-bg); }
.notification-target-card.jumping { cursor:wait; opacity:.88; transform:none; }
.notification-target-image { width:54px; height:54px; flex:0 0 54px; object-fit:cover; border-radius:10px; background:var(--notice-card); border:1px solid var(--notice-border); }
.notification-target-text { min-width:0; flex:1 1 160px; font-size:14px; line-height:1.58; word-break:break-word; }
.target-owner { color:var(--notice-link); font-weight:700; }
.notification-target-jumping { margin-left:auto; display:inline-flex; align-items:center; gap:4px; flex:0 0 auto; color:var(--notice-link); font-size:12px; font-weight:700; }
.notification-target-jumping-icon { width:14px; height:14px; animation:notification-spin 1s linear infinite; }
.inline-reply-box { margin-top:12px; padding-top:12px; border-top:1px solid var(--notice-border); }
.inline-reply-box :deep(.builtin-comments) { color:var(--notice-text); }
.inline-reply-box :deep(.waline-wrapper) { padding-left:0; padding-right:0; background:transparent !important; }
.inline-reply-box :deep(.reply-input-only) { padding-top:0; padding-bottom:0; }
.inline-reply-box :deep(.reply-input-only .space-y-4) { margin-top:0; }
.inline-reply-box :deep(.reply-input-only .comment-input-card) { padding:10px; border:1px solid var(--notice-border); border-radius:12px; background:var(--notice-input); }
.empty-state { min-height:220px; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:8px; text-align:center; padding:24px; border:1px solid var(--notice-border); background:var(--notice-card); color:var(--notice-text); border-radius:12px; }
.empty-state.compact { min-height:220px; border:0; background:transparent; }
.error-state .empty-icon { color:#ef4444; opacity:.82; }
.empty-icon { width:28px; height:28px; opacity:.62; }
.empty-title { font-size:14px; font-weight:600; color:var(--notice-muted); }
.notification-feed-error { display:flex; align-items:center; justify-content:center; gap:10px; padding:12px 16px; border:1px solid rgba(248,113,113,.22); border-radius:10px; background:rgba(254,242,242,.78); color:#b91c1c; font-size:13px; }
:global(.dark) .notification-feed-error,
.notification-center.notification-theme-dark .notification-feed-error { background:rgba(127,29,29,.22); border-color:rgba(248,113,113,.2); color:#fca5a5; }
.pager-shell {
  --pager-shell-bg: rgba(255, 255, 255, 0.85);
  --pager-shell-border: rgba(15, 23, 42, 0.12);
  --pager-shell-text: #334155;
  --pager-shell-muted: #64748b;
  --pager-input-bg: rgba(255, 255, 255, 0.92);
  --pager-input-border: rgba(15, 23, 42, 0.16);
  --pager-input-text: #0f172a;
  --pager-input-placeholder: rgba(15, 23, 42, 0.45);
  display:flex;
  align-items:center;
  justify-content:center;
  gap:16px;
  width:100%;
  margin:16px 0 72px;
  padding:10px 14px;
  border:1px solid var(--pager-shell-border);
  border-radius:999px;
  background:var(--pager-shell-bg);
  color:var(--pager-shell-text);
  box-shadow:0 8px 22px rgba(15,23,42,.10);
  flex-wrap:wrap;
}
.pager-shell.is-dark {
  --pager-shell-bg: rgba(39, 50, 66, 0.68);
  --pager-shell-border: rgba(255, 255, 255, 0.16);
  --pager-shell-text: #e2e8f0;
  --pager-shell-muted: #cbd5e1;
  --pager-input-bg: rgba(17, 24, 39, 0.58);
  --pager-input-border: rgba(255, 255, 255, 0.18);
  --pager-input-text: #f8fafc;
  --pager-input-placeholder: rgba(226, 232, 240, 0.58);
  box-shadow:0 8px 22px rgba(0,0,0,.24);
}
.pager-nav-group,
.pager-jump-group {
  display:inline-flex;
  align-items:center;
  justify-content:center;
  gap:10px;
  flex-wrap:wrap;
}
.pager-btn,
.pager-jump-btn {
  min-height:34px;
  padding-inline:14px;
  font-size:13px;
  font-weight:700;
  --nw-action-bg:rgba(15,23,42,.06);
  --nw-action-text:var(--notice-text);
  --nw-action-border:var(--notice-border);
}
.pager-icon-wrap {
  width:1.35rem;
  height:1.35rem;
  border-radius:999px;
  display:inline-flex;
  align-items:center;
  justify-content:center;
  background:color-mix(in srgb, var(--nw-action-text) 10%, transparent);
}
.pager-icon { line-height:1; }
.pager-page-text,
.pager-status-text {
  color:var(--pager-shell-muted);
  font-size:13px;
  font-weight:650;
  text-shadow:none;
}
.pager-number-control {
  display:inline-flex;
  align-items:stretch;
  min-height:34px;
  border:1px solid var(--pager-input-border);
  border-radius:12px;
  background:var(--pager-input-bg);
  color:var(--pager-input-text);
  overflow:hidden;
  transition:border-color .15s ease, box-shadow .15s ease, background-color .15s ease;
}
.pager-number-control:focus-within {
  border-color:rgba(249,115,22,.72);
  box-shadow:0 0 0 2px rgba(249,115,22,.18);
}
.pager-page-input {
  width:42px;
  min-height:32px;
  padding:0 6px;
  border:0;
  outline:none;
  background:transparent;
  color:var(--pager-input-text);
  font-size:14px;
  font-weight:700;
  text-align:center;
  appearance:textfield;
}
.pager-page-input::placeholder { color:var(--pager-input-placeholder); }
.pager-stepper {
  display:grid;
  grid-template-rows:1fr 1fr;
  width:24px;
  border-left:1px solid var(--pager-input-border);
}
.pager-stepper-btn {
  width:24px;
  min-width:24px;
  height:16px;
  min-height:16px;
  padding:0;
  border:0;
  border-radius:0;
}
.pager-stepper-btn + .pager-stepper-btn { border-top:1px solid var(--pager-input-border); }
.pager-stepper-btn svg { width:12px; height:12px; }
.pager-shell.is-dark .pager-btn,
.pager-shell.is-dark .pager-jump-btn {
  --nw-action-bg:rgba(51,65,85,.96);
  --nw-action-text:#cbd5e1;
  --nw-action-border:rgba(148,163,184,.28);
}
.spin { animation:notification-spin 1s linear infinite; }
@keyframes notification-spin { to { transform:rotate(360deg); } }
@media (max-width: 720px) {
  .notification-header { text-align:center; margin-bottom:0; padding:0; }
  .notification-title-row { display:block; min-height:0; margin:0; }
  .notification-title { line-height:1.5; }
  .notification-subtitle { margin:2px auto 20px; }
  .notification-board-head { align-items:center; flex-wrap:wrap; }
  .notification-actions { justify-content:flex-end; max-width:100%; }
  .notification-feed-item { gap:10px; padding:14px 12px; }
  .notification-feed-item.unread::before { left:7px; top:24px; }
  .notification-avatar { width:40px; height:40px; flex-basis:40px; }
  .notification-actor-name { font-size:16px; }
  .notification-actor-content { font-size:15px; line-height:1.68; }
  .notification-target-card { padding:10px; }
  .notification-target-image { width:54px; height:54px; flex-basis:54px; }
  .notification-target-jumping { width:100%; justify-content:flex-end; }
  .pager-shell { border-radius:18px; gap:10px; }
  .pager-nav-group,
  .pager-jump-group { width:100%; }
}
</style>
