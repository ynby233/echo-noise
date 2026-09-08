const mediaScopes = new WeakMap<HTMLElement, { options: Record<string, any>, load: () => Promise<any> }>()
export const registerMediaScope = (root: HTMLElement, options: Record<string, any>, load: () => Promise<any>) => mediaScopes.set(root, { options, load })
export const unregisterMediaScope = (root: HTMLElement) => mediaScopes.delete(root)

// Delegation keeps newly rendered media working without eager viewer creation.
// The nearest registered root owns grouping/options; the application owns cleanup.
export const installMediaFancybox = (onError: () => void) => {
  let disposed = false
  let opening = false
  const onClick = async (event: MouseEvent) => {
    if (event.defaultPrevented || event.button !== 0 || event.ctrlKey || event.metaKey || event.altKey || event.shiftKey) return
    const trigger = event.target instanceof Element ? event.target.closest<HTMLElement>('[data-fancybox]') : null
    if (!trigger) return
    let root: HTMLElement | null = trigger
    while (root && !mediaScopes.has(root)) root = root.parentElement
    if (!root) return
    event.preventDefault()
    if (opening) return
    opening = true
    trigger.setAttribute('aria-busy', 'true')
    const group = trigger.dataset.fancybox
    const nodes = group ? [...root.querySelectorAll<HTMLElement>('[data-fancybox]')].filter(node => node.dataset.fancybox === group) : [trigger]
    try {
      const scope = mediaScopes.get(root)!
      const Fancybox = await scope.load()
      if (disposed || !trigger.isConnected) return
      Fancybox.fromNodes(nodes, { ...scope.options, startIndex: Math.max(0, nodes.indexOf(trigger)), triggerEl: trigger })
    } catch { if (!disposed) onError() }
    finally { opening = false; trigger.removeAttribute('aria-busy') }
  }
  document.addEventListener('click', onClick)
  return () => { disposed = true; document.removeEventListener('click', onClick) }
}
