import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const tmp = await mkdtemp(join(tmpdir(), 'echo-noise-api-auth-'))
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
