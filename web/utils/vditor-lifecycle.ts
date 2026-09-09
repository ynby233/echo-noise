import type Vditor from 'vditor'

export type VditorLifecycle = {
  mount: () => Vditor | null
  setTheme: (theme: 'dark' | 'classic') => void
  ready: () => boolean
  dispose: () => void
}

type VditorLifecycleOptions = {
  container: () => HTMLElement | null
  options: () => Record<string, any>
  create: (container: HTMLElement, options: Record<string, any>) => Vditor
  onInstance: (instance: Vditor | null) => void
  onReady: (instance: Vditor) => void
}

// Owns Vditor construction, asynchronous readiness, theme gating and destroy.
// The caller supplies behavior callbacks but never constructs or destroys it.
export const createVditorLifecycle = (options: VditorLifecycleOptions): VditorLifecycle => {
  let instance: Vditor | null = null
  let isReady = false
  let generation = 0

  const mount = () => {
    const container = options.container()
    if (!container || instance) return instance
    const currentGeneration = ++generation
    const editorOptions = options.options()
    let readyBeforeInstance = false
    const finishReady = () => {
      if (!instance || isReady || currentGeneration !== generation) return
      isReady = true
      options.onReady(instance)
    }
    instance = options.create(container, {
      ...editorOptions,
      after: () => {
        if (!instance) {
          readyBeforeInstance = true
          return
        }
        finishReady()
      },
    })
    options.onInstance(instance)
    if (readyBeforeInstance) finishReady()
    return instance
  }

  const setTheme = (theme: 'dark' | 'classic') => {
    if (instance && isReady) instance.setTheme(theme)
  }

  const dispose = () => {
    generation += 1
    isReady = false
    const current = instance
    instance = null
    options.onInstance(null)
    current?.destroy()
  }

  return { mount, setTheme, ready: () => isReady, dispose }
}
