import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { readAdminPanelSource } from './admin-panel-source.mjs'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const panel = await readAdminPanelSource()

assert.match(
  panel,
  /stagedRestoreDescription[\s\S]*?payload\?\.warning[\s\S]*?fetch\('\/api\/backup\/restore',[\s\S]*?body\?\.code === 1 && body\?\.pendingRestart[\s\S]*?title: '恢复已暂存'/,
  'local SQLite restore must describe a staged restore instead of claiming the live process already changed',
)

assert.match(
  panel,
  /fetch\('\/api\/backup\/storage\/sync-now',[\s\S]*?body\?\.code === 1 && body\?\.pendingRestart[\s\S]*?title: '恢复已暂存'[\s\S]*?stagedRestoreDescription\(body\)/,
  'manual cloud synchronization must surface a staged cloud restore distinctly',
)

assert.match(
  panel,
  /const handleCloudRestoreResponse[\s\S]*?pendingRestart[\s\S]*?title: '恢复已暂存'[\s\S]*?stagedRestoreDescription\(response\)[\s\S]*?response\?\.code !== 1/,
  'cloud restore responses must distinguish staged completion from immediate completion',
)
for (const functionName of ['restoreCloudBackup', 'restoreFromConfiguredCloud']) {
  const start = panel.indexOf(`const ${functionName}`)
  const end = panel.indexOf('\nconst ', start + 1)
  const source = panel.slice(start, end === -1 ? undefined : end)
  assert.match(
    source,
    /handleCloudRestoreResponse\(await postRequest<any>\('backup\/storage\/restore'/,
    `${functionName} must not call a staged restore successful before restart`,
  )
}

console.log('backup restore restart contract tests passed')
