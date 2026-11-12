import assert from 'node:assert/strict'
import { access, readFile } from 'node:fs/promises'

const read = async relativePath => readFile(new URL(`../${relativePath}`, import.meta.url), 'utf8')

const panel = await read('components/index/StatusPanel.vue')
const registry = await read('components/admin/sections/registry.ts')

for (const [key, file] of [
  ['storage', 'StorageSection.vue'],
  ['cloud-backup', 'CloudBackupSection.vue'],
  ['backup-data', 'DataBackupSection.vue'],
  ['backup-attachments', 'AttachmentBackupSection.vue'],
  ['backup-server-config', 'ServerConfigBackupSection.vue'],
]) {
  await access(new URL(`../components/admin/sections/${file}`, import.meta.url))
  assert.match(
    registry,
    new RegExp(`(?:'${key}'|${key}):\\s*\\(\\)\\s*=>\\s*import\\('\\./${file.replace('.', '\\.')}'\\)`),
    `${key} must be exposed through its own dynamic import`,
  )
}
assert.doesNotMatch(registry, /\bdb:\s*\(\)\s*=>|import\('\.\/DatabaseSection\.vue'\)/, 'legacy database entry must not be navigable')

assert.match(panel, /adminSectionLoaders\[activeSection\.value\]/, 'the selected section must still resolve through a dynamic loader')
assert.match(panel, /accessibleSectionKeys\.value\.filter/, 'entry prewarming must use permission-filtered section keys')
assert.match(panel, /sectionModuleCache\.load\(loader\)/, 'prewarming must load code through the session cache')
assert.match(panel, /:module-cache="sectionModuleCache"/, 'the host must share successful warm imports')
assert.doesNotMatch(panel, /import\s+StorageSection\s+from/, 'the shell must not eagerly import storage')
assert.doesNotMatch(panel, /import\s+DatabaseSection\s+from/, 'the shell must not eagerly import database')

for (const endpoint of ['backup/storage/config', 'backup/download', 'backup/restore']) {
  assert.doesNotMatch(panel, new RegExp(endpoint.replace('/', '\\/'), 'i'), `the shell must not own ${endpoint} requests`)
}

console.log('admin section lazy-loading contract passed')
