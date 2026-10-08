import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { readSettingServiceSource } from './setting-service-source.mjs'

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))))
const webRoot = join(repoRoot, 'web')
const indexPage = await readFile(join(webRoot, 'pages/index.vue'), 'utf8')
const router = await readFile(join(repoRoot, 'internal/routers/routers.go'), 'utf8')
const settingService = await readSettingServiceSource()
const publicMusicHook = await readFile(join(webRoot, 'composables/usePublicMusic.ts'), 'utf8')

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
assert.match(guardBody, /const cfg(?::\s*any)?\s*=\s*musicPlaybackConfig\.value/, 'visibility must use the authoritative public music playback config')
assert.match(guardBody, /!!cfg\.musicEnabled/, 'music player should still respect the administrator enable switch')
assert.match(guardBody, /source\.hasSource/, 'music player should require a configured playlist or song source')
assert.match(guardBody, /musicHideOnMobile/, 'music player should still respect the mobile hiding switch')
assert.doesNotMatch(
  guardBody,
  /\bis(Login|LoggedIn|Admin|Online)\b|userStore|auth|token/i,
  'guest music visibility must not depend on login, admin, token, or auth state'
)
assert.match(
  indexPage,
  /\.netease-mini-player\s*\{\s*font-family:\s*inherit\s*!important;\s*\}/,
  'the music player must inherit the site typeface instead of using its bundled system-font stack'
)

const reconcileStart = indexPage.indexOf('const reconcileMusicPlayer = async (reason = \'state\') => {')
const reconcileEnd = indexPage.indexOf('const dedupeStrings', reconcileStart)
assert.notEqual(reconcileStart, -1, 'home page must define one public music reconciler')
assert.notEqual(reconcileEnd, -1, 'home page must keep the reconciler before NMP asset helpers')
const reconcileBody = indexPage.slice(reconcileStart, reconcileEnd)

assert.match(
  indexPage,
  /const\s+isNmpMinimized\s*=\s*\(el: any\)\s*=>\s*!!el\s*&&\s*el\.classList\.contains\('minimized'\)/,
  'music minimized detection must only read the root player class, not descendant .minimized classes'
)
assert.match(
  indexPage,
  /const\s+minimized\s*=\s*!!cfg\.musicDefaultMinimized\s*\?\s*true\s*:\s*\(typeof saved\.minimized === 'boolean' \? saved\.minimized : false\)/,
  'administrator default-minimized must win over stale saved expanded state'
)
assert.match(
  indexPage,
  /const\s+scheduleMusicPlayerReconcile\s*=\s*\(reason = 'state'\) => \{[\s\S]*?await\s+reconcileMusicPlayer\(reason\)/,
  'all music startup triggers must be funneled through scheduleMusicPlayerReconcile'
)
assert.match(
  reconcileBody,
  /syncNmpAttributes\(el, cfg\)[\s\S]*?loadNMPAssets\(source\.kind\)/,
  'the reconciler must write public music attributes before loading the self-initializing NMP script'
)
assert.match(
  reconcileBody,
  /refreshNmpConfig\(player\)[\s\S]*?await\s+syncNmpSource\(el, player, source\)/,
  'the reconciler must refresh reused NMP config before syncing source and theme'
)
assert.match(
  indexPage,
  /const\s+syncNmpSource[\s\S]*?await\s+player\.loadPlaylist\?\.\(source\.playlistId\)[\s\S]*?await\s+player\.loadSingleSong\?\.\(source\.songId\)[\s\S]*?await\s+loadNmpCurrentSong\(player\)/,
  'source synchronization must reload playlist/song sources and then load the current song'
)
assert.match(
  indexPage,
  /const\s+syncNmpSource[\s\S]*?if \(source\.kind === 'local'\)[\s\S]*?await player\.setLocalPlaylist\(source\.tracks, sourceKey\)[\s\S]*?return !!player\.currentSong/,
  'local playlists must synchronize through the local source API before the NetEase source branch'
)
assert.match(indexPage, /hasSource: kind === 'local' \? tracks\.length > 0 : !!playlistId \|\| !!songId/, 'local visibility requires tracks while legacy visibility requires a playlist or song')
assert.match(indexPage, /musicEnabled: !!publicMusic\.state\.value\?\.frontendSettings\.musicEnabled/, 'playback must stop when public music authorization is absent')
assert.match(reconcileBody, /const cfg = musicPlaybackConfig\.value/, 'reconcile must use the same public playback config as visibility')
assert.match(reconcileBody, /const sourceStillCurrent = [^\n]*generation === nmpGeneration[^\n]*resolveMusicSource\(musicPlaybackConfig\.value\)\.kind === source\.kind/, 'async startup must reject stale or replaced sources')
assert.match(reconcileBody, /await syncNmpSource\(el, player, source\)\s+if \(!sourceStillCurrent\(\)\) return false/, 'source completion must be guarded before theme or autoplay')
assert.match(indexPage, /const getNmpPlayer[\s\S]*?nmpInstance = player[\s\S]*?await waitForNmpPlayerReady\(player\)[\s\S]*?if \(player.ready\) await player.ready/, 'initializing player must be held before awaits so source changes can cancel it')
assert.match(indexPage, /cssCandidates = source === 'local' \? \[NMP_LOCAL_CSS\]/, 'local music must use bundled styles')
assert.match(indexPage, /jsCandidates = source === 'local' \? \[NMP_LOCAL_JS\]/, 'local music must use the bundled player with the local source API')
assert.match(indexPage, /source === 'local' && !loaded\.supportsLocalPlaylist/, 'local music must reject a player without the local source API')
assert.match(publicMusicHook, /\$fetch<unknown>\(`\$\{baseApi\}\/music\/public`,\s*\{\s*method: 'GET'/, 'public music must use the dedicated guest GET API')
assert.doesNotMatch(publicMusicHook, /userStore|isLoggedIn|isOnline|Authorization/, 'public music requests must not require authenticated or online context')
assert.match(
  indexPage,
  /watch\(\(\) => \[[\s\S]*?musicConfigLoaded\.value[\s\S]*?frontendConfig\.value\.musicEnabled[\s\S]*?scheduleMusicPlayerReconcile\('public-config'\)/,
  'public music config changes must trigger the same reconciler'
)
assert.match(indexPage, /watch\(\(\) => \[[\s\S]*?publicMusic\.state\.value\?\.revision[\s\S]*?scheduleMusicPlayerReconcile\('public-config'\)/, 'public revision changes must reconcile local playlists')
assert.match(
  indexPage,
  /watch\(\(\) => \[isLoggedIn\.value, isOnline\.value, route\.fullPath, activeTab\.value\][\s\S]*?scheduleMusicPlayerReconcile\('context-change'\)/,
  'login/logout and route/tab changes must only resync through the same reconciler'
)
assert.match(
  indexPage,
  /scheduleMusicPlayerReconcile\('mounted'\)[\s\S]*?scheduleMusicPlayerReconcile\('mounted-idle'\)[\s\S]*?scheduleMusicPlayerReconcile\('first-interaction'\)/,
  'mount and first-interaction triggers must use the same music reconciler'
)

const nmpScript = await readFile(join(webRoot, 'public/assets/netease-mini-player/netease-mini-player-v2.js'), 'utf8')
assert.match(nmpScript, /async apiRequest\(endpoint, params = \{\}\)\s*\{\s*if \(this\.destroyed \|\| this\.config\.source === 'local'\) throw/, 'local players must never issue NetEase API requests')
assert.match(nmpScript, /const baseUrl = 'https:\/\/api\.hypcvgm\.top\/NeteaseMiniPlayer\/nmp\.php'[\s\S]*?fetch\(url, \{ signal: controller\.signal \}\)/, 'online NetEase requests must retain their endpoint and cancellation')
assert.match(nmpScript, /async loadCurrentSong\(\)[\s\S]*?if \(this\.config\.source === 'local'\) return this\.loadLocalSong\(this\.playlist\[this\.currentIndex\]\)/, 'local current-song playback must branch before online lookup')
assert.match(nmpScript, /this\.destroyed \|\| this\.config\.source === 'local' \|\| String\(this\.currentSong\?\.id\) !== String\(songId\)/, 'late online song responses must reject local or replaced songs')
assert.match(
  nmpScript,
  /async prepareLocalMedia\(url, controller, generation\)[\s\S]*?controller\.signal\.addEventListener\('abort', cancelRequest[\s\S]*?method: 'HEAD', credentials: 'same-origin', cache: 'no-store', signal: request\.signal/,
  'local preparation must probe media with HEAD and retain cancellation and same-origin credentials'
)
assert.match(
  nmpScript,
  /while \(!this\.destroyed && generation === this\.loadGeneration && !controller\.signal\.aborted\)/,
  'local media polling must stop when the player, source generation, or request is invalidated'
)
assert.match(
  nmpScript,
  /setMinimized\(minimized, userInitiated = false\)[\s\S]*?const shouldMinimize = minimized !== false[\s\S]*?this\.element\.classList\.toggle\('minimized', shouldMinimize\)/,
  'NMP minimized changes must be idempotent so pre-applied default-minimized classes are not toggled open during async init'
)
assert.match(
  nmpScript,
  /if \(this\.element\.classList\.contains\('minimized'\)\) \{[\s\S]*?this\.setMinimized\(false, true\)[\s\S]*?return/,
  'clicking the minimized CD must expand the whole music player instead of toggling an unused album-cover state'
)
assert.doesNotMatch(
  nmpScript,
  /albumCoverContainer\.classList\.toggle\('expanded'\)/,
  'minimized album clicks must not be swallowed by the unused expanded class'
)
assert.match(
  nmpScript,
  /if \(this\.config\.defaultMinimized && !this\.config\.embed && this\.config\.position !== 'static' && !this\.userMinimizeIntent\) \{[\s\S]*?this\.setMinimized\(true\)/,
  'NMP default-minimized startup must set the desired state directly without overriding a user click during async init'
)

console.log('public music player tests passed')
