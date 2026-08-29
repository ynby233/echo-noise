import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))))
const read = (path) => readFile(join(repoRoot, path), 'utf8')
const [statusPanel, personalContent, noteManager, routes, controller] = await Promise.all([
  read('web/components/index/StatusPanel.vue'),
  read('web/components/index/PersonalContentManager.vue'),
  read('web/components/admin/NoteManager.vue'),
  read('internal/routers/routers.go'),
  read('internal/controllers/comment_management_controller.go'),
])

assert.equal((statusPanel.match(/<PersonalContentManager\b/g) || []).length, 4, 'the four personal content areas must be independent pages rather than one panel under user information')
for (const label of ['个人笔记', '个人笔记回收站', '个人互动', '个人互动回收站']) {
  assert.match(statusPanel, new RegExp(`key: '[^']+', label: '${label}'`), `${label} must have a sidebar entry`)
}
assert.ok(statusPanel.indexOf("label: '站点与展示'") < statusPanel.indexOf("{ key: 'widgets', label: '小组件'"), 'widgets must remain under site and display')
assert.match(statusPanel, /:aria-expanded="navGroupOpen\[group\.key\]"/, 'group headings must expose their expanded state')
assert.match(statusPanel, /v-if="!sidebarCollapsed && navGroupOpen\[group\.key\]"/, 'collapsed groups must hide all child entries')
assert.match(statusPanel, /const toggleNavGroup[\s\S]*?navGroupOpen\[groupKey\] = !navGroupOpen\[groupKey\]/, 'clicking a group heading must toggle that group')
assert.match(personalContent, /section: PersonalSection/, 'personal content pages must use an explicit fixed section')
assert.match(personalContent, /notes: 'user\/notes'/, 'personal notes must use the authenticated personal endpoint')
assert.match(routes, /GET\("\/user\/notes", controllers\.ListPersonalNotes\)/, 'personal notes route must be registered behind authenticated routes')
assert.match(controller, /func ListPersonalNotes[\s\S]*?checkUser\(c\)/, 'personal notes endpoint must require the current user')
assert.match(routes, /DELETE\("\/user\/recycle-bin\/notes\/:id", controllers\.PermanentlyDeletePersonalNote\)/, 'personal note recycle bin must expose an owner-scoped permanent-delete route')
assert.match(routes, /POST\("\/user\/recycle-bin\/notes\/batch-permanent-delete", controllers\.BatchPermanentlyDeletePersonalNotes\)/, 'personal note recycle bin must expose an owner-scoped batch permanent-delete route')
assert.match(personalContent, /section === 'note-recycle-bin'[\s\S]*?batchPurgeNotes/, 'personal note recycle bin must show a batch permanent-delete action')
assert.match(personalContent, /section === 'note-recycle-bin'[\s\S]*?@click="purgeNote\(row\)"/, 'personal note recycle bin must show a single permanent-delete action')
for (const legacyReason of ['author request', 'author batch request', 'admin batch request']) {
  assert.ok(personalContent.includes(`'${legacyReason}'`), `legacy deletion reason ${legacyReason} must have a user-facing mapping`)
}
assert.match(noteManager, /\.note-row-actions\s*\{[\s\S]*?flex-wrap:\s*nowrap;[\s\S]*?white-space:\s*nowrap;/, 'note row actions must stay on one line')

console.log('personal content navigation contract checks passed')
