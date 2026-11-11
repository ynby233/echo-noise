import assert from 'node:assert/strict'
import test from 'node:test'
import { ALICE, PASSWORD, TEST_OPTIONS, mockBackend, toolClient, httpServer, httpClient, assertToolError, assertNoSecrets } from './helpers.js'
for (const transport of ['stdio-protocol', 'http']) {
  test(`${transport}: expired login has a recovery hint and blocks all business calls`, TEST_OPTIONS, async t => {
    const backend = await mockBackend(t, request => request.path === '/api/auth/identity'
      ? { status: 401, body: { code: 0, error: 'login_expired', msg: `untrusted ${ALICE} ${PASSWORD}` } }
      : undefined)
    const client = transport === 'http' ? (await httpClient(t, (await httpServer(t, backend)).url)).client : await toolClient(t, backend)
    const result = await client.callTool({ name: 'search', arguments: { keyword: 'private' } })
    assertToolError(result)
    assert.match(result.content[0].text, /登录已过期/)
    assert.match(result.content[0].text, /重新登录站点/)
    assert.match(result.content[0].text, /login.*同账号密码.*续期/)
    assert.doesNotMatch(result.content[0].text, /HTTP 401|后端请求失败|untrusted/)
    assert.deepEqual(backend.received.map(r => r.path), ['/api/auth/identity'])
    assertNoSecrets(result)
  })
}

for (const [label, reply] of [
  ['revoked', { status: 401, body: { code: 0, error: 'invalid_credentials', msg: '登录已过期' } }],
  ['untyped', { status: 401, body: { code: 0, msg: '登录已过期' } }],
  ['malformed', { status: 401, raw: '<html>登录已过期</html>' }],
]) {
  test(`${label} credentials are not mislabeled as expired`, TEST_OPTIONS, async t => {
    const backend = await mockBackend(t, () => reply)
    const client = await toolClient(t, backend)
    const result = await client.callTool({ name: 'identity', arguments: {} })
    assertToolError(result)
    assert.match(result.content[0].text, /凭据无效或已撤销/)
    assert.match(result.content[0].text, /创建新凭据/)
    assert.doesNotMatch(result.content[0].text, /登录已过期/)
  })
}

test('expiry during a write is reported clearly without retry', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/token/messages'
    ? { status: 401, body: { code: 0, error: 'login_expired' } } : undefined)
  const client = await toolClient(t, backend)
  const result = await client.callTool({ name: 'publish', arguments: { content: 'note' } })
  assertToolError(result)
  assert.match(result.content[0].text, /登录已过期/)
  assert.equal(backend.received.filter(r => r.method === 'POST').length, 1)
})

test('permission rejection is not described as login expiry', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/token/settings'
    ? { status: 403, body: { code: 0, error: 'login_expired' } } : undefined)
  const client = await toolClient(t, backend)
  const result = await client.callTool({ name: 'settings', arguments: { allowRegistration: false } })
  assertToolError(result)
  assert.match(result.content[0].text, /权限不足/)
  assert.doesNotMatch(result.content[0].text, /登录已过期/)
})
