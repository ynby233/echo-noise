import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { reactive, computed, watch, effectScope, nextTick } from 'vue'
import { createPinia, defineStore, setActivePinia } from 'pinia'
import { transformSync } from 'esbuild'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const read = name => fs.readFileSync(path.join(root, name), 'utf8')
const source = transformSync(read('utils/author-subscriptions.ts'), { loader: 'ts', format: 'cjs' }).code
const module = { exports: {} }
new Function('module', 'exports', source)(module, module.exports)
const { createAuthorSubscriptions, validAuthorId } = module.exports
for (const value of [undefined, null, '', 0, -1, 1.5, NaN, Infinity, true, {}, '1.1', '1x', ' 1 ', Number.MAX_SAFE_INTEGER + 1]) {
  assert.equal(validAuthorId(value), 0, `invalid author ID: ${String(value)}`)
}
assert.equal(validAuthorId('15'), 15)
const entries = reactive({})
const calls = []
const request = (method, authorId) => new Promise((resolve, reject) => calls.push({ method, authorId, resolve, reject }))
const subscriptions = createAuthorSubscriptions(entries, request)
const session = (userId, token = 'member') => subscriptions.setSession({ isLogin: true, userId, token })
const response = (author_id, subscribed) => ({ code: 1, data: { author_id, subscribed } })
subscriptions.setSession({ isLogin: false, userId: 3, token: '' })
await subscriptions.load(8)
await subscriptions.toggle(8)
assert.equal(subscriptions.entry(8), undefined, 'guest has no subscription state or requests')
assert.equal(calls.length, 0)
session(3)
await subscriptions.load(3)
await subscriptions.toggle(3)
assert.equal(calls.length, 0, 'self subscription is forbidden in the frontend')
for (const value of [0, undefined, -1, 'oops']) await subscriptions.load(value)
assert.equal(calls.length, 0)
const first = subscriptions.load(8)
const second = subscriptions.load(8)
assert.equal(first, second, 'cards share one inflight GET')
assert.equal(calls.length, 1)
assert.equal(entries[8].busy, true)
calls.shift().resolve(response(8, false))
await first
const cardA = subscriptions.entry(8)
const cardB = subscriptions.entry(8)
assert.equal(cardA, cardB, 'same author has one reactive state')
assert.equal(cardA.ready, true)
assert.equal(cardA.busy, false)
await subscriptions.load(8)
assert.equal(calls.length, 0, 'loaded author is cached within a session')
const create = subscriptions.toggle(8)
assert.equal(subscriptions.toggle(8), create, 'duplicate clicks cannot create duplicate writes')
assert.equal(calls.length, 1)
assert.equal(calls[0].method, 'PUT')
assert.equal(cardA.subscribed, false, 'pending create is not optimistically successful')
calls.shift().resolve(response(8, true))
await create
assert.equal(cardB.subscribed, true, 'all cards reflect confirmed creation')
const cancel = subscriptions.toggle(8)
assert.equal(calls[0].method, 'DELETE')
assert.equal(cardB.subscribed, true, 'pending cancellation preserves confirmed state')
calls.shift().resolve({ code: 0, msg: '权限不足' })
await cancel
assert.equal(cardA.subscribed, true)
assert.equal(cardA.ready, false)
assert.equal(cardA.error, '权限不足')
await subscriptions.toggle(8)
assert.equal(calls.length, 0, 'failed state disables writes until GET retry')
const retry = subscriptions.load(8)
assert.equal(calls[0].method, 'GET')
calls.shift().resolve(response(8, true))
await retry
assert.equal(cardA.error, '')
const cancelled = subscriptions.toggle(8)
calls.shift().resolve(response(8, false))
await cancelled
assert.equal(cardB.subscribed, false)
const loadError = subscriptions.load(9)
calls.shift().reject(new Error('网络断开'))
await loadError
assert.equal(entries[9].ready, false)
assert.equal(entries[9].busy, false)
assert.equal(entries[9].error, '网络断开')
const malformed = subscriptions.load(10)
calls.shift().resolve(response(11, true))
await malformed
assert.equal(entries[10].ready, false, 'wrong-author response cannot succeed')
const staleRead = subscriptions.load(12)
const oldRead = calls.shift()
session(4, 'admin')
assert.equal(Object.keys(entries).length, 0, 'account change clears cached author statuses')
const freshRead = subscriptions.load(12)
const newRead = calls.shift()
oldRead.resolve(response(12, true))
await staleRead
assert.equal(entries[12].busy, true, 'old completion cannot settle new account request')
newRead.resolve(response(12, false))
await freshRead
assert.equal(entries[12].subscribed, false)
const staleWrite = subscriptions.toggle(12)
const oldWrite = calls.shift()
subscriptions.setSession({ isLogin: false, userId: null, token: '' })
assert.equal(Object.keys(entries).length, 0)
oldWrite.resolve(response(12, true))
await staleWrite
assert.equal(Object.keys(entries).length, 0, 'logout ignores late mutation results')
session(4, 'admin-new-session')
const adminRead = subscriptions.load(12)
calls.shift().resolve(response(12, true))
await adminRead
assert.equal(entries[12].subscribed, true, 'administrators use the same subscription flow')
session(4, 'rotated-token')
assert.equal(Object.keys(entries).length, 0, 'session token replacement clears cached statuses')

// Exercise the actual Pinia wrapper and its store-owned synchronous watcher.
const user = reactive({ isLogin: true, user: { userid: 3, is_admin: false }, token: 'a' })
setActivePinia(createPinia())
const apiCalls = []
const getRequest = (url, body, options) => new Promise(resolve => apiCalls.push({ url, body, options, resolve }))
const storeSource = transformSync(read('store/author-subscriptions.ts'), { loader: 'ts', format: 'esm' }).code
  .replace(/^import .*\r?\n/gm, '').replace(/export\s*\{[\s\S]*?\};?\s*$/, '')
const useStore = new Function('defineStore', 'computed', 'reactive', 'watch', 'useUserStore', 'getRequest', 'putRequest', 'deleteRequest', 'createAuthorSubscriptions', `${storeSource}\nreturn useAuthorSubscriptionStore`)(defineStore, computed, reactive, watch, () => user, getRequest, getRequest, getRequest, createAuthorSubscriptions)
const scope = effectScope()
const store = scope.run(() => useStore())
const cardVisible = computed(() => { void store.sessionKey; return store.canSubscribe(8) })
const loading = store.load(8)
assert.equal(apiCalls[0].url, 'authors/8/subscription')
assert.equal(apiCalls[0].options.credentials, 'include')
assert.equal(apiCalls[0].body, undefined, 'recipient identity never enters request payload')
scope.stop()
user.isLogin = false
assert.equal(cardVisible.value, false)
assert.equal(Object.keys(store.entries).length, 0, 'store watcher survives the first consuming component scope')
apiCalls.shift().resolve(response(8, true))
await loading
assert.equal(Object.keys(store.entries).length, 0)
user.user = { userid: 4, is_admin: true }
user.isLogin = true
await nextTick()
assert.equal(cardVisible.value, true)
store.$dispose()

const card = read('components/index/AuthorCard.vue')
const header = card.slice(card.indexOf('<div class="author-card-header">'), card.indexOf('<p class="author-card-description">'))
assert.match(header, /笔记 \{\{ count \}\}/, 'count is in the header')
assert.doesNotMatch(card, /<p class="author-card-count">/, 'count is not duplicated in footer')
assert.match(header, /nw-action-btn nw-action-btn--label/)
assert.match(header, /nw-action-btn--primary.*subscription\?\.subscribed/)
assert.match(header, /v-if="canSubscribe"/)
assert.match(header, /:disabled="!subscription\?\.ready \|\| subscription.busy \|\| !!subscription.error"/)
assert.match(card, />重试<\/button>/)
assert.match(card, /if \(opened.value && canSubscribe.value\) void subscriptions.load/)
const list = read('components/index/MessageList.vue')
assert.match(list, /:author-id="authorId\(msg\)"/)
assert.match(list, /generation === authorProfileGeneration/)
assert.match(list, /authorProfiles.value = \{\}/)
console.log('author subscription state and frontend integration tests passed')
