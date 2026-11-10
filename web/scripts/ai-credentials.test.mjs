import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { stripTypeScriptTypes } from 'node:module'

const source = await readFile(new URL('../utils/ai-credentials.ts', import.meta.url), 'utf8')
const moduleURL = `data:text/javascript;base64,${Buffer.from(stripTypeScriptTypes(source)).toString('base64')}`
const { createAiCredentialManager, createAiCredentialState } = await import(moduleURL)
const profile = await readFile(new URL('../components/admin/sections/ProfileSection.vue', import.meta.url), 'utf8')
const card = await readFile(new URL('../components/admin/sections/AiCredentialsCard.vue', import.meta.url), 'utf8')
const reply = data => ({ ok: true, json: async () => ({ code: 1, data }) })
const entry = { id: 7, name: 'My client', created_at: '2026-11-10T12:00:00Z' }
function deferred() {
  let resolve
  let reject
  const promise = new Promise((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}
function setup(request) {
  const state = createAiCredentialState()
  const manager = createAiCredentialManager(state, request)
  manager.reset(1)
  return { state, manager }
}

// Exercise the production manager with controlled network replies, not a duplicate implementation.
const calls = []
let response = []
const { state, manager } = setup(async (url, options) => {
  calls.push({ url, options })
  return reply(response)
})
await manager.load()
assert.deepEqual(state.items, [], 'an empty list is a successful initial state')
assert.equal(state.error, '')
for (const name of ['', '   ', 'x'.repeat(101)]) {
  state.name = name
  await manager.create()
}
assert.equal(calls.length, 1, 'invalid names must not create credentials')
state.name = '  My client  '
response = { ...entry, token: 'one-time-test-secret' }
await manager.create()
assert.equal(calls.at(-1).url, '/api/user/ai-credentials')
assert.equal(calls.at(-1).options.method, 'POST')
assert.deepEqual(JSON.parse(calls.at(-1).options.body), { name: 'My client' })
assert.equal(state.secret, 'one-time-test-secret')
assert.deepEqual(state.items, [entry], 'the creation token must not leak into list metadata')
assert.equal(state.name, '')
state.name = 'Another client'
await manager.create()
assert.equal(calls.length, 2, 'an unread one-time secret must not be replaced')
manager.clearSecret()
assert.equal(state.secret, '')
assert.equal(state.secretId, null)
response = [{ ...entry, token: 'unexpected-list-secret', hash: 'unexpected-hash' }]
await manager.load()
assert.deepEqual(state.items, [entry], 'list handling allowlists metadata even if a server returns excess fields')
assert.equal(state.secret, '', 'refresh cannot redisplay a credential')
await manager.revoke(999)
assert.equal(calls.length, 3, 'only credentials in the current account list may be revoked')
response = undefined
await manager.revoke(entry.id)
assert.equal(calls.at(-1).url, '/api/user/ai-credentials/7')
assert.equal(calls.at(-1).options.method, 'DELETE')
assert.deepEqual(state.items, [])
for (const call of calls) {
  assert.equal(call.options.credentials, 'include')
  assert.equal(call.options.cache, 'no-store')
  assert.equal(new Headers(call.options.headers).get('Authorization'), null, 'management must use the current session, never a bearer credential')
}

const pendingCreate = deferred()
let creates = 0
const concurrent = setup(() => { creates++; return pendingCreate.promise })
concurrent.state.name = 'Client'
const creation = concurrent.manager.create()
await concurrent.manager.create()
await concurrent.manager.load()
assert.equal(creates, 1, 'duplicate clicks and refresh cannot race an active creation')
pendingCreate.resolve(reply({ ...entry, token: 'created-secret' }))
await creation
assert.equal(concurrent.state.busy, false)
assert.equal(concurrent.state.secret, 'created-secret', 'logout must be exercised while a one-time secret is present')
concurrent.manager.reset(null)
assert.deepEqual(concurrent.state, createAiCredentialState(), 'logout clears list, draft, status and secret')
await concurrent.manager.create()
await concurrent.manager.load()
assert.equal(creates, 1, 'logged-out state cannot make management requests')

// Include A -> B -> A so merely checking the current account ID cannot pass these tests.
for (const action of ['load', 'create', 'revoke']) {
  for (const completion of ['resolve', 'reject', 'json-resolve', 'json-reject']) {
    const oldRequest = deferred()
    const oldBody = deferred()
    const jsonStarted = deferred()
    const freshRequest = deferred()
    let count = 0
    let oldSignal
    const subject = setup((_url, options) => {
      if (++count === 1) { oldSignal = options.signal; return oldRequest.promise }
      return freshRequest.promise
    })
    subject.state.items = [entry]
    subject.state.name = 'Pending client'
    const pending = action === 'revoke' ? subject.manager.revoke(7) : subject.manager[action]()
    if (completion.startsWith('json-')) {
      oldRequest.resolve({ ok: true, json: () => { jsonStarted.resolve(); return oldBody.promise } })
      await jsonStarted.promise
    }
    subject.manager.reset(2)
    assert.equal(oldSignal.aborted, true, 'switching accounts aborts the old request')
    subject.manager.reset(1)
    const freshEntry = { ...entry, id: 9 }
    subject.state.items = [freshEntry]
    subject.state.name = 'Current client'
    const freshPending = action === 'revoke' ? subject.manager.revoke(9) : subject.manager[action]()
    assert.equal(count, 2, 'the current account operation must start before the old operation settles')
    const currentState = structuredClone(subject.state)
    const oldData = action === 'load' ? [entry] : { ...entry, token: 'stale-secret' }
    if (completion === 'resolve') oldRequest.resolve(reply(oldData))
    if (completion === 'reject') oldRequest.reject(new Error('stale-transport-secret'))
    if (completion === 'json-resolve') oldBody.resolve({ code: 1, data: oldData })
    if (completion === 'json-reject') oldBody.reject(new Error('stale-json-secret'))
    await pending
    assert.deepEqual(subject.state, currentState, `${action}/${completion}: stale success, catch and finally must leave the current request untouched`)
    freshRequest.resolve(reply(action === 'load' ? [freshEntry] : action === 'create' ? { ...freshEntry, token: 'fresh-secret' } : undefined))
    await freshPending
    assert.deepEqual(subject.state.items, action === 'revoke' ? [] : [freshEntry])
    assert.equal(subject.state.secret, action === 'create' ? 'fresh-secret' : '')
    assert.equal(subject.state.loading, false)
    assert.equal(subject.state.busy, false)
    assert.equal(subject.state.error, '')
  }
}

// Logout also invalidates a response whose headers arrived while a secret was visible.
const logoutBody = deferred()
const logoutJsonStarted = deferred()
const logout = setup(async () => ({ ok: true, json: () => { logoutJsonStarted.resolve(); return logoutBody.promise } }))
logout.state.items = [entry]
logout.state.name = 'Unsaved draft'
logout.state.secret = 'visible-before-logout'
logout.state.secretId = entry.id
const logoutLoad = logout.manager.load()
await logoutJsonStarted.promise
logout.manager.reset(null)
assert.deepEqual(logout.state, createAiCredentialState(), 'logout immediately clears an existing secret and pending status')
logoutBody.resolve({ code: 1, data: [{ ...entry, token: 'late-logout-secret' }] })
await logoutLoad
assert.deepEqual(logout.state, createAiCredentialState(), 'late JSON cannot restore metadata or secrets after logout')

const failed = setup(async () => { throw new Error('sensitive-response-secret') })
failed.state.name = 'Failure'
await failed.manager.create()
assert.equal(failed.state.secret, '')
assert.equal(failed.state.busy, false)
assert.ok(failed.state.error)
assert.doesNotMatch(failed.state.error, /sensitive-response-secret/, 'transport errors must not surface raw response details')
const denied = setup(async () => ({ ok: false, json: async () => ({ code: 0, msg: 'private token value' }) }))
await denied.manager.load()
assert.ok(denied.state.error)
assert.doesNotMatch(denied.state.error, /private token value/)
assert.equal(denied.state.loading, false)

// Keep HTTP status and business status independent; either failure must reject valid-looking data.
for (const action of ['load', 'create', 'revoke']) {
  for (const failure of ['http', 'business', 'json']) {
    const data = action === 'load' ? [] : { ...entry, token: 'private-response-secret' }
    const subject = setup(async () => ({
      ok: failure !== 'http',
      json: async () => {
        if (failure === 'json') throw new Error('private-response-secret')
        return { code: failure === 'business' ? 0 : 1, data, msg: 'private-response-secret' }
      },
    }))
    subject.state.items = [entry]
    subject.state.name = 'Retry client'
    if (action !== 'create') {
      subject.state.secret = 'existing-one-time-secret'
      subject.state.secretId = entry.id
    }
    const before = structuredClone(subject.state)
    if (action === 'revoke') await subject.manager.revoke(entry.id)
    else await subject.manager[action]()
    assert.ok(subject.state.error, `${action}/${failure}: failure must be visible`)
    assert.doesNotMatch(subject.state.error, /private-response-secret/)
    assert.deepEqual({ ...subject.state, error: '' }, before, `${action}/${failure}: failure must preserve metadata, draft and any displayed secret, and release loading/busy`)
  }
}

const revocation = setup(async () => reply(undefined))
revocation.state.items = [entry, { ...entry, id: 8 }]
revocation.state.secret = 'recent-secret'
revocation.state.secretId = 7
await revocation.manager.revoke(8)
assert.equal(revocation.state.secret, 'recent-secret', 'revoking another credential must not discard the newly generated credential')
await revocation.manager.revoke(7)
assert.equal(revocation.state.secret, '', 'revoking the newly generated credential clears its secret')

assert.match(profile, /<AiCredentialsCard :theme="theme" \/>/)
assert.match(profile, /API Token/)
assert.match(profile, /const regenerateToken/)
assert.match(card, /watch\(accountID/)
assert.match(card, /flush: 'sync'/)
assert.match(card, /onBeforeUnmount\(.*manager\.reset\(null\)/)
assert.match(card, /writeClipboardText\(state\.secret\)/)
assert.match(card, /MCP 必须使用此凭据/)
assert.match(card, /账号登录期限/)
assert.doesNotMatch(`${source}\n${card}`, /localStorage|sessionStorage|console\.|useState\(|defineStore\(/, 'secrets must stay in component-scoped memory')
assert.doesNotMatch(card, /is_admin|isAdmin|hasCapability/, 'personal credential management must not impose administrator-only visibility')
console.log('AI credential manager logic and profile contracts passed')
