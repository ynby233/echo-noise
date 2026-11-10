import assert from 'node:assert/strict'
import test from 'node:test'
import http from 'node:http'
import { startFromEnv } from '../src/transports.js'
import {
  ALICE, BOB, PASSWORD, ROTATED, COOKIE, TEST_OPTIONS, TOOL_NAMES,
  mockBackend, httpServer, httpClient, rawHttp, rpc, ok, dataOf,
  assertToolError, assertNoSecrets, closeHttp, captureOutput, deferred,
} from './helpers.js'

const call = (client, name, args = {}) => client.callTool({ name, arguments: args })

// Flat header pairs preserve duplicates and permit deliberately missing headers.
function wireHttp(url, { agent, headers = [], omit = [], body = rpc('identity') } = {}) {
  const bytes = JSON.stringify(body)
  const defaults = [
    ['Host', new URL(url).host], ['Accept', 'application/json, text/event-stream'],
    ['Content-Type', 'application/json'], ['MCP-Protocol-Version', '2025-03-26'],
    ['Authorization', `Bearer ${ALICE}`], ['Content-Length', String(Buffer.byteLength(bytes))],
  ]
  const replaced = new Set([...omit, ...headers.map(([name]) => name)].map(name => name.toLowerCase()))
  return new Promise((resolve, reject) => {
    const req = http.request(url, {
      method: 'POST', agent,
      headers: [...defaults.filter(([name]) => !replaced.has(name.toLowerCase())), ...headers].flat(),
    }, res => {
      const chunks = []
      res.on('data', chunk => chunks.push(chunk))
      res.on('error', reject)
      res.on('end', () => resolve({ status: res.statusCode, headers: res.headers, text: Buffer.concat(chunks).toString('utf8') }))
    })
    req.setTimeout(5000, () => req.destroy(new Error('test HTTP request deadline exceeded')))
    req.on('error', reject)
    req.end(bytes)
  })
}

test('official HTTP SDK: stateless JSON, two simultaneous identities, per-request Bearer', TEST_OPTIONS, async t => {
  const output = captureOutput(t)
  const backend = await mockBackend(t, request => {
    if (request.path === '/api/messages/page') return ok({ owner: request.headers.authorization === `Bearer ${ALICE}` ? 'alice' : 'bob' })
    if (request.path === '/api/ai/login') return {
      ...ok({ renewed: true, token: ROTATED, password: PASSWORD }), headers: { 'Set-Cookie': `session=${COOKIE}` },
    }
  })
  const { url } = await httpServer(t, backend)
  const [{ client: alice, transport: aliceTransport }, { client: bob, transport: bobTransport }] = await Promise.all([
    httpClient(t, url, ALICE, { Cookie: `session=${COOKIE}` }), httpClient(t, url, BOB),
  ])
  assert.deepEqual((await alice.listTools()).tools.map(tool => tool.name).sort(), [...TOOL_NAMES].sort())
  const [alicePage, bobPage] = await Promise.all([call(alice, 'page'), call(bob, 'page')])
  assert.equal(dataOf(alicePage).owner, 'alice')
  assert.equal(dataOf(bobPage).owner, 'bob')
  const [renewed, bobDuringRenewal] = await Promise.all([
    call(alice, 'login', { username: 'alice', password: PASSWORD }), call(bob, 'identity'),
  ])
  assert.equal(dataOf(renewed).identity.username, 'alice')
  assert.equal(dataOf(bobDuringRenewal).username, 'bob')
  const [aliceAfter, bobAfter] = await Promise.all([call(alice, 'identity'), call(bob, 'identity')])
  assert.equal(dataOf(aliceAfter).username, 'alice')
  assert.equal(dataOf(bobAfter).username, 'bob')
  assert.equal(aliceTransport.sessionId, undefined)
  assert.equal(bobTransport.sessionId, undefined)
  assert.ok(backend.received.every(request => [`Bearer ${ALICE}`, `Bearer ${BOB}`].includes(request.headers.authorization)))
  assertNoSecrets([alicePage, bobPage, renewed, bobDuringRenewal, aliceAfter, bobAfter, output])

  // A fresh raw request has no session or initialization state to inherit.
  const response = await rawHttp(url, { token: BOB })
  assert.equal(response.status, 200)
  assert.match(response.headers['content-type'], /^application\/json/)
  assert.equal(response.headers['mcp-session-id'], undefined)
  assert.equal(dataOf(JSON.parse(response.text).result).username, 'bob')
  assertNoSecrets(response.text)
})

test('HTTP env mode ignores NOTE_TOKEN and defaults to 127.0.0.1:1315', TEST_OPTIONS, async t => {
  // Observe the configured port, but bind an ephemeral port to avoid collisions.
  const listen = http.Server.prototype.listen
  const ports = []
  t.mock.method(http.Server.prototype, 'listen', function (port, ...args) {
    ports.push(port)
    return listen.call(this, 0, ...args)
  })
  const backend = await mockBackend(t)
  const server = await startFromEnv({
    MCP_TRANSPORT: 'http', NOTE_HOST: backend.url,
    get NOTE_TOKEN() { throw new Error('HTTP must never read NOTE_TOKEN') },
  })
  t.after(() => closeHttp(server))
  assert.equal(server.address().address, '127.0.0.1')
  assert.equal(ports.at(-1), 1315)
  const url = `http://127.0.0.1:${server.address().port}/mcp`
  for (const headers of [{}, { Cookie: `token=${ALICE}; session=${COOKIE}` }, { Authorization: `Basic ${ALICE}` }, { Authorization: 'Bearer' }]) {
    const response = await rawHttp(url, { token: null, headers })
    assert.equal(response.status, 401)
    assert.match(response.headers['www-authenticate'], /Bearer/)
    assertNoSecrets(response.text)
  }
  assert.deepEqual(backend.received, [])
  const response = await rawHttp(url, { token: BOB })
  assert.equal(dataOf(JSON.parse(response.text).result).username, 'bob')
  assert.equal(backend.received[0].headers.authorization, `Bearer ${BOB}`)
})

test('every HTTP request needs Bearer even after an authenticated request', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url } = await httpServer(t, backend)
  assert.equal((await rawHttp(url)).status, 200)
  const before = backend.received.length
  assert.equal((await rawHttp(url, { token: null, headers: { Cookie: `session=${COOKIE}` } })).status, 401)
  assert.equal(backend.received.length, before)
})

test('GET/DELETE are 405 and removed custom endpoints are 410', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url } = await httpServer(t, backend)
  for (const method of ['GET', 'DELETE']) {
    const response = await rawHttp(url, { method })
    assert.equal(response.status, 405)
    assert.match(response.headers.allow, /POST/)
  }
  for (const path of ['/mcp/tools', '/mcp/call', '/mcp/sse', '/mcp/events', '/mcp/session', '/mcp/anything']) {
    for (const method of ['GET', 'POST', 'DELETE']) assert.equal((await rawHttp(new URL(path, url), { method })).status, 410)
  }
  assert.deepEqual(backend.received, [])
})

test('default Host list accepts only loopback authorities with the actual port', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url, server } = await httpServer(t, backend)
  const port = server.address().port
  for (const Host of [`localhost:${port}`, `127.0.0.1:${port}`, `[::1]:${port}`]) {
    assert.equal((await rawHttp(url, { headers: { Host } })).status, 200)
  }
  const before = backend.received.length
  for (const Host of ['evil.example', `evil.example:${port}`, 'localhost', `localhost:${port + 1}`, `localhost.evil:${port}`, `127.0.0.1:${port}.evil`]) {
    assert.equal((await rawHttp(url, { headers: { Host } })).status, 403)
  }
  assert.equal(backend.received.length, before)
})

test('explicit allowedHosts is an exact replacement list', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url } = await httpServer(t, backend, { allowedHosts: ['mcp.test:8080'] })
  assert.equal((await rawHttp(url, { headers: { Host: 'mcp.test:8080' } })).status, 200)
  for (const Host of ['mcp.test', 'mcp.test:8081', 'mcp.test.evil:8080', new URL(url).host]) {
    assert.equal((await rawHttp(url, { headers: { Host } })).status, 403)
  }
  assert.equal(backend.received.length, 1)
})

test('Origin is denied by default, including same-origin and opaque origins', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url } = await httpServer(t, backend)
  for (const Origin of ['https://evil.example', 'null', new URL(url).origin]) {
    const response = await rawHttp(url, { headers: { Origin } })
    assert.equal(response.status, 403)
    assert.equal(response.headers['access-control-allow-origin'], undefined)
  }
  assert.deepEqual(backend.received, [])
})

test('explicit allowedOrigins compares whole strings and reflects only listed origins', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const allowed = 'https://trusted.example:8443'
  const { url } = await httpServer(t, backend, { allowedOrigins: [allowed] })
  const accepted = await rawHttp(url, { headers: { Origin: allowed } })
  assert.equal(accepted.status, 200)
  assert.equal(accepted.headers['access-control-allow-origin'], allowed)
  for (const Origin of ['https://trusted.example', 'http://trusted.example:8443', `${allowed}/`, `${allowed}.evil`, 'https://sub.trusted.example:8443', 'null']) {
    assert.equal((await rawHttp(url, { headers: { Origin } })).status, 403)
  }
  assert.equal(backend.received.length, 1)
})

test('HTTP oversized declared and chunked bodies are 413 before upstream access', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url } = await httpServer(t, backend, { maxBodyBytes: 1024 })
  const body = rpc('publish', { content: 'x'.repeat(4096) })
  for (const chunked of [false, true]) assert.equal((await rawHttp(url, { body, chunked })).status, 413)
  assert.deepEqual(backend.received, [])
})

test('request rate limit returns 429 with Retry-After', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url } = await httpServer(t, backend, { requestsPerMinute: 2 })
  assert.equal((await rawHttp(url)).status, 200)
  assert.equal((await rawHttp(url)).status, 200)
  const limited = await rawHttp(url)
  assert.equal(limited.status, 429)
  assert.ok(Number(limited.headers['retry-after']) > 0)
  assert.equal(backend.received.length, 2)
})

test('concurrency limit returns 429 and releases capacity when the request completes', TEST_OPTIONS, async t => {
  const entered = deferred()
  const release = deferred()
  t.after(() => release.resolve())
  let block = true
  const backend = await mockBackend(t, async request => {
    if (request.path === '/api/auth/identity' && block) {
      block = false
      entered.resolve()
      await release.promise
    }
  })
  const { url } = await httpServer(t, backend, { maxConcurrent: 1, timeoutMs: 3000 })
  const first = rawHttp(url)
  await entered.promise
  try {
    const limited = await rawHttp(url, { token: BOB })
    assert.equal(limited.status, 429)
    assert.equal(backend.received.length, 1)
  } finally { release.resolve() }
  assert.equal((await first).status, 200)
  const next = await rawHttp(url, { token: BOB })
  assert.equal(next.status, 200)
  assert.equal(dataOf(JSON.parse(next.text).result).username, 'bob')
})

test('invalid JSON, batches, sessions and unsupported media cannot reach the backend', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url } = await httpServer(t, backend)
  for (const [options, status] of [
    [{ body: '{broken' }, 400], [{ body: [rpc('identity'), rpc('identity', {}, 2)] }, 400],
    [{ headers: { 'MCP-Session-Id': 'fake-session' } }, 400],
    [{ headers: { 'Content-Type': 'text/plain' } }, 415],
    [{ headers: { 'Content-Encoding': 'gzip' } }, 415],
  ]) assert.equal((await rawHttp(url, options)).status, status)
  assert.deepEqual(backend.received, [])
})

test('official HTTP SDK preserves permission failure as tool isError', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/token/messages/42' ? { status: 403, body: { code: 0, message: ALICE } } : undefined)
  const { url } = await httpServer(t, backend)
  const { client } = await httpClient(t, url)
  assertToolError(await call(client, 'delete', { id: 42 }))
  assert.deepEqual(backend.received.map(request => request.path), ['/api/auth/identity', '/api/token/messages/42'])
})

test('one keep-alive connection never retains identity, login state or credentials', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/ai/login'
    ? { ...ok({ renewed: true, token: ROTATED }), headers: { 'Set-Cookie': `session=${COOKIE}` } }
    : undefined)
  const { url, server } = await httpServer(t, backend)
  const agent = new http.Agent({ keepAlive: true, maxSockets: 1 })
  t.after(() => agent.destroy())
  const sockets = new Set()
  server.prependListener('request', req => sockets.add(req.socket))
  const login = await wireHttp(url, { agent, body: rpc('login', { username: 'alice', password: PASSWORD }) })
  assert.equal(login.status, 200)
  assert.equal(dataOf(JSON.parse(login.text).result).identity.username, 'alice')
  const before = backend.received.length
  const missing = await wireHttp(url, { agent, omit: ['Authorization'], headers: [['Cookie', `session=${COOKIE}`]] })
  assert.equal(missing.status, 401)
  assert.equal(backend.received.length, before)
  for (const [token, username] of [[BOB, 'bob'], [ALICE, 'alice'], [BOB, 'bob']]) {
    const response = await wireHttp(url, { agent, headers: [['Authorization', `Bearer ${token}`]] })
    assert.equal(response.status, 200)
    assert.equal(dataOf(JSON.parse(response.text).result).username, username)
    assert.equal(response.headers['mcp-session-id'], undefined)
    assert.equal(response.headers['set-cookie'], undefined)
    assert.equal(backend.received.at(-1).headers.authorization, `Bearer ${token}`)
  }
  assert.equal(sockets.size, 1, 'all requests must actually reuse one TCP connection')
  assertNoSecrets(login.text)
})

test('initialization negotiates versions and notifications return an empty 202 without sessions', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url } = await httpServer(t, backend)
  for (const [requested, negotiated] of [['2025-03-26', '2025-03-26'], ['2025-06-18', '2025-06-18'], ['2099-01-01', '2025-06-18']]) {
    const response = await wireHttp(url, {
      omit: ['MCP-Protocol-Version'],
      body: { jsonrpc: '2.0', id: 'init', method: 'initialize', params: {
        protocolVersion: requested, capabilities: {}, clientInfo: { name: 'raw-contract', version: '1' },
      } },
    })
    assert.equal(response.status, 200)
    assert.match(response.headers['content-type'], /^application\/json/)
    assert.equal(response.headers['mcp-session-id'], undefined)
    const message = JSON.parse(response.text)
    assert.equal(message.jsonrpc, '2.0')
    assert.equal(message.id, 'init')
    assert.equal(message.result.protocolVersion, negotiated)
    assert.ok(message.result.capabilities.tools)
  }
  for (const method of ['notifications/initialized', 'notifications/cancelled']) {
    const body = { jsonrpc: '2.0', method, ...(method.endsWith('cancelled') ? { params: { requestId: 123 } } : {}) }
    const response = await wireHttp(url, { body })
    assert.equal(response.status, 202)
    assert.equal(response.text, '')
    assert.equal(response.headers['mcp-session-id'], undefined)
    assert.equal((await wireHttp(url, { body, omit: ['Authorization'] })).status, 401)
  }
  assert.deepEqual(backend.received, [])
  const response = await wireHttp(url, { omit: ['Authorization'], body: {
    jsonrpc: '2.0', id: 1, method: 'initialize', params: {
      protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'anonymous', version: '1' },
    },
  } })
  assert.equal(response.status, 401)
})

test('protocol versions, Accept and JSON-RPC envelopes are enforced without shared initialization', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url } = await httpServer(t, backend)
  for (const options of [
    { headers: [['MCP-Protocol-Version', '2099-01-01']] },
    { headers: [['MCP-Protocol-Version', '']] },
    { headers: [['MCP-Protocol-Version', '2025-03-26'], ['mcp-protocol-version', '2025-06-18']] },
    { headers: [['MCP-Session-Id', '']] },
    { body: { ...rpc('identity'), jsonrpc: '1.0' } },
    { body: { jsonrpc: '2.0', id: 1 } },
  ]) assert.equal((await wireHttp(url, options)).status, 400)
  for (const options of [{ omit: ['Accept'] }, { headers: [['Accept', 'application/json']] }, { headers: [['Accept', 'text/event-stream']] }]) {
    assert.equal((await wireHttp(url, options)).status, 406)
  }
  assert.deepEqual(backend.received, [])
  for (const options of [{ omit: ['MCP-Protocol-Version'] }, { headers: [['MCP-Protocol-Version', '2025-06-18']] }]) {
    const response = await wireHttp(url, { ...options, body: { jsonrpc: '2.0', id: 9, method: 'tools/list' } })
    assert.equal(response.status, 200)
    const message = JSON.parse(response.text)
    assert.equal(message.id, 9)
    assert.deepEqual(message.result.tools.map(tool => tool.name).sort(), [...TOOL_NAMES].sort())
  }
  const unknown = await wireHttp(url, { body: { jsonrpc: '2.0', id: 17, method: 'unknown/method' } })
  assert.equal(unknown.status, 200)
  assert.equal(JSON.parse(unknown.text).error.code, -32601)
  assert.deepEqual(backend.received, [])
})

test('only one real Authorization Bearer header supplies HTTP credentials', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const { url } = await httpServer(t, backend)
  for (const authorization of ['', 'Bearer', 'Basic abc', `Bearer ${ALICE},${BOB}`, `Bearer ${ALICE} ${BOB}`, `Bearer "${ALICE}"`, `Bearer ${ALICE}:bad`]) {
    assert.equal((await wireHttp(url, { headers: [['Authorization', authorization]] })).status, 401)
  }
  for (const values of [[ALICE, BOB], [ALICE, ALICE], [BOB, ALICE]]) {
    const response = await wireHttp(url, { headers: [['Authorization', `Bearer ${values[0]}`], ['authorization', `Bearer ${values[1]}`]] })
    assert.equal(response.status, 401)
    assert.match(response.headers['www-authenticate'], /Bearer/)
    assertNoSecrets(response.text)
  }
  for (const headers of [
    [['Cookie', `token=${ALICE}; session=${COOKIE}`]], [['X-Authorization', `Bearer ${ALICE}`]],
    [['X-Forwarded-Authorization', `Bearer ${ALICE}`]], [['X-Api-Key', ALICE]],
  ]) assert.equal((await wireHttp(url, { omit: ['Authorization'], headers })).status, 401)
  const body = { ...rpc('identity'), token: ALICE, authorization: `Bearer ${ALICE}`, headers: { Authorization: `Bearer ${ALICE}` } }
  assert.equal((await wireHttp(url, { omit: ['Authorization'], body })).status, 401)
  assert.equal((await wireHttp(`${url}?access_token=${ALICE}`, { omit: ['Authorization'] })).status, 404)
  assert.deepEqual(backend.received, [])
  const accepted = await wireHttp(url, { headers: [['Authorization', `bEaReR ${BOB}`]], body: rpc('identity') })
  assert.equal(accepted.status, 200)
  assert.equal(dataOf(JSON.parse(accepted.text).result).username, 'bob')
  assert.equal(backend.received.at(-1).headers.authorization, `Bearer ${BOB}`)
})

test('Host and Origin trust only single real headers, never forwarding fields or JSON', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const allowed = 'https://trusted.example'
  const { url } = await httpServer(t, backend, { allowedOrigins: [allowed, `${allowed}, ${allowed}`] })
  const authority = new URL(url).host
  const body = { ...rpc('identity'), headers: { Host: authority, Origin: allowed }, host: authority, origin: allowed }
  for (const headers of [
    [['Host', 'evil.example'], ['X-Forwarded-Host', authority], ['Forwarded', `host=${authority}`]],
    [['Host', authority], ['hOsT', 'evil.example']], [['Host', 'evil.example'], ['Host', authority]],
    [['Host', authority], ['Host', authority]],
    [['Origin', 'https://evil.example'], ['X-Forwarded-Origin', allowed]],
    [['Origin', allowed], ['oRiGiN', allowed]], [['Origin', allowed], ['Origin', 'https://evil.example']],
  ]) {
    const response = await wireHttp(url, { headers, body })
    assert.equal(response.status, 403)
    assert.equal(response.headers['access-control-allow-origin'], undefined)
  }
  const missingHost = await wireHttp(url, { omit: ['Host'], headers: [['X-Forwarded-Host', authority]], body })
  // Node versions may reject HTTP/1.1 without Host before the request handler.
  assert.ok([400, 403].includes(missingHost.status))
  assert.deepEqual(backend.received, [])
  const accepted = await wireHttp(url, { body: rpc('identity'), headers: [['X-Forwarded-Host', 'evil.example'], ['X-Forwarded-Origin', 'https://evil.example']] })
  assert.equal(accepted.status, 200)
  assert.equal(accepted.headers['access-control-allow-origin'], undefined)
  assert.equal(backend.received.length, 1)
})

test('MCP_ENABLED false and zero stop before transport or credential configuration is read', TEST_OPTIONS, async () => {
  for (const MCP_ENABLED of ['false', '0', 'FALSE', ' false ', false, 0]) {
    const server = await startFromEnv({
      MCP_ENABLED,
      get MCP_TRANSPORT() { throw new Error('disabled transport must not start') },
      get NOTE_TOKEN() { throw new Error('disabled transport must not read credentials') },
    })
    assert.equal(server, undefined)
  }
})
