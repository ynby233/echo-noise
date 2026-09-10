import { inject, onActivated, onDeactivated, onMounted, onUnmounted, ref, type InjectionKey, type Ref } from 'vue'

// Owns lightweight admin form drafts across lazy-section switches. Callers
// apply server state first; locally edited fields are then restored. Account
// changes and unmount invalidate in-flight loads and release the global event.

type Draft = { value: Record<string, any>, base: Record<string, any> }
export const adminDraftsKey: InjectionKey<Map<string, Draft>> = Symbol('admin drafts')
export const adminDraftAccountKey: InjectionKey<Readonly<Ref<string>>> = Symbol('admin draft account')
const copy = <T>(value: T): T => JSON.parse(JSON.stringify(value))
const assign = (target: Record<string, any>, source: Record<string, any>) => {
  for (const [field, value] of Object.entries(source)) {
    if (Array.isArray(target[field]) && Array.isArray(value)) target[field].splice(0, target[field].length, ...copy(value))
    else if (value && typeof value === 'object' && target[field] && typeof target[field] === 'object') assign(target[field], value)
    else target[field] = copy(value)
  }
}

export function useConfigDraft(key: string, form: Record<string, any>, read: () => Promise<any>, apply: (data: any) => void) {
  const drafts = inject(adminDraftsKey, new Map<string, Draft>())
  const account = inject(adminDraftAccountKey, ref(''))
  const owner = account.value
  const previous = drafts.get(key)
  let base = previous ? copy(previous.base) : copy(form)
  if (previous) assign(form, previous.value)
  const ready = ref(!!previous)
  const loading = ref(false)
  const error = ref('')
  let sequence = 0
  let disposed = false
  let active = true
  let stale = true
  const remember = () => { if (ready.value && owner === account.value) drafts.set(key, { value: copy(form), base: copy(base) }) }
  const load = async () => {
    if (disposed || owner !== account.value) return
    const request = ++sequence
    loading.value = true
    error.value = ''
    try {
      const data = await read()
      if (disposed || owner !== account.value || request !== sequence) return
      const edited = Object.fromEntries(Object.keys(form).filter(field => ready.value && JSON.stringify(form[field]) !== JSON.stringify(base[field])).map(field => [field, copy(form[field])]))
      apply(data)
      base = copy(form)
      assign(form, edited)
      ready.value = true
      stale = false
      remember()
    } catch (cause: any) {
      if (!disposed && request === sequence) error.value = cause?.message || '获取配置失败，请重试'
    } finally {
      if (!disposed && request === sequence) loading.value = false
    }
  }
  const saved = (fields: string[] = Object.keys(form)) => {
    if (disposed || owner !== account.value) return
    for (const field of fields) base[field] = copy(form[field])
    remember()
    return load()
  }
  const invalidate = () => { stale = true; if (active) void load() }
  onMounted(() => { window.addEventListener('frontend-config-updated', invalidate); void load() })
  onActivated(() => { active = true; if (stale && !loading.value) void load() })
  onDeactivated(() => { active = false; remember() })
  onUnmounted(() => { remember(); disposed = true; ++sequence; window.removeEventListener('frontend-config-updated', invalidate) })
  return { ready, loading, error, load, saved }
}
