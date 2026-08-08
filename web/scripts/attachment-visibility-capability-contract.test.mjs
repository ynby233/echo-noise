import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const webRoot = new URL('..', import.meta.url)
const repoRoot = new URL('../..', import.meta.url)
const readWeb = (file) => readFile(new URL(file, webRoot), 'utf8')
const readRepo = (file) => readFile(new URL(file, repoRoot), 'utf8')
const manager = await readWeb('components/admin/AttachmentManager.vue')
const routers = await readRepo('internal/routers/routers.go')
const controller = await readRepo('internal/controllers/attachment_controller.go')

for (const capability of ['View', 'Download', 'DeleteReference', 'PurgeBlob']) {
  assert.match(routers, new RegExp(`CapabilityAttachments${capability}`), `attachments ${capability} capability must be present on a management route`)
}
assert.match(manager, /useAdminCapabilities/)
assert.match(manager, /can\('attachments\.download'\)/)
assert.match(manager, /can\('attachments\.delete_reference'\)/)
assert.match(manager, /can\('attachments\.purge_blob'\)/)
assert.match(controller, /VisibleAttachmentSources/)
assert.match(controller, /PurgeBlobScoped/)
console.log('attachment visibility capability checks passed')
