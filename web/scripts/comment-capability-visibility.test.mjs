import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const component = await readFile(join(root, 'components/comments/BuiltinComments.vue'), 'utf8')
const manager = await readFile(join(root, 'components/admin/CommentManager.vue'), 'utf8')
const panel = await readFile(join(root, 'components/index/StatusPanel.vue'), 'utf8')

assert.match(component, /useAdminCapabilities/, 'thread UI must consume delegated-admin capabilities')
assert.match(component, /can\(['"]comments\.edit['"]\)/, 'cross-author body editing must require comments.edit')
assert.match(component, /can\(['"]comments\.trash['"]\)/, 'cross-author trashing must require comments.trash')
assert.match(component, /can\(['"]comments\.change_visibility['"]\)/, 'cross-author visibility editing must use its independent capability')
assert.match(component, /v-if="canChangeCommentVisibility\(c\)"/, 'root visibility control must be hidden without its capability')
assert.match(component, /v-if="canChangeCommentVisibility\(child\)"/, 'reply visibility control must be hidden without its capability')
assert.match(manager, /v-if="!recycleBin && canTrash && row\.can_trash"/, 'management trash action must be capability and target controlled')
assert.match(manager, /v-if="!recycleBin && canEdit && row\.can_edit"/, 'management edit action must be capability and target controlled')
assert.match(manager, /v-if="!recycleBin && canChangeVisibility && row\.can_change_visibility"/, 'management visibility action must be independently controlled')
assert.match(manager, /v-if="recycleBin && canRestore/, 'restore action must be capability controlled')
assert.match(manager, /v-if="recycleBin && canDeletePermanently && row\.can_permanently_delete"/, 'permanent delete action must be capability and target controlled')
assert.match(panel, /canSection\('comment-recycle-bin'\)/, 'comment recycle-bin panel must be section-gated')
assert.doesNotMatch(component, /can\(['"]comments\.delete['"]\)/, 'retired comments.delete capability must not remain')

for (const target of ['c', 'child']) {
  assert.match(component, new RegExp(`<button\\s+v-if="canEditComment\\(${target}\\)"[^>]*>编辑<\\/button>`))
  assert.match(component, new RegExp(`<button\\s+v-if="canDeleteComment\\(${target}\\)"[^>]*>删除<\\/button>`))
}

assert.match(component, /comment\?\.can_interact\s*===\s*true/, 'reply controls must consume the server interaction decision')
console.log('comment capability visibility contract passed')
