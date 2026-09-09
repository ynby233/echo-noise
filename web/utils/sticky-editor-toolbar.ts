export type StickyEditorToolbar = {
  mount: () => void
  update: () => void
  element: () => HTMLElement | null
  dispose: () => void
}

type StickyEditorToolbarOptions = {
  container: () => HTMLElement | null
  onReady?: () => void
}

// Owns the sticky-toolbar placeholder, observers, retry timers and listeners.
// Vditor renders the toolbar asynchronously, so mount includes bounded retries.
export const createStickyEditorToolbar = (options: StickyEditorToolbarOptions): StickyEditorToolbar => {
  let toolbar: HTMLElement | null = null
  let placeholder: HTMLElement | null = null
  let mutationObserver: MutationObserver | null = null
  let resizeObserver: ResizeObserver | null = null
  let root: HTMLElement | null = null
  let scrollContainers: HTMLElement[] = []
  let retryTimers: Array<ReturnType<typeof setTimeout>> = []
  let resizeFrame: number | null = null

  const update = () => {
    if (!root || !toolbar) return
    const fullscreen = root.classList.contains('vditor--fullscreen')
    const height = toolbar.offsetHeight
    if (fullscreen) {
      toolbar.style.position = 'fixed'
      toolbar.style.top = '0px'
      toolbar.style.left = '0px'
      toolbar.style.width = `${window.innerWidth}px`
    } else {
      root.style.position = root.style.position || 'relative'
      const rect = root.getBoundingClientRect()
      if (rect.top < 0 && rect.bottom > height) {
        toolbar.style.position = 'fixed'
        toolbar.style.top = '0px'
        toolbar.style.left = `${rect.left}px`
        toolbar.style.width = `${rect.width}px`
      } else if (rect.top >= 0) {
        toolbar.style.position = 'absolute'
        toolbar.style.top = '0px'
        toolbar.style.left = '0px'
        toolbar.style.width = '100%'
      } else if (rect.bottom <= height) {
        toolbar.style.position = 'absolute'
        toolbar.style.top = `${root.offsetHeight - height}px`
        toolbar.style.left = '0px'
        toolbar.style.width = '100%'
      }
    }
    toolbar.style.zIndex = '1002'
    if (placeholder) placeholder.style.height = `${height}px`
  }

  const attach = () => {
    if (placeholder) {
      update()
      return true
    }
    const container = options.container()
    root = container?.matches('.vditor') ? container : container?.querySelector('.vditor') as HTMLElement | null
    toolbar = root?.querySelector('.vditor-toolbar') as HTMLElement | null
    if (!root || !toolbar) return false
    placeholder = document.createElement('div')
    placeholder.style.width = '100%'
    placeholder.style.height = `${toolbar.offsetHeight}px`
    placeholder.style.pointerEvents = 'none'
    root.insertBefore(placeholder, toolbar.nextSibling)
    scrollContainers = []
    for (let ancestor = root.parentElement; ancestor; ancestor = ancestor.parentElement) {
      if (ancestor.matches('.center-col, .content-wrapper')) scrollContainers.push(ancestor)
    }
    scrollContainers.forEach((element) => element.addEventListener('scroll', update, { passive: true }))
    window.addEventListener('resize', update)
    window.addEventListener('scroll', update, { passive: true })
    if (typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(() => {
        if (resizeFrame !== null) cancelAnimationFrame(resizeFrame)
        resizeFrame = requestAnimationFrame(() => {
          resizeFrame = null
          update()
        })
      })
      resizeObserver.observe(root)
      const content = root.querySelector('.vditor-content') as HTMLElement | null
      if (content) resizeObserver.observe(content)
    }
    if (typeof MutationObserver !== 'undefined') {
      mutationObserver = new MutationObserver(update)
      mutationObserver.observe(root, { attributes: true, attributeFilter: ['class'] })
    }
    update()
    options.onReady?.()
    return true
  }

  const mount = () => {
    if (attach()) return
    retryTimers.forEach(clearTimeout)
    retryTimers = [50, 250, 1000, 2000].map((delay) => setTimeout(attach, delay))
  }

  const dispose = () => {
    retryTimers.forEach(clearTimeout)
    retryTimers = []
    scrollContainers.forEach((element) => element.removeEventListener('scroll', update))
    scrollContainers = []
    window.removeEventListener('resize', update)
    window.removeEventListener('scroll', update)
    mutationObserver?.disconnect()
    mutationObserver = null
    resizeObserver?.disconnect()
    resizeObserver = null
    if (resizeFrame !== null) cancelAnimationFrame(resizeFrame)
    resizeFrame = null
    if (toolbar) {
      toolbar.style.position = ''
      toolbar.style.top = ''
      toolbar.style.left = ''
      toolbar.style.width = ''
      toolbar.style.zIndex = ''
    }
    placeholder?.remove()
    placeholder = null
    toolbar = null
    root = null
  }

  return { mount, update, element: () => toolbar, dispose }
}
