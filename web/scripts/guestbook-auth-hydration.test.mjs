import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const component = await readFile(join(root, 'components/comments/BuiltinComments.vue'), 'utf8')
const homePage = await readFile(join(root, 'pages/index.vue'), 'utf8')

assert.match(
  component,
  /const\s+guestbookAuthPending\s*=\s*ref\(false\)/,
  'guestbook input must track the initial authentication check instead of showing a login prompt during hydration'
)
assert.match(
  component,
  /const\s+ensureGuestbookAuth\s*=\s*async\s*\(\)\s*=>\s*\{[\s\S]*?user\.getUser\(\)/,
  'the guestbook surface must revalidate the current session when the public page mounts before the store is hydrated'
)
assert.match(
  component,
  /<div\s+v-else-if="props\.showInput\s*&&\s*enabled\s*&&\s*guestbookAuthPending"[^>]*>正在检查登录状态\.\.\.<\/div>/,
  'the guestbook surface must use a neutral auth-check state rather than claiming that a logged-in user must sign in'
)
assert.match(
  component,
  /<div\s+v-else-if="showLoginRequired"[^>]*>\{\{\s*loginRequiredText\s*\}\}<\/div>/,
  'the login-required copy must only render after guestbook authentication hydration finishes'
)
assert.match(
  component,
  /onMounted\(\(\)\s*=>\s*\{[\s\S]*?void\s+ensureGuestbookAuth\(\)/,
  'guestbook authentication hydration must start when the comment component mounts'
)
assert.match(
  homePage,
  /<BuiltinComments\s+v-if="guestbookMessageId"[\s\S]*?:show-input="true"[\s\S]*?:can-interact="true"[\s\S]*context-label="留言"/,
  'the guestbook page must explicitly enable interaction because canInteract is a Boolean prop'
)

console.log('guestbook auth hydration contract passed')
