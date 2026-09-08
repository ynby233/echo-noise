import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const panel = await readFile(join(webRoot, 'components/index/StatusPanel.vue'), 'utf8')

assert.match(
  panel,
  /fetch\('\/api\/backup\/restore',[\s\S]*?data\.code === 1 && data\.pendingRestart[\s\S]*?title: '恢复已暂存'[\s\S]*?请重启服务后完成恢复/,
  'local SQLite restore must describe a staged restore instead of claiming the live process already changed',
)

assert.match(
  panel,
  /fetch\('\/api\/backup\/storage\/sync-now',[\s\S]*?data\?\.code === 1 && data\?\.pendingRestart[\s\S]*?title: '恢复已暂存'/,
  'manual cloud synchronization must surface a staged cloud restore distinctly',
)

for (const functionName of ['restoreCloudBackup', 'restoreFromConfiguredCloud']) {
  const start = panel.indexOf(`const ${functionName}`)
  const end = panel.indexOf('\nconst ', start + 1)
  const source = panel.slice(start, end === -1 ? undefined : end)
  assert.match(
    source,
    /pendingRestart[\s\S]*?title: '恢复已暂存'[\s\S]*?else if \(res\?\.code === 1\)/,
    `${functionName} must not call a staged restore successful before restart`,
  )
}

console.log('backup restore restart contract tests passed')
