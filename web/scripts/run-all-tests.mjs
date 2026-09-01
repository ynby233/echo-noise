import { spawnSync } from 'node:child_process'
import { readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptsDirectory = dirname(fileURLToPath(import.meta.url))
const webDirectory = dirname(scriptsDirectory)
const tests = readdirSync(scriptsDirectory)
  .filter(name => name.endsWith('.test.mjs'))
  .sort()

for (const test of tests) {
  const result = spawnSync(process.execPath, [join(scriptsDirectory, test)], {
    cwd: webDirectory,
    stdio: 'inherit',
  })
  if (result.error) throw result.error
  if (result.status !== 0) process.exit(result.status ?? 1)
}

console.log(`Frontend test suite passed (${tests.length} files)`)
