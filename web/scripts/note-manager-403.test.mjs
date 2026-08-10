import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const manager = fs.readFileSync(path.join(root, 'components/admin/NoteManager.vue'), 'utf8')
const models = fs.readFileSync(path.join(root, 'types/models.ts'), 'utf8')
assert.match(models, /status\?:\s*number/, 'Response<T> must expose HTTP error status')
assert.match(manager, /createNoteManagerPermissionHandler/, 'NoteManager must use the guarded 403 handler')
assert.doesNotMatch(manager, /const permissionChanged\s*=\s*async[\s\S]*?await load\(\)/, '403 handler must not recursively reload the same endpoint')

const outfile = path.join(root, '.note-manager-403-test.mjs')
await build({
  entryPoints: [path.join(root, 'utils/note-manager-permission.ts')],
  outfile,
  bundle: true,
  format: 'esm',
  platform: 'node',
  target: 'node18',
})
try {
  const { createNoteManagerPermissionHandler } = await import(pathToFileURL(outfile).href)
  const state = { rows: [{ id: 1 }], total: 1, selected: [1], detailId: 1 }
  let refreshes = 0
  let toasts = 0
  let clears = 0
  const handleForbidden = createNoteManagerPermissionHandler({
    clearState: () => {
      clears += 1
      state.rows = []
      state.total = 0
      state.selected = []
      state.detailId = null
    },
    refreshCapabilities: async () => { refreshes += 1 },
    notify: () => { toasts += 1 },
  })
  let requests = 0
  const load = async () => {
    requests += 1
    const response = { status: 403 }
    if (response.status === 403) await handleForbidden()
  }
  await load()
  await handleForbidden()
  await handleForbidden()
  assert.equal(requests, 1, 'a continuous 403 load must not issue a recursive request')
  assert.equal(refreshes, 1, 'capability refresh must be guarded to one in-flight/once refresh')
  assert.equal(toasts, 1, 'permission toast must appear only once')
  assert.equal(clears, 3)
  assert.deepEqual(state, { rows: [], total: 0, selected: [], detailId: null })
  console.log('note manager 403 tests passed')
} finally {
  fs.rmSync(outfile, { force: true })
}
