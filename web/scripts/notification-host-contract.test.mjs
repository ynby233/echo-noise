import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const read = (path) => readFile(join(webRoot, path), 'utf8')
const [app, home, status, login, register, notification] = await Promise.all([
  read('app.vue'),
  read('pages/index.vue'),
  read('pages/status.vue'),
  read('pages/auth/login.vue'),
  read('pages/auth/register.vue'),
  read('components/widgets/Notification.vue'),
])

assert.match(app, /<Notification\s*\/>/, 'the application shell must own the global notification host')
assert.match(notification, /<UNotifications class="site-notifications"\s*\/>/, 'the notification host must render the shared toast container')

for (const [name, source] of Object.entries({ home, status, login, register })) {
  assert.equal(source.includes('<Notification />'), false, `${name} must not mount a second notification host`)
  assert.equal(source.includes('<UNotifications />'), false, `${name} must not mount a second toast container`)
}

console.log('notification host contract checks passed')
