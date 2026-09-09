import { bindMediaFancybox, createMediaFancyboxOptions, unbindMediaFancybox } from './media-fancybox'

type MessageListMediaOptions = {
  root: () => HTMLElement | null
}

// Owns message-list image wrapping, Fancybox binding and deferred work. Repeated
// updates are idempotent; dispose cancels pending work and unbinds the root.
export const createMessageListMedia = (options: MessageListMediaOptions) => {
  let mounted = false
  let scheduled = false
  let generation = 0
  let timeoutId: ReturnType<typeof setTimeout> | null = null
  let idleId: number | null = null

  const apply = () => {
    const root = options.root()
    if (!mounted || !root) return
    const fancyboxOptions = createMediaFancyboxOptions({ carouselInfinite: false, video: true })
    root.querySelectorAll<HTMLImageElement>('.markdown-preview img:not(.github-card-avatar)').forEach((image) => {
      if (image.closest('.image-grid-item')) return
      const src = image.getAttribute('src') || ''
      const parent = image.parentElement
      if (parent?.tagName === 'A') {
        parent.setAttribute('data-fancybox', 'uploaded-image')
        const href = parent.getAttribute('href') || ''
        const isImageHref = /\.(png|jpe?g|gif|webp|bmp|svg)(\?.*)?$/i.test(href) || href.startsWith('data:') || href.startsWith('blob:')
        if (!href || href === '#' || href.startsWith('javascript:') || !isImageHref) parent.setAttribute('href', src)
        return
      }
      if (!image.parentNode) return
      const wrapper = document.createElement('a')
      wrapper.href = src
      wrapper.setAttribute('data-fancybox', 'uploaded-image')
      wrapper.style.display = 'block'
      image.parentNode.insertBefore(wrapper, image)
      wrapper.appendChild(image)
    })
    bindMediaFancybox(root, fancyboxOptions)
  }

  const update = () => {
    if (!mounted || scheduled) return
    scheduled = true
    const currentGeneration = generation
    const run = () => {
      scheduled = false
      timeoutId = null
      idleId = null
      if (mounted && currentGeneration === generation) apply()
    }
    const browserWindow = window as Window & {
      requestIdleCallback?: (callback: () => void) => number
      cancelIdleCallback?: (id: number) => void
    }
    if (typeof browserWindow.requestIdleCallback === 'function') idleId = browserWindow.requestIdleCallback(run)
    else timeoutId = setTimeout(run, 0)
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
    const browserWindow = window as Window & { cancelIdleCallback?: (id: number) => void }
    if (idleId !== null) browserWindow.cancelIdleCallback?.(idleId)
    if (timeoutId !== null) clearTimeout(timeoutId)
    idleId = null
    timeoutId = null
    scheduled = false
    const root = options.root()
    if (root) unbindMediaFancybox(root)
  }

  return { mount, update, dispose }
}
