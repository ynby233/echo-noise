import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { runInNewContext } from 'node:vm'

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))))
const popupHTML = await readFile(join(repoRoot, 'chromeExpand/popup.html'), 'utf8')
const popupJS = await readFile(join(repoRoot, 'chromeExpand/js/popup.js'), 'utf8')
const extensionVisibility = await readFile(join(repoRoot, 'chromeExpand/js/visibility.js'), 'utf8')


assert.match(popupHTML, /<select id="visibilitySelect"[^>]*>[\s\S]*value="public"[\s\S]*value="users"[\s\S]*value="contacts"[\s\S]*value="private"/, 'extension must expose a four-state visibility selector')
assert.match(popupHTML, /<script src="js\/visibility\.js"><\/script>[\s\S]*<script src="js\/popup\.js"><\/script>/, 'extension visibility helper must load before popup code')
assert.match(popupJS, /EchoNoiseVisibility\.buildPublishPayload\(content, visibility/, 'extension publish paths must share the executable payload builder')
assert.match(popupJS, /messageVisibility = "public"/, 'extension must preserve the legacy public default')

const extensionContext = {}
runInNewContext(extensionVisibility, extensionContext)
for (const visibility of ['public', 'users', 'contacts', 'private']) {
  const payload = extensionContext.EchoNoiseVisibility.buildPublishPayload('extension payload', visibility, { notify: true })
  assert.deepEqual(
    JSON.parse(JSON.stringify(payload)),
    { content: 'extension payload', visibility, private: visibility !== 'public', notify: true },
    `extension payload for ${visibility} must preserve the four-state contract`
  )
}
assert.equal(extensionContext.EchoNoiseVisibility.normalizeVisibility('invalid'), 'public', 'extension invalid visibility must fail back to public')


console.log('external visibility client executable payload tests passed')
