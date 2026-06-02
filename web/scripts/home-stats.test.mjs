import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))))
const webRoot = join(repoRoot, 'web')
const indexPage = await readFile(join(webRoot, 'pages/index.vue'), 'utf8')
const router = await readFile(join(repoRoot, 'internal/routers/routers.go'), 'utf8')
const controller = await readFile(join(repoRoot, 'internal/controllers/message_controller.go'), 'utf8')

assert.match(
  router,
  /authRoutes\.GET\("\/users\/me\/stats",\s*controllers\.GetCurrentUserHomeStats\)/,
  'personal home stats must be registered behind authenticated routes'
)
assert.doesNotMatch(
  router,
  /api\.GET\("\/users\/me\/stats"/,
  'personal home stats must not be exposed as a public route'
)

assert.match(
  controller,
  /Where\("user_id = \?",\s*user\.ID\)/,
  'personal stats must query messages scoped to the current user id'
)
assert.match(
  controller,
  /"total_messages":\s*totalMessages[\s\S]*"total_tags":\s*len\(tagSet\)[\s\S]*"total_images":\s*totalImages/,
  'personal stats response must include message, unique tag, and image counts'
)
assert.match(
  controller,
  /isHomeStatsExcludedMessage[\s\S]*#guestbook[\s\S]*#留言[\s\S]*关于本站[\s\S]*友情链接/,
  'home stats should exclude guestbook/about/friend-link system messages'
)

const statsCardStart = indexPage.indexOf('<UCard class="sidebar-card no-padding-card mt-2" :class="sidebarThemeCard">')
assert.notEqual(statsCardStart, -1, 'home page must keep the left stats card')
const statsCardEnd = indexPage.indexOf('</UCard>', statsCardStart)
assert.notEqual(statsCardEnd, -1, 'home stats card must have a closing card tag')
const statsCard = indexPage.slice(statsCardStart, statsCardEnd)

assert.match(
  statsCard,
  /<div\s+v-if="isLoggedIn"\s+class="p-0 grid grid-cols-3 gap-2 text-center text-sm">[\s\S]*?profileTotalMessages[\s\S]*?profileTotalTags[\s\S]*?profileTotalImages/,
  'home stats card should render numeric stats only for logged-in users'
)
assert.match(
  statsCard,
  /<button\s+v-else\s+type="button"\s+class="stats-login-prompt"[\s\S]*?登录查看统计/,
  'guests should see only the login prompt in the stats card'
)
assert.match(
  indexPage,
  /getRequest<ProfileHomeStats>\('users\/me\/stats',[\s\S]*credentials:\s*'include'[\s\S]*silent:\s*true/,
  'home page must fetch personal stats from the authenticated current-user endpoint'
)
assert.match(
  indexPage,
  /if\s*\(!isLoggedIn\.value\)\s*\{[\s\S]*?profileHomeStats\.value\s*=\s*null[\s\S]*?return/,
  'guest state must clear cached personal stats instead of showing stale numbers'
)
assert.doesNotMatch(
  statsCard,
  /status\?\.total_messages[\s\S]*?笔记数/,
  'home stats card must not use global status.total_messages'
)
assert.doesNotMatch(
  statsCard,
  /tagsCount[\s\S]*?标签/,
  'home stats card must not use the global tag list count'
)
assert.doesNotMatch(
  statsCard,
  /images\?\.length[\s\S]*?图片/,
  'home stats card must not use the global image gallery count'
)

console.log('home personal stats tests passed')
