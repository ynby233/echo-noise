import assert from 'node:assert/strict'
import { createJiti } from 'jiti'

const { createMessageTargetNavigation } = await createJiti(import.meta.url).import('../utils/message-target-navigation.ts')
let resolveOld
let target = 1
let consumed = 0
const navigation = createMessageTargetNavigation({
  root: () => null,
  targetMessageId: () => target,
  targetCommentId: () => 0,
  ready: () => true,
  loadMessage: id => id === 1 ? new Promise(resolve => { resolveOld = resolve }) : Promise.resolve(true),
  expandComments() {},
  focusComment: async () => false,
  consume: () => { consumed += 1 },
})
navigation.mount()
const oldRequest = navigation.focus()
target = 2
await navigation.focus()
assert.equal(consumed, 1)
resolveOld(true)
await oldRequest
assert.equal(consumed, 1, 'an older navigation must not consume a newer notification target')

target = 1
const disposedRequest = navigation.focus()
navigation.dispose()
resolveOld(false)
await disposedRequest
assert.equal(consumed, 1, 'a disposed navigation must not emit target-consumed')
console.log('message target stale-navigation tests passed')
