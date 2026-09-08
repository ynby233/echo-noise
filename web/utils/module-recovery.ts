import { parse } from 'es-module-lexer/js'

type RecoveredModule = { url: string, module: Record<string, any> }
const modules = new Map<string, Promise<RecoveredModule>>()
let repaired = false
export const hasRecoveredModules = () => repaired

// Native imports retain both successful modules and failed static dependencies.
// Reuse successful modules verbatim. Only poisoned branches are fetched and
// relinked, so Vue/Pinia and already-running singletons keep their identity.
const recover = (url: string, parents: Set<string>): Promise<RecoveredModule> => {
  if (parents.has(url)) return Promise.reject(new Error(`Cannot recover a circular failed module graph: ${url}`))
  const existing = modules.get(url)
  if (existing) return existing
  const pending = (async () => {
    try { return { url, module: await import(/* @vite-ignore */ url) } }
    catch { /* fetch is independent of the browser's failed module record. */ }
    const response = await fetch(url, { credentials: 'same-origin' })
    if (!response.ok) throw new Error(`Unable to reload module ${url}: ${response.status}`)
    const source = await response.text()
    const [imports] = parse(source)
    const edits: Array<{ start: number, end: number, value: string }> = []
    const nextParents = new Set(parents).add(url)
    for (const item of imports) {
      if (item.d === -2) {
        // Relative assets/imports in a recovered chunk need its original base.
        if (source.slice(item.e, item.e + 4) === '.url') edits.push({ start: item.s, end: item.e + 4, value: JSON.stringify(url) })
        continue
      }
      if (!item.n) continue
      const dependency = new URL(item.n, url).href
      if (item.d === -1) {
        const target = await recover(dependency, nextParents)
        edits.push({ start: item.s - 1, end: item.e + 1, value: JSON.stringify(target.url) })
      } else edits.push({ start: item.s, end: item.e, value: JSON.stringify(dependency) })
    }
    let code = source
    for (const edit of edits.sort((a, b) => b.start - a.start)) code = code.slice(0, edit.start) + edit.value + code.slice(edit.end)
    const blob = URL.createObjectURL(new Blob([code], { type: 'text/javascript' }))
    try {
      const module = await import(/* @vite-ignore */ blob)
      repaired = true
      // Keep successful URLs for this page lifetime so other recovered parents
      // share exactly the same namespace and live export bindings.
      return { url: blob, module }
    } catch (error) { URL.revokeObjectURL(blob); throw error }
  })().catch(error => { modules.delete(url); throw error })
  modules.set(url, pending)
  return pending
}

export const recoverModule = (url: string) => recover(url, new Set()).then(result => result.module)
