import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { stripTypeScriptTypes } from 'node:module'
import vm from 'node:vm'
import { effectScope, reactive, ref, watch } from 'vue'

const source = await readFile(new URL('../store/message.ts', import.meta.url), 'utf8')
const pending = []
const toasts = []
const errors = []
const user = reactive({ token: 'session-a', isLogin: true, user: { userid: 1, username: 'alice', is_admin: false } })
const context = vm.createContext({
  ref,
  watch,
  defineStore: (_, setup) => setup,
  useUserStore: () => user,
  useToast: () => ({ add: value => toasts.push(value) }),
  // Deliberately ignore abort: guards must reject late responses even if transport cancellation loses the race.
  postRequest: (url, query, options) => new Promise((resolve, reject) => pending.push({ url, query, options, resolve, reject })),
  AbortController,
  console: { error: (...args) => errors.push(args) },
})
vm.runInContext(stripTypeScriptTypes(source.replace(/^import .*;?\r?\n/gm, '').replace('export const useMessageStore', 'var useMessageStore')), context)
const scope = effectScope()
const store = scope.run(() => context.useMessageStore())
const query = page => ({ page, pageSize: 2 })
const respond = (items, total = 5) => pending.shift().resolve({ code: 1, data: { items, total } })
const ids = () => Array.from(store.messages.value, item => item.id)
let load = store.loadMessagePage(query(1)); respond([{ id: 1 }, { id: 2 }]); await load
load = store.loadMessagePage(query(2), { append: true }); respond([{ id: 2 }, { id: 3 }, { id: 3 }]); await load
assert.deepEqual(ids(), [1, 2, 3], '追加应去除与旧列表和同一响应内的重复项')
const stale = store.loadMessagePage(query(3), { append: true })
const oldSignal = pending[0].options.signal
const refreshed = store.loadMessagePage({ ...query(1), tag: '新筛选' })
assert.equal(oldSignal.aborted, true)
respond([{ id: 4 }]); assert.equal(await stale, null)
assert.equal(store.loading.value, true, '旧请求 finally 不得结束当前请求的 loading')
respond([{ id: 20 }, { id: 21 }], 3); await refreshed
assert.deepEqual(ids(), [20, 21], '筛选变化后旧请求不得覆盖新结果')
load = store.loadMessagePage({ ...query(2), tag: '新筛选' }, { append: true }); pending.shift().resolve({ code: 0 }); await load
assert.deepEqual(ids(), [20, 21], '下一段失败应保留已有内容')
assert.equal(store.page.value, 1, '下一段失败不得推进页码')
load = store.loadMessagePage({ ...query(2), tag: '新筛选' }, { append: true }); respond([{ id: 22 }], 3); await load
assert.deepEqual(ids(), [20, 21, 22])
assert.equal(store.hasMore.value, false, '最后一段后停止加载')
load = store.loadMessagePage(query(1)); respond([{ id: 1 }, { id: 2 }]); await load
assert.deepEqual(ids(), [1, 2], '普通分页和刷新仍然替换列表')

for (const filter of [{ tag: 'other' }, { keyword: 'search' }, { authorId: 2 }, { username: 'bob' }, { date: '2026-11-03' }, { pinScope: 'personal' }, { excludeId: 4 }, { pageSize: 3 }]) {
  load = store.loadMessagePage(query(1)); respond([{ id: 1 }, { id: 2 }]); await load
  const changedQuery = { ...query(2), ...filter }
  load = store.loadMessagePage(changedQuery, { append: true }); respond([{ id: 9 }]); await load
  assert.deepEqual(ids(), [9], `append 遇到筛选变化应替换：${JSON.stringify(filter)}`)
  assert.equal(store.currentListQueryKey.value, store.listQueryKey(changedQuery))
}

// Legacy entry keeps first-page replacement, subsequent append and count-based hasMore.
load = store.getMessages(query(1)); respond([{ id: 1 }, { id: 2 }], 4); await load
load = store.getMessages(query(2)); respond([{ id: 2 }, { id: 3 }, { id: 3 }], 4); await load
assert.deepEqual(ids(), [1, 2, 3])
assert.equal(store.hasMore.value, true, 'getMessages 必须沿用已加载条数判断 hasMore')
load = store.getMessages({ ...query(3), tag: 'replacement' }); respond([{ id: 8 }], 1); await load
assert.deepEqual(ids(), [8], '旧入口也不能跨筛选追加')
load = store.getMessages(query(1)); respond([{ id: 7 }], 1); await load
assert.deepEqual(ids(), [7])
assert.equal(store.hasMore.value, false)

for (const method of ['getMessages', 'loadMessagePage', 'locateMessagePage']) {
  const before = ids()
  const request = store[method]({ ...query(2), messageId: 88 })
  const call = pending.shift()
  store.cancelRequests()
  assert.equal(call.options.signal.aborted, true, `${method} 必须取消网络请求`)
  assert.equal(store.loading.value, false, '取消立即结束 loading')
  assert.deepEqual(ids(), before, 'cancelRequests 保留已显示列表')
  call.resolve({ code: 1, data: { items: [{ id: 88 }], total: 90, page: 8 } })
  assert.equal(await request, null, '取消后即使成功返回也不得接受')
  assert.deepEqual(ids(), before)
}

// Both directions share a generation; a late old locate response must be rejected after a newer request.
for (const [first, second] of [['locateMessagePage', 'locateMessagePage'], ['locateMessagePage', 'loadMessagePage'], ['loadMessagePage', 'locateMessagePage'], ['getMessages', 'loadMessagePage']]) {
  const older = store[first]({ ...query(1), messageId: 10 })
  const olderCall = pending.shift()
  const newer = store[second]({ ...query(1), messageId: 20 })
  const newerCall = pending.shift()
  assert.equal(olderCall.options.signal.aborted, true)
  const result = { items: [{ id: 20 }], total: 1, page: 1, pageSize: 2, messageId: 20 }
  newerCall.resolve({ code: 1, data: result })
  assert.equal(await newer, result)
  olderCall.resolve({ code: 1, data: { items: [{ id: 10 }], total: 1, page: 10 } })
  assert.equal(await older, null, `${first} 的旧结果不能越过 ${second}`)
  assert.equal(store.loading.value, false)
}

// Real Vue sync watchers: identity changes must clear state before the next tick, even A -> B -> A.
for (const changeIdentity of [
  () => { user.token = 'session-b' },
  () => { user.user = { userid: 2, username: 'bob', is_admin: false } },
  () => { user.user.userid = 3 },
  () => { user.user.is_admin = true },
  () => { user.user = null; user.token = ''; user.isLogin = false },
  () => { user.user = { userid: 4, username: 'carol', is_admin: false }; user.token = 'session-c'; user.isLogin = true },
  () => { const token = user.token; user.token = 'temporary'; user.token = token },
]) {
  load = store.loadMessagePage(query(2)); respond([{ id: 5 }], 9); await load
  const request = store.loadMessagePage(query(3), { append: true })
  const call = pending.shift()
  changeIdentity()
  assert.deepEqual(ids(), [], '身份变化须同步清空旧身份列表')
  assert.equal(store.total.value, 0)
  assert.equal(store.page.value, 1)
  assert.equal(store.hasMore.value, true)
  assert.equal(store.currentListQueryKey.value, '')
  assert.equal(store.loading.value, false)
  assert.equal(call.options.signal.aborted, true)
  call.resolve({ code: 1, data: { items: [{ id: 6 }], total: 9 } })
  assert.equal(await request, null)
  assert.deepEqual(ids(), [])
}
const identityLocate = store.locateMessagePage({ ...query(1), messageId: 30 })
const identityLocateCall = pending.shift()
user.token = 'session-d'
assert.equal(identityLocateCall.options.signal.aborted, true)
identityLocateCall.resolve({ code: 1, data: { messageId: 30, page: 4, total: 9 } })
assert.equal(await identityLocate, null)

// Profile refresh for the same identity must not cancel pagination.
load = store.loadMessagePage(query(1))
const profileCall = pending.shift()
user.user = { ...user.user, avatar_url: '/new-avatar.png', total_messages: 99 }
assert.equal(profileCall.options.signal.aborted, false)
profileCall.resolve({ code: 1, data: { items: [{ id: 31 }], total: 1 } })
assert.ok(await load)

for (const method of ['getMessages', 'loadMessagePage', 'locateMessagePage']) {
  const request = store[method]({ ...query(2), messageId: 32 })
  const call = pending.shift()
  store.reset()
  assert.equal(call.options.signal.aborted, true)
  call.reject(new Error('late transport failure'))
  assert.equal(await request, null)
  assert.deepEqual(ids(), [])
  assert.equal(store.loading.value, false)
}
assert.deepEqual(toasts, [], '取消和过期请求不应产生错误提示')
assert.deepEqual(errors, [], '取消和过期请求不应产生错误日志')
assert.equal('prefetchPage' in store, false)
assert.equal('applyPrefetchedOrLoad' in store, false)
assert.equal(pending.length, 0)
scope.stop()
console.log('continuous message loading, cancellation and identity tests passed')
