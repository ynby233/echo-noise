import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const root = new URL('../..', import.meta.url)
const read = (path) => readFile(new URL(path, root), 'utf8')
const [dockerfile, healthcheck, routes] = await Promise.all([
  read('Dockerfile'),
  read('docker-healthcheck.sh'),
  read('internal/routers/routers.go'),
])

assert.match(routes, /GET\("\/api\/health\/live", controllers\.HealthLive\)/, 'router must expose a middleware-independent liveness probe')
assert.match(routes, /GET\("\/api\/health\/ready", controllers\.HealthReady\)/, 'router must expose a fail-fast database readiness probe')
assert.match(healthcheck, /runtime\.env/, 'healthcheck must read the persisted runtime environment')
assert.match(healthcheck, /HTTP_PORT:-1314/, 'healthcheck must preserve the default HTTP port')
assert.match(healthcheck, /\/api\/health\/ready/, 'container health must reflect database readiness')
assert.match(dockerfile, /COPY \.\/docker-healthcheck\.sh \/app\/docker-healthcheck\.sh/, 'runtime image must include the healthcheck script')
assert.match(dockerfile, /HEALTHCHECK[\s\S]*\/app\/docker-healthcheck\.sh/, 'runtime image must declare its healthcheck')

console.log('docker healthcheck contract passed')
