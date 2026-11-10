import assert from 'node:assert/strict'
import test from 'node:test'
import {
  ALICE, TOOL_NAMES, TEST_OPTIONS, mockBackend, toolClient, ok,
  dataOf, assertToolError, businessRequests,
} from './helpers.js'

const call = (client, name, args = {}) => client.callTool({ name, arguments: args })

// SDK listTools is the public schema contract; do not introspect private tool maps.
test('all fourteen tools have English names and strict, usable input schemas', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const client = await toolClient(t, backend)
  const { tools } = await client.listTools()
  assert.deepEqual(tools.map(tool => tool.name).sort(), [...TOOL_NAMES].sort())
  const expected = {
    search: ['keyword', 'query', 'page', 'page_number', 'pageSize', 'page_size', 'authorId', 'username', 'format'],
    page: ['page', 'page_number', 'pageSize', 'page_size', 'authorId', 'username', 'date', 'keyword', 'tag', 'pinScope', 'excludeId', 'format'],
    message: ['id'],
    publish: ['content', 'type', 'private', 'visibility', 'created_at', 'image', 'images', 'imageURL', 'image_url', 'video_url', 'notify'],
    update: ['id', 'content', 'private', 'visibility', 'created_at'],
    delete: ['id'], pin: ['id', 'pinned', 'scope'],
    settings: ['allowRegistration', 'autoApproveRegistration', 'frontendSettings'],
    status: [], calendar: ['authorId'], config: [], login: ['username', 'password'], rss: [], identity: [],
  }
  const required = { message: ['id'], update: ['id'], delete: ['id'], pin: ['id', 'pinned'], login: ['username', 'password'] }
  for (const tool of tools) {
    assert.match(tool.name, /^[a-z]+$/)
    assert.ok(tool.description?.length)
    assert.equal(tool.inputSchema.type, 'object')
    assert.equal(tool.inputSchema.additionalProperties, false, tool.name)
    assert.deepEqual(Object.keys(tool.inputSchema.properties).sort(), expected[tool.name].sort(), tool.name)
    assert.deepEqual([...(tool.inputSchema.required ?? [])].sort(), [...(required[tool.name] ?? [])].sort(), tool.name)
  }
  for (const name of ['publish', 'update']) {
    const schema = tools.find(tool => tool.name === name).inputSchema
    assert.deepEqual(schema.properties.visibility.enum, ['public', 'users', 'contacts', 'private'])
    assert.equal(schema.properties.private.type, 'boolean')
  }
  const resolveSchema = (root, node) => {
    const seen = new Set()
    while (node?.$ref) {
      assert.match(node.$ref, /^#\//, 'schema references must be local JSON pointers')
      assert.ok(!seen.has(node.$ref), 'schema references must not cycle')
      seen.add(node.$ref)
      node = node.$ref.slice(2).split('/').reduce((value, part) => value?.[decodeURIComponent(part).replace(/~1/g, '/').replace(/~0/g, '~')], root)
      assert.ok(node, 'schema reference must resolve')
    }
    return node
  }
  for (const name of ['search', 'page']) {
    const schema = tools.find(tool => tool.name === name).inputSchema
    const properties = Object.fromEntries(Object.entries(schema.properties).map(([key, value]) => [key, resolveSchema(schema, value)]))
    for (const key of ['page', 'page_number', 'pageSize', 'page_size']) assert.equal(properties[key].type, 'integer')
    assert.equal(properties.pageSize.maximum, 100)
    assert.equal(properties.page_size.maximum, 100)
  }
  for (const name of ['search', 'page']) assert.deepEqual(tools.find(tool => tool.name === name).inputSchema.properties.format.enum, ['text', 'json'])
  assert.deepEqual(tools.find(tool => tool.name === 'publish').inputSchema.properties.type.enum, ['text', 'markdown', 'md', 'image', 'multipart'])
  assert.equal(backend.received.length, 0, 'enumeration must not call the backend')
})

const reads = [
  ['search', { keyword: 'hello' }, '/api/messages/search', { keyword: 'hello', page: '1', pageSize: '10' }],
  ['page', {}, '/api/messages/page', { page: '1', pageSize: '10' }],
  ['message', { id: '42' }, '/api/messages/42', {}],
  ['status', {}, '/api/status', {}],
  ['calendar', { authorId: 101 }, '/api/messages/calendar', { authorId: '101' }],
  ['config', {}, '/api/frontend/config', {}],
  ['rss', {}, '/rss', {}],
]
for (const [name, args, path, query] of reads) {
  test(`${name}: verifies AI identity and authenticates the read without Cookie`, TEST_OPTIONS, async t => {
    const backend = await mockBackend(t, request => request.path === path
      ? name === 'rss' ? { status: 200, headers: { 'Content-Type': 'application/rss+xml' }, raw: '<rss><channel><title>fixture</title></channel></rss>' } : ok({ marker: name })
      : undefined)
    const client = await toolClient(t, backend)
    const result = await call(client, name, args)
    assert.notEqual(result.isError, true)
    if (name === 'rss') assert.match(result.content[0].text, /^<rss>/)
    else assert.deepEqual(dataOf(result), { marker: name })
    assert.deepEqual(backend.received.map(request => request.path), ['/api/auth/identity', path])
    for (const request of backend.received) {
      assert.equal(request.method, 'GET')
      assert.equal(request.headers.authorization, `Bearer ${ALICE}`)
      assert.equal(request.headers.cookie, undefined)
    }
    assert.deepEqual(backend.received[1].query, query)
  })
}

for (const name of ['publish', 'update']) {
  for (const visibility of ['public', 'users', 'contacts', 'private']) {
    test(`${name}: preserves explicit ${visibility} visibility`, TEST_OPTIONS, async t => {
      const path = name === 'publish' ? '/api/token/messages' : '/api/token/messages/42'
      const backend = await mockBackend(t, request => request.path === path ? ok({ id: 42, visibility }) : undefined)
      const client = await toolClient(t, backend)
      const args = { content: 'fixture note', visibility, ...(name === 'update' ? { id: 42 } : {}) }
      assert.equal(dataOf(await call(client, name, args)).visibility, visibility)
      const [request] = businessRequests(backend)
      assert.equal(businessRequests(backend).length, 1)
      assert.equal(request.method, name === 'publish' ? 'POST' : 'PUT')
      assert.deepEqual(request.body, { content: 'fixture note', visibility })
      assert.equal(request.headers.authorization, `Bearer ${ALICE}`)
    })
  }
}

test('legacy private maps to visibility; publish defaults public; update omission preserves visibility', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path.startsWith('/api/token/messages') ? ok({ id: 42 }) : undefined)
  const client = await toolClient(t, backend)
  for (const [name, extra, expected] of [
    ['publish', {}, 'public'], ['publish', { private: true }, 'private'], ['publish', { private: false }, 'public'],
    ['update', {}, undefined], ['update', { private: true }, 'private'], ['update', { private: false }, 'public'],
  ]) {
    const args = { content: 'changed', ...(name === 'update' ? { id: '42' } : {}), ...extra }
    dataOf(await call(client, name, args))
    const request = businessRequests(backend).at(-1)
    assert.equal(request.body.visibility, expected)
    assert.equal(Object.hasOwn(request.body, 'visibility'), expected !== undefined)
    assert.equal(Object.hasOwn(request.body, 'private'), false)
  }
  dataOf(await call(client, 'update', { id: 42, visibility: 'contacts' }))
  assert.deepEqual(businessRequests(backend).at(-1).body, { visibility: 'contacts' })
})

test('explicit visibility takes precedence over either legacy private value', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path.startsWith('/api/token/messages') ? ok({ id: 42 }) : undefined)
  const client = await toolClient(t, backend)
  for (const name of ['publish', 'update']) {
    for (const visibility of ['public', 'users', 'contacts', 'private']) {
      for (const legacy of [true, false]) {
        dataOf(await call(client, name, { content: 'note', visibility, private: legacy, ...(name === 'update' ? { id: 42 } : {}) }))
        assert.deepEqual(businessRequests(backend).at(-1).body, { content: 'note', visibility })
      }
    }
  }
})

test('consistent legacy visibility pairs are accepted', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path.startsWith('/api/token/messages') ? ok({ id: 42 }) : undefined)
  const client = await toolClient(t, backend)
  for (const name of ['publish', 'update']) {
    for (const visibility of ['public', 'users', 'contacts', 'private']) {
      dataOf(await call(client, name, { content: 'note', visibility, private: visibility === 'private', ...(name === 'update' ? { id: 42 } : {}) }))
      assert.equal(businessRequests(backend).at(-1).body.visibility, visibility)
    }
  }
})

test('keyword/query and pagination aliases normalize to the backend canonical names', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => ['/api/messages/search', '/api/messages/page'].includes(request.path) ? ok([]) : undefined)
  const client = await toolClient(t, backend)
  for (const search of [{ keyword: 'needle' }, { query: 'needle' }, { keyword: 'needle', query: 'needle' }]) {
    for (const paging of [{ page: 2, pageSize: 7 }, { page_number: 2, page_size: 7 }, { page: 2, page_number: 2, pageSize: 7, page_size: 7 }]) {
      dataOf(await call(client, 'search', { ...search, ...paging }))
      assert.deepEqual(businessRequests(backend).at(-1).query, { keyword: 'needle', page: '2', pageSize: '7' })
    }
  }
  for (const paging of [{ page: 3, pageSize: 8 }, { page_number: 3, page_size: 8 }, { page: 3, page_number: 3, pageSize: 8, page_size: 8 }]) {
    dataOf(await call(client, 'page', paging))
    assert.deepEqual(businessRequests(backend).at(-1).query, { page: '3', pageSize: '8' })
  }
})

test('conflicting aliases and invalid schema arguments never reach the backend', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const client = await toolClient(t, backend)
  const cases = [
    ['search', { keyword: 'a', query: 'b' }], ['search', { keyword: '', query: 'b' }], ['search', { keyword: null }],
    ['publish', {}], ['publish', { content: '' }], ['publish', { content: '   ' }], ['publish', { content: 'note', visibility: 'friends' }],
    ['publish', { type: 'image', content: 'caption' }], ['publish', { type: 'unknown', content: 'note' }],
    ['publish', { images: [] }], ['publish', { image: '' }], ['publish', { images: ['https://example.test/a.png', '   '] }],
    ['publish', { imageURL: 'javascript:alert(1)' }], ['publish', { video_url: '   ' }], ['publish', { content: { text: 'note' } }],
    ['page', { format: 'xml' }], ['search', { format: 'xml' }],
    ['update', { id: 42 }], ['settings', {}], ['pin', { id: 42, pinned: true, scope: 'all' }],
    ['login', { username: 'alice' }], ['message', { id: '../42' }], ['message', { id: 0 }],
    ['delete', { id: -1 }], ['delete', { id: 1.2 }], ['delete', { id: Number.MAX_SAFE_INTEGER + 1 }],
  ]
  for (const name of ['page', 'search']) {
    const base = name === 'search' ? { keyword: 'needle' } : {}
    for (const args of [{ page: 1, page_number: 2 }, { pageSize: 2, page_size: 3 }, { page: 0 }, { page_number: -1 }, { pageSize: 101 }, { page_size: 101 }, { pageSize: 1.5 }, { page: '2' }]) cases.push([name, { ...base, ...args }])
  }
  for (const [name, args] of cases) assertToolError(await call(client, name, args))
  assert.deepEqual(backend.received, [])
})

test('delete requires an explicit ID and only invokes the token soft-delete endpoint', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/token/messages/42' ? ok({ id: 42, deleted_at: '2026-11-10T00:00:00Z' }) : undefined)
  const client = await toolClient(t, backend)
  for (const args of [{ keyword: 'note' }, { query: 'note' }, { id: 42, keyword: 'note' }, { id: 42, force: true }, { ids: [42] }, {}]) {
    assertToolError(await call(client, 'delete', args))
  }
  assert.deepEqual(backend.received, [], 'keyword deletion must not search or authenticate')
  dataOf(await call(client, 'delete', { id: '42' }))
  assert.deepEqual(backend.received.map(request => [request.method, request.path]), [
    ['GET', '/api/auth/identity'], ['DELETE', '/api/token/messages/42'],
  ])
  assert.equal(businessRequests(backend)[0].body, undefined)
})

for (const [name, args, path, method, body] of [
  ['pin', { id: 42, pinned: true }, '/api/token/messages/42/pin/global', 'PUT', { pinned: true }],
  ['pin', { id: 42, pinned: false, scope: 'personal' }, '/api/token/messages/42/pin/personal', 'PUT', { pinned: false }],
  ['pin', { id: 42, pinned: false, scope: 'global' }, '/api/token/messages/42/pin/global', 'PUT', { pinned: false }],
  ['pin', { id: 42, pinned: true, scope: 'personal' }, '/api/token/messages/42/pin/personal', 'PUT', { pinned: true }],
  ['settings', { allowRegistration: false }, '/api/token/settings', 'PUT', { allowRegistration: false }],
]) {
  test(`${name}: uses its authorized route ${path}`, TEST_OPTIONS, async t => {
    const backend = await mockBackend(t, request => request.path === path ? ok({ updated: true }) : undefined)
    const client = await toolClient(t, backend)
    dataOf(await call(client, name, args))
    const requests = businessRequests(backend)
    assert.equal(requests.length, 1)
    assert.equal(requests[0].path, path)
    assert.equal(requests[0].method, method)
    assert.deepEqual(requests[0].body, body)
    assert.equal(requests[0].headers.authorization, `Bearer ${ALICE}`)
  })
}

for (const [name, args, path] of [
  ['publish', { content: 'note' }, '/api/token/messages'],
  ['update', { id: 42, content: 'note' }, '/api/token/messages/42'],
  ['delete', { id: 42 }, '/api/token/messages/42'],
  ['pin', { id: 42, pinned: true }, '/api/token/messages/42/pin/global'],
  ['settings', { allowRegistration: true }, '/api/token/settings'],
  ['pin', { id: 42, pinned: false, scope: 'personal' }, '/api/token/messages/42/pin/personal'],
  ['page', {}, '/api/messages/page'],
]) {
  for (const [label, reply] of [
    ['403', { status: 403, body: { code: 1, data: { success: true } } }],
    ['business code 0', { status: 200, body: { code: 0, data: { success: true } } }],
  ]) {
    test(`${name}: ${label} isError, no search, session route or fallback`, TEST_OPTIONS, async t => {
      const backend = await mockBackend(t, request => request.path === path ? reply : undefined)
      const client = await toolClient(t, backend)
      assertToolError(await call(client, name, args))
      assert.deepEqual(backend.received.map(request => request.path), ['/api/auth/identity', path])
    })
  }
}

test('search routes omitted and blank terms to page and tags through the client, preserving author filters', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path.startsWith('/api/messages/') ? ok({ items: [] }) : undefined)
  const client = await toolClient(t, backend)
  for (const terms of [{}, { keyword: '' }, { keyword: ' \t ' }, { query: '\n ' }, { keyword: ' ', query: '' }]) {
    dataOf(await call(client, 'search', { ...terms, page_number: 2, page_size: 7, authorId: 101, username: 'alice' }))
    const request = businessRequests(backend).at(-1)
    assert.equal(request.path, '/api/messages/page')
    assert.deepEqual(request.query, { page: '2', pageSize: '7', authorId: '101', username: 'alice' })
  }
  for (const terms of [{ keyword: ' #生活/工作 ' }, { query: '#生活/工作' }, { keyword: '#生活/工作', query: ' #生活/工作 ' }]) {
    dataOf(await call(client, 'search', { ...terms, page: 3, pageSize: 8, authorId: 101 }))
    const request = businessRequests(backend).at(-1)
    assert.equal(request.path, `/api/messages/tags/${encodeURIComponent('生活/工作')}`)
    assert.deepEqual(request.query, { page: '3', pageSize: '8', authorId: '101' })
  }
})

test('page preserves every supported filter and strips pagination aliases', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/messages/page' ? ok([]) : undefined)
  const client = await toolClient(t, backend)
  dataOf(await call(client, 'page', { page_number: 2, page_size: 5, authorId: 101, username: 'alice', date: '2026-11-10', keyword: 'note', tag: '生活', pinScope: 'personal', excludeId: 42 }))
  assert.deepEqual(businessRequests(backend)[0].query, { page: '2', pageSize: '5', authorId: '101', username: 'alice', date: '2026-11-10', keyword: 'note', tag: '生活', pinScope: 'personal', excludeId: '42' })
})

test('format defaults to compatible JSON and explicit text renders all search branches without upstream format', TEST_OPTIONS, async t => {
  const item = { id: 42, username: 'alice', created_at: '2026-11-10T12:00:00Z', content: 'fixture note', image_url: '/api/images/a.png' }
  const payload = { items: [item], total: 1 }
  const backend = await mockBackend(t, request => request.path.startsWith('/api/messages/') ? ok(payload) : undefined)
  const client = await toolClient(t, backend)
  for (const [name, args, path, query, header] of [
    ['page', {}, '/api/messages/page', {}, '以下是分页内容'],
    ['search', {}, '/api/messages/page', {}, '以下是最新内容'],
    ['search', { query: '#tag' }, '/api/messages/tags/tag', {}, '关于“#tag”'],
    ['search', { keyword: 'needle' }, '/api/messages/search', { keyword: 'needle' }, '关于“needle”'],
  ]) {
    const implicit = await call(client, name, args)
    const json = await call(client, name, { ...args, format: 'json' })
    assert.deepEqual(implicit, json)
    assert.deepEqual(dataOf(json), payload)
    const result = await call(client, name, { ...args, format: 'text' })
    assert.notEqual(result.isError, true)
    assert.ok(result.content[0].text.includes(header))
    for (const value of ['[42]', 'alice', item.created_at, 'fixture note', '![image](/api/images/a.png)', '共1条']) assert.ok(result.content[0].text.includes(value), value)
    for (const request of businessRequests(backend).slice(-3)) {
      assert.equal(request.path, path)
      assert.deepEqual(request.query, { ...query, page: '1', pageSize: '10' })
    }
  }
})

test('text handles array results, empty results, and existing Markdown images', TEST_OPTIONS, async t => {
  let payload = []
  const backend = await mockBackend(t, request => request.path === '/api/messages/page' ? ok(payload) : undefined)
  const client = await toolClient(t, backend)
  for (const empty of [[], { items: [] }]) {
    payload = empty
    assert.equal((await call(client, 'page', { format: 'text' })).content[0].text, '无匹配结果')
  }
  payload = [{ id: 1, content: '![a](/a.png)', images: ['/a.png', '/b.png'] }]
  const result = await call(client, 'page', { format: 'text' })
  assert.equal(result.content[0].text.split('/a.png').length - 1, 1)
  assert.ok(result.content[0].text.includes('/b.png'))
})

test('publish normalizes all types and image aliases to exact backend bodies', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/token/messages' ? ok({ id: 42 }) : undefined)
  const client = await toolClient(t, backend)
  const a = 'https://example.test/a.png'
  const b = '/api/images/b.png'
  const c = '/api/images/c.png'
  const cases = [
    ...['text', 'markdown', 'md'].map(type => [{ type, content: '  # note\n正文  ' }, { content: '# note\n正文', visibility: 'public' }]),
    ...['image', 'imageURL', 'image_url'].map(key => [{ type: 'image', [key]: a }, { image_url: a, visibility: 'public' }]),
    [{ type: 'image', image: a, content: '' }, { image_url: a, visibility: 'public' }],
    [{ type: 'image', image: a, content: '  caption  ' }, { content: 'caption', image_url: a, visibility: 'public' }],
    [{ images: [a] }, { image_url: a, visibility: 'public' }],
    [{ type: 'image', images: [a, b] }, { content: `![image](${a})\n![image](${b})`, image_url: a, visibility: 'public' }],
    [{ type: 'multipart', content: 'caption', images: [a, b] }, { content: `caption\n\n![image](${a})\n![image](${b})`, image_url: a, visibility: 'public' }],
    [{ image: a, images: [a, b], imageURL: c, image_url: c }, { content: `![image](${a})\n![image](${b})\n![image](${c})`, image_url: c, visibility: 'public' }],
    [{ images: ['/api/images/a(1).png', b] }, { content: `![image](/api/images/a%281%29.png)\n![image](${b})`, image_url: '/api/images/a(1).png', visibility: 'public' }],
    [{ type: 'multipart', image_url: b, video_url: '/api/videos/v.mp4', content: 'note', notify: false, created_at: '2026-11-10T12:00:00Z', visibility: 'users', private: true }, { image_url: b, video_url: '/api/videos/v.mp4', content: 'note\n\n[video](/api/videos/v.mp4)', notify: false, created_at: '2026-11-10T12:00:00Z', visibility: 'users' }],
    [{ video_url: '/api/videos/v.mp4', notify: true }, { content: '[video](/api/videos/v.mp4)', video_url: '/api/videos/v.mp4', notify: true, visibility: 'public' }],
  ]
  for (const [args, body] of cases) {
    dataOf(await call(client, 'publish', args))
    const request = businessRequests(backend).at(-1)
    assert.equal(request.method, 'POST')
    assert.equal(request.path, '/api/token/messages')
    assert.deepEqual(request.body, body)
  }
  assert.equal(businessRequests(backend).length, cases.length)
})

test('alias conflicts fail before identity checks in empty, tag and keyword search branches', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t)
  const client = await toolClient(t, backend)
  for (const keyword of ['', '#tag', 'needle']) {
    for (const paging of [{ page: 1, page_number: 2 }, { pageSize: 1, page_size: 2 }]) {
      assertToolError(await call(client, 'search', { keyword, ...paging, format: 'text' }))
    }
  }
  assertToolError(await call(client, 'search', { keyword: '#a', query: '#b' }))
  assert.deepEqual(backend.received, [])
})

test('timestamp-only updates preserve visibility and content', TEST_OPTIONS, async t => {
  const backend = await mockBackend(t, request => request.path === '/api/token/messages/42' ? ok({ id: 42 }) : undefined)
  const client = await toolClient(t, backend)
  dataOf(await call(client, 'update', { id: 42, created_at: '2026-11-10T12:00:00Z' }))
  assert.deepEqual(businessRequests(backend)[0].body, { created_at: '2026-11-10T12:00:00Z' })
})
