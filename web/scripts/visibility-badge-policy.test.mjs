import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const policyPath = fileURLToPath(new URL('../utils/visibility-badge.ts', import.meta.url))
const messageListPath = fileURLToPath(new URL('../components/index/MessageList.vue', import.meta.url))
const commentsPath = fileURLToPath(new URL('../components/comments/BuiltinComments.vue', import.meta.url))
const [policySource, messageList, comments] = await Promise.all([
  readFile(policyPath, 'utf8'),
  readFile(messageListPath, 'utf8'),
  readFile(commentsPath, 'utf8'),
])

assert.match(policySource, /export const shouldShowVisibilityBadge/, 'visibility badge policy must be shared')
assert.match(policySource, /if \(isAdmin\) return true/, 'admins must see every visibility badge')
assert.match(policySource, /if \(normalizedVisibility === 'public'\) return true/, 'public badges must remain visible to every viewer')
assert.match(policySource, /return isAuthenticated && isOwner/, 'ordinary users must only gain non-public badge access for their own content')

assert.match(messageList, /v-if="shouldShowMessageVisibility\(msg\)"[^>]*class="visibility-indicator/, 'note badges must use the shared viewer policy')
assert.match(messageList, /shouldShowVisibilityBadge\(\{[\s\S]*?isAdmin:\s*currentUserIsAdmin\.value[\s\S]*?isAuthenticated:\s*isLogin\.value[\s\S]*?isOwner:\s*isCurrentUserMessage\(msg\)/, 'note badge policy must include role, login, and ownership')
assert.equal((comments.match(/v-if="canShowCommentVisibility\(/g) || []).length, 2, 'root comments and replies must both use the viewer policy')
assert.match(comments, /shouldShowVisibilityBadge\(\{[\s\S]*?isAdmin:\s*isAdmin\.value[\s\S]*?isAuthenticated:\s*user\.isLogin[\s\S]*?isOwner:/, 'comment badge policy must include role, login, and ownership')

console.log('visibility badge policy checks passed')
