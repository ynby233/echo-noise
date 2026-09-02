import assert from 'node:assert/strict'
import fs from 'node:fs'

const componentURL = new URL('../components/comments/BuiltinComments.vue', import.meta.url)
const component = fs.readFileSync(componentURL, 'utf8')

assert.equal(
  component.includes('收回回复'),
  false,
  'all collapse interaction labels must use the unified wording “收回”',
)
assert.match(
  component,
  /class="reply-pagination-row"[\s\S]*加载更多回复/,
  'reply pagination must have a dedicated layout row instead of inheriting the indented reply body width',
)
assert.match(
  component,
  /\.comment-thread-item\s*\{[^}]*flex-wrap:\s*wrap/s,
  'the root comment card must allow the reply pagination row to span a separate line',
)
assert.match(
  component,
  /\.reply-pagination-row\s*\{[^}]*flex:\s*0 0 100%[^}]*justify-content:\s*center[^}]*margin-top:/s,
  'reply pagination must be centered across the full comment card with spacing above it',
)

console.log('Comment reply pagination layout test passed')
