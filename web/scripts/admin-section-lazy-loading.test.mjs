import assert from 'node:assert/strict'
import { access, readFile } from 'node:fs/promises'

const read = async relativePath => readFile(new URL(`../${relativePath}`, import.meta.url), 'utf8')

const panel = await read('components/index/StatusPanel.vue')
const registry = await read('components/admin/sections/registry.ts')

for (const [key, file] of [
  ['storage', 'StorageSection.vue'],
  ['backups', 'BackupSection.vue'],
]) {
  await access(new URL(`../components/admin/sections/${file}`, import.meta.url))
  assert.match(
    registry,
    new RegExp(`(?:'${key}'|${key}):\\s*\\(\\)\\s*=>\\s*import\\('\\./${file.replace('.', '\\.')}'\\)`),
    `${key} must be exposed through its own dynamic import`,
  )
}
assert.doesNotMatch(registry, /\bdb:\s*\(\)\s*=>|import\('\.\/DatabaseSection\.vue'\)/, 'legacy database entry must not be navigable')
assert.doesNotMatch(registry, /'(?:cloud-backup|backup-data|backup-attachments|backup-server-config)':\s*\(\)\s*=>/, 'legacy backup keys resolve as aliases rather than independent pages')
const workspace = await read('components/admin/sections/BackupSection.vue')
assert.match(workspace, /defineAsyncComponent\(\(\) => import\('\.\/BackupWorkbench\.vue'\)\)/)
assert.match(workspace, /v-if="activeTab === 'cloud'"/, 'cloud connections mount only when selected')
assert.match(workspace, /<BackupWorkbench v-else :key="activeTab"/, 'only the selected backup kind is mounted')
assert.doesNotMatch(workspace, /v-show/, 'inactive tabs must not all render at once')
assert.match(workspace, /<KeepAlive :max="4">/, 'visited tabs retain form drafts while their workbenches are deactivated')
assert.match(workspace, /role="tablist"/)
assert.match(workspace, /embedded/, 'children render within the shared workspace heading')
const storageGroup = panel.split("{ key: 'storage-maintain'")[1]?.split('\n')[0]
assert.ok(storageGroup)
assert.match(storageGroup, /key: 'storage', label: '附件存储'/)
assert.match(storageGroup, /key: 'backups', label: '备份与恢复'/)
assert.match(storageGroup, /key: 'attachments', label: '附件管理'/)
assert.match(storageGroup, /key: 'version', label: '版本与更新'/)
assert.doesNotMatch(storageGroup, /key: '(?:cloud-backup|backup-data|backup-attachments|backup-server-config)'/)
assert.match(panel, /Object\.hasOwn\(backupSectionAliases, key\)/, 'legacy hashes must resolve through backup aliases')
assert.match(panel, /backupTab\.value = backupSectionAliases\[/, 'legacy hashes select the matching workspace tab')
assert.match(panel, /:backup-tab="backupTab"/)
assert.match(panel, /@update:backup-tab="backupTab = \$event"/, 'tab selection stays synchronized for later hash navigation')

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
