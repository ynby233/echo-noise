import type { AdminSectionLoader, AdminSectionModule } from './registry'

export interface AdminSectionModuleCache {
  get(loader: AdminSectionLoader): AdminSectionModule | undefined
  load(loader: AdminSectionLoader): Promise<AdminSectionModule>
  clear(): void
}

// Code only: loading a module must not mount it or request its configuration.
// Keep this cache inside StatusPanel so account/permission changes can discard it.
export function createAdminSectionModuleCache(): AdminSectionModuleCache {
  const resolved = new Map<AdminSectionLoader, AdminSectionModule>()
  const pending = new Map<AdminSectionLoader, Promise<AdminSectionModule>>()
  let generation = 0
  return {
    get: (loader: AdminSectionLoader) => resolved.get(loader),
    load(loader: AdminSectionLoader): Promise<AdminSectionModule> {
      const cached = resolved.get(loader)
      if (cached) return Promise.resolve(cached)
      const existing = pending.get(loader)
      if (existing) return existing
      const owner = generation
      const request = Promise.resolve().then(loader).then(module => {
        if (owner === generation) resolved.set(loader, module)
        return module
      }).finally(() => {
        if (pending.get(loader) === request) pending.delete(loader)
      })
      pending.set(loader, request)
      return request
    },
    clear() {
      ++generation
      resolved.clear()
      pending.clear()
    },
  }
}
