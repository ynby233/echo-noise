import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const read = path => readFile(new URL(`../${path}`, import.meta.url), 'utf8')
const [panel, version] = await Promise.all([
  read('components/index/StatusPanel.vue'),
  read('components/admin/sections/VersionSection.vue'),
])

assert.doesNotMatch(panel, /当前版本:\s*\{\{/, 'the sidebar must not render its old version footer')
assert.match(version, /info\.currentVersion\s*=\s*String\(body\.data\?\.currentTag/, 'current version must use the repository tag')
assert.match(version, /onMounted\(\(\)\s*=>\s*\{[^}]*loadReleaseInfo\(\)/, 'repository version info must load when the section opens')

console.log('version display contract passed')
