import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'
import { stripTypeScriptTypes } from 'node:module'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const tmp = await mkdtemp(join(tmpdir(), 'echo-site-api-auth-'))
const outfile = join(tmp, 'api-bundle.mjs')

await build({
  entryPoints: [join(root, 'utils/api.ts')],
  outfile,
  bundle: true,
  format: 'esm',
  platform: 'node',
  target: 'node18',
  banner: {
    js: 'const useRuntimeConfig = (...args) => globalThis.__useRuntimeConfig(...args); const $fetch = (...args) => globalThis.__fetch(...args);'
  },
  plugins: [{
    name: 'nuxt-test-stubs',
    setup(build) {
      build.onResolve({ filter: /^~\/store\/user$/ }, () => ({ path: 'store-user', namespace: 'stub' }))
      build.onResolve({ filter: /^#imports$/ }, () => ({ path: 'imports', namespace: 'stub' }))
      build.onLoad({ filter: /.*/, namespace: 'stub' }, ({ path }) => {
        if (path === 'store-user') {
          return { contents: 'export const useUserStore = () => globalThis.__userStore', loader: 'js' }
        }
        if (path === 'imports') {
          return { contents: 'export const useToast = () => globalThis.__toast', loader: 'js' }
        }
        return null
      })
    }
  }]
})

const api = await import(pathToFileURL(outfile).href)

const makeWindow = () => ({
  matchMedia: () => ({ matches: false }),
  location: { pathname: '/messages', search: '?page=1', href: '' }
})

const reset = () => {
  globalThis.__events = { toasts: [], cleared: false }
  globalThis.__useRuntimeConfig = () => ({ public: { baseApi: '/api' } })
  globalThis.__toast = { add: (event) => globalThis.__events.toasts.push(event) }
  globalThis.__userStore = {
    isLogin: true,
    token: 'valid-token',
    clearUserStatus() {
      globalThis.__events.cleared = true
      this.isLogin = false
      this.token = ''
    }
  }
  globalThis.window = makeWindow()
}

try {
  reset()
  const timeouts = []
  globalThis.__fetch = async (_url, options) => { timeouts.push([options.timeout, options.retry]); return { code: 1, data: {} } }
  await api.getRequest('updates', undefined, { silent: true, timeout: 60000 })
  await api.postRequest('updates/tasks', { channel: 'edge' }, { silent: true, timeout: 60000 })
  await api.getRequest('updates/state', undefined, { silent: true })
  assert.deepEqual(timeouts, [[60000, 0], [60000, 0], [8000, 0]], 'remote discovery gets enough time; no automatic POST retries; local polling remains bounded')

  for (const status of [409, 412]) {
    reset()
    globalThis.__fetch = async () => {
      const error = new Error('Rejected')
      error.response = { status, _data: { msg: '请重新检查并确认' } }
      throw error
    }
    const rejected = await api.postRequest('updates/tasks', {}, { silent: true })
    assert.equal(rejected.status, status, 'known rejection must remain distinguishable from a lost response')
    assert.equal(rejected.msg, '请重新检查并确认')
  }

  const profile = await readFile(join(root, 'components/admin/sections/ProfileSection.vue'), 'utf8')
  const saveBody = profile.match(/const updateCommonTags = async \(\) => \{([\s\S]*?)\n\}/)?.[1]
  assert.ok(saveBody)
  const saveScript = stripTypeScriptTypes(`async function save() {${saveBody}\n}`)
  const createSave = new Function('userStore', 'commonTagsText', 'commonTagsSaving', 'putRequest', 'toast',
    `const commonTagsAccountVersion = 0; const commonTagsDraftVersion = 0; ${saveScript}; return save`)
  for (const httpError of [false, true]) {
    reset()
    const message = '常用标签不能包含空格、标点或符号'
    globalThis.__fetch = async () => {
      if (httpError) {
        const error = new Error('Bad Request')
        error.response = { status: 400, _data: { msg: message } }
        throw error
      }
      return { code: 0, msg: message, data: null }
    }
    const text = { value: 'bad tag，工作\n阅读!' }
    const saving = { value: false }
    const save = createSave({ user: { userid: 2, commonTags: ['old'] } }, text, saving, api.putRequest, globalThis.__toast)
    await save()
    assert.equal(globalThis.__events.toasts.length, 1, 'actual profile + API must show exactly one validation failure')
    assert.equal(globalThis.__events.toasts[0].description, message)
    assert.equal(text.value, 'bad tag，工作\n阅读!', 'validation errors must preserve the draft')
    assert.equal(saving.value, false)
  }

  reset()
  globalThis.__fetch = async () => ({ code: 0, msg: '常用标签不能包含空格、标点或符号', data: null })
  const invalidTags = await api.putRequest('user/update', { commonTags: ['bad tag'] }, { silent: true })
  assert.equal(invalidTags.msg, '常用标签不能包含空格、标点或符号', 'silent PUT must preserve the validation message for the form')
  assert.equal(globalThis.__events.toasts.length, 0, 'silent validation failure must not add a second request toast')

  reset()
  globalThis.__fetch = async () => {
    const error = new Error('Bad Request')
    error.response = { status: 400, _data: { msg: '每个常用标签最多 64 个字符' } }
    throw error
  }
  const invalidLength = await api.putRequest('user/update', {}, { silent: true })
  assert.equal(invalidLength.msg, '每个常用标签最多 64 个字符', 'HTTP failures must also retain the server validation message')
  assert.equal(globalThis.__events.toasts.length, 0)

  reset()
  globalThis.__fetch = async () => ({ code: 0, msg: '原有调用错误', data: null })
  assert.equal(await api.putRequest('other/update', {}), null, 'non-silent callers keep their existing failure contract')
  assert.equal(globalThis.__events.toasts.length, 1)

  reset()
  globalThis.__fetch = async () => {
    const err = new Error('Forbidden')
    err.status = 403
    err.response = { status: 403, statusText: 'Forbidden', _data: { msg: '无权限回复该内容' } }
    throw err
  }
  const forbidden = await api.postRequest('messages/1/comments', { content: 'reply' }, { credentials: 'include' })
  assert.equal(forbidden.code, 0)
  assert.equal(forbidden.msg, '无权限回复该内容')
  assert.equal(globalThis.__events.cleared, false, '403 权限错误不能清除当前登录状态')
  assert.equal(globalThis.window.location.href, '', '403 权限错误不能跳转到登录过期引导页')
  assert.ok(
    globalThis.__events.toasts.some((event) => event.title === '请求失败' || event.title === '没有权限'),
    '403 权限错误应该只给出权限/请求失败提示'
  )

  reset()
  globalThis.__fetch = async () => {
    const err = new Error('Unauthorized')
    err.status = 401
    err.response = { status: 401, statusText: 'Unauthorized', _data: { msg: '未登录或登录已过期' } }
    throw err
  }
  const unauthorized = await api.getRequest('settings', undefined, { credentials: 'include' })
  assert.equal(unauthorized.code, 0)
  assert.equal(globalThis.__events.cleared, true, '401 才应清除登录状态')
  assert.match(globalThis.window.location.href, /\/auth\/guide\?reason=expired/, '401 应跳转到登录过期引导页')

  console.log('api auth handling tests passed')
} finally {
  delete globalThis.window
  await rm(tmp, { recursive: true, force: true })
}
