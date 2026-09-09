export type RenderedTableEnhancer = {
  mount: () => void
  update: () => void
  prepare: (table: HTMLTableElement) => void
  dispose: () => void
}

type RenderedTableEnhancerOptions = {
  root: () => HTMLElement | null
  open: (table: HTMLTableElement) => void
}

const TABLE_CELL_BREAK_RE = /<br\s*\/?\s*>/gi

// Owns the DOM inserted around rendered tables and every listener attached to
// that DOM. The expanded dialog remains a caller-provided action at this seam.
export const createRenderedTableEnhancer = (options: RenderedTableEnhancerOptions): RenderedTableEnhancer => {
  const buttons = new Map<HTMLButtonElement, { down: (event: MouseEvent) => void; click: (event: MouseEvent) => void }>()

  const releaseButton = (button: HTMLButtonElement) => {
    const listeners = buttons.get(button)
    if (!listeners) return
    button.removeEventListener('mousedown', listeners.down)
    button.removeEventListener('click', listeners.click)
    button.remove()
    buttons.delete(button)
  }

  const replaceBreakTextNodes = (table: HTMLTableElement) => {
    table.querySelectorAll('td,th').forEach((cell) => {
      const walker = document.createTreeWalker(cell, NodeFilter.SHOW_TEXT, {
        acceptNode(node) {
          return /<br\s*\/?\s*>/i.test(node.textContent || '') ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT
        },
      })
      const nodes: Text[] = []
      while (walker.nextNode()) nodes.push(walker.currentNode as Text)
      nodes.forEach((node) => {
        const parts = String(node.textContent || '').split(TABLE_CELL_BREAK_RE)
        if (parts.length <= 1) return
        const fragment = document.createDocumentFragment()
        parts.forEach((part, index) => {
          if (part) fragment.appendChild(document.createTextNode(part))
          if (index < parts.length - 1) fragment.appendChild(document.createElement('br'))
        })
        node.parentNode?.replaceChild(fragment, node)
      })
    })
  }

  const normalizeStructure = (table: HTMLTableElement) => {
    const head = table.tHead
    if (head) {
      const body = table.tBodies[0] || table.createTBody()
      Array.from(head.rows).reverse().forEach((row) => body.insertBefore(row, body.firstChild))
      head.remove()
    }
    Array.from(table.querySelectorAll('th')).forEach((cell) => {
      const replacement = document.createElement('td')
      Array.from(cell.attributes).forEach((attribute) => replacement.setAttribute(attribute.name, attribute.value))
      while (cell.firstChild) replacement.appendChild(cell.firstChild)
      cell.replaceWith(replacement)
    })
  }

  const prepare = (table: HTMLTableElement) => {
    normalizeStructure(table)
    replaceBreakTextNodes(table)
  }

  const ensureButton = (wrapper: HTMLElement, table: HTMLTableElement) => {
    if (wrapper.querySelector('.site-rendered-table-expand-button')) return
    const button = document.createElement('button')
    button.type = 'button'
    button.className = 'site-rendered-table-expand-button editor-table-expand-button nw-action-btn nw-tooltip-anchor'
    button.setAttribute('aria-label', '放大查看表格')
    button.setAttribute('data-tooltip', '放大查看表格')
    button.textContent = '⛶'
    const down = (event: MouseEvent) => {
      event.preventDefault()
      event.stopPropagation()
    }
    const click = (event: MouseEvent) => {
      event.preventDefault()
      event.stopPropagation()
      options.open(table)
    }
    button.addEventListener('mousedown', down)
    button.addEventListener('click', click)
    buttons.set(button, { down, click })
    wrapper.appendChild(button)
  }

  const update = () => {
    const root = options.root()
    buttons.forEach((_listeners, button) => {
      if (!root?.contains(button)) releaseButton(button)
    })
    if (!root) return
    root.querySelectorAll<HTMLTableElement>('table').forEach((table) => {
      prepare(table)
      const existing = table.closest<HTMLElement>('.site-table-scroll')
      if (existing) {
        ensureButton(existing, table)
        return
      }
      const parent = table.parentElement
      if (!parent) return
      const wrapper = document.createElement('div')
      wrapper.className = 'site-table-scroll'
      wrapper.dataset.renderedTableEnhancer = 'true'
      parent.insertBefore(wrapper, table)
      wrapper.appendChild(table)
      table.classList.add('site-scrollable-table')
      ensureButton(wrapper, table)
    })
  }

  const dispose = () => {
    buttons.forEach((_listeners, button) => releaseButton(button))
  }

  return { mount: update, update, prepare, dispose }
}
