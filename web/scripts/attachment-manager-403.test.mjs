import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const manager = await readFile(new URL('../components/admin/AttachmentManager.vue', import.meta.url), 'utf8')

assert.match(manager, /import \{ getRequest \} from '~\/utils\/api'/, 'attachment lists must use the shared 403-aware request adapter')
assert.match(manager, /createAdminModulePermissionHandler/, 'attachment management must share the guarded permission-change handler')
for (const endpoint of ['images', 'video', 'audio', 'other']) {
  assert.match(manager, new RegExp('getRequest<any>\\(\\s*[`\\\'\\"]attachments\\/' + endpoint), `${endpoint} attachment loading must use getRequest`)
  assert.equal(manager.includes("fetch(`${baseApi}/attachments/" + endpoint), false, `${endpoint} attachment loading must not bypass shared 403 handling`)
}
assert.match(manager, /const clearAttachmentState = \(\) => \{[\s\S]*?images\.value = \[\][\s\S]*?videos\.value = \[\][\s\S]*?audios\.value = \[\][\s\S]*?others\.value = \[\][\s\S]*?selected\.value = \{\}/, 'revocation must clear cached attachment rows and selection')
assert.match(manager, /responses\.some\(\(response\) => response\?\.status === 403\)[\s\S]*?await permissionChanged\(\)/, 'one refresh generation must coalesce list 403s into one permission transition')
assert.match(manager, /admin-capabilities-invalidated/, 'a later permission generation must reset the one-shot guard')

console.log('attachment manager 403 tests passed')
