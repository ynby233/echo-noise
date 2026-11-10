import assert from 'node:assert/strict'
import http from 'node:http'
import { fileURLToPath } from 'node:url'
import { Client } from '@modelcontextprotocol/sdk/client/index.js'
import { InMemoryTransport } from '@modelcontextprotocol/sdk/inMemory.js'
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js'
import { NoteClient } from '../src/client.js'
import { createMcpServer } from '../src/tools.js'
import { startHttp } from '../src/transports.js'

// All credentials are inert fixtures. Never load the developer's NOTE_TOKEN.
export const ALICE = 'fixture-ai-alice-32aeb1'
export const BOB = 'fixture-ai-bob-76bcf2'
export const ORDINARY = 'fixture-ordinary-token-4e63'
export const PASSWORD = 'fixture-password-98b7'
export const ROTATED = 'fixture-unwanted-new-token-19c6'
export const COOKIE = 'fixture-session-cookie-57d2'
export const ENTRY = fileURLToPath(new URL('../server.js', import.meta.url))
export const MCP_DIR = fileURLToPath(new URL('../', import.meta.url))
export const TEST_OPTIONS = { timeout: 15000, concurrency: false }
export const TOOL_NAMES = ['search', 'page', 'message', 'publish', 'update', 'delete', 'pin', 'settings', 'status', 'calendar', 'config', 'login', 'rss', 'identity']

export const ok = data => ({ status: 200, body: { code: 1, data } })
export const identity = (username = 'alice', authVia = 'ai_token') => ({
  user_id: username === 'alice' ? 101 : 202,
  username,
  auth_via: authVia,
  credential_id: username === 'alice' ? 301 : 302,
})

export async function closeHttp(server) {
  if (!server.listening) return
  const closed = new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
  server.closeAllConnections()
  await closed
}

export async function mockBackend(t, handler = () => undefined) {
  const received = []
  const errors = []
  const server = http.createServer(async (req, res) => {
    try {
      const chunks = []
      for await (const chunk of req) chunks.push(chunk)
      const raw = Buffer.concat(chunks).toString('utf8')
      const url = new URL(req.url, 'http://mock.invalid')
      const request = {
        method: req.method, path: url.pathname, query: Object.fromEntries(url.searchParams),
        headers: { ...req.headers }, raw, body: raw ? JSON.parse(raw) : undefined,
      }
      received.push(request)
      let reply = await handler(request, received)
      if (reply === undefined) {
        if (request.path === '/api/auth/identity') {
          const bearer = request.headers.authorization
          reply = bearer === `Bearer ${ALICE}` ? ok(identity())
            : bearer === `Bearer ${BOB}` ? ok(identity('bob'))
              : bearer === `Bearer ${ORDINARY}` ? ok(identity('alice', 'token'))
                : { status: 401, body: { code: 0, message: 'invalid credential' } }
        } else {
          // An unexpected route must fail: no catch-all success can hide a fallback.
          reply = { status: 404, body: { code: 0, message: 'unhandled mock route' } }
        }
      }
      if (res.destroyed) return
      res.writeHead(reply.status ?? 200, { 'Content-Type': 'application/json', ...reply.headers })
      res.end(reply.raw ?? JSON.stringify(reply.body))
    } catch (error) {
      errors.push(error)
      if (!res.destroyed) { res.writeHead(500); res.end('{"code":0}') }
    }
  })
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  t.after(async () => {
    await closeHttp(server)
    assert.equal(errors.length, 0, 'mock handler must not throw')
    assertNoCookies(received)
  })
  return { server, received, url: `http://127.0.0.1:${server.address().port}` }
}

export function assertNoCookies(received) {
  for (const request of received) {
    assert.equal(request.headers.cookie, undefined, 'upstream must never receive Cookie')
  }
}

export function assertNoSecrets(value, secrets = [ALICE, BOB, ORDINARY, PASSWORD, ROTATED, COOKIE]) {
  const text = typeof value === 'string' ? value : JSON.stringify(value)
  for (const secret of secrets) assert.equal(text.includes(secret), false, 'credential leaked in output')
}

export function dataOf(result) {
  assert.notEqual(result.isError, true, 'expected successful tool response')
  assert.ok(Array.isArray(result.content))
  assert.equal(result.content[0]?.type, 'text')
  return JSON.parse(result.content[0].text)
}

export function assertToolError(result) {
  assert.equal(result.isError, true, 'upstream/argument failure must be an MCP tool error')
  assert.ok(result.content.some(item => item.type === 'text' && item.text.length > 0))
  assertNoSecrets(result)
}

export const businessRequests = backend => backend.received.filter(request => request.path !== '/api/auth/identity')

export async function toolClient(t, backend, { token = ALICE, timeoutMs = 1000 } = {}) {
  const server = createMcpServer(new NoteClient({ host: backend.url, token, timeoutMs }))
  const client = new Client({ name: 'contract-tests', version: '1.0.0' })
  const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair()
  t.after(async () => { await client.close(); await server.close() })
  await server.connect(serverTransport)
  await client.connect(clientTransport)
  return client
}

export async function httpServer(t, backend, options = {}) {
  const server = await startHttp({ host: backend.url, bindHost: '127.0.0.1', port: 0, ...options })
  assert.ok(server instanceof http.Server)
  t.after(() => closeHttp(server))
  return { server, url: `http://127.0.0.1:${server.address().port}/mcp` }
}

export async function httpClient(t, url, token = ALICE, headers = {}) {
  const transport = new StreamableHTTPClientTransport(new URL(url), {
    requestInit: { headers: { Authorization: `Bearer ${token}`, ...headers } },
  })
  const client = new Client({ name: 'http-contract-tests', version: '1.0.0' })
  t.after(() => client.close())
  await client.connect(transport)
  return { client, transport }
}

export function rpc(name, args = {}, id = 1) {
  return { jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args } }
}

// node:http permits deliberate Host spoofing and chunked bodies, unlike fetch.
export function rawHttp(url, { method = 'POST', token = ALICE, headers = {}, body = rpc('identity'), chunked = false } = {}) {
  const bytes = typeof body === 'string' ? body : JSON.stringify(body)
  return new Promise((resolve, reject) => {
    const req = http.request(url, {
      method,
      headers: {
        Accept: 'application/json, text/event-stream', 'Content-Type': 'application/json',
        'MCP-Protocol-Version': '2025-03-26',
        ...(token === null ? {} : { Authorization: `Bearer ${token}` }),
        ...(chunked ? { 'Transfer-Encoding': 'chunked' } : { 'Content-Length': Buffer.byteLength(bytes) }),
        ...headers,
      },
    }, res => {
      const chunks = []
      res.on('data', chunk => chunks.push(chunk))
      res.on('error', reject)
      res.on('end', () => resolve({ status: res.statusCode, headers: res.headers, text: Buffer.concat(chunks).toString('utf8') }))
    })
    req.setTimeout(5000, () => req.destroy(new Error('test HTTP request deadline exceeded')))
    req.on('error', reject)
    if (chunked) { req.write(bytes.slice(0, 512)); req.end(bytes.slice(512)) }
    else req.end(bytes)
  })
}

export function deferred() {
  let resolve
  const promise = new Promise(done => { resolve = done })
  return { promise, resolve }
}

export function captureOutput(t) {
  const captured = { stdout: '', stderr: '' }
  for (const name of ['stdout', 'stderr']) {
    const stream = process[name]
    const original = stream.write
    stream.write = function (chunk, ...args) {
      captured[name] += Buffer.isBuffer(chunk) ? chunk.toString('utf8') : String(chunk)
      return original.call(this, chunk, ...args)
    }
    t.after(() => { stream.write = original })
  }
  return captured
}
