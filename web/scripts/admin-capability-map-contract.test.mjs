import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const panel = await readFile(new URL('../components/index/StatusPanel.vue', import.meta.url), 'utf8')
const map = JSON.parse(await readFile(new URL('../config/admin-section-capabilities.json', import.meta.url), 'utf8'))
const authorizationSource = await readFile(new URL('../../internal/authorization/authorization.go', import.meta.url), 'utf8')

const sectionType = panel.match(/type AdminSectionKey =([\s\S]*?)const activeSection/)
assert.ok(sectionType, 'StatusPanel must declare the admin section key union')
const sections = [...sectionType[1].matchAll(/'([^']+)'/g)].map((match) => match[1])
const intentionallyUnprotected = new Set([
  'dashboard', 'user', 'hitokoto', 'life-countdown', 'widgets',
  'system-push', 'personal-notes', 'personal-note-recycle-bin', 'personal-interactions', 'personal-interaction-recycle-bin',
])
const primaryAdminOnly = new Set(['site-rss'])

for (const section of sections) {
  if (intentionallyUnprotected.has(section)) continue
  if (primaryAdminOnly.has(section)) {
    assert.equal(map[section], undefined, `primary-admin-only section ${section} must not be capability-mapped`)
    continue
  }
  assert.equal(typeof map[section], 'string', `admin section ${section} must have an explicit capability mapping`)
}
for (const [section, capability] of Object.entries(map)) {
  assert.match(section, /^[a-z][a-z0-9-]*$/, `invalid admin section key ${section}`)
  assert.match(capability, /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/, `invalid capability mapping for ${section}`)
}

const declaredCapabilities = new Set(
  [...authorizationSource.matchAll(/Capability\w+\s+Capability\s*=\s*"([^"]+)"/g)].map((match) => match[1])
)
for (const [section, capability] of Object.entries(map)) {
  assert.ok(declaredCapabilities.has(capability), `${section} maps to undeclared capability ${capability}`)
}

assert.match(panel, /import adminSectionCapabilities from '~\/config\/admin-section-capabilities\.json'/)
assert.match(panel, /const sectionCapabilities: Partial<Record<AdminSectionKey, string>> = adminSectionCapabilities/)
assert.doesNotMatch(panel, /const sectionCapabilities: Partial<Record<AdminSectionKey, string>> = \{[\s\S]*?\}/)
assert.equal(map.authorization, 'authorization.manage')
assert.equal(map['admin-audit'], 'audit.view')
assert.equal(map['site-feed'], 'feed.view')
assert.equal(map['site-rss'], undefined)
assert.doesNotMatch(authorizationSource, /CapabilityRSS(?:View|Manage)|rss\.(?:view|manage)/)
assert.match(panel, /if \(section === 'site-rss'\) return isPrimaryAdmin\.value/)
assert.match(panel, /isPrimaryAdmin\.value \? \[\{ key: 'site-rss' as AdminSectionKey/)
assert.match(panel, /id="site-rss-section" v-if="isPrimaryAdmin && isSectionVisible\('site-rss'\)"/)
assert.match(panel, /const stripRSSManagementSettings = \(settings: Record<string, any>\)/)
assert.match(panel, /frontendSettings: stripRSSManagementSettings\(frontendConfig as any\)/)
