import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { runInNewContext } from 'node:vm'

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))))
const mcp = await readFile(join(repoRoot, 'mcp/server.js'), 'utf8')
const popupHTML = await readFile(join(repoRoot, 'chromeExpand/popup.html'), 'utf8')
const popupJS = await readFile(join(repoRoot, 'chromeExpand/js/popup.js'), 'utf8')
const extensionVisibility = await readFile(join(repoRoot, 'chromeExpand/js/visibility.js'), 'utf8')

assert.match(mcp, /z\.enum\(\['public', 'users', 'contacts', 'private'\]\)/, 'MCP schemas must expose all four visibility states')
assert.match(mcp, /const body = \{ type, visibility, private: visibility !== 'public' \}/, 'MCP publish must send visibility and legacy private')
assert.match(mcp, /legacyPrivate === true \? 'private' : 'public'/, 'MCP legacy publish default must remain public unless private=true')
assert.match(mcp, /body\.visibility = visibility[\s\S]*body\.private = visibility !== 'public'/, 'MCP update must support visibility-only changes')
for (const endpoint of ['/api/messages/${id}', '/api/status', '/api/messages/calendar', '/api/frontend/config']) {
  assert.ok(mcp.includes(`fetch(\`${'${host}'}${endpoint}\`, { headers: authHeaders() })`), `MCP read ${endpoint} must carry auth headers`)
}
assert.match(mcp, /fetch\(url, \{ headers: authHeaders\(\) \}\)/, 'MCP page/search/tag reads must carry auth headers')

assert.match(popupHTML, /<select id="visibilitySelect"[^>]*>[\s\S]*value="public"[\s\S]*value="users"[\s\S]*value="contacts"[\s\S]*value="private"/, 'extension must expose a four-state visibility selector')
assert.match(popupHTML, /<script src="js\/visibility\.js"><\/script>[\s\S]*<script src="js\/popup\.js"><\/script>/, 'extension visibility helper must load before popup code')
assert.match(popupJS, /EchoNoiseVisibility\.buildPublishPayload\(content, visibility/, 'extension publish paths must share the executable payload builder')
assert.match(popupJS, /messageVisibility = "public"/, 'extension must preserve the legacy public default')

const extensionContext = {}
runInNewContext(extensionVisibility, extensionContext)
for (const visibility of ['public', 'users', 'contacts', 'private']) {
  const payload = extensionContext.EchoNoiseVisibility.buildPublishPayload('extension payload', visibility, { notify: true })
  assert.deepEqual(
    JSON.parse(JSON.stringify(payload)),
    { content: 'extension payload', visibility, private: visibility !== 'public', notify: true },
    `extension payload for ${visibility} must preserve the four-state contract`
  )
}
assert.equal(extensionContext.EchoNoiseVisibility.normalizeVisibility('invalid'), 'public', 'extension invalid visibility must fail back to public')

const previousFetch = globalThis.fetch
const previousHost = process.env.NOTE_HOST
const previousToken = process.env.NOTE_TOKEN
const requests = []
process.env.NOTE_HOST = 'https://notes.example.test'
process.env.NOTE_TOKEN = 'ordinary-user-token'
globalThis.fetch = async (url, init = {}) => {
  requests.push({ url: String(url), init })
  return {
    ok: true,
    status: 200,
    json: async () => ({ code: 1 })
  }
}
try {
  const { publishTool, updateTool } = await import(pathToFileURL(join(repoRoot, 'mcp/server.js')).href + '?visibility-payload-test')
  for (const visibility of ['public', 'users', 'contacts', 'private']) {
    requests.length = 0
    await publishTool({ content: 'mcp payload', visibility })
    assert.equal(requests.length, 1)
    assert.equal(requests[0].url, 'https://notes.example.test/api/token/messages')
    assert.equal(requests[0].init.headers.Authorization, 'Bearer ordinary-user-token')
    assert.deepEqual(
      JSON.parse(requests[0].init.body),
      { type: 'text', visibility, private: visibility !== 'public', content: 'mcp payload' }
    )
  }

  requests.length = 0
  await publishTool({ content: 'legacy private', private: true })
  assert.equal(JSON.parse(requests[0].init.body).visibility, 'private')
  requests.length = 0
  await publishTool({ content: 'legacy default' })
  assert.equal(JSON.parse(requests[0].init.body).visibility, 'public')

  requests.length = 0
  await updateTool({ id: '42', visibility: 'contacts' })
  assert.equal(requests[0].url, 'https://notes.example.test/api/token/messages/42')
  assert.deepEqual(JSON.parse(requests[0].init.body), { visibility: 'contacts', private: true })
} finally {
  globalThis.fetch = previousFetch
  if (previousHost === undefined) delete process.env.NOTE_HOST
  else process.env.NOTE_HOST = previousHost
  if (previousToken === undefined) delete process.env.NOTE_TOKEN
  else process.env.NOTE_TOKEN = previousToken
}

console.log('external visibility client executable payload tests passed')
