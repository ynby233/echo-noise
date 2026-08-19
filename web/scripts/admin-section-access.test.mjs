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
  const { resolveAccessibleAdminSection } = await import(pathToFileURL(outfile).href)
  assert.equal(resolveAccessibleAdminSection('notes', ['dashboard', 'notes'], 'dashboard'), 'notes')
  assert.equal(resolveAccessibleAdminSection('notes', ['dashboard', 'user'], 'dashboard'), 'dashboard')
  assert.equal(resolveAccessibleAdminSection('attachments', ['user'], 'dashboard'), 'user')
  console.log('admin section access tests passed')
} finally {
  fs.rmSync(outfile, { force: true })
}
