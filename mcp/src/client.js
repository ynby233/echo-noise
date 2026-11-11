const MAX_RESPONSE_BYTES = 2 * 1024 * 1024
// Match credential fields, not arbitrary substrings such as credential_id,
// token_count, session_timeout or a note's description of authentication.
const SENSITIVE_KEYS = new Set([
  'password', 'passwd', 'passphrase', 'token', 'accesstoken', 'refreshtoken',
  'authtoken', 'aitoken', 'notetoken', 'secret', 'clientsecret', 'credential',
  'credentials', 'authorization', 'proxyauthorization', 'cookie', 'setcookie',
  'session', 'sessionid', 'sessiontoken', 'sessioncookie', 'apikey',
  'smtppass', 'smtppassword', 'smtpuser', 'smtpusername', 'privatekey',
  'bottoken', 'telegramtoken', 'webhooksecret', 'signingsecret',
])
const sensitiveKey = key => {
  const words = key.replace(/([a-z0-9])([A-Z])/g, '$1_$2').toLowerCase()
  return SENSITIVE_KEYS.has(words.replace(/[-_\s]/g, '')) ||
    /(?:^|[-_\s])(?:password|passwd|passphrase|token|secret|credentials?|authorization|cookie|api[-_\s]?key|private[-_\s]?key|password[-_\s]?(?:hash|salt))$/.test(words)
}

export class NoteClientError extends Error {
  constructor(code, message) {
    super(message)
    this.name = 'NoteClientError'
    this.code = code
  }
}

function failure(code, message) {
  return new NoteClientError(code, message)
}

/** A client belongs to one incoming MCP request; never reuse it across identities. */
export class NoteClient {
  #host
  #token
  #timeoutMs

  constructor({ host, token, timeoutMs = 10000 }) {
    let url
    try { url = new URL(host) } catch { throw failure('CONFIG', '服务地址无效') }
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
      throw failure('CONFIG', '服务地址必须是不含凭据、查询和片段的 HTTP(S) 地址')
    }
    if (typeof token !== 'string' || /[\s\x00-\x1f\x7f]/.test(token)) {
      throw failure('CONFIG', '必须提供有效的 AI 凭据')
    }
    if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > 2147483647) {
      throw failure('CONFIG', '超时时间无效')
    }
    this.#host = url.href.replace(/\/$/, '')
    this.#token = token
    this.#timeoutMs = timeoutMs
  }

  sanitize(value, secrets = [], depth = 0) {
    if (depth > 64) return '[已省略]'
    if (typeof value === 'string') {
      let result = value
      for (const secret of [this.#token, ...secrets]) {
        if (typeof secret === 'string' && secret) result = result.split(secret).join('[已隐藏]')
      }
      // Free text may discuss Bearer authentication. Only redact actual known
      // secrets here; credential-bearing object fields are removed below.
      return result
    }
    if (Array.isArray(value)) return value.map(item => this.sanitize(item, secrets, depth + 1))
    if (value && typeof value === 'object') {
      return Object.fromEntries(Object.entries(value)
        .filter(([key]) => !sensitiveKey(key) && !['__proto__', 'constructor', 'prototype'].includes(key))
        .map(([key, item]) => [this.sanitize(key, secrets, depth + 1), this.sanitize(item, secrets, depth + 1)]))
    }
    return value
  }

  async #request(path, { method = 'GET', query, body, text = false, secrets = [] } = {}) {
    if (!this.#token) throw failure('IDENTITY', '必须提供 AI 凭据')
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), this.#timeoutMs)
    try {
      const url = new URL(this.#host + path)
      for (const [key, value] of Object.entries(query || {})) {
        if (value !== undefined) url.searchParams.set(key, String(value))
      }
      const response = await fetch(url, {
        method,
        headers: {
          Authorization: `Bearer ${this.#token}`,
          Accept: text ? 'application/rss+xml, application/xml, text/xml, text/plain' : 'application/json',
          ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
        },
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: controller.signal,
        redirect: 'error',
        credentials: 'omit',
        cache: 'no-store',
      })
      if (!response.ok && response.status !== 401) {
        controller.abort()
        if (response.status === 403) throw failure('FORBIDDEN', '权限不足，当前账号无权执行此操作；重新登录不会增加权限。')
        throw failure('HTTP', `后端请求失败（HTTP ${response.status}）`)
      }
      if (Number(response.headers.get('content-length')) > MAX_RESPONSE_BYTES) {
        controller.abort()
        throw failure('LIMIT', '后端响应超过大小限制')
      }
      const reader = response.body?.getReader()
      const chunks = []
      let size = 0
      if (reader) {
        try {
          while (true) {
            const { done, value } = await reader.read()
            if (done) break
            size += value.byteLength
            if (size > MAX_RESPONSE_BYTES) {
              controller.abort()
              throw failure('LIMIT', '后端响应超过大小限制')
            }
            chunks.push(value)
          }
        } finally { reader.releaseLock() }
      }
      const bytes = new Uint8Array(size)
      let offset = 0
      for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength }
      const raw = new TextDecoder('utf-8', { fatal: true }).decode(bytes)
      if (response.status === 401) {
        let authError
        try { authError = JSON.parse(raw) } catch {}
        // Only stable error codes select local text. Never echo arbitrary remote messages.
        if (authError?.code === 0 && authError?.error === 'login_expired') {
          throw failure('LOGIN_EXPIRED', '登录已过期，请重新登录站点，或使用 login 工具以同账号密码续期后重试。')
        }
        if (path === '/api/ai/login') {
          throw failure('INVALID_CREDENTIALS', '登录续期失败，请检查同账号用户名、密码及 AI 凭据；已撤销的凭据需重新创建。')
        }
        throw failure('INVALID_CREDENTIALS', 'AI 凭据无效或已撤销，请检查配置或在个人设置中创建新凭据。')
      }
      if (text) {
        const type = response.headers.get('content-type') || ''
        if (!/^(application\/(rss\+xml|xml)|text\/(xml|plain))(;|$)/i.test(type)) {
          throw failure('RESPONSE', 'RSS 响应格式无效')
        }
        return this.sanitize(raw, secrets)
      }
      let result
      try { result = JSON.parse(raw) } catch { throw failure('RESPONSE', '后端响应格式无效') }
      if (!result || typeof result !== 'object' || Array.isArray(result) || result.code !== 1) {
        throw failure('BUSINESS', '后端拒绝操作或返回无效业务状态')
      }
      // Remote messages are deliberately omitted even on success.
      return this.sanitize(result.data ?? null, secrets)
    } catch (error) {
      if (error instanceof NoteClientError) throw error
      if (controller.signal.aborted) throw failure('TIMEOUT', '后端请求超时')
      throw failure('NETWORK', '后端连接失败或响应无效')
    } finally { clearTimeout(timer) }
  }

  async identity() {
    const identity = await this.#request('/api/auth/identity')
    if (!identity || identity.auth_via !== 'ai_token' || !identity.user_id || typeof identity.username !== 'string') {
      throw failure('IDENTITY', '必须使用经后端验证的 AI 凭据')
    }
    return identity
  }

  async #authorized(path, options) {
    await this.identity()
    return this.#request(path, options)
  }

  search({ keyword = '', ...query } = {}) {
    const term = keyword.trim()
    if (!term) return this.page(query)
    if (term.startsWith('#')) {
      return this.#authorized(`/api/messages/tags/${encodeURIComponent(term.slice(1))}`, { query })
    }
    return this.#authorized('/api/messages/search', { query: { ...query, keyword: term } })
  }
  page(args) { return this.#authorized('/api/messages/page', { query: args }) }
  message({ id }) { return this.#authorized(`/api/messages/${encodeURIComponent(id)}`) }
  publish(args) { return this.#authorized('/api/token/messages', { method: 'POST', body: args }) }
  update({ id, ...body }) { return this.#authorized(`/api/token/messages/${encodeURIComponent(id)}`, { method: 'PUT', body }) }
  delete({ id }) { return this.#authorized(`/api/token/messages/${encodeURIComponent(id)}`, { method: 'DELETE' }) }
  pin({ id, pinned, scope = 'global' }) {
    return this.#authorized(`/api/token/messages/${encodeURIComponent(id)}/pin/${scope}`, { method: 'PUT', body: { pinned } })
  }
  settings(args) { return this.#authorized('/api/token/settings', { method: 'PUT', body: args }) }
  status() { return this.#authorized('/api/status') }
  calendar(args) { return this.#authorized('/api/messages/calendar', { query: args }) }
  config() { return this.#authorized('/api/frontend/config') }
  rss() { return this.#authorized('/rss', { text: true }) }

  async login({ username, password }) {
    // Expired login windows must be renewable; do not preflight this request.
    await this.#request('/api/ai/login', { method: 'POST', body: { username, password }, secrets: [password] })
    const identity = await this.identity()
    if (identity.username !== username) throw failure('IDENTITY', '续期身份不匹配')
    return { renewed: true, identity: this.sanitize(identity, [password]) }
  }
}
