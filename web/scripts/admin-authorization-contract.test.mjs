import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const panel = await readFile(new URL('../components/index/StatusPanel.vue', import.meta.url), 'utf8')
const center = await readFile(new URL('../components/admin/AuthorizationCenter.vue', import.meta.url), 'utf8')
const audit = await readFile(new URL('../components/admin/AuditLogPanel.vue', import.meta.url), 'utf8')

assert.match(panel, /admin\/authorization\/me/)
assert.match(panel, /isPrimaryAdmin/)
assert.match(panel, /canViewAdminAudit/)
assert.match(center, /admin\/authorization\/admins/)
assert.match(center, /admin\/authorization\/catalog/)
assert.match(center, /capabilities/)
assert.match(audit, /admin\/audit-logs/)
assert.match(audit, /page_size/)
assert.doesNotMatch(center + audit, /echo-noise\s*\/\s*说说笔记/i)
