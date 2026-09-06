import type { ObjectDirective } from 'vue'

// Keep DOM order and existing card instances while accounting for images and expanded content.
const instances = new WeakMap<HTMLElement, () => void>()

export const vMasonry: ObjectDirective<HTMLElement, boolean> = {
  mounted: sync,
  updated: sync,
  beforeUnmount(el) { instances.get(el)?.() },
}

function sync(el: HTMLElement, { value }: { value: boolean }) {
  if (!value) {
    instances.get(el)?.()
    return
  }
  if (instances.has(el)) return
  let frame = 0
  const cards = new Set<HTMLElement>()
  const measure = () => {
    cancelAnimationFrame(frame)
    frame = requestAnimationFrame(() => {
      const gap = parseFloat(getComputedStyle(el).getPropertyValue('--masonry-gap')) || 12
      for (const card of cards) {
        const span = Math.ceil(card.getBoundingClientRect().height + gap)
        card.style.gridRowEnd = `span ${Math.max(1, span)}`
      }
    })
  }
  const resize = new ResizeObserver(measure)
  const observeCards = () => {
    for (const card of cards) {
      if (card.parentElement !== el) {
        resize.unobserve(card)
        card.style.removeProperty('grid-row-end')
        cards.delete(card)
      }
    }
    for (const child of el.children) {
      if (child instanceof HTMLElement && !cards.has(child)) {
        cards.add(child)
        resize.observe(child)
      }
    }
    measure()
  }
  const mutations = new MutationObserver(observeCards)
  mutations.observe(el, { childList: true })
  resize.observe(el)
  observeCards()
  instances.set(el, () => {
    cancelAnimationFrame(frame)
    mutations.disconnect()
    resize.disconnect()
    for (const card of cards) card.style.removeProperty('grid-row-end')
    instances.delete(el)
  })
}
