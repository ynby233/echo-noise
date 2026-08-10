export interface NoteManagerPermissionHandlerOptions {
  clearState: () => void
  refreshCapabilities: () => Promise<unknown> | unknown
  notify: () => void
}

export function createNoteManagerPermissionHandler(options: NoteManagerPermissionHandlerOptions) {
  let refreshPromise: Promise<unknown> | null = null
  let notified = false

  return async () => {
    options.clearState()
    if (!refreshPromise) {
      refreshPromise = Promise.resolve(options.refreshCapabilities()).catch(() => undefined)
    }
    await refreshPromise
    if (!notified) {
      notified = true
      options.notify()
    }
  }
}
