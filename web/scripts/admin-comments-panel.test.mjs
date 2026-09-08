import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { readAdminPanelSource } from './admin-panel-source.mjs'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const component = await readFile(join(root, 'components/admin/CommentManager.vue'), 'utf8')

assert.match(
  component,
  /row\.username\s*\|\|\s*`用户 \$\{row\.user_id \|\| '—'\}`/,
  'admin comments manager must render the account username with an ID fallback'
)
assert.match(
  component,
  /v-for="row in rows"/,
  'admin comments list should render server-provided rows'
)
assert.match(
  component,
  /endpoint = computed\(\(\) => props\.recycleBin \? 'admin\/comment-recycle-bin' : 'admin\/comments'\)/,
  'admin comments manager should use the managed interaction endpoints'
)

console.log('admin comments panel tests passed')
