import http from 'node:http'
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js'
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js'
import { NoteClient } from './client.js'
import { createMcpServer } from './tools.js'

const integer = (value, fallback, min, max) => {
  const number = value === undefined || value === '' ? fallback : Number(value)
  if (!Number.isInteger(number) || number < min || number > max) throw new Error('Invalid numeric transport configuration')
  return number
}
const list = (value, fallback) => value === undefined ? fallback : value.split(',').map(s => s.trim()).filter(Boolean)

// Node may discard duplicate Host/Authorization fields in req.headers.
// Read the wire headers so ambiguous values cannot become trusted singletons.
const headerValues = (req, name) => {
  const values = []
  for (let i = 0; i < req.rawHeaders.length; i += 2) {
    if (req.rawHeaders[i].toLowerCase() === name) values.push(req.rawHeaders[i + 1])
  }
  return values
}

export async function startStdio({ host, token, timeoutMs } = {}) {
  const client = new NoteClient({ host, token, timeoutMs })
  const server = createMcpServer(client)
  await server.connect(new StdioServerTransport())
  return server
}

// Each HTTP request owns its entire MCP server and API client. No session map,
// cookie jar, server token, login state, or event broadcast is shared.
export async function startHttp({
  host, bindHost = '127.0.0.1', port = 1315, allowedHosts, allowedOrigins = [],
  timeoutMs = 10000, maxBodyBytes = 1048576, maxConcurrent = 32, requestsPerMinute = 120,
} = {}) {
  port = integer(port, 1315, 0, 65535)
  timeoutMs = integer(timeoutMs, 10000, 100, 120000)
  maxBodyBytes = integer(maxBodyBytes, 1048576, 1024, 10485760)
  maxConcurrent = integer(maxConcurrent, 32, 1, 1024)
  requestsPerMinute = integer(requestsPerMinute, 120, 1, 100000)
  // Validate the upstream URL before listening; never take it from the request.
  new NoteClient({ host, token: '', timeoutMs })
  let hosts = new Set(allowedHosts || [])
  const origins = new Set(allowedOrigins)
  let active = 0
  let windowStart = Date.now()
  let requests = 0
  const reject = (res, status, message, headers = {}) => {
    if (res.headersSent) { res.destroy(); return }
    res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store', ...headers })
    res.end(JSON.stringify({ error: message }))
  }
  const srv = http.createServer({ maxHeaderSize: 16384 }, async (req, res) => {
    res.setHeader('Cache-Control', 'no-store')
    res.setHeader('X-Content-Type-Options', 'nosniff')
    const hostHeaders = headerValues(req, 'host')
    if (hostHeaders.length !== 1 || !hosts.has(hostHeaders[0])) return reject(res, 403, 'Host is not allowed')
    const originHeaders = headerValues(req, 'origin')
    const origin = originHeaders[0]
    if (originHeaders.length > 1 || (origin !== undefined && !origins.has(origin))) return reject(res, 403, 'Origin is not allowed')
    if (origin !== undefined) {
      res.setHeader('Access-Control-Allow-Origin', origin)
      res.setHeader('Vary', 'Origin')
      res.setHeader('Access-Control-Expose-Headers', 'MCP-Protocol-Version')
    }
    if (req.url !== '/mcp') {
      if (req.url?.startsWith('/mcp/')) return reject(res, 410, 'Legacy custom HTTP/SSE endpoints were removed; use Streamable HTTP POST /mcp')
      return reject(res, 404, 'Not found')
    }
    if (req.method === 'OPTIONS') {
      res.writeHead(204, {
        'Access-Control-Allow-Methods': 'POST, OPTIONS',
        'Access-Control-Allow-Headers': 'Authorization, Content-Type, Accept, MCP-Protocol-Version',
      })
      res.end()
      return
    }
    if (req.method !== 'POST') return reject(res, 405, 'Only POST is supported by this stateless transport', { Allow: 'POST, OPTIONS' })
    const authorizations = headerValues(req, 'authorization')
    const authorization = authorizations[0]
    if (authorizations.length !== 1 || !/^Bearer [A-Za-z0-9._~+\/-]+=*$/i.test(authorization)) {
      return reject(res, 401, 'A client AI Bearer credential is required', { 'WWW-Authenticate': 'Bearer' })
    }
    if (!/^application\/json(?:\s*;|$)/i.test(req.headers['content-type'] || '')) return reject(res, 415, 'Content-Type must be application/json')
    if (req.headers['content-encoding'] && req.headers['content-encoding'] !== 'identity') return reject(res, 415, 'Compressed request bodies are not supported')
    if (headerValues(req, 'mcp-session-id').length) return reject(res, 400, 'This endpoint is stateless; omit MCP-Session-Id')
    if (Date.now() - windowStart >= 60000) { windowStart = Date.now(); requests = 0 }
    if (++requests > requestsPerMinute || active >= maxConcurrent) return reject(res, 429, 'Request limit exceeded', { 'Retry-After': '60' })
    if (Number(req.headers['content-length']) > maxBodyBytes) return reject(res, 413, 'Request body is too large')
    active++
    let server
    let transport
    let timer
    let released = false
    const release = () => {
      if (released) return
      released = true
      clearTimeout(timer)
      active--
      if (server) void server.close().catch(() => {})
    }
    res.once('close', release)
    res.once('finish', release)
    timer = setTimeout(() => {
      reject(res, 408, 'Request timed out')
    }, timeoutMs + 5000)
    timer.unref()
    try {
      const chunks = []
      let bytes = 0
      for await (const chunk of req) {
        bytes += chunk.length
        if (bytes > maxBodyBytes) { reject(res, 413, 'Request body is too large'); return }
        chunks.push(chunk)
      }
      if (released) return
      let body
      try { body = JSON.parse(Buffer.concat(chunks).toString('utf8')) }
      catch { reject(res, 400, 'Invalid JSON'); return }
      // Batch calls could bypass the request/concurrency limit.
      if (!body || Array.isArray(body) || typeof body !== 'object') return reject(res, 400, 'A single JSON-RPC message is required')
      const client = new NoteClient({ host, token: authorization.slice(7), timeoutMs })
      server = createMcpServer(client)
      transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined, enableJsonResponse: true })
      await server.connect(transport)
      await transport.handleRequest(req, res, body)
    } catch {
      if (!released) reject(res, 500, 'MCP request failed')
    }
  })
  srv.requestTimeout = timeoutMs + 5000
  srv.headersTimeout = Math.min(timeoutMs + 5000, 10000)
  srv.keepAliveTimeout = 5000
  srv.maxRequestsPerSocket = 100
  await new Promise((resolve, rejectListen) => {
    srv.once('error', rejectListen)
    srv.listen(port, bindHost, () => { srv.off('error', rejectListen); resolve() })
  })
  if (!allowedHosts) {
    const actualPort = srv.address().port
    hosts = new Set([`localhost:${actualPort}`, `127.0.0.1:${actualPort}`, `[::1]:${actualPort}`])
  }
  return srv
}

export async function startFromEnv(env = process.env) {
  if (['false', '0'].includes(String(env.MCP_ENABLED).trim().toLowerCase())) return undefined
  if (env.NOTE_HTTP_PORT || env.NOTE_SESSION) throw new Error('Legacy NOTE_HTTP_PORT and NOTE_SESSION are unsupported; migrate to MCP_TRANSPORT and AI credentials')
  const mode = env.MCP_TRANSPORT || 'stdio'
  const options = { host: env.NOTE_HOST || 'http://localhost:1314', timeoutMs: integer(env.NOTE_TIMEOUT_MS, 10000, 100, 120000) }
  if (mode === 'stdio') return startStdio({ ...options, token: env.NOTE_TOKEN || '' })
  if (mode !== 'http') throw new Error('MCP_TRANSPORT must be stdio or http')
  // Deliberately never read NOTE_TOKEN for HTTP, including login requests.
  return startHttp({
    ...options,
    bindHost: env.MCP_HTTP_HOST || '127.0.0.1',
    port: integer(env.MCP_HTTP_PORT, 1315, 1, 65535),
    allowedHosts: list(env.MCP_ALLOWED_HOSTS, undefined),
    allowedOrigins: list(env.MCP_ALLOWED_ORIGINS, []),
    maxBodyBytes: integer(env.MCP_MAX_BODY_BYTES, 1048576, 1024, 10485760),
    maxConcurrent: integer(env.MCP_MAX_CONCURRENT, 32, 1, 1024),
    requestsPerMinute: integer(env.MCP_REQUESTS_PER_MINUTE, 120, 1, 100000),
  })
}
