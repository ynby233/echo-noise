export type RenderedTaskListEnhancer = {
  mount: () => void
  update: () => void
  dispose: () => void
}

type RenderedTaskListOptions = {
  root: () => HTMLElement | null
  content: () => string
  setContent: (content: string) => void
  editable: () => boolean
  messageId: () => number
  persist: (messageId: number, content: string) => Promise<unknown>
  reportError?: (error: unknown) => void
}

const TASK_LINE_REG = /^(\s*(?:[-*+]|\d+[.)])\s+\[)([ xX])(\]\s+)/

// Owns task-checkbox DOM state, persistence and cleanup. The renderer only
// supplies current state and calls mount/update/dispose at its lifecycle seam.
export const createRenderedTaskListEnhancer = (options: RenderedTaskListOptions): RenderedTaskListEnhancer => {
  const inFlight = new Set<number>()
  let observer: MutationObserver | null = null
  let updateTimer: ReturnType<typeof setTimeout> | null = null
  let mountedRoot: HTMLElement | null = null
  let generation = 0

  const updateContent = (content: string, taskIndex: number, checked: boolean) => {
    let seen = -1
    const lines = String(content || '').split('\n')
    const nextLines = lines.map((line) => {
      if (!TASK_LINE_REG.test(line)) return line
      seen += 1
      if (seen !== taskIndex) return line
      return line.replace(TASK_LINE_REG, `$1${checked ? 'x' : ' '}$3`)
    })
    return seen >= taskIndex ? nextLines.join('\n') : ''
  }

  const checkedInContent = (taskIndex: number) => {
    let seen = -1
    for (const line of String(options.content() || '').split('\n')) {
      const match = line.match(TASK_LINE_REG)
      if (!match) continue
      seen += 1
      if (seen === taskIndex) return match[2].toLowerCase() === 'x'
    }
    return false
  }

  const inputs = () => Array.from(options.root()?.querySelectorAll<HTMLInputElement>('input[type="checkbox"]') || [])
  const indexFor = (input: HTMLInputElement) => {
    const index = Number(input.dataset.taskIndex)
    return Number.isInteger(index) && index >= 0 ? index : inputs().indexOf(input)
  }
  const syncVisualState = (input: HTMLInputElement) => {
    const item = input.closest('li')
    item?.classList.add('markdown-task-list-item')
    item?.classList.toggle('is-task-checked', input.checked)
    input.setAttribute('aria-label', input.checked ? '已完成任务' : '未完成任务')
  }
  const reset = (input: HTMLInputElement, index = indexFor(input)) => {
    input.checked = index >= 0 ? checkedInContent(index) : input.defaultChecked
    syncVisualState(input)
  }
  const persist = async (input: HTMLInputElement, index: number, checked: boolean) => {
    if (!options.editable() || !options.messageId() || inFlight.has(index)) return false
    const currentGeneration = generation
    const currentRoot = mountedRoot
    const previousContent = options.content()
    const nextContent = updateContent(previousContent, index, checked)
    if (!nextContent) return false
    options.setContent(nextContent)
    inFlight.add(index)
    input.disabled = true
    try {
      const response = await options.persist(options.messageId(), nextContent)
      if (currentGeneration !== generation || currentRoot !== mountedRoot) return false
      if (!response) throw new Error('更新任务状态失败')
      return true
    } catch (error) {
      if (currentGeneration !== generation || currentRoot !== mountedRoot) return false
      if (options.content() === nextContent) options.setContent(previousContent)
      if (currentRoot?.contains(input)) {
        input.checked = checkedInContent(index)
        syncVisualState(input)
      }
      options.reportError?.(error)
      return false
    } finally {
      if (currentGeneration === generation) {
        inFlight.delete(index)
        if (currentRoot?.contains(input)) input.disabled = !options.editable()
      }
    }
  }

  const updateNow = () => {
    const root = options.root()
    if (!root) return
    const editable = options.editable()
    root.dataset.taskListEditable = editable ? 'true' : 'false'
    inputs().forEach((input, index) => {
      input.dataset.taskIndex = String(index)
      input.checked = checkedInContent(index)
      input.disabled = !editable
      if (editable) input.removeAttribute('disabled')
      else input.setAttribute('disabled', 'disabled')
      input.setAttribute('aria-disabled', editable ? 'false' : 'true')
      input.tabIndex = editable ? 0 : -1
      input.style.pointerEvents = editable ? 'auto' : 'none'
      input.style.cursor = editable ? 'pointer' : 'default'
      syncVisualState(input)
    })
  }

  const update = () => {
    if (updateTimer) clearTimeout(updateTimer)
    updateTimer = setTimeout(() => {
      updateTimer = null
      updateNow()
    }, 0)
  }
  const onClick = (event: Event) => {
    const input = (event.target as HTMLElement | null)?.closest('input[type="checkbox"]') as HTMLInputElement | null
    if (!input || !options.root()?.contains(input)) return
    event.stopPropagation()
    if (!options.editable()) {
      event.preventDefault()
      ;(event as any).stopImmediatePropagation?.()
      reset(input)
      update()
    }
  }
  const onChange = async (event: Event) => {
    const input = (event.target as HTMLElement | null)?.closest('input[type="checkbox"]') as HTMLInputElement | null
    if (!input || !options.root()?.contains(input)) return
    event.stopPropagation()
    const index = indexFor(input)
    if (!options.editable() || index < 0) {
      event.preventDefault()
      reset(input, index)
      update()
      return
    }
    syncVisualState(input)
    await persist(input, index, input.checked)
  }
  const mount = () => {
    const root = options.root()
    if (!root || root === mountedRoot) return
    if (mountedRoot) dispose()
    generation += 1
    mountedRoot = root
    root.addEventListener('click', onClick, true)
    root.addEventListener('change', onChange, true)
    if (typeof MutationObserver !== 'undefined') {
      observer = new MutationObserver(update)
      observer.observe(root, { childList: true, subtree: true })
    }
    update()
  }
  const dispose = () => {
    generation += 1
    mountedRoot?.removeEventListener('click', onClick, true)
    mountedRoot?.removeEventListener('change', onChange, true)
    mountedRoot = null
    observer?.disconnect()
    observer = null
    if (updateTimer) clearTimeout(updateTimer)
    updateTimer = null
    inFlight.clear()
  }

  return { mount, update, dispose }
}
