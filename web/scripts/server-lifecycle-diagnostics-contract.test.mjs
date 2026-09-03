import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('../../cmd/server/main.go', import.meta.url), 'utf8')

for (const stage of ['config_load', 'database_init', 'default_data_seed', 'workers_start', 'router_setup', 'http_listen']) {
  assert.ok(source.includes(`"${stage}"`), `startup diagnostics must include ${stage}`)
}
for (const stage of ['workers_stop', 'http_shutdown', 'access_log_flush', 'database_close']) {
  assert.ok(source.includes(`"${stage}"`), `shutdown diagnostics must include ${stage}`)
}
assert.match(source, /func logLifecycleStage\(/, 'server must use one stable lifecycle log format')
assert.match(source, /phase=%s stage=%s status=%s elapsed=%s/, 'lifecycle logs must expose phase, stage, status and elapsed time')
assert.match(source, /databaseCloseTimeout/, 'database close must have an independent timeout')

console.log('server lifecycle diagnostics contract passed')
