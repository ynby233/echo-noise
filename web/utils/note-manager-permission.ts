export interface NoteManagerPermissionHandlerOptions {
  clearState: () => void
  refreshCapabilities: () => Promise<unknown> | unknown
  notify: () => void
}

export function createNoteManagerPermissionHandler(options: NoteManagerPermissionHandlerOptions) {
  let refreshPromise: Promise<unknown> | null = null
  let refreshStarted = false
  let notified = false

  const handler = async () => {
    options.clearState()
    if (!refreshStarted) {
      refreshStarted = true
      refreshPromise = Promise.resolve(options.refreshCapabilities()).catch(() => undefined).finally(() => {
        refreshPromise = null
      })
    }
    if (refreshPromise) await refreshPromise
    if (!notified) {
      notified = true
      options.notify()
    }
  }

  handler.reset = () => {
    refreshPromise = null
    refreshStarted = false
    notified = false
  }
  return handler
}
