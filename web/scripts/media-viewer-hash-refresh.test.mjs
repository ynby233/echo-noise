import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const messageList = await readFile(join(root, 'components/index/MessageList.vue'), 'utf8')
const homePage = await readFile(join(root, 'pages/index.vue'), 'utf8')
const fancyboxOptions = await readFile(join(root, 'utils/media-fancybox.ts'), 'utf8')
const hashUtils = await readFile(join(root, 'utils/message-route-hash.ts'), 'utf8')

assert.match(
  hashUtils,
  /export\s+const\s+getMessageIdFromRouteHash\s*=\s*\(hash: unknown\)[\s\S]*?\^#\\\/messages\\\/\(\\d\+\)/,
  'message route hash parsing must only accept #/messages/<numeric-id> hashes'
)

assert.match(
  fancyboxOptions,
  /Hash:\s*false/,
  'media Fancybox must not write gallery group hashes like #grid-... into the browser URL'
)

assert.match(
  messageList,
  /import\s+\{\s*getMessageIdFromRouteHash\s*\}\s+from\s+'~\/utils\/message-route-hash'/,
  'MessageList must use the shared strict message hash parser'
)

assert.doesNotMatch(
  messageList,
  /route\.hash\.split\('\/messages\/'\)|newHash\.split\('\/messages\/'\)|route\.hash\.includes\('\/messages\/'\)/,
  'MessageList must not treat arbitrary non-message hashes as message detail ids'
)

assert.match(
  messageList,
  /const\s+messageId\s*=\s*getMessageIdFromRouteHash\(route\.hash\)[\s\S]*?if\s*\(messageId\)\s*\{[\s\S]*?fetch\(`\$\{BASE_API\}\/messages\/\$\{messageId\}`/,
  'initial load must fetch a single message only when the hash is a strict message hash'
)

assert.match(
  messageList,
  /watch\(\(\) => route\.hash,\s*async\s*\(newHash\) => \{[\s\S]*?const\s+messageId\s*=\s*getMessageIdFromRouteHash\(newHash\)[\s\S]*?if\s*\(!messageId\)\s*\{/,
  'hash watcher must refresh the list for stale media hashes instead of fetching them as message ids'
)

assert.match(
  homePage,
  /getMessageIdFromRouteHash\(newHash\)/,
  'home page target-message sync must use the same strict message hash parser'
)

console.log('media viewer hash refresh tests passed')
