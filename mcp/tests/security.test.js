import assert from 'node:assert/strict'
import test from 'node:test'
import { NoteClient } from '../src/client.js'
import {
  ALICE, BOB, ORDINARY, PASSWORD, ROTATED, COOKIE, TEST_OPTIONS,
  mockBackend, toolClient, ok, identity, dataOf, assertToolError,
  assertNoSecrets, businessRequests, captureOutput, deferred,
} from './helpers.js'

const call = (client, name, args = {}) => client.callTool({ name, arguments: args })

test('identity accepts backend-verified ai_token and preserves the public credential ID', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const client = await toolClient(t, backend)
  const result = dataOf(await call(client, 'identity'))
  assert.equal(result.user_id, 101)
  assert.equal(result.username, 'alice')
  assert.equal(result.auth_via, 'ai_token')
  assert.equal(result.credential_id, 301)
  assert.deepEqual(backend.received.map(request => [request.method, request.path]), [['GET', '/api/auth/identity']])
  assert.equal(backend.received[0].headers.authorization, `Bearer ${ALICE}`)
})

for (const [label, token, reply] of [
  ['ordinary token', ORDINARY, undefined],
  ['expired AI credential', ALICE, { status: 401, body: { code: 0 } }],
  ['business failure', ALICE, { status: 200, body: { code: 0, data: identity() } }],
  ['revoked AI credential', ALICE, { status: 403, body: { code: 0 } }],
  ['missing auth_via', ALICE, ok({ user_id: 101, username: 'alice' })],
]) {
  test(`identity rejects ${label} and prevents business requests`, TEST_OPTIONS, async t => {
    const backend = await mockBackend(t, request => request.path === '/api/auth/identity' ? reply : undefined)
    const client = await toolClient(t, backend, { token })
    assertToolError(await call(client, 'identity'))
    for (const [name, args] of [
      ['search', {}], ['search', { query: '#tag' }], ['search', { keyword: 'note' }],
      ['page', {}], ['message', { id: 42 }], ['status', {}], ['calendar', {}],
      ['config', {}], ['rss', {}], ['publish', { content: 'note' }], ['delete', { id: 42 }],
    ]) {
      assertToolError(await call(client, name, args))
    }
    assert.equal(backend.received.length, 12)
    assert.ok(backend.received.every(request => request.headers.authorization === `Bearer ${token}`))
    assert.equal(businessRequests(backend).length, 0)
  })
}

test('expired AI login renews first, never preflights identity or returns credentials', TEST_OPTIONS, async t => {
  const output = captureOutput(t)
  let renewed = false
  const backend = await mockBackend(t, request => {
    if (request.path === '/api/ai/login') {
      renewed = true
      return { ...ok({ renewed: true }), headers: { 'Set-Cookie': `session=${COOKIE}; HttpOnly` } }
    }
    if (request.path === '/api/auth/identity' && !renewed) return { status: 401, body: { code: 0 } }
  })
  const client = await toolClient(t, backend)
  const result = await call(client, 'login', { username: 'alice', password: PASSWORD })
  const data = dataOf(result)
  assert.equal(data.renewed, true)
  assert.equal(data.identity.username, 'alice')
  assert.deepEqual(backend.received.map(request => [request.method, request.path]), [
    ['POST', '/api/ai/login'], ['GET', '/api/auth/identity'],
  ])
  assert.deepEqual(backend.received[0].body, { username: 'alice', password: PASSWORD })
  for (const request of backend.received) assert.equal(request.headers.authorization, `Bearer ${ALICE}`)
  assertNoSecrets(result)
  assertNoSecrets(output)
})

test('login ignores injected token/cookie data and cannot replace another client identity', TEST_OPTIONS, async t => {
  const output = captureOutput(t)
  const backend = await mockBackend(t, request => request.path === '/api/ai/login' ? {
    ...ok({ token: ROTATED, password: PASSWORD, session: COOKIE, message: `${ALICE} ${PASSWORD}` }),
    headers: { 'Set-Cookie': `session=${COOKIE}` },
  } : undefined)
  const alice = await toolClient(t, backend)
  const bob = await toolClient(t, backend, { token: BOB })
  const [renewal, bobBefore] = await Promise.all([
    call(alice, 'login', { username: 'alice', password: PASSWORD }), call(bob, 'identity'),
  ])
  assert.equal(dataOf(renewal).identity.username, 'alice')
  assert.equal(dataOf(bobBefore).username, 'bob')
  assert.equal(dataOf(await call(alice, 'identity')).username, 'alice')
  assert.equal(dataOf(await call(bob, 'identity')).username, 'bob')
  assert.ok(backend.received.every(request => [`Bearer ${ALICE}`, `Bearer ${BOB}`].includes(request.headers.authorization)))
  assertNoSecrets([renewal, bobBefore, output])
})

test('login identity mismatch is an error and preserves the original credential', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/ai/login' ? ok({ renewed: true }) : undefined)
  const client = await toolClient(t, backend)
  assertToolError(await call(client, 'login', { username: 'bob', password: PASSWORD }))
  assert.equal(dataOf(await call(client, 'identity')).username, 'alice')
  assert.ok(backend.received.every(request => request.headers.authorization === `Bearer ${ALICE}`))
})

for (const [label, reply] of [
  ['HTTP 401', { status: 401, body: { code: 1, data: { token: ROTATED, password: PASSWORD }, message: `${ALICE} ${PASSWORD}` } }],
  ['business code 0', { status: 200, body: { code: 0, message: `${ALICE} ${PASSWORD} ${ROTATED}` } }],
]) {
  test(`login ${label} is sanitized and does not proceed to identity`, TEST_OPTIONS, async t => {
    const output = captureOutput(t)
    const backend = await mockBackend(t, request => request.path === '/api/ai/login' ? reply : undefined)
    const client = await toolClient(t, backend)
    assertToolError(await call(client, 'login', { username: 'alice', password: PASSWORD }))
    assert.deepEqual(backend.received.map(request => request.path), ['/api/ai/login'])
    assertNoSecrets(output)
  })
}

test('successful nested backend data is sanitized before returning tools results', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/frontend/config' ? ok({
    title: 'visible', token: ROTATED, password: PASSWORD, cookie: COOKIE,
    nested: [{ authorization: `Bearer ${BOB}`, smtpPassword: PASSWORD, text: `current ${ALICE}` }],
  }) : undefined)
  const client = await toolClient(t, backend)
  const result = await call(client, 'config')
  const data = dataOf(result)
  assert.equal(data.title, 'visible')
  assert.equal(data.token, undefined)
  assert.equal(data.password, undefined)
  assert.equal(data.nested[0].authorization, undefined)
  assertNoSecrets(result)
})

for (const response of [
  { status: 500, body: { code: 1, data: { success: true } } },
  { status: 200, body: { code: '1', data: { success: true } } },
  { status: 200, body: { data: { success: true } } },
  { status: 200, raw: '<html>not json</html>' },
]) {
  test(`invalid backend envelope (${response.status}, ${JSON.stringify(response.body?.code)}) cannot become success`, TEST_OPTIONS, async t => {
    const backend = await mockBackend(t, request => request.path === '/api/status' ? response : undefined)
    const client = await toolClient(t, backend)
    assertToolError(await call(client, 'status'))
  })
}

test('NoteClient timeout aborts a pending backend response and tool reports isError', TEST_OPTIONS, async t => {
  const entered = deferred()
  const release = deferred()
  t.after(() => release.resolve())
  const backend = await mockBackend(t, async request => {
    if (request.path === '/api/status') { entered.resolve(); await release.promise; return ok({ ready: true }) }
  })
  const client = await toolClient(t, backend, { timeoutMs: 100 })
  const pending = call(client, 'status')
  await entered.promise
  try {
    const result = await pending
    assertToolError(result)
    assert.match(result.content[0].text, /超时|timeout/i)
  } finally { release.resolve() }
  assert.deepEqual(backend.received.map(request => request.path), ['/api/auth/identity', '/api/status'])
})

for (const status of [301, 302, 303, 307, 308]) {
  test(`HTTP ${status} redirect is not followed and cannot leak Bearer to another host`, TEST_OPTIONS, async t => {
    const sink = await mockBackend(t, () => ok({ caught: true }))
    const backend = await mockBackend(t, request => request.path === '/api/status' ? {
      status, headers: { Location: `${sink.url}/credential-sink` }, body: { code: 1 },
    } : undefined)
    const client = await toolClient(t, backend)
    assertToolError(await call(client, 'status'))
    assert.equal(sink.received.length, 0, 'redirect target must receive no request, including one with stripped headers')
    assert.deepEqual(backend.received.map(request => request.path), ['/api/auth/identity', '/api/status'])
  })
}

test('same-origin redirects also cannot reroute authenticated requests', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/status' ? {
    status: 307, headers: { Location: '/credential-sink' }, body: { code: 1 },
  } : undefined)
  const client = await toolClient(t, backend)
  assertToolError(await call(client, 'status'))
  assert.equal(backend.received.some(request => request.path === '/credential-sink'), false)
})

test('NoteClient rejects a credential embedded in the upstream URL', TEST_OPTIONS, () => {
  assert.throws(() => new NoteClient({ host: `http://alice:${PASSWORD}@127.0.0.1`, token: ALICE }))
})

test('missing credentials block every read before any network request', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const client = await toolClient(t, backend, { token: '' })
  for (const [name, args] of [
    ['identity', {}], ['search', {}], ['search', { query: '#tag' }], ['search', { keyword: 'note' }],
    ['page', {}], ['message', { id: 42 }], ['status', {}], ['calendar', {}], ['config', {}], ['rss', {}],
  ]) assertToolError(await call(client, name, args))
  assert.deepEqual(backend.received, [])
})

test('sanitization preserves ordinary content and identity metadata while removing credential fields', TEST_OPTIONS, async t => {
  const content = 'Bearer authentication, tokenization, password policy and credential_id are normal documentation.'
  const safe = {
    content, credential_id: 301, token_count: 42, session_timeout: 3600,
    password_policy: 'strong', secret_description: 'a story', tokenization: 'words',
  }
  const backend = await mockBackend(t, request => request.path === '/api/messages/42' ? ok({
    ...safe, access_token: ROTATED, smtpPassword: PASSWORD, 'Set-Cookie': COOKIE,
    nested: [{ ...safe, api_key: ROTATED, password_hash: PASSWORD, telegram_bot_token: ROTATED }],
    echo: `credential ${ALICE}`,
  }) : undefined)
  const client = await toolClient(t, backend)
  const result = await call(client, 'message', { id: 42 })
  assert.deepEqual(dataOf(result), { ...safe, nested: [safe], echo: 'credential [已隐藏]' })
  assertNoSecrets(result)
})

for (const reply of [
  { status: 204, raw: '' },
  { status: 200, raw: 'not json' },
  { status: 200, body: { code: 0, message: '删除成功' } },
  { status: 404, body: { code: 1 } },
]) {
  test(`delete rejects ambiguous response ${reply.status}/${reply.raw ?? reply.body?.code} without rewriting or retrying`, TEST_OPTIONS, async t => {
    const backend = await mockBackend(t, request => request.path === '/api/token/messages/42' ? reply : undefined)
    const client = await toolClient(t, backend)
    assertToolError(await call(client, 'delete', { id: 42 }))
    assert.deepEqual(backend.received.map(request => [request.method, request.path]), [
      ['GET', '/api/auth/identity'], ['DELETE', '/api/token/messages/42'],
    ])
    assert.equal(businessRequests(backend)[0].body, undefined)
  })
}
