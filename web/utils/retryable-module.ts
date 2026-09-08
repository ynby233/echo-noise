import { hasRecoveredModules, recoverModule } from './module-recovery'

type Module<T> = { default: T }
type PreloadOwner = (load: () => Promise<any>, dependencies: string[]) => Promise<any>
let currentOwner: PreloadOwner | undefined
const stylesheets = new Map<string, Promise<void>>()

const loadStylesheet = (href: string) => {
  const existing = stylesheets.get(href)
  if (existing) return existing
  if ([...document.querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"]')].some(link => link.href === href && link.sheet)) return Promise.resolve()
  const pending = new Promise<void>((resolve, reject) => {
    const link = document.createElement('link')
    link.rel = 'stylesheet'
    link.href = href
    link.crossOrigin = ''
    const nonce = document.querySelector<HTMLMetaElement>('meta[property=csp-nonce]')?.nonce
    if (nonce) link.nonce = nonce
    const cleanup = () => { link.onload = link.onerror = null }
    link.onload = () => { cleanup(); resolve() }
    link.onerror = () => { cleanup(); link.remove(); reject(new Error(`Unable to preload CSS for ${href}`)) }
    document.head.appendChild(link)
  }).catch(error => { stylesheets.delete(href); throw error })
  stylesheets.set(href, pending)
  return pending
}

const preloadScripts = (urls: string[]) => {
  const linked = new Set([...document.querySelectorAll<HTMLLinkElement>('link[href]')].map(link => link.href))
  for (const url of urls) {
    if (linked.has(url)) continue
    const link = document.createElement('link')
    link.rel = 'modulepreload'
    link.as = 'script'
    link.crossOrigin = ''
    link.href = url
    document.head.appendChild(link)
  }
}

// The build bridge invokes this synchronously inside loader(). Ownership ends
// when that call returns, not when its network finishes; errors stay scoped.
export const runScopedPreload = (load: () => Promise<any>, dependencies: string[]) => currentOwner?.(load, dependencies)

const failedModuleURL = (error: unknown) => {
  const message = String(error instanceof Error ? error.message : error)
  const match = message.match(/(?:https?:\/\/[^\s]+|\/_nuxt\/[^\s]+)\.js(?:\?[^\s]*)?/)
  if (!match) return null
  const url = new URL(match[0], location.href)
  return url.origin === location.origin && url.pathname.includes('/_nuxt/') ? url.href : null
}

/** One in-flight/successful result per feature. Failed CSS is requested again;
 * failed JS is recovered with static dependencies without reloading the page.
 */
export const createRetryableModule = <T>(loader: () => Promise<Module<T>>) => {
  let pending: Promise<Module<T>> | undefined
  let failedURL: string | null = null
  let styles: string[] = []
  const invoke = () => {
    const previous = currentOwner
    currentOwner = async (load, dependencies) => {
      const urls = dependencies.map(dep => new URL(dep, location.href).href)
      styles = urls.filter(url => new URL(url).pathname.endsWith('.css'))
      preloadScripts(urls.filter(url => !new URL(url).pathname.endsWith('.css')))
      await Promise.all(styles.map(loadStylesheet))
      return load()
    }
    try { return loader() }
    finally { currentOwner = previous }
  }
  const load = async (): Promise<Module<T>> => {
    if (failedURL) {
      await Promise.all(styles.map(loadStylesheet))
      return recoverModule(failedURL) as Promise<Module<T>>
    }
    try { return await invoke() }
    catch (error) {
      failedURL = failedModuleURL(error)
      // Another feature may use a dependency that has already been repaired.
      if (failedURL && hasRecoveredModules()) return recoverModule(failedURL) as Promise<Module<T>>
      throw error
    }
  }
  return () => {
    pending ??= load().catch(error => { pending = undefined; throw error })
    return pending
  }
}
