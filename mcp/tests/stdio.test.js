import assert from 'node:assert/strict'
import test from 'node:test'
import { spawn } from 'node:child_process'
import { Client } from '@modelcontextprotocol/sdk/client/index.js'
import { StdioClientTransport, getDefaultEnvironment } from '@modelcontextprotocol/sdk/client/stdio.js'
import {
  ALICE, BOB, PASSWORD, ROTATED, COOKIE, ENTRY, MCP_DIR, TOOL_NAMES, TEST_OPTIONS,
  mockBackend, ok, dataOf, assertToolError, assertNoSecrets,
} from './helpers.js'

// SDK 1.23.0 exposes stderr but no raw stdout observer. Observe the spawned
// process solely for output auditing; all protocol traffic still uses the SDK.
class AuditedStdioTransport extends StdioClientTransport {
  stdout = ''
  errors = ''
  exited

  constructor(env) {
    super({ command: process.execPath, args: [ENTRY], cwd: MCP_DIR, env, stderr: 'pipe' })
    this.stderr.on('data', chunk => { this.errors += chunk.toString('utf8') })
  }

  start() {
    const started = super.start()
    assert.ok(this._process?.stdout, 'SDK 1.23 stdout observer requires the spawned process')
    this._process.stdout.on('data', chunk => { this.stdout += chunk.toString('utf8') })
    this.exited = new Promise(resolve => this._process.once('close', resolve))
    return started
  }

  async waitForExit() {
    let timer
    try {
      await Promise.race([
        this.exited,
        new Promise((resolve, reject) => { timer = setTimeout(() => reject(new Error('stdio child did not exit')), 3000) }),
      ])
    } finally { clearTimeout(timer) }
  }
}

async function stdioClient(t, backend, token = ALICE) {
  // Do not spread process.env: transport defaults provide OS essentials only.
  // In particular MCP_TRANSPORT is absent, exercising default stdio startup.
  const transport = new AuditedStdioTransport({ NOTE_HOST: backend.url, NOTE_TOKEN: token, NOTE_TIMEOUT_MS: '1000' })
  const client = new Client({ name: 'stdio-contract-tests', version: '1.0.0' })
  let closed = false
  const close = async () => {
    if (closed) return
    closed = true
    await client.close()
    await transport.waitForExit()
  }
  t.after(close)
  await client.connect(transport)
  assert.ok(transport.pid > 0, 'must spawn the real server.js')
  return { client, transport, close }
}

function assertProtocolOnly(transport) {
  assertNoSecrets(transport.stdout)
  assertNoSecrets(transport.errors)
  const lines = transport.stdout.trim().split(/\r?\n/).filter(Boolean)
  assert.ok(lines.length > 0)
  for (const line of lines) {
    const message = JSON.parse(line)
    assert.equal(message.jsonrpc, '2.0', 'stdout may contain only JSON-RPC messages')
    assert.ok(Object.hasOwn(message, 'result') || Object.hasOwn(message, 'error') || typeof message.method === 'string')
  }
}

test('real server.js defaults to stdio, enumerates tools and authenticates reads using NOTE_HOST/NOTE_TOKEN', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/messages/page' ? ok({ items: [{ id: 42 }] }) : undefined)
  const { client, transport, close } = await stdioClient(t, backend)
  const tools = await client.listTools()
  assert.deepEqual(tools.tools.map(tool => tool.name).sort(), [...TOOL_NAMES].sort())
  assert.deepEqual(backend.received, [], 'initialization and tools/list must not call the API')
  const initialized = transport.stdout.trim().split(/\r?\n/).map(line => JSON.parse(line)).find(message => message.result?.protocolVersion)
  assert.ok(initialized, 'stdio must return an initialize response')
  assert.equal(initialized.jsonrpc, '2.0')
  assert.equal(typeof initialized.id, 'number')
  assert.ok(['2025-06-18', '2025-03-26'].includes(initialized.result.protocolVersion))
  assert.ok(initialized.result.capabilities.tools)
  const result = await client.callTool({ name: 'page', arguments: { page_number: 2, page_size: 5 } })
  assert.deepEqual(dataOf(result), { items: [{ id: 42 }] })
  assert.deepEqual(backend.received.map(request => [request.method, request.path]), [
    ['GET', '/api/auth/identity'], ['GET', '/api/messages/page'],
  ])
  assert.deepEqual(backend.received[1].query, { page: '2', pageSize: '5' })
  assert.ok(backend.received.every(request => request.headers.authorization === `Bearer ${ALICE}`))
  await close()
  assertProtocolOnly(transport)
})

test('real stdio login, errors and validation never print password, token or Cookie', TEST_OPTIONS, async t => {
  let loginCount = 0
  const backend = await mockBackend(t, request => {
    if (request.path === '/api/ai/login') {
      loginCount++
      return loginCount === 1
        ? { ...ok({ renewed: true, token: ROTATED, password: PASSWORD }), headers: { 'Set-Cookie': `session=${COOKIE}` } }
        : { status: 401, body: { code: 0, message: `${ALICE} ${PASSWORD} ${ROTATED} ${COOKIE}` } }
    }
    if (request.path === '/api/token/messages/42') return { status: 403, body: { code: 0, message: ALICE } }
  })
  const { client, transport, close } = await stdioClient(t, backend)
  const login = await client.callTool({ name: 'login', arguments: { username: 'alice', password: PASSWORD } })
  assert.equal(dataOf(login).identity.username, 'alice')
  assert.deepEqual(backend.received.slice(0, 2).map(request => request.path), ['/api/ai/login', '/api/auth/identity'])
  assertToolError(await client.callTool({ name: 'login', arguments: { username: 'alice', password: PASSWORD } }))
  assertToolError(await client.callTool({ name: 'delete', arguments: { id: 42 } }))
  assertToolError(await client.callTool({ name: 'login', arguments: { username: 'alice', password: PASSWORD, [ALICE]: ROTATED } }))
  assert.ok(backend.received.every(request => request.headers.authorization === `Bearer ${ALICE}`))
  assertNoSecrets(login)
  await close()
  assertProtocolOnly(transport)
})

test('independent stdio processes retain their own identities during renewal', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/ai/login' ? ok({ renewed: true }) : undefined)
  const [alice, bob] = await Promise.all([stdioClient(t, backend, ALICE), stdioClient(t, backend, BOB)])
  const [renewed, bobIdentity] = await Promise.all([
    alice.client.callTool({ name: 'login', arguments: { username: 'alice', password: PASSWORD } }),
    bob.client.callTool({ name: 'identity', arguments: {} }),
  ])
  assert.equal(dataOf(renewed).identity.username, 'alice')
  assert.equal(dataOf(bobIdentity).username, 'bob')
  assert.equal(dataOf(await bob.client.callTool({ name: 'identity', arguments: {} })).username, 'bob')
  await Promise.all([alice.close(), bob.close()])
  assertProtocolOnly(alice.transport)
  assertProtocolOnly(bob.transport)
})

test('real server.js exits cleanly with MCP_ENABLED=false/0 in default stdio and HTTP modes', TEST_OPTIONS, async t => {
  for (const MCP_ENABLED of ['false', '0']) {
    for (const MCP_TRANSPORT of [undefined, 'http']) {
      const child = spawn(process.execPath, [ENTRY], {
        cwd: MCP_DIR, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'],
        env: {
          ...getDefaultEnvironment(), MCP_ENABLED,
          ...(MCP_TRANSPORT === undefined ? {} : { MCP_TRANSPORT }),
          NOTE_HOST: 'invalid-disabled-host', NOTE_TOKEN: ALICE, MCP_HTTP_PORT: 'invalid-disabled-port',
        },
      })
      let stdout = ''
      let stderr = ''
      child.stdout.on('data', chunk => { stdout += chunk.toString('utf8') })
      child.stderr.on('data', chunk => { stderr += chunk.toString('utf8') })
      const cleanup = () => { if (child.exitCode === null && child.signalCode === null) child.kill() }
      t.after(cleanup)
      let timer
      try {
        const result = await new Promise((resolve, reject) => {
          child.once('error', reject)
          child.once('close', (code, signal) => resolve({ code, signal }))
          timer = setTimeout(() => { cleanup(); reject(new Error('disabled MCP process did not exit')) }, 3000)
        })
        assert.deepEqual(result, { code: 0, signal: null })
        assert.equal(stdout, '')
        assert.equal(stderr, '')
        assertNoSecrets([stdout, stderr])
      } finally { clearTimeout(timer); cleanup() }
    }
  }
})
