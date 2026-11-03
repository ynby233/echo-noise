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
// and the global count event. resetViewer synchronously invalidates viewer-owned
// state; the next mount/update loads engagement for the new message list.
export const createMessageListEngagement = (options: MessageListEngagementOptions) => {
  const likes = ref<Record<number, number>>({})
  const liked = ref<Record<number, boolean>>({})
  const commentCounts = ref<Record<number, number>>({})
  const expandedComments = ref<Record<number, boolean>>({})
  const activeCommentId = ref<number | null>(null)
  const commentRefreshKey = ref<Record<number, number>>({})
  const fetchedCommentIds = new Set<number>()
  const pendingCommentIds: number[] = []
  const inFlightCommentIds = new Set<number>()
  const collapsedCommentIds = new Set<number>()
  const builtinCommentRefs = new Map<number, BuiltinCommentsExpose>()
  const observedElements = new Map<HTMLElement, number>()
  const likeRequests = new Map<number, AbortController>()
  let observer: IntersectionObserver | null = null
  let activeBatch: AbortController | null = null
  let queueScheduled = false
  let mounted = false
  let generation = 0
  const focusWaits = new Map<number, () => void>()

  const messageById = (id: number) => options.messages().find((message) => Number(message?.id || 0) === Number(id))
  const canCount = (id: number) => {
    const message = messageById(id)
    return !!id && !!message && !options.isGuestbook(message)
  }

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

  const fetchCommentCounts = async (ids: number[], controller: AbortController) => {
    const currentGeneration = generation
    try {
      const response = await postRequest<unknown>('messages/comments/counts', { ids }, {
        credentials: 'include', silent: true, signal: controller.signal,
      })
      if (!mounted || currentGeneration !== generation || controller.signal.aborted || response?.code !== 1) return
      const rows = Array.isArray(response?.data) ? response.data : []
      rows.forEach((row: unknown) => {
        if (!row || typeof row !== 'object' || !('id' in row) || !('count' in row)) return
        const id = Number(row.id)
        const count = Number(row.count)
        if (!ids.includes(id) || !canCount(id) || fetchedCommentIds.has(id) || !Number.isFinite(count) || count < 0) return
        commentCounts.value[id] = count
        fetchedCommentIds.add(id)
        const hasVisibleComments = 'hasVisibleComments' in row && row.hasVisibleComments === true
        if ((count > 0 || hasVisibleComments) && !collapsedCommentIds.has(id)) expandedComments.value[id] = true
      })
    } catch {}
  }

  const runQueue = () => {
    if (!mounted || activeBatch || !pendingCommentIds.length) return
    const ids: number[] = []
    while (ids.length < 20 && pendingCommentIds.length) {
      const id = pendingCommentIds.shift() as number
      if (!canCount(id) || fetchedCommentIds.has(id)) continue
      ids.push(id)
      inFlightCommentIds.add(id)
    }
    if (!ids.length) return
    const controller = new AbortController()
    activeBatch = controller
    fetchCommentCounts(ids, controller).finally(() => {
      // An aborted viewer's finally must not unlock the next viewer's batch.
      if (activeBatch !== controller) return
      activeBatch = null
      ids.forEach((id) => inFlightCommentIds.delete(id))
      runQueue()
    })
  }

  const scheduleCount = (id: number) => {
    if (!mounted || !canCount(id) || fetchedCommentIds.has(id) || inFlightCommentIds.has(id) || pendingCommentIds.includes(id)) return
    pendingCommentIds.push(id)
    if (queueScheduled || activeBatch) return
    queueScheduled = true
    const currentGeneration = generation
    queueMicrotask(() => {
      if (!mounted || currentGeneration !== generation) return
      queueScheduled = false
      runQueue()
    })
  }

  const observeVisibleMessages = () => {
    if (!mounted) return
    const root = options.root()
    const elements = new Set(root?.querySelectorAll<HTMLElement>('.content-container[data-msg-id]') || [])
    observedElements.forEach((id, element) => {
      if (!elements.has(element) || Number(element.dataset.msgId) !== id || !canCount(id) || fetchedCommentIds.has(id)) {
        observer?.unobserve(element)
        observedElements.delete(element)
      }
    })
    builtinCommentRefs.forEach((_, id) => {
      if (!canCount(id)) builtinCommentRefs.delete(id)
    })
    if (!observer) {
      // Mobile and browsers without IntersectionObserver still need counts.
      options.messages().forEach((message) => scheduleCount(Number(message?.id || 0)))
      return
    }
    elements.forEach((element) => {
      const id = Number(element.dataset.msgId || 0)
      if (!canCount(id) || fetchedCommentIds.has(id) || inFlightCommentIds.has(id) || pendingCommentIds.includes(id) || observedElements.has(element)) return
      observedElements.set(element, id)
      observer?.observe(element)
    })
  }

  const createObserver = () => {
    if (observer || window.matchMedia('(max-width: 1024px)').matches || typeof IntersectionObserver === 'undefined') return
    const currentGeneration = generation
    const currentObserver = new IntersectionObserver((entries) => {
      if (!mounted || generation !== currentGeneration || observer !== currentObserver) return
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return
        const element = entry.target as HTMLElement
        const id = observedElements.get(element)
        if (!id || Number(element.dataset.msgId) !== id) return
        currentObserver.unobserve(element)
        observedElements.delete(element)
        if (options.root()?.contains(element)) scheduleCount(id)
      })
    }, { rootMargin: '256px 0px' })
    observer = currentObserver
  }

  const like = async (id: number) => {
    if (!mounted) return
    const currentGeneration = generation
    if (!options.isLoggedIn()) {
      useToast().add({ title: '请先登录后再点赞', color: 'orange', timeout: 2000 })
      return
    }
    const target = messageById(id)
    if (!options.canInteract(target)) return
    likeRequests.get(id)?.abort()
    const controller = new AbortController()
    likeRequests.set(id, controller)
    const isCurrent = () => mounted && currentGeneration === generation && !controller.signal.aborted && likeRequests.get(id) === controller && !!messageById(id)
    try {
      const response = await postRequest<unknown>(`messages/${id}/like/toggle`, {}, { credentials: 'include', silent: true, signal: controller.signal })
      if (!isCurrent()) return
      if (response?.code !== 1) throw new Error(response?.msg || '点赞失败')
      const data = response.data
      if (!data || typeof data !== 'object' || !('like_count' in data) || !('liked' in data)) return
      const count = Number(data.like_count)
      if (!Number.isFinite(count) || count < 0 || typeof data.liked !== 'boolean') return
      likes.value[id] = count
      liked.value[id] = data.liked
    } catch (error: unknown) {
      if (isCurrent()) useToast().add({ title: error instanceof Error ? error.message : '点赞失败', color: 'red', timeout: 2000 })
    } finally {
      if (likeRequests.get(id) === controller) likeRequests.delete(id)
    }
  }

  const toggleComments = async (messageId: number) => {
    if (!mounted) return
    const currentGeneration = generation
    const message = messageById(messageId)
    if (options.isGuestbook(message)) return
    const shown = !!(expandedComments.value[messageId] || activeCommentId.value === messageId)
    if (shown) {
      collapsedCommentIds.add(messageId)
      expandedComments.value[messageId] = false
      if (activeCommentId.value === messageId) activeCommentId.value = null
      return
    }
    collapsedCommentIds.delete(messageId)
    activeCommentId.value = messageId
    commentRefreshKey.value[messageId] = (commentRefreshKey.value[messageId] || 0) + 1
    expandedComments.value[messageId] = true
    await nextTick()
    if (!mounted || generation !== currentGeneration) return
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
      if (comments?.focusCommentById && await comments.focusCommentById(commentId, { scroll: false })) {
        return mounted && currentGeneration === generation
      }
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
    if (mounted && canCount(id) && Number.isFinite(count) && count >= 0) {
      commentCounts.value[id] = count
      fetchedCommentIds.add(id)
    }
  }

  const mount = () => {
    if (mounted) return
    mounted = true
    generation += 1
    hydrate()
    window.addEventListener('comment-count-updated', onCommentCountUpdated)
    createObserver()
    observeVisibleMessages()
  }

  const update = () => {
    if (!mounted) return
    createObserver()
    hydrate()
    observeVisibleMessages()
  }

  const resetViewer = () => {
    generation += 1
    activeBatch?.abort()
    activeBatch = null
    likeRequests.forEach((controller) => controller.abort())
    likeRequests.clear()
    queueScheduled = false
    pendingCommentIds.splice(0)
    inFlightCommentIds.clear()
    fetchedCommentIds.clear()
    collapsedCommentIds.clear()
    observer?.disconnect()
    observer = null
    observedElements.clear()
    builtinCommentRefs.clear()
    likes.value = {}
    liked.value = {}
    commentCounts.value = {}
    expandedComments.value = {}
    activeCommentId.value = null
    commentRefreshKey.value = {}
    focusWaits.forEach((resolve, timer) => {
      window.clearTimeout(timer)
      resolve()
    })
    focusWaits.clear()
  }

  const dispose = () => {
    mounted = false
    resetViewer()
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
    resetViewer,
    dispose,
  }
}
