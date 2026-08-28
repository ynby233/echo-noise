import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), 'utf8')
const [authorization, comments, messageList, personal, manager, routes] = await Promise.all([
  read('components/admin/AuthorizationCenter.vue'),
  read('components/comments/BuiltinComments.vue'),
  read('components/index/MessageList.vue'),
  read('components/index/PersonalContentManager.vue'),
  read('components/admin/CommentManager.vue'),
  read('../internal/routers/routers.go'),
])

assert.match(authorization, /flattenCapabilityTree/, 'authorization center must flatten every capability depth for rendering')
assert.match(authorization, /capabilityDepth/, 'authorization center must visually preserve nested capability depth')
assert.doesNotMatch(authorization, /v-for="item in childrenFor\(parent\.capability\)"/, 'authorization center must not stop after one child level')

assert.match(comments, /messageOwnerId/, 'comment actions need the owning note user id')
assert.match(comments, /isMessageOwner/, 'note owners must receive interaction cleanup controls')
assert.match(comments, /ownsCommentAncestor/, 'comment and reply owners must receive descendant cleanup controls')
assert.match(messageList, /:message-owner-id="msg\.user_id"/, 'message list must pass note ownership into comments')

for (const token of ['selected', 'toggleAll', 'batchTrashNotes', 'batchTrashInteractions', 'batchRestore', 'batchPurgeInteractions']) {
  assert.ok(personal.includes(token), `personal note/interaction surfaces must implement ${token}`)
}
for (const token of ['selected', 'toggleAll', 'batchTrash', 'batchRestore', 'batchPermanentDelete']) {
  assert.ok(manager.includes(token), `admin interaction surfaces must implement ${token}`)
}
for (const endpoint of [
  '/user/notes/batch-trash',
  '/user/interactions/batch-trash',
  '/user/recycle-bin/notes/batch-restore',
  '/user/recycle-bin/comments/batch-restore',
  '/user/recycle-bin/comments/batch-purge',
  '/admin/comments/batch-trash',
  '/batch-restore',
  '/batch-permanent-delete',
]) {
  assert.ok(routes.includes(endpoint), `router must expose ${endpoint}`)
}

console.log('workline C completion contract checks passed')
