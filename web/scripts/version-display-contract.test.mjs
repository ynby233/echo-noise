import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const read = path => readFile(new URL(`../${path}`, import.meta.url), 'utf8')
const [panel, version] = await Promise.all([
  read('components/index/StatusPanel.vue'),
  read('components/admin/sections/VersionSection.vue'),
])

assert.doesNotMatch(panel, /当前版本:\s*\{\{/, 'the sidebar must not render its old version footer')
assert.match(version, /已安装版本/, 'installed version must be distinct from channel targets')
assert.match(version, /onMounted\(\(\)\s*=>\s*\{[^}]*checkChannels\(\)/, 'channel info must load when the section opens')
assert.match(version, /执行器未配置，只能检查更新/, 'unconfigured deployments must remain check-only')
assert.doesNotMatch(version, /EventSource|\/version\/update/, 'U1 must not retain the old self-update trigger or fallback')

console.log('version display contract passed')
