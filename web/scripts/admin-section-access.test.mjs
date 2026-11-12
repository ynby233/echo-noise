import assert from 'node:assert/strict'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'
import fs from 'node:fs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const outfile = path.join(root, '.admin-section-access-test.mjs')
await build({
  entryPoints: [path.join(root, 'utils/admin-section-access.ts')],
  outfile,
  bundle: true,
  format: 'esm',
  platform: 'node',
  target: 'node18',
})

try {
  const { resolveAccessibleAdminSection, accessibleBackupTabs, resolveBackupTab, backupSectionAliases } = await import(pathToFileURL(outfile).href)
  assert.equal(resolveAccessibleAdminSection('notes', ['dashboard', 'notes'], 'dashboard'), 'notes')
  assert.equal(resolveAccessibleAdminSection('notes', ['dashboard', 'user'], 'dashboard'), 'dashboard')
  assert.equal(resolveAccessibleAdminSection('attachments', ['user'], 'dashboard'), 'user')
  const grants = (...values) => capability => values.includes(capability)
  assert.deepEqual(accessibleBackupTabs(false, grants()), [])
  assert.deepEqual(accessibleBackupTabs(false, grants('storage.view', 'backup.data.create')), [])
  assert.deepEqual(accessibleBackupTabs(false, grants('backup.data.view')), ['data'])
  assert.deepEqual(accessibleBackupTabs(false, grants('backup.attachments.view')), ['attachments'])
  assert.deepEqual(accessibleBackupTabs(false, grants('backup.data.view', 'backup.attachments.view', 'backup.server_config.view')), ['data', 'attachments'])
  assert.deepEqual(accessibleBackupTabs(true, grants()), ['data', 'attachments', 'server-config', 'cloud'])
  assert.equal(resolveBackupTab('data', []), undefined)
  assert.equal(resolveBackupTab('data', ['attachments']), 'attachments')
  assert.equal(resolveBackupTab('cloud', ['data', 'attachments']), 'data')
  assert.equal(resolveBackupTab('attachments', ['data', 'attachments']), 'attachments')
  assert.deepEqual(backupSectionAliases, { 'backup-data': 'data', 'backup-attachments': 'attachments', 'backup-server-config': 'server-config', 'cloud-backup': 'cloud' })
  for (const requested of Object.values(backupSectionAliases)) {
    assert.equal(resolveBackupTab(requested, ['attachments']), 'attachments', 'every legacy alias falls back to an accessible tab')
  }
  console.log('admin section access tests passed')
} finally {
  fs.rmSync(outfile, { force: true })
}
