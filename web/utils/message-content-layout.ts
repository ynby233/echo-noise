import { nextTick, ref } from 'vue'

type MessageContentLayoutOptions = {
  root: () => HTMLElement | null
  messages: () => any[]
  afterMeasure?: () => void
}

// Owns content-height measurement plus every ResizeObserver, media listener,
// timer and animation frame created for it. The list calls update after its
// keyed messages change and dispose before dropping the root.
export const createMessageContentLayout = (options: MessageContentLayoutOptions) => {
  const expanded = ref<Record<number, boolean>>({})
  const showExpandButton = ref<Record<number, boolean>>({})
  const hasGrid = ref<Record<number, boolean>>({})
  const hasFileAttachment = ref<Record<number, boolean>>({})
  const measuredHeights = ref<Record<number, number>>({})
  const resizeObservers = new Map<HTMLElement, ResizeObserver>()
  const mediaListeners = new Map<Element, () => void>()
  let frame: number | null = null
  let timer: number | null = null
  let generation = 0
  let mounted = false

  const toggle = (messageId: number) => {
    expanded.value[messageId] = !expanded.value[messageId]
  }

  const isAttachmentShadowOpen = (messageId: number) => (
    !!hasFileAttachment.value[messageId] && !(showExpandButton.value[messageId] && !expanded.value[messageId])
  )

  const isAttachmentShadowClipped = (messageId: number) => (
    !!hasFileAttachment.value[messageId] && !!showExpandButton.value[messageId] && !expanded.value[messageId]
  )

  const pruneDetachedResources = (root: HTMLElement) => {
    resizeObservers.forEach((observer, element) => {
      if (root.contains(element)) return
      observer.disconnect()
      resizeObservers.delete(element)
    })
    mediaListeners.forEach((listener, element) => {
      if (root.contains(element)) return
      ;['load', 'loadedmetadata', 'loadeddata', 'canplay', 'error'].forEach((name) => element.removeEventListener(name, listener))
      mediaListeners.delete(element)
    })
  }

  const measure = async () => {
    const currentGeneration = generation
    await nextTick()
    const root = options.root()
    if (!mounted || currentGeneration !== generation || !root) return
    pruneDetachedResources(root)

    for (const message of options.messages()) {
      const messageId = Number(message?.id || 0)
      if (!messageId) continue
      const content = root.querySelector<HTMLElement>(`.content-container[data-msg-id="${messageId}"] .overflow-y-hidden`)
      if (!content) continue
      const measureEl = content.querySelector<HTMLElement>('.markdown-preview') || content
      if (typeof ResizeObserver !== 'undefined' && !resizeObservers.has(measureEl)) {
        const observer = new ResizeObserver(update)
        observer.observe(measureEl)
        resizeObservers.set(measureEl, observer)
      }

      const previousVisibility = measureEl.style.contentVisibility
      const previousIntrinsicSize = measureEl.style.containIntrinsicSize
      if (previousVisibility) measureEl.style.contentVisibility = 'visible'
      if (previousIntrinsicSize) measureEl.style.containIntrinsicSize = ''
      try {
        const imageGrid = !!measureEl.querySelector('.image-grid')
        hasGrid.value[messageId] = imageGrid
        hasFileAttachment.value[messageId] = !!measureEl.querySelector(
          '.site-attachment-file, .site-attachment-audio, .site-attachment-failure, .github-card',
        )
        const fullHeight = measureEl.scrollHeight
        const previousHeight = measuredHeights.value[messageId]
        if (imageGrid) {
          measuredHeights.value[messageId] = fullHeight
          showExpandButton.value[messageId] = false
          expanded.value[messageId] = true
        } else {
          const needsExpand = fullHeight > 708
          if (!(typeof previousHeight === 'number'
            && Math.abs(fullHeight - previousHeight) <= 8
            && showExpandButton.value[messageId] === needsExpand)) {
            measuredHeights.value[messageId] = fullHeight
            showExpandButton.value[messageId] = needsExpand
            if (needsExpand && expanded.value[messageId] === undefined) expanded.value[messageId] = false
          }
        }
      } finally {
        if (previousVisibility) measureEl.style.contentVisibility = previousVisibility
        if (previousIntrinsicSize) measureEl.style.containIntrinsicSize = previousIntrinsicSize
      }

      measureEl.querySelectorAll('img, video, audio').forEach((element) => {
        if (mediaListeners.has(element)) return
        const listener = () => update()
        ;['load', 'loadedmetadata', 'loadeddata', 'canplay', 'error'].forEach((name) => element.addEventListener(name, listener))
        mediaListeners.set(element, listener)
      })
    }
    options.afterMeasure?.()
  }

  function update() {
    if (!mounted || frame !== null) return
    frame = window.requestAnimationFrame(() => {
      frame = null
      if (timer !== null) window.clearTimeout(timer)
      timer = window.setTimeout(() => {
        timer = null
        void measure()
      }, 80)
    })
  }

  const mount = () => {
    if (mounted) return
    mounted = true
    generation += 1
    update()
  }

  const dispose = () => {
    mounted = false
    generation += 1
    if (frame !== null) window.cancelAnimationFrame(frame)
    if (timer !== null) window.clearTimeout(timer)
    frame = null
    timer = null
    resizeObservers.forEach((observer) => observer.disconnect())
    resizeObservers.clear()
    mediaListeners.forEach((listener, element) => {
      ;['load', 'loadedmetadata', 'loadeddata', 'canplay', 'error'].forEach((name) => element.removeEventListener(name, listener))
    })
    mediaListeners.clear()
  }

  return {
    expanded,
    showExpandButton,
    hasGrid,
    measuredHeights,
    toggle,
    isAttachmentShadowOpen,
    isAttachmentShadowClipped,
    mount,
    update,
    dispose,
  }
}
