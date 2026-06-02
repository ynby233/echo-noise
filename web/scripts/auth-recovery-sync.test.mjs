import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const app = await readFile(join(webRoot, 'app.vue'), 'utf8')
const api = await readFile(join(webRoot, 'utils/api.ts'), 'utf8')

assert.match(
  app,
  /window\.addEventListener\('pageshow',\s*syncAuthState\)/,
  'root app must check auth when a cached or backgrounded page is shown again'
)
assert.match(
  app,
  /window\.addEventListener\('focus',\s*syncAuthState\)/,
  'root app must check auth when the browser tab regains focus'
)
assert.match(
  app,
  /document\.addEventListener\('visibilitychange',\s*syncAuthStateWhenVisible\)/,
  'root app must check auth when the document becomes visible'
)
assert.match(
  app,
  /window\.setInterval\(syncAuthStateWhenVisible,\s*AUTH_SYNC_INTERVAL_MS\)/,
  'root app must keep logged-in foreground sessions in sync with a low-frequency timer'
)
assert.match(
  app,
  /const hasLocalLoginState = \(\) => !!userStore\.isLogin \|\| !!userStore\.token/,
  'auth recovery checks must only run when the browser still has local login state'
)
assert.match(
  app,
  /if \(!hasLocalLoginState\(\)\) return[\s\S]*?if \(authSyncInFlight\) return[\s\S]*?AUTH_SYNC_MIN_INTERVAL_MS[\s\S]*?userStore\.getUser\(\)/,
  'auth recovery checks must be deduped, throttled, and reuse the user store check'
)
assert.match(
  app,
  /removeEventListener\('pageshow',\s*syncAuthState\)[\s\S]*?removeEventListener\('focus',\s*syncAuthState\)[\s\S]*?removeEventListener\('visibilitychange',\s*syncAuthStateWhenVisible\)[\s\S]*?clearInterval\(authSyncTimer\)/,
  'auth recovery listeners and timer must be cleaned up'
)
assert.match(
  api,
  /normalizedStatus === 401[\s\S]*?handleAuthExpired\(authMsg, options\)/,
  'API wrapper must keep handling 401 responses through the centralized auth expiry path'
)
assert.match(
  api,
  /isAuthExpiredMsg\(msg\)[\s\S]*?handleAuthExpired\(msg, options\)/,
  'API wrapper must keep handling auth-expired business responses centrally'
)

console.log('auth recovery sync tests passed')
