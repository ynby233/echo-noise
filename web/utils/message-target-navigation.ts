import { nextTick } from 'vue'

type MessageTargetNavigationOptions = {
  root: () => HTMLElement | null
  targetMessageId: () => number
  targetCommentId: () => number
  ready: () => boolean
  loadMessage: (messageId: number) => Promise<boolean>
  expandComments: (messageId: number) => void
  focusComment: (messageId: number, commentId: number) => Promise<boolean>
  consume: () => void
}

export const isMessageListScrollable = (element: HTMLElement | null) => {
  if (!element || typeof window === 'undefined') return false
  const style = window.getComputedStyle(element)
  return /(auto|scroll|overlay)/.test(`${style.overflowY || ''} ${style.overflow || ''}`) && element.scrollHeight > element.clientHeight
}

export const getMessageListScrollContainer = (target?: HTMLElement | null) => {
  if (typeof document === 'undefined') return null as HTMLElement | null
  const candidates = [
    target?.closest('.center-col') as HTMLElement | null,
    target?.closest('.content-wrapper') as HTMLElement | null,
    document.querySelector('.content-wrapper') as HTMLElement | null,
    document.querySelector('.center-col') as HTMLElement | null,
  ]
  return candidates.find(isMessageListScrollable) || candidates.find(Boolean) || null
}

// Owns notification-target loading follow-up, scroll stabilization, highlighting
// and retry timers. The caller provides list loading and comment expansion at a
// narrow seam; all browser lifecycle is released by dispose.
export const createMessageTargetNavigation = (options: MessageTargetNavigationOptions) => {
  let mounted = false
  let generation = 0
  let retryTimer: ReturnType<typeof setTimeout> | null = null
  let retryKey = ''
  let retryCount = 0
  const classTimers = new Set<ReturnType<typeof setTimeout>>()
  const pendingWaits = new Set<() => void>()

  const queryTarget = (messageId: number, commentId = 0) => {
    const root = options.root()
    if (!root) return null
    const selector = commentId
      ? `.content-container[data-msg-id="${messageId}"] [data-comment-id="${commentId}"]`
      : `.content-container[data-msg-id="${messageId}"]`
    return root.querySelector<HTMLElement>(selector)
  }

  const scrollToFocus = (element: HTMLElement, behavior: ScrollBehavior = 'smooth') => {
    const wrapper = getMessageListScrollContainer(element)
    if (!wrapper) {
      element.scrollIntoView({ behavior, block: 'start' })
      return
    }
    const wrapperRect = wrapper.getBoundingClientRect()
    const elementRect = element.getBoundingClientRect()
    const focusOffset = Math.min(140, Math.max(72, wrapper.clientHeight * 0.18))
    wrapper.scrollTo({
      top: Math.max(0, wrapper.scrollTop + elementRect.top - wrapperRect.top - focusOffset),
      behavior,
    })
  }

  const focusDistance = (element: HTMLElement) => {
    const wrapper = getMessageListScrollContainer(element)
    const elementRect = element.getBoundingClientRect()
    if (!wrapper) {
      const focusOffset = Math.min(140, Math.max(72, window.innerHeight * 0.18))
      return elementRect.top - focusOffset
    }
    const wrapperRect = wrapper.getBoundingClientRect()
    const focusOffset = Math.min(140, Math.max(72, wrapper.clientHeight * 0.18))
    return elementRect.top - wrapperRect.top - focusOffset
  }

  const delay = (ms: number) => new Promise<void>((resolve) => {
    const finish = () => {
      window.clearTimeout(timer)
      pendingWaits.delete(finish)
      resolve()
    }
    const timer = window.setTimeout(finish, ms)
    pendingWaits.add(finish)
  })
  const frame = () => new Promise<void>((resolve) => {
    if (typeof window.requestAnimationFrame !== 'function') resolve()
    else {
      const finish = () => {
        window.cancelAnimationFrame(id)
        pendingWaits.delete(finish)
        resolve()
      }
      const id = window.requestAnimationFrame(finish)
      pendingWaits.add(finish)
    }
  })

  const waitForMediaElement = (element: HTMLImageElement | HTMLVideoElement, timeout: number) => new Promise<void>((resolve) => {
    let settled = false
    const eventNames = element instanceof HTMLImageElement ? ['load', 'error'] : ['loadeddata', 'loadedmetadata', 'error']
    const finish = () => {
      if (settled) return
      settled = true
      eventNames.forEach((name) => element.removeEventListener(name, onReady))
      window.clearTimeout(timer)
      pendingWaits.delete(finish)
      resolve()
    }
    const onReady = () => {
      if (element instanceof HTMLImageElement && element.complete && element.naturalWidth > 0 && typeof element.decode === 'function') {
        void element.decode().catch(() => {}).finally(finish)
      } else finish()
    }
    const timer = window.setTimeout(finish, timeout)
    pendingWaits.add(finish)
    eventNames.forEach((name) => element.addEventListener(name, onReady))
    if (element instanceof HTMLImageElement) {
      element.loading = 'eager'
      try { (element as HTMLImageElement & { fetchPriority?: string }).fetchPriority = 'high' } catch {}
      if (element.complete && element.naturalWidth > 0) {
        if (typeof element.decode === 'function') void element.decode().catch(() => {}).finally(finish)
        else finish()
      }
    } else {
      element.preload = 'metadata'
      try { element.load() } catch {}
      if (element.readyState >= 2) finish()
    }
  })

  const waitForMedia = async (messageId: number, timeout = 2400) => {
    const container = queryTarget(messageId)
    if (!container) return
    const media = Array.from(container.querySelectorAll('img, video')) as Array<HTMLImageElement | HTMLVideoElement>
    await Promise.all(media.map((element) => waitForMediaElement(element, timeout)))
  }

  const waitForStableLayout = async (element: HTMLElement, currentGeneration: number, duration = 900) => {
    let lastTop = Number.NaN
    let lastHeight = Number.NaN
    let stableFrames = 0
    const startedAt = Date.now()
    while (Date.now() - startedAt < duration) {
      await frame()
      if (!mounted || currentGeneration !== generation || !document.contains(element)) return false
      const rect = element.getBoundingClientRect()
      stableFrames = Math.abs(rect.top - lastTop) < 1 && Math.abs(rect.height - lastHeight) < 1 ? stableFrames + 1 : 0
      lastTop = rect.top
      lastHeight = rect.height
      if (stableFrames >= 4) break
      await delay(80)
    }
    return mounted && currentGeneration === generation && document.contains(element)
  }

  const stabilizeScroll = async (
    element: HTMLElement,
    messageId: number,
    currentGeneration: number,
    behavior: ScrollBehavior = 'smooth',
  ) => {
    await waitForMedia(messageId)
    if (!await waitForStableLayout(element, currentGeneration)) return
    if (Math.abs(focusDistance(element)) > 2) scrollToFocus(element, behavior)
    if (behavior === 'smooth') {
      await delay(520)
      if (mounted && currentGeneration === generation && document.contains(element) && Math.abs(focusDistance(element)) > 18) {
        scrollToFocus(element, 'instant')
      }
    }
  }

  const removeClassLater = (element: HTMLElement, className: string, delayMs: number) => {
    const timer = window.setTimeout(() => {
      classTimers.delete(timer)
      element.classList.remove(className)
    }, delayMs)
    classTimers.add(timer)
  }

  const resetRetry = () => {
    if (retryTimer) clearTimeout(retryTimer)
    retryTimer = null
    retryKey = ''
    retryCount = 0
  }

  const scheduleRetry = (key: string) => {
    if (retryKey !== key) {
      retryKey = key
      retryCount = 0
    }
    if (retryCount >= 6) return false
    retryCount += 1
    if (retryTimer) clearTimeout(retryTimer)
    retryTimer = window.setTimeout(() => {
      retryTimer = null
      void focus()
    }, 360)
    return true
  }

  async function focus() {
    if (!mounted || !options.ready()) return
    const currentGeneration = ++generation
    const messageId = options.targetMessageId()
    if (!messageId) return
    const loaded = await options.loadMessage(messageId)
    if (!mounted || currentGeneration !== generation) return
    if (!loaded) {
      resetRetry()
      options.consume()
      return
    }
    await nextTick()
    if (!mounted || currentGeneration !== generation) return
    const commentId = options.targetCommentId()
    const messageElement = queryTarget(messageId)
    if (messageElement) {
      messageElement.classList.add('highlight-message')
      if (!commentId) scrollToFocus(messageElement, 'instant')
      removeClassLater(messageElement, 'highlight-message', 2000)
    }
    if (!commentId) {
      if (messageElement) await stabilizeScroll(messageElement, messageId, currentGeneration)
      if (!mounted || currentGeneration !== generation) return
      resetRetry()
      options.consume()
      return
    }

    options.expandComments(messageId)
    await nextTick()
    if (!mounted || currentGeneration !== generation) return
    if (await options.focusComment(messageId, commentId) && mounted && currentGeneration === generation) {
      const commentElement = queryTarget(messageId, commentId)
      if (commentElement) {
        await stabilizeScroll(commentElement, messageId, currentGeneration)
        if (!mounted || currentGeneration !== generation) return
        commentElement.classList.add('notification-comment-highlight')
        removeClassLater(commentElement, 'notification-comment-highlight', 2200)
      }
      resetRetry()
      options.consume()
      return
    }
    for (let attempt = 0; attempt < 12 && mounted && currentGeneration === generation; attempt += 1) {
      const commentElement = queryTarget(messageId, commentId)
      if (commentElement) {
        await stabilizeScroll(commentElement, messageId, currentGeneration)
        if (!mounted || currentGeneration !== generation) return
        commentElement.classList.add('notification-comment-highlight')
        removeClassLater(commentElement, 'notification-comment-highlight', 2200)
        resetRetry()
        options.consume()
        return
      }
      await delay(160)
    }
    if (mounted && currentGeneration === generation && !scheduleRetry(`${messageId}:${commentId}`)) {
      resetRetry()
      options.consume()
    }
  }

  const mount = () => {
    if (mounted) return
    mounted = true
    generation += 1
  }
  const update = () => { void focus() }
  const dispose = () => {
    mounted = false
    generation += 1
    resetRetry()
    pendingWaits.forEach(finish => finish())
    pendingWaits.clear()
    classTimers.forEach(clearTimeout)
    classTimers.clear()
  }

  return { focus, mount, update, dispose }
}
