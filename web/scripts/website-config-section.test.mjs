import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import test from 'node:test'
import ts from 'typescript'
import * as vue from 'vue'

const directory = new URL('../components/admin/sections/', import.meta.url)
const plain = value => JSON.parse(JSON.stringify(value))

// Execute the real setup, transport, URL resolver and draft composable.
// Only network responses and the component lifecycle are controlled here.
function website({ settings = {}, status = { data: { users: [] } } } = {}) {
  let server = plain(settings)
  const requests = []
  const writes = []
  const notices = []
  const drafts = new Map()
  const unmounted = []
  const scope = vue.effectScope()
  const sandbox = vm.createContext({
    ...vue, exports: {}, Event, AbortSignal, URL,
    defineProps: () => vue.reactive({ theme: {}, adminShellCardClass: [], adminSubtleCardClass: [] }),
    useToast: () => ({ add: notice => notices.push(plain(notice)) }),
    useRuntimeConfig: () => ({ public: { baseApi: '/api' } }),
    useAdminCapabilities: () => ({ can: capability => capability === 'site_settings.manage' }),
    inject: key => key.description === 'admin drafts' ? drafts : vue.ref('1'),
    onMounted() {},
    onActivated() {},
    onDeactivated() {},
    onUnmounted: hook => unmounted.push(hook),
    window: new EventTarget(),
    fetch: async (url, options = {}) => {
      requests.push({ url, method: options.method || 'GET', credentials: options.credentials })
      let body
      if (url === '/api/status') body = plain(status)
      else if (url.startsWith('/api/frontend/config?')) body = { code: 1, data: { frontendSettings: plain(server) } }
      else if (url === '/api/settings' && options.method === 'PUT') {
        const payload = JSON.parse(options.body)
        writes.push({ payload, headers: plain(options.headers), credentials: options.credentials })
        server = { ...server, ...payload.frontendSettings }
        body = { code: 1 }
      } else throw new Error(`Unexpected request: ${url}`)
      return { ok: true, json: async () => body }
    },
  })
  const run = (source, filename) => {
    const code = ts.transpileModule(source.replace(/^import .*$/gm, ''), {
      compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
    }).outputText
    scope.run(() => vm.runInContext(`{\n${code}\n}`, sandbox, { filename }))
    Object.assign(sandbox, sandbox.exports)
  }
  for (const filename of ['config-draft.ts', 'frontend-settings.ts']) {
    run(readFileSync(new URL(filename, directory), 'utf8'), filename)
  }
  run(readFileSync(new URL('../utils/media-url.ts', import.meta.url), 'utf8'), 'media-url.ts')
  const source = readFileSync(new URL('WebsiteConfigSection.vue', directory), 'utf8')
  const setup = source.split('<script setup lang="ts">')[1].split('</script>')[0]
  run(`${setup}\nglobalThis.subject = { form, load, saveWelcome, applyPrimaryAdmin, ready, error }`, 'WebsiteConfigSection.vue')
  return {
    ...sandbox.subject, requests, writes, notices, drafts,
    get server() { return plain(server) },
    dispose() { for (const hook of unmounted) hook(); scope.stop() },
  }
}

for (const legacyKeys of [false, true]) {
  test(`primary admin fill is a snapshot; manual edits survive save and reload (${legacyKeys ? 'legacy' : 'current'} status keys)`, async t => {
    const primary = legacyKeys
      ? { ID: '1', IsAdmin: true, Username: '  站长  ', AvatarURL: 'https://old.example/api/images/primary.webp', description: '  站长简介  ' }
      : { id: 1, is_admin: true, username: '  站长  ', avatar_url: 'https://old.example/api/images/primary.webp', description: '  站长简介  ' }
    const users = [
      { id: 2, is_admin: true, username: '其他管理员', avatar_url: '/wrong-admin.svg' },
      { id: 1, is_admin: false, username: '非管理员', avatar_url: '/wrong-user.svg' },
      primary,
    ]
    const panel = website({
      settings: { pwaTitle: '已保存 PWA 标题' },
      status: { data: legacyKeys ? { Users: users } : { users } },
    })
    t.after(() => panel.dispose())
    await panel.load()
    await panel.applyPrimaryAdmin()
    assert.deepEqual(plain({
      welcomeName: panel.form.welcomeName,
      welcomeAvatarURL: panel.form.welcomeAvatarURL,
      welcomeDescription: panel.form.welcomeDescription,
      welcomeUseAdmin: panel.form.welcomeUseAdmin,
    }), {
      welcomeName: '站长',
      welcomeAvatarURL: '/api/images/primary.webp',
      welcomeDescription: '站长简介',
      welcomeUseAdmin: false,
    })
    assert.equal(panel.writes.length, 0, '填充只修改表单，不自动保存')
    assert.deepEqual(panel.requests.filter(request => request.url === '/api/status'), [
      { url: '/api/status', method: 'GET', credentials: 'include' },
    ])

    panel.form.welcomeName = '  手动显示名称  '
    panel.form.welcomeAvatarURL = '  /custom-avatar.svg  '
    panel.form.welcomeDescription = '  手动欢迎文案  '
    panel.form.pwaTitle = '未保存的 PWA 草稿'
    Object.assign(primary, { username: '后来修改的站长', Username: '后来修改的站长', avatar_url: '/later.svg', AvatarURL: '/later.svg', description: '后来修改的简介' })
    await panel.saveWelcome()
    const expected = {
      welcomeName: '手动显示名称',
      welcomeAvatarURL: '/custom-avatar.svg',
      welcomeDescription: '手动欢迎文案',
      welcomeUseAdmin: false,
    }
    assert.deepEqual(panel.writes, [{
      payload: { frontendSettings: expected },
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
    }], '保存只提交修剪后的欢迎字段及显式 false')
    await panel.load()
    for (const [field, value] of Object.entries(expected)) assert.equal(panel.form[field], value)
    assert.equal(panel.server.welcomeUseAdmin, false, '持久化配置不得重新跟随站长')
    assert.equal(panel.server.pwaTitle, '已保存 PWA 标题')
    assert.equal(panel.form.pwaTitle, '未保存的 PWA 草稿', '欢迎组件保存及刷新保留其他未保存字段')
    assert.equal(panel.drafts.get('website').value.pwaTitle, '未保存的 PWA 草稿')
    assert.equal(panel.requests.filter(request => request.url === '/api/status').length, 1, '保存及刷新不能重新拉取站长覆盖手动编辑')
  })
}

for (const legacyFlag of [true, 'true', 1, '1']) {
  test(`saving old welcomeUseAdmin=${JSON.stringify(legacyFlag)} clears the flag without requiring another fill`, async t => {
    const fields = {
      welcomeName: '原有欢迎名称',
      welcomeAvatarURL: '/original.svg',
      welcomeDescription: '原有欢迎文案',
    }
    const panel = website({ settings: { ...fields, welcomeUseAdmin: legacyFlag } })
    t.after(() => panel.dispose())
    await panel.saveWelcome()
    assert.equal(panel.writes.length, 0, '配置就绪前仍禁止保存')
    await panel.load()
    assert.equal(panel.ready.value, true)
    assert.equal(panel.form.welcomeUseAdmin, false, '旧标记不能改变加载后的表单语义')
    for (const [field, value] of Object.entries(fields)) assert.equal(panel.form[field], value)
    await panel.saveWelcome()
    assert.deepEqual(panel.writes[0].payload, { frontendSettings: { ...fields, welcomeUseAdmin: false } })
    assert.equal(panel.server.welcomeUseAdmin, false)
    assert.equal(panel.requests.some(request => request.url === '/api/status'), false)
  })
}

test('welcome save explicitly submits false even if a restored draft carries the old flag', async t => {
  const panel = website()
  t.after(() => panel.dispose())
  await panel.load()
  // Simulate an older in-memory draft: the write must not trust this flag.
  panel.form.welcomeUseAdmin = true
  await panel.saveWelcome()
  assert.deepEqual(panel.writes[0].payload, { frontendSettings: {
    welcomeName: '', welcomeAvatarURL: '', welcomeDescription: '', welcomeUseAdmin: false,
  } })
  assert.equal(panel.form.welcomeUseAdmin, false, '保存后的刷新应恢复一次性填充语义')
})

test('fill rejects other administrators and an ID1 account without administrator privileges', async t => {
  const panel = website({
    settings: { welcomeName: '保留名称', welcomeAvatarURL: '/keep.svg', welcomeDescription: '保留文案' },
    status: { data: { users: [
      { id: 2, is_admin: true, username: '其他管理员' },
      { id: 1, is_admin: false, username: '普通账户' },
    ] } },
  })
  t.after(() => panel.dispose())
  await panel.load()
  const before = plain(panel.form)
  await panel.applyPrimaryAdmin()
  assert.deepEqual(plain(panel.form), before, '找不到有效 PrimaryAdmin 时不得覆盖表单')
  assert.equal(panel.writes.length, 0)
  assert.equal(panel.notices.at(-1).description, '未找到有效站长账户')
})
