import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'

const root = new URL('..', import.meta.url)
const read = (path) => readFile(new URL(path, root), 'utf8')

const [middleware, page, router, workflow, packageJson] = await Promise.all([
  read('middleware/mobile-setup.global.ts'),
  read('pages/setup.vue'),
  read('../internal/routers/routers.go'),
  read('../.github/workflows/tauri-release-sign.yml'),
  read('package.json'),
])

assert.match(middleware, /setup\/status/, 'global route middleware must query Android setup state')
assert.match(middleware, /state\s*===\s*['"]required['"]/, 'required state must be handled explicitly')
assert.match(middleware, /state\s*===\s*['"]invalid['"]/, 'invalid state must fail closed')
assert.match(middleware, /navigateTo\(['"]\/setup/, 'uninitialized navigation must be forced to the setup page')
assert.match(middleware, /to\.path\s*===\s*['"]\/setup['"]/s, 'the setup route must avoid a redirect loop')

assert.match(page, /confirm_password/, 'setup must submit password confirmation')
assert.match(page, /setup\/owner/, 'setup must call the dedicated owner initialization endpoint')
assert.match(page, /router\.replace\(['"]\/['"]\)/, 'successful setup must enter the home page')
assert.match(page, /setupState[^\n]*invalid|setup_state[^\n]*invalid/s, 'invalid retained data must render a locked state')

assert.match(router, /api\.GET\("\/setup\/status"/, 'router must expose setup status')
assert.match(router, /api\.POST\("\/setup\/owner"/, 'router must expose explicit owner creation')
assert.match(router, /MobileSetupGate/, 'router must install the Android-only API gate')

const androidWorkflow = workflow.split('\n  android:')[1] || ''
assert.match(androidWorkflow, /NUXT_PUBLIC_BASE_API:\s*['"]http:\/\/localhost:1314\/api['"]/, 'Android web assets must use a same-site localhost API origin')
assert.match(androidWorkflow, /androidScheme[^\n]*http/, 'Capacitor Android must use the HTTP localhost scheme for same-site sessions')
assert.match(androidWorkflow, /:\s*>\s*android\/app\/src\/main\/assets\/data\/noise\.db/, 'APK must generate a deterministic empty database asset')
assert.doesNotMatch(androidWorkflow, /SRC_DB=.*noise\.db/, 'APK must never select a tracked runtime database snapshot')

const scripts = JSON.parse(packageJson).scripts
assert.equal(scripts['test:mobile-owner-setup'], 'node scripts/mobile-owner-setup.test.mjs')

console.log('mobile owner setup contract passed')
