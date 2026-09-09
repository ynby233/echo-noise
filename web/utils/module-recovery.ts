type RecoveredModule = Record<string, any>
const modules = new Map<string, Promise<RecoveredModule>>()
let repaired = false
export const hasRecoveredModules = () => repaired

const recoveryURL = (url: string) => {
  const target = new URL(url, location.href)
  const slash = target.pathname.lastIndexOf('/')
  target.pathname = `${target.pathname.slice(0, slash + 1)}__retry__/${target.pathname.slice(slash + 1)}`
  target.search = ''
  target.hash = ''
  return target.href
}

// Recovery chunks are generated with the production bundle and execute as
// ordinary same-origin modules. Their static graph uses a second set of URLs,
// while imports of the already-running app/core graph retain their identity.
export const recoverModule = (url: string) => {
  const target = recoveryURL(url)
  const existing = modules.get(target)
  if (existing) return existing
  const pending = import(/* @vite-ignore */ target)
    .then((module) => { repaired = true; return module as RecoveredModule })
    .catch((error) => {
      modules.delete(target)
      // The browser retains a rejected native module even after our Promise is
      // removed. A failed recovery graph needs a new document, not another
      // import of the same URL. The app offers an explicit, draft-safe reload.
      window.dispatchEvent(new Event('module-recovery-failed'))
      throw error
    })
  modules.set(target, pending)
  return pending
}
