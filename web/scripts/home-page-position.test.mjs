import assert from 'node:assert/strict'
import { parseHomePagePosition, readReloadPagePosition, savePagePosition } from '../utils/home-page-position.ts'

for (const tab of ['latest', 'personal', 'feed']) {
  assert.deepEqual(parseHomePagePosition(JSON.stringify({ tab, page: 3 })), { tab, page: 3 })
}
for (const raw of [null, '{', '{}', '{"tab":"latest","page":0}', '{"tab":"feed","page":1.5}', '{"tab":"about","page":2}']) {
  assert.equal(parseHomePagePosition(raw), null)
}
const values = new Map()
globalThis.sessionStorage = { getItem: key => values.get(key), setItem: (key, value) => values.set(key, value), removeItem: key => values.delete(key) }
globalThis.window = { location: { href: 'https://example.com/' } }
let navigationType = 'reload'
Object.defineProperty(globalThis, 'performance', { value: { getEntriesByType: () => [{ type: navigationType }] }, configurable: true })
savePagePosition({ tab: 'personal', page: 4 })
assert.deepEqual(readReloadPagePosition(), { tab: 'personal', page: 4 })
navigationType = 'navigate'
assert.equal(readReloadPagePosition(), null, 'ordinary navigation must not reopen an old page')
navigationType = 'reload'
for (const suffix of ['#message-1', '?message_id=1', '?notification_id=1', '?tab=feed']) {
  window.location.href = 'https://example.com/' + suffix
  assert.equal(readReloadPagePosition(), null, 'explicit navigation takes precedence')
}
window.location.href = 'https://example.com/'
savePagePosition(null)
assert.equal(readReloadPagePosition(), null, 'masonry or filtered browsing clears the position')
console.log('Home page reload position checks passed')
