import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const component = await readFile(join(root, 'components/comments/BuiltinComments.vue'), 'utf8')

assert.match(
  component,
  /import\s+\{\s*useAdminCapabilities\s*\}\s+from\s+['"]~\/composables\/useAdminCapabilities['"]/,
  'comment UI must consume the shared delegated-admin capability snapshot'
)
assert.match(
  component,
  /const\s+\{\s*can\s*\}\s*=\s*useAdminCapabilities\(\)/,
  'comment UI must use the capability predicate'
)
assert.match(
  component,
  /const\s+canEditComment\s*=\s*\(c:\s*any\)\s*=>\s*isCommentOwner\(c\)\s*\|\|\s*\(!isPrimaryAdminComment\(c\)\s*&&\s*can\(['"]comments\.edit['"]\)\)/,
  'editing another user’s comment must require comments.edit and preserve primary-admin content protection'
)
assert.match(
  component,
  /const\s+canDeleteComment\s*=\s*\(c:\s*any\)\s*=>\s*isCommentOwner\(c\)\s*\|\|\s*\(!isPrimaryAdminComment\(c\)\s*&&\s*can\(['"]comments\.delete['"]\)\)/,
  'deleting another user’s comment must require comments.delete and preserve primary-admin content protection'
)

for (const target of ['c', 'child']) {
  assert.match(
    component,
    new RegExp(`<button\\s+v-if="canEditComment\\(${target}\\)"[^>]*>编辑<\\/button>`),
    `${target} edit button must be hidden without comments.edit`
  )
  assert.match(
    component,
    new RegExp(`<button\\s+v-if="canDeleteComment\\(${target}\\)"[^>]*>删除<\\/button>`),
    `${target} delete button must be hidden without comments.delete`
  )
}

assert.doesNotMatch(
  component,
  /const\s+canManageComment\s*=\s*\(c:\s*any\)\s*=>\s*isAdmin\.value/,
  'binary admin status must not expose cross-author comment mutation controls'
)

console.log('comment capability visibility contract passed')
