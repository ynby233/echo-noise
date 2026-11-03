export type AuthorSubscription = { author_id: number; subscribed: boolean }
export type AuthorSubscriptionEntry = {
  subscribed: boolean
  ready: boolean
  busy: boolean
  error: string
}
type Session = { isLogin: boolean; userId: unknown; token: string }
type SubscriptionResponse = { code: number; data?: AuthorSubscription | null; msg?: string } | null
type Request = (method: 'GET' | 'PUT' | 'DELETE', authorId: number) => Promise<SubscriptionResponse>

export const validAuthorId = (value: unknown): number => {
  if (typeof value !== 'number' && (typeof value !== 'string' || !/^[1-9]\d*$/.test(value))) return 0
  const id = Number(value)
  return Number.isSafeInteger(id) && id > 0 ? id : 0
}

// The caller supplies reactive entries; all cards for an author share one record.
export function createAuthorSubscriptions(entries: Record<number, AuthorSubscriptionEntry>, request: Request) {
  let identity = ''
  let viewerId = 0
  let generation = 0
  const flights = new Map<number, Promise<void>>()
  const setSession = (session: Session) => {
    const id = session.isLogin ? validAuthorId(session.userId) : 0
    const next = JSON.stringify([session.isLogin, id, session.token])
    if (next === identity) return
    identity = next
    viewerId = id
    generation++
    flights.clear()
    for (const key of Object.keys(entries)) delete entries[Number(key)]
  }
  const canSubscribe = (value: unknown) => {
    const id = validAuthorId(value)
    return viewerId > 0 && id > 0 && viewerId !== id
  }
  const entry = (value: unknown) => {
    const id = validAuthorId(value)
    if (!canSubscribe(id)) return undefined
    if (!entries[id]) entries[id] = { subscribed: false, ready: false, busy: false, error: '' }
    return entries[id]
  }
  const run = (value: unknown, mutate: boolean): Promise<void> => {
    const id = validAuthorId(value)
    const state = entry(id)
    if (!state) return Promise.resolve()
    const existing = flights.get(id)
    if (existing) return existing
    if (mutate ? (!state.ready || !!state.error) : state.ready) return Promise.resolve()
    const epoch = generation
    const method = mutate ? (state.subscribed ? 'DELETE' : 'PUT') : 'GET'
    state.busy = true
    state.error = ''
    const current = () => epoch === generation && entries[id] === state
    const flight = (async () => {
      try {
        const response = await request(method, id)
        if (!current()) return
        const data = response?.data
        if (response?.code !== 1 || data?.author_id !== id || typeof data?.subscribed !== 'boolean') {
          throw new Error(response?.msg || '订阅状态加载失败，请重试')
        }
        state.subscribed = data.subscribed
        state.ready = true
      } catch (error: unknown) {
        if (!current()) return
        state.error = error instanceof Error ? error.message : '订阅操作失败，请重试'
        state.ready = false
      } finally {
        if (current()) {
          state.busy = false
          flights.delete(id)
        }
      }
    })()
    flights.set(id, flight)
    return flight
  }
  return { setSession, canSubscribe, entry, load: (id: unknown) => run(id, false), toggle: (id: unknown) => run(id, true) }
}
