import assert from 'node:assert/strict'
import fs from 'node:fs'

const read = (path) => fs.readFileSync(new URL(path, import.meta.url), 'utf8')
const exists = (path) => fs.existsSync(new URL(path, import.meta.url))

const nuxtConfig = read('../nuxt.config.ts')
const pwaPlugin = read('../plugins/pwa.client.ts')
const statusPanel = read('../components/index/StatusPanel.vue')
const homePage = read('../pages/index.vue')

assert.match(nuxtConfig, /['"]@vite-pwa\/nuxt['"]/, 'Nuxt must enable the maintained PWA module')
assert.match(nuxtConfig, /strategies:\s*['"]injectManifest['"]/, 'PWA must build the single custom service worker with injectManifest')
assert.match(nuxtConfig, /manifest:\s*false/, 'the Go dynamic manifest must remain the only manifest authority')
assert.match(nuxtConfig, /injectRegister:\s*false/, 'service worker registration must be owned by the PWA manager')
assert.match(nuxtConfig, /__PWA_BUILD_ID__/, 'the service worker build must receive an explicit build identity')

assert.equal(exists('../public/manifest.json'), false, 'the retired static manifest.json must not remain')
assert.equal(exists('../public/manifest.webmanifest'), false, 'the retired static manifest.webmanifest must not remain')
assert.equal(exists('../server/routes/manifest.webmanifest.get.ts'), false, 'the retired Nuxt manifest route must not remain')
assert.equal(exists('../public/sw.js'), false, 'the retired public service worker must not remain')
assert.equal(exists('../server/routes/sw.js.get.ts'), false, 'the retired Nuxt service worker route must not remain')
assert.doesNotMatch(statusPanel, /navigator\.serviceWorker|rel:\s*['"]manifest['"]/, 'admin settings must delegate PWA runtime work to the single manager')
assert.doesNotMatch(homePage, /rel:\s*['"]manifest['"]/, 'the page must not create a second manifest link')

const serviceWorker = read('../service-worker/sw.ts')
for (const required of ['precacheAndRoute', "addEventListener('push'", "addEventListener('notificationclick'", 'setAppBadge', 'SKIP_WAITING']) {
  assert.ok(serviceWorker.includes(required), `service worker must contain ${required}`)
}
assert.ok(serviceWorker.includes('__PWA_BUILD_ID__'), 'cache names must carry the deployed build identity')

for (const required of ['virtual:pwa-register', 'beforeinstallprompt', '/api/web-push/subscriptions', '/api/web-push/preferences', '/api/web-push/test']) {
  assert.ok(pwaPlugin.includes(required), `PWA manager must contain ${required}`)
}
assert.ok(pwaPlugin.includes('subscriptionUsesPublicKey'), 'the manager must recover from VAPID public-key rotation')

assert.ok(exists('../public/offline.html'), 'the minimal offline status page must exist')
assert.ok(serviceWorker.includes("matchPrecache('/offline')"), 'the offline fallback must use the route emitted by static generation')
assert.ok(exists('../components/index/PwaPushSettings.vue'), 'logged-in users need push preference controls')
assert.ok(exists('../components/index/PwaRuntimeNotices.vue'), 'install, update, and offline guidance must be mounted once')

console.log('PWA runtime contract passed')
