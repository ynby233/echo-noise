type SetupState = 'unknown' | 'not_applicable' | 'required' | 'ready' | 'invalid' | 'unavailable'

const wait = (milliseconds: number) => new Promise(resolve => setTimeout(resolve, milliseconds))

export default defineNuxtRouteMiddleware(async (to) => {
  if (import.meta.server) return

  const baseApi = String(useRuntimeConfig().public.baseApi || '/api').replace(/\/$/, '')
  const localEmbeddedApi = /^http:\/\/(localhost|127\.0\.0\.1):1314\/api$/i.test(baseApi)
  const setupState = useState<SetupState>('mobile-setup-state', () => 'unknown')

  if (setupState.value === 'not_applicable') return
  if (setupState.value === 'ready' && to.path !== '/setup') return
  if ((setupState.value === 'required' || setupState.value === 'invalid') && to.path === '/setup') return

  const loadState = async (): Promise<SetupState> => {
    const attempts = localEmbeddedApi ? 20 : 1
    for (let attempt = 0; attempt < attempts; attempt += 1) {
      try {
        const response = await fetch(`${baseApi}/setup/status`, {
          credentials: 'include',
          cache: 'no-store',
        })
        if (response.status === 404) return 'not_applicable'
        const body = await response.json().catch(() => ({}))
        const state = String(body?.data?.setup_state || '')
        if (state === 'required' || state === 'ready' || state === 'invalid') return state
        return localEmbeddedApi ? 'unavailable' : 'not_applicable'
      } catch {
        if (attempt + 1 < attempts) await wait(250)
      }
    }
    return localEmbeddedApi ? 'unavailable' : 'not_applicable'
  }

  setupState.value = await loadState()
  const state = setupState.value
  if (state === 'required' || state === 'invalid' || state === 'unavailable') {
    if (to.path === '/setup') return
    return navigateTo('/setup', { replace: true })
  }
  if (state === 'ready' && to.path === '/setup') {
    return navigateTo('/', { replace: true })
  }
})
