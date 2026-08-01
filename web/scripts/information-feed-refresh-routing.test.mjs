import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { pathToFileURL, fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const read = (path) => readFile(join(webRoot, path), 'utf8')

const { resolveInfoFeedLink, getInfoFeedLinkHost } = await import(
  pathToFileURL(join(webRoot, 'utils/info-feed-link.ts')).href
)

assert.equal(
  resolveInfoFeedLink('/#/messages/233', 'http://192.168.220.218:27184'),
  'http://192.168.220.218:27184/#/messages/233',
  'local feed links must use the origin through which the visitor opened the site',
)
assert.equal(
  getInfoFeedLinkHost('/#/messages/233', 'https://notes.example.com'),
  'notes.example.com',
  'the displayed feed-link host must match the visitor-facing host',
)
assert.equal(
  resolveInfoFeedLink('https://external.example/messages/9', 'https://notes.example.com'),
  'https://external.example/messages/9',
  'external source links must keep their own origin',
)

const [feed, home, messageList, routerSource] = await Promise.all([
  read('components/index/InfoFeedList.vue'),
  read('pages/index.vue'),
  read('components/index/MessageList.vue'),
  read('../internal/routers/routers.go'),
])

assert.match(
  routerSource,
  /api\.POST\("\/feed\/refresh", middleware\.SessionAuthMiddleware\(\), middleware\.(?:AdminAuthMiddleware\(\)|RequireCapability\(authorization\.CapabilityFeedManage\)), controllers\.RefreshInfoFeedItems\)/,
  'manual source refresh must require authenticated capability authorization',
)
assert.match(
  home,
  /<button\s+v-if="isAdmin"[\s\S]*?aria-label="刷新信息流"[\s\S]*?@click="refreshInfoFeed"/,
  'the upstream-refresh action must only be shown to administrators',
)
assert.match(
  feed,
  /const loadFeed = async \(options: \{ force\?: boolean \} = \{\}\)[\s\S]*?options\.force \? '\/feed\/refresh' : '\/feed\/items'[\s\S]*?method: options\.force \? 'POST' : 'GET'/,
  'the refresh action must request a server-side source refresh instead of rereading the snapshot',
)
assert.match(feed, /refreshFeed: \(\) => loadFeed\(\{ force: true \}\)/)
assert.match(feed, /resolveInfoFeedLink\(item\.link, browserOrigin\.value\)/)
assert.match(
  feed,
  /if \(activeFeedRequest\)[\s\S]*?if \(!options\.force\) return activeFeedRequest[\s\S]*?await activeFeedRequest/,
  'a manual refresh must wait for an incidental snapshot read and then still execute its POST refresh',
)

assert.match(
  home,
  /const clearMessageRouteHash = async \(\)[\s\S]*?getMessageIdFromRouteHash\(route\.hash\)[\s\S]*?router\.replace\([\s\S]*?hash: ''/,
  'home interactions need one shared message-deep-link cleanup path',
)
for (const handler of ['switchActiveTab', 'handleCalendarDateSelect', 'handleSearchResult', 'handleTagClick']) {
  assert.match(
    home,
    new RegExp(`const ${handler} = async \\([^)]*\\)[\\s\\S]*?await clearMessageRouteHash\\(\\)`),
    `${handler} must clear a stale message deep link before changing the main view`,
  )
}
assert.match(
  messageList,
  /watch\(\(\) => route\.hash,\s*async\s*\(newHash, oldHash\)[\s\S]*?getMessageIdFromRouteHash\(oldHash\)[\s\S]*?await refreshList\(\)/,
  'leaving a message deep link must restore the normal list rather than keeping the single-message snapshot',
)

console.log('information feed refresh/link/routing contract passed')
