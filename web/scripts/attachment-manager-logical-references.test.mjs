import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('../components/admin/AttachmentManager.vue', import.meta.url), 'utf8')

assert.match(source, /item\.logical_id/, 'attachment cards must display or use the logical id')
assert.match(source, /附件 ID[：:]\s*\{\{\s*item\.logical_id\s*\}\}/, 'attachment cards must visibly show the full logical id')
assert.match(source, /attachments\/references\/\$\{encodeURIComponent\(item\.logical_id\)\}/, 'logical references must use the reference-only delete endpoint')
assert.match(source, /logical_id:\s*entry\.item\?\.logical_id/, 'zip downloads must identify logical references explicitly')
assert.match(source, /const itemIdentity = \(item: any\).*logical_id/s, 'same-name cards must use logical identity for selection and expansion')

console.log('attachment manager logical-reference checks passed')
