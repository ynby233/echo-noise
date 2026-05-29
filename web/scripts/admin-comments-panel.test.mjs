import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const component = await readFile(join(root, 'components/index/StatusPanel.vue'), 'utf8')

assert.match(
  component,
  /const\s+adminCommentAuthorName\s*=\s*\(c:\s*any\)\s*=>/,
  'admin comments panel must define the author-name helper used by the template'
)
assert.match(
  component,
  /adminCommentAuthorName\(c\)/,
  'admin comments list should render the account username helper'
)
assert.match(
  component,
  /user\.username\s*\|\|\s*user\.Username/,
  'admin comments author helper should read the bound account user object'
)

console.log('admin comments panel tests passed')
