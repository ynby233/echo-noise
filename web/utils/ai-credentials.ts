export interface AiCredential {
  id: number
  name: string
  created_at: string
}

export interface AiCredentialState {
  items: AiCredential[]
  name: string
  secret: string
  secretId: number | null
  loading: boolean
  busy: boolean
  error: string
}

export function createAiCredentialState(): AiCredentialState {
  return { items: [], name: '', secret: '', secretId: null, loading: false, busy: false, error: '' }
}

// Each profile instance owns this state. Never pass responses to a persisted store.
export function createAiCredentialManager(state: AiCredentialState, request: typeof fetch = fetch) {
  let version = 0
  let account: number | null = null
  let controller = new AbortController()
  const endpoint = '/api/user/ai-credentials'
  const metadata = (item: AiCredential): AiCredential => ({ id: item.id, name: item.name, created_at: item.created_at })
  const clearSecret = () => { state.secret = ''; state.secretId = null }
  const reset = (accountID: number | null) => {
    version++
    controller.abort()
    controller = new AbortController()
    account = accountID
    Object.assign(state, createAiCredentialState())
  }
  const send = async (path: string, options: RequestInit = {}) => {
    const response = await request(path, {
      ...options,
      credentials: 'include',
      cache: 'no-store',
      signal: controller.signal,
    })
    const body = await response.json()
    // Do not surface response bodies or exception messages: creation may contain a secret.
    if (!response.ok || body?.code !== 1) throw new Error('AI credential request failed')
    return body.data
  }
  const load = async () => {
    if (!account || state.loading || state.busy) return
    const current = version
    state.loading = true
    state.error = ''
    try {
      const data = await send(endpoint)
      if (current !== version) return
      if (!Array.isArray(data)) throw new Error('Invalid list')
      state.items = data.map(metadata)
    } catch {
      if (current === version) state.error = '无法加载 AI 凭据，请重试；登录失效时请重新登录。'
    } finally {
      if (current === version) state.loading = false
    }
  }
  const create = async () => {
    if (!account || state.busy || state.loading || state.secret) return
    const name = state.name.trim()
    if (!name || Array.from(name).length > 100) {
      state.error = '请输入 1–100 个字符的凭据名称。'
      return
    }
    const current = version
    state.busy = true
    state.error = ''
    try {
      const data = await send(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      })
      if (current !== version) return
      if (!data || typeof data.token !== 'string' || !data.token || !data.id) throw new Error('Invalid credential')
      state.items = [...state.items.filter(item => item.id !== data.id), metadata(data)]
      state.secret = data.token
      state.secretId = data.id
      state.name = ''
    } catch {
      if (current === version) state.error = '生成失败，凭据可能已创建但未收到响应。请刷新列表，撤销不再使用的凭据后重试。'
    } finally {
      if (current === version) state.busy = false
    }
  }
  const revoke = async (id: number) => {
    if (!account || state.busy || state.loading || !state.items.some(item => item.id === id)) return
    const current = version
    state.busy = true
    state.error = ''
    try {
      await send(`${endpoint}/${encodeURIComponent(id)}`, { method: 'DELETE' })
      if (current !== version) return
      state.items = state.items.filter(item => item.id !== id)
      if (state.secretId === id) clearSecret()
    } catch {
      if (current === version) state.error = '撤销失败，请重试；登录失效时请重新登录。'
    } finally {
      if (current === version) state.busy = false
    }
  }
  return { reset, load, create, revoke, clearSecret }
}
