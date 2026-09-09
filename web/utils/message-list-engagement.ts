import { nextTick, ref } from 'vue'
import { useToast } from '#ui/composables/useToast'
import { postRequest } from './api'

type BuiltinCommentsExpose = {
  focusCommentById?: (commentId: number, options?: { scroll?: boolean }) => Promise<boolean>
}

type MessageListEngagementOptions = {
  root: () => HTMLElement | null
  messages: () => any[]
  isGuestbook: (message: any) => boolean
  isLoggedIn: () => boolean
  canInteract: (message: any) => boolean
}

// Owns likes, comment panel state, batched count loading, viewport observation
// and the global count event. Async batches are generation-checked; mount/update/
// dispose are the only lifecycle calls the list needs.
export const createMessageListEngagement = (options: MessageListEngagementOptions) => {
  const likes = ref<Record<number, number>>({})
  const liked = ref<Record<number, boolean>>({})
  const commentCounts = ref<Record<number, number>>({})
  const expandedComments = ref<Record<number, boolean>>({})
  const activeCommentId = ref<number | null>(null)
  const commentRefreshKey = ref<Record<number, number>>({})
  const fetchedCommentIds = new Set<number>()
  const pendingCommentIds: number[] = []
  const builtinCommentRefs = new Map<number, BuiltinCommentsExpose>()
  let observer: IntersectionObserver | null = null
  let activeBatch = false
  let mounted = false
  let generation = 0
  const focusWaits = new Map<number, () => void>()

  const messageById = (id: number) => options.messages().find((message) => Number(message?.id || 0) === Number(id))

  const hydrate = (items = options.messages()) => {
    items.forEach((item) => {
      const id = Number(item?.id || 0)
      if (!id) return
      if (item?.like_count !== undefined && item?.like_count !== null) {
        const count = Number(item.like_count)
        if (Number.isFinite(count)) likes.value[id] = count
      }
      liked.value[id] = item?.liked === true
    })
  }

  const fetchCommentCounts = async (ids: number[]) => {
    if (!ids.length) return
    const currentGeneration = generation
    try {
      const response = await postRequest<any>('messages/comments/counts', { ids }, { credentials: 'include', silent: true })
      if (!mounted || currentGeneration !== generation || response?.code !== 1) return
      const rows = Array.isArray(response?.data) ? response.data : []
      rows.forEach((row: any) => {
        const id = Number(row?.id || 0)
        const count = Number(row?.count || 0)
        if (!id) return
        commentCounts.value[id] = count
        fetchedCommentIds.add(id)
        if (count > 0) expandedComments.value[id] = true
      })
    } catch {}
  }

  const runQueue = () => {
    if (!mounted || activeBatch || !pendingCommentIds.length) return
    const ids: number[] = []
    const seen = new Set<number>()
    while (ids.length < 20 && pendingCommentIds.length) {
      const id = pendingCommentIds.shift() as number
      if (seen.has(id) || fetchedCommentIds.has(id)) continue
      seen.add(id)
      ids.push(id)
    }
    if (!ids.length) return
    activeBatch = true
    fetchCommentCounts(ids).finally(() => {
      activeBatch = false
      runQueue()
    })
  }

  const scheduleCount = (id: number) => {
    if (!id || fetchedCommentIds.has(id) || pendingCommentIds.includes(id)) return
    pendingCommentIds.push(id)
    runQueue()
  }

  const observeVisibleMessages = () => {
    const root = options.root()
    if (!observer || !root) return
    root.querySelectorAll<HTMLElement>('.content-container[data-msg-id]').forEach((element) => {
      const id = Number(element.dataset.msgId || 0)
      const message = messageById(id)
      if (id && message && !options.isGuestbook(message) && !fetchedCommentIds.has(id)) observer?.observe(element)
    })
  }

  const like = async (id: number) => {
    if (!options.isLoggedIn()) {
      useToast().add({ title: '请先登录后再点赞', color: 'orange', timeout: 2000 })
      return
    }
    const target = messageById(id)
    if (!options.canInteract(target)) return
    try {
      const response = await postRequest<any>(`messages/${id}/like/toggle`, {}, { credentials: 'include' })
      if (response?.code !== 1) throw new Error(response?.msg || '点赞失败')
      likes.value[id] = Number(response?.data?.like_count ?? (likes.value[id] || 0))
      liked.value[id] = !!response?.data?.liked
    } catch (error: any) {
      useToast().add({ title: error?.message || '点赞失败', color: 'red', timeout: 2000 })
    }
  }

  const toggleComments = async (messageId: number) => {
    const message = messageById(messageId)
    if (options.isGuestbook(message)) return
    const shown = !!(expandedComments.value[messageId] || activeCommentId.value === messageId)
    if (shown) {
      expandedComments.value[messageId] = false
      if (activeCommentId.value === messageId) activeCommentId.value = null
      return
    }
    activeCommentId.value = messageId
    commentRefreshKey.value[messageId] = (commentRefreshKey.value[messageId] || 0) + 1
    expandedComments.value[messageId] = true
    await nextTick()
    window.dispatchEvent(new Event(`refresh-comments-${messageId}`))
  }

  const handleCancel = (messageId: number, payload?: { empty?: boolean }) => {
    if (payload?.empty === true) {
      void toggleComments(messageId)
      return
    }
    if (activeCommentId.value === messageId) activeCommentId.value = null
    commentRefreshKey.value[messageId] = (commentRefreshKey.value[messageId] || 0) + 1
  }

  const commentsRefFor = (messageId: number) => (instance: unknown) => {
    const id = Number(messageId || 0)
    if (!id) return
    if (instance) builtinCommentRefs.set(id, instance as BuiltinCommentsExpose)
    else builtinCommentRefs.delete(id)
  }

  const focusComment = async (messageId: number, commentId: number) => {
    const currentGeneration = generation
    for (let attempt = 0; attempt < 10; attempt += 1) {
      if (!mounted || currentGeneration !== generation) return false
      const comments = builtinCommentRefs.get(messageId)
      if (comments?.focusCommentById && await comments.focusCommentById(commentId, { scroll: false })) return true
      if (!mounted || currentGeneration !== generation) return false
      await new Promise<void>((resolve) => {
        const timer = window.setTimeout(() => {
          focusWaits.delete(timer)
          resolve()
        }, 140)
        focusWaits.set(timer, resolve)
      })
    }
    return false
  }

  const onCommentCountUpdated = (event: Event) => {
    const detail = (event as CustomEvent)?.detail || {}
    const id = Number(detail?.messageId || 0)
    const count = Number(detail?.count || 0)
    if (id) commentCounts.value[id] = count
  }

  const mount = () => {
    if (mounted) return
    mounted = true
    generation += 1
    hydrate()
    window.addEventListener('comment-count-updated', onCommentCountUpdated)
    if (!window.matchMedia('(max-width: 1024px)').matches && typeof IntersectionObserver !== 'undefined') {
      observer = new IntersectionObserver((entries) => {
        entries.forEach((entry) => {
          if (!entry.isIntersecting) return
          const element = entry.target as HTMLElement
          scheduleCount(Number(element.dataset.msgId || 0))
          observer?.unobserve(element)
        })
      }, { rootMargin: '256px 0px' })
    }
    observeVisibleMessages()
  }

  const update = () => {
    hydrate()
    observeVisibleMessages()
  }

  const dispose = () => {
    mounted = false
    generation += 1
    observer?.disconnect()
    observer = null
    pendingCommentIds.splice(0)
    builtinCommentRefs.clear()
    focusWaits.forEach((resolve, timer) => {
      window.clearTimeout(timer)
      resolve()
    })
    focusWaits.clear()
    window.removeEventListener('comment-count-updated', onCommentCountUpdated)
  }

  return {
    likes,
    liked,
    commentCounts,
    expandedComments,
    activeCommentId,
    commentRefreshKey,
    hydrate,
    like,
    toggleComments,
    handleCancel,
    commentsRefFor,
    focusComment,
    mount,
    update,
    dispose,
  }
}
