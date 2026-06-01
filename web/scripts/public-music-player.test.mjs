import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))))
const webRoot = join(repoRoot, 'web')
const indexPage = await readFile(join(webRoot, 'pages/index.vue'), 'utf8')
const router = await readFile(join(repoRoot, 'internal/routers/routers.go'), 'utf8')
const settingService = await readFile(join(repoRoot, 'internal/services/setting_service.go'), 'utf8')

assert.match(
  router,
  /\/\/ 公共路由[\s\S]*api\.GET\("\/frontend\/config",\s*controllers\.GetFrontendConfig\)/,
  'frontend/config must stay public so guests can read music player settings'
)

assert.match(
  settingService,
  /"musicEnabled"\s*:\s*config\.MusicEnabled/,
  'public frontend config must include musicEnabled'
)
assert.match(
  settingService,
  /"musicPlaylistId"\s*:\s*choose\(config\.MusicPlaylistId,\s*""\)/,
  'public frontend config must include the administrator playlist id'
)

assert.match(
  indexPage,
  /<div\s+v-if="shouldShowMusicPlayer"\s+class="music-player-wrapper">[\s\S]*?<div\s+class="netease-mini-player"><\/div>/,
  'home page should render the music player from the public visibility guard'
)

const computedMatch = indexPage.match(/const\s+shouldShowMusicPlayer\s*=\s*computed\(\(\)\s*=>\s*\{([\s\S]*?)\n\}\)/)
assert.ok(computedMatch, 'home page must define shouldShowMusicPlayer computed guard')
const guardBody = computedMatch[1]

assert.match(guardBody, /musicConfigLoaded\.value/, 'music player should wait until frontend config has loaded')
assert.match(guardBody, /!!cfg\.musicEnabled/, 'music player should still respect the administrator enable switch')
assert.match(guardBody, /source\.hasSource/, 'music player should require a configured playlist or song source')
assert.match(guardBody, /musicHideOnMobile/, 'music player should still respect the mobile hiding switch')
assert.doesNotMatch(
  guardBody,
  /\bis(Login|LoggedIn|Admin|Online)\b|userStore|auth|token/i,
  'guest music visibility must not depend on login, admin, token, or auth state'
)

console.log('public music player tests passed')
