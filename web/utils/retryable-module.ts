type Module<T> = { default: T }
const recoveredModules = new Map<string, Module<unknown>>()

const failedAssetURL = (error: unknown) => {
  const message = String(error instanceof Error ? error.message : error)
  const match = message.match(/(?:https?:\/\/[^\s]+|\/_nuxt\/[^\s]+)\.(?:js|css)(?:\?[^\s]*)?/)
  if (!match || typeof location === 'undefined') return null
  const url = new URL(match[0], location.href)
  return url.origin === location.origin && url.pathname.includes('/_nuxt/') ? url : null
}

const retryStylesheet = (url: URL) => new Promise<void>((resolve, reject) => {
  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.href = url.href
  link.onload = () => { link.onload = link.onerror = null; resolve() }
  link.onerror = () => { link.remove(); reject(new Error(`Unable to preload CSS for ${url.href}`)) }
  document.head.appendChild(link)
})

/** Share a feature download and allow the caller to retry without reloading its
 * page. Entry modules export default so a browser retry retains the same shape.
 * Chromium caches rejected module URLs; Vite also remembers failed CSS preloads.
 * Only after an actual failure do we request a fresh URL or repair a stylesheet.
 */
export const createRetryableModule = <T>(loader: () => Promise<Module<T>>) => {
  let pending: Promise<Module<T>> | undefined
  let failedURL: URL | null = null
  let attempt = 0
  const loadWithStyles = async () => {
    // Nuxt suppresses Vite CSS preload rejections. Remember the actual failure
    // during this download so we do not present an unstyled dialog as ready.
    let styleError: unknown
    const onPreloadError = (event: Event) => {
      const error = (event as Event & { payload?: unknown }).payload
      if (failedAssetURL(error)?.pathname.endsWith('.css')) styleError = error
    }
    window.addEventListener('vite:preloadError', onPreloadError)
    try {
      const module = await loader()
      if (styleError) throw styleError
      return module
    } finally { window.removeEventListener('vite:preloadError', onPreloadError) }
  }
  const load = async (): Promise<Module<T>> => {
    if (!failedURL) return loadWithStyles()
    const key = failedURL.origin + failedURL.pathname
    if (recoveredModules.has(key)) return recoveredModules.get(key)! as Module<T>
    const url = new URL(failedURL)
    url.searchParams.set('load_retry', String(++attempt))
    if (url.pathname.endsWith('.css')) {
      await retryStylesheet(url)
      return loadWithStyles()
    }
    const module = await import(/* @vite-ignore */ url.href)
    if (!module.default) throw new Error('模块加载失败，请重试。')
    recoveredModules.set(key, module)
    return module
  }
  return () => {
    pending ??= load().catch(error => {
      pending = undefined
      failedURL = failedAssetURL(error) || failedURL
      const key = failedURL && failedURL.origin + failedURL.pathname
      if (key && recoveredModules.has(key)) return recoveredModules.get(key)! as Module<T>
      throw error
    })
    return pending
  }
}
