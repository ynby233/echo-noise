import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'
import { nextTick, ref } from 'vue'

const source = await readFile(new URL('../utils/message-list-engagement.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText

const deferred = () => {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const flush = async () => {
  for (let i = 0; i < 12; i += 1) await Promise.resolve()
}
const makeMessages = count => Array.from({ length: count }, (_, index) => ({ id: index + 1, like_count: 0, liked: false }))

function harness({ mobile = false, observerAvailable = true, messages = makeMessages(3) } = {}) {
  let currentMessages = messages
  let elements = messages.map(message => ({ dataset: { msgId: String(message.id) } }))
  const root = { querySelectorAll: () => elements, contains: element => elements.includes(element) }
  const requests = [], observers = [], toasts = [], events = []
  const listeners = new Map(), timers = new Map()
  let nextTimer = 0
  const window = {
    matchMedia: () => ({ matches: mobile }),
    addEventListener: (name, listener) => {
      if (!listeners.has(name)) listeners.set(name, new Set())
      listeners.get(name).add(listener)
    },
    removeEventListener: (name, listener) => listeners.get(name)?.delete(listener),
    dispatchEvent: event => {
      events.push(event)
      for (const listener of listeners.get(event.type) || []) listener(event)
    },
    setTimeout: callback => { timers.set(++nextTimer, callback); return nextTimer },
    clearTimeout: timer => timers.delete(timer),
  }
  class Observer {
    observed = new Set()
    observeCalls = []
    unobserved = []
    disconnected = false
    constructor(callback) { this.callback = callback; observers.push(this) }
    observe(element) { this.observed.add(element); this.observeCalls.push(element) }
    unobserve(element) { this.observed.delete(element); this.unobserved.push(element) }
    disconnect() { this.disconnected = true; this.observed.clear() }
    // Permit delivery of already queued entries after unobserve/disconnect.
    emit(targets) { this.callback(targets.map(target => ({ target, isIntersecting: true }))) }
  }
  const postRequest = (url, body, options) => {
    const result = deferred()
    requests.push({ url, body, options, ...result })
    // Deliberately ignore abort, so generation checks must reject late results.
    return result.promise
  }
  const modules = {
    vue: { ref, nextTick },
    '#ui/composables/useToast': { useToast: () => ({ add: toast => toasts.push(toast) }) },
    './api': { postRequest },
  }
  const exports = {}
  new Function('require', 'exports', 'window', 'IntersectionObserver', compiled)(
    name => { assert.ok(name in modules, `unexpected import ${name}`); return modules[name] },
    exports, window, observerAvailable ? Observer : undefined,
  )
  const engagement = exports.createMessageListEngagement({
    root: () => root,
    messages: () => currentMessages,
    isGuestbook: message => !!message?.guestbook,
    isLoggedIn: () => true,
    canInteract: message => !!message,
  })
  return {
    engagement, requests, observers, toasts, events, timers, listeners, window,
    get elements() { return elements },
    setElements(value) { elements = value },
    setMessages(value) { currentMessages = value },
    resolveCounts(request, count = 2) {
      request.resolve({ code: 1, data: request.body.ids.map(id => ({ id, count })) })
    },
  }
}

// The first visible note joins the first full batch; no duplicate requests while pending.
{
  const h = harness({ messages: makeMessages(45) })
  h.engagement.mount()
  const observer = h.observers[0]
  h.engagement.update()
  assert.equal(observer.observeCalls.length, 45, 'updates must not observe an unchanged DOM twice')
  observer.emit(h.elements.slice(0, 1))
  observer.emit(h.elements.slice(1))
  assert.equal(h.requests.length, 0, 'scheduling defers dispatch until the current turn finishes')
  await flush()
  assert.deepEqual(h.requests[0].body.ids, Array.from({ length: 20 }, (_, i) => i + 1))
  h.engagement.update()
  observer.emit(h.elements)
  await flush()
  assert.equal(h.requests.length, 1, 'only one count batch may be in flight')
  h.resolveCounts(h.requests[0])
  await flush()
  assert.deepEqual(h.requests[1].body.ids, Array.from({ length: 20 }, (_, i) => i + 21))
  h.resolveCounts(h.requests[1])
  await flush()
  assert.deepEqual(h.requests[2].body.ids, [41, 42, 43, 44, 45])
  h.resolveCounts(h.requests[2], 0)
  await flush()
  assert.equal(h.engagement.expandedComments.value[1], true, 'positive counts still auto-expand')
  assert.equal(h.engagement.expandedComments.value[45], undefined, 'zero counts do not auto-expand')
  h.engagement.update()
  await flush()
  assert.equal(h.requests.length, 3)
  h.engagement.dispose()
}

// Mobile, absent IO, and a not-yet-rendered root all receive counts from current messages.
for (const config of [{ mobile: true }, { observerAvailable: false }]) {
  const h = harness({ ...config, messages: [...makeMessages(3), { id: 9, guestbook: true }] })
  h.setElements([])
  h.engagement.mount()
  assert.equal(h.observers.length, 0)
  await flush()
  assert.deepEqual(h.requests[0].body.ids, [1, 2, 3], 'guestbook counts are excluded')
  h.resolveCounts(h.requests[0])
  await flush()
  assert.equal(h.engagement.commentCounts.value[3], 2)
  h.engagement.dispose()
}

// A user's collapse wins over a pending positive summary; an explicit reopen still works.
{
  const h = harness({ mobile: true })
  h.engagement.mount()
  await flush()
  await h.engagement.toggleComments(1)
  await h.engagement.toggleComments(1)
  h.resolveCounts(h.requests[0])
  await flush()
  assert.equal(h.engagement.expandedComments.value[1], false)
  assert.equal(h.engagement.expandedComments.value[2], true)
  await h.engagement.toggleComments(1)
  assert.equal(h.engagement.expandedComments.value[1], true)
  h.engagement.dispose()
}

// A visible reply under a tombstone still opens the tree even when count is zero.
{
  const h = harness({ mobile: true })
  h.engagement.mount()
  await flush()
  await h.engagement.toggleComments(1)
  await h.engagement.toggleComments(1)
  h.requests[0].resolve({ code: 1, data: [
    { id: 1, count: 0, hasVisibleComments: true },
    { id: 2, count: 0, hasVisibleComments: true },
    { id: 3, count: 0, hasVisibleComments: false, hasHiddenComments: true },
  ] })
  await flush()
  assert.equal(h.engagement.expandedComments.value[1], false, 'explicit collapse also wins over visible tombstones')
  assert.equal(h.engagement.expandedComments.value[2], true, 'visible comment trees open without inflating the count')
  assert.equal(h.engagement.commentCounts.value[2], 0)
  assert.equal(h.engagement.expandedComments.value[3], undefined, 'hidden comments alone do not open a panel')
  h.engagement.dispose()
}

// Observation reconciles detached/recycled nodes and ignores queued entries from old DOM.
{
  const h = harness()
  h.engagement.mount()
  const observer = h.observers[0]
  const detached = h.elements[0]
  const replacement = { dataset: { msgId: '1' } }
  h.setElements([replacement, ...h.elements.slice(1)])
  h.engagement.update()
  assert.ok(observer.unobserved.includes(detached))
  assert.ok(observer.observed.has(replacement))
  observer.emit([detached])
  await flush()
  assert.equal(h.requests.length, 0)
  replacement.dataset.msgId = '4'
  h.setMessages([...makeMessages(3), { id: 4 }])
  h.engagement.update()
  assert.ok(observer.unobserved.includes(replacement), 'recycled nodes must be unobserved before re-registering')
  observer.emit([replacement])
  await flush()
  assert.deepEqual(h.requests[0].body.ids, [4])
  h.setMessages(makeMessages(3))
  h.setElements([])
  h.engagement.update()
  assert.equal(observer.observed.size, 0)
  h.resolveCounts(h.requests[0])
  await flush()
  assert.equal(h.engagement.commentCounts.value[4], undefined, 'removed messages reject pending summaries')
  h.engagement.dispose()
  assert.equal(observer.disconnected, true)
}

// Reset cancels old work and must not let an old finally unlock the new viewer's batch.
{
  const h = harness({ mobile: true, messages: makeMessages(21) })
  h.engagement.mount()
  await flush()
  const oldCounts = h.requests[0]
  const oldLike = h.engagement.like(1)
  const oldLikeRequest = h.requests[1]
  await h.engagement.toggleComments(1)
  h.engagement.resetViewer()
  assert.equal(oldCounts.options.signal.aborted, true)
  assert.equal(oldLikeRequest.options.signal.aborted, true)
  for (const key of ['likes', 'liked', 'commentCounts', 'expandedComments', 'commentRefreshKey']) {
    assert.deepEqual(h.engagement[key].value, {}, `${key} is viewer-owned`)
  }
  assert.equal(h.engagement.activeCommentId.value, null)
  h.engagement.update()
  await flush()
  assert.equal(h.requests.length, 3, 'new viewer starts without waiting for aborted requests')
  const newCounts = h.requests[2]
  h.resolveCounts(oldCounts, 99)
  oldLikeRequest.resolve({ code: 1, data: { like_count: 99, liked: true } })
  await oldLike
  await flush()
  assert.equal(h.requests.length, 3, 'old finally must not start a concurrent new batch')
  assert.deepEqual(h.engagement.commentCounts.value, {})
  assert.equal(h.engagement.liked.value[1], false)
  h.resolveCounts(newCounts, 4)
  await flush()
  assert.equal(h.requests.length, 4)
  h.resolveCounts(h.requests[3], 4)
  await flush()
  assert.equal(h.engagement.commentCounts.value[1], 4)
  assert.equal(h.engagement.expandedComments.value[1], true, 'old viewer collapse does not carry across reset')
  h.engagement.dispose()
}

// Reset/dispose discard scheduled microtasks and old observer callbacks.
{
  const h = harness()
  h.engagement.mount()
  const oldObserver = h.observers[0]
  oldObserver.emit(h.elements)
  h.engagement.resetViewer()
  assert.equal(oldObserver.disconnected, true)
  oldObserver.emit(h.elements)
  await flush()
  assert.equal(h.requests.length, 0)
  h.engagement.update()
  const newObserver = h.observers[1]
  newObserver.emit(h.elements)
  h.engagement.dispose()
  await flush()
  assert.equal(h.requests.length, 0)
  assert.equal(newObserver.disconnected, true)
  assert.equal(h.listeners.get('comment-count-updated').size, 0)
  h.engagement.update()
  assert.equal(h.observers.length, 2, 'late updates after disposal must do nothing')
  h.engagement.mount()
  h.observers[2].emit(h.elements)
  await flush()
  assert.equal(h.requests.length, 1, 'remount has a fresh queue and observer')
  const pendingLike = h.engagement.like(1)
  h.engagement.dispose()
  assert.equal(h.requests[0].options.signal.aborted, true)
  assert.equal(h.requests[1].options.signal.aborted, true)
  h.resolveCounts(h.requests[0], 99)
  h.requests[1].reject(new Error('late failure'))
  await pendingLike
  await flush()
  assert.deepEqual(h.engagement.commentCounts.value, {})
  assert.equal(h.toasts.length, 0, 'late request failures must not toast after disposal')
}

// Like completion order is isolated both within a viewer and across viewer changes.
{
  const h = harness()
  h.engagement.mount()
  const first = h.engagement.like(1)
  const second = h.engagement.like(1)
  assert.equal(h.requests[0].options.signal.aborted, true)
  h.requests[1].resolve({ code: 1, data: { like_count: 2, liked: false } })
  await second
  h.requests[0].resolve({ code: 1, data: { like_count: 3, liked: true } })
  await first
  assert.equal(h.engagement.likes.value[1], 2)
  assert.equal(h.engagement.liked.value[1], false)
  const old = h.engagement.like(1)
  h.engagement.resetViewer()
  h.requests[2].reject(new Error('old viewer failure'))
  await old
  assert.equal(h.toasts.length, 0)
  h.engagement.update()
  const current = h.engagement.like(1)
  h.requests[3].resolve({ code: 0, msg: 'current failure' })
  await current
  assert.equal(h.toasts[0].title, 'current failure', 'current errors still surface')
  h.engagement.dispose()
}

// A newer comment event wins over an earlier summary; failures remain retryable.
{
  const h = harness({ mobile: true })
  h.engagement.mount()
  await flush()
  h.window.dispatchEvent({ type: 'comment-count-updated', detail: { messageId: 1, count: 5 } })
  h.window.dispatchEvent({ type: 'comment-count-updated', detail: { messageId: 99, count: 5 } })
  h.resolveCounts(h.requests[0], 1)
  await flush()
  assert.equal(h.engagement.commentCounts.value[1], 5)
  assert.equal(h.engagement.commentCounts.value[99], undefined)
  h.engagement.resetViewer()
  h.engagement.update()
  await flush()
  h.requests[1].reject(new Error('network error'))
  await flush()
  h.engagement.update()
  await flush()
  assert.equal(h.requests.length, 3)
  h.resolveCounts(h.requests[2])
  await flush()
  h.engagement.dispose()
}

// Focus waits and nextTick refreshes cannot survive a viewer reset.
{
  const h = harness()
  h.engagement.mount()
  const focus = h.engagement.focusComment(1, 7)
  assert.equal(h.timers.size, 1)
  const toggle = h.engagement.toggleComments(1)
  h.engagement.resetViewer()
  assert.equal(h.timers.size, 0)
  assert.equal(await focus, false)
  await toggle
  assert.equal(h.events.length, 0)
  const result = deferred()
  h.engagement.commentsRefFor(1)({ focusCommentById: () => result.promise })
  const staleFocus = h.engagement.focusComment(1, 7)
  h.engagement.resetViewer()
  result.resolve(true)
  assert.equal(await staleFocus, false)
  h.engagement.dispose()
}

console.log('message list engagement behavior tests passed')
