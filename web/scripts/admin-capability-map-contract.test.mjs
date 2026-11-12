import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { readAdminPanelSource } from './admin-panel-source.mjs'

const registry = await readFile(new URL('../components/admin/sections/registry.ts', import.meta.url), 'utf8')
const panel = await readAdminPanelSource()
const map = JSON.parse(await readFile(new URL('../config/admin-section-capabilities.json', import.meta.url), 'utf8'))
const authorizationSource = await readFile(new URL('../../internal/authorization/authorization.go', import.meta.url), 'utf8')

const sectionType = registry.match(/type AdminSectionKey =([\s\S]*?)export type AdminSectionModule/)
assert.ok(sectionType, 'the lazy section registry must declare the admin section key union')
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
assert.equal(map['site-music'], 'music.view')
assert.equal(map['site-feed'], 'feed.view')
assert.equal(map['site-rss'], undefined)
assert.doesNotMatch(authorizationSource, /CapabilityRSS(?:View|Manage)|rss\.(?:view|manage)/)
assert.match(panel, /if \(section === 'site-rss'\) return isPrimaryAdmin\.value/)
assert.match(panel, /isPrimaryAdmin\.value \? \[\{ key: 'site-rss' as AdminSectionKey/)
assert.match(registry, /'site-rss': \(\) => import\('\.\/RssSection\.vue'\)/)
assert.match(panel, /if \(section === 'site-rss'\) return isPrimaryAdmin\.value/)
assert.match(panel, /'site-rss': \(\) => import\('\.\/RssSection\.vue'\)/)
assert.match(panel, /'site-music': \(\) => import\('\.\/MusicSection\.vue'\)/)

assert.equal(map.storage, 'storage.view')
assert.equal(map['cloud-backup'], 'storage.view', 'legacy storage mapping must still have a separate primary-only navigation gate')
assert.equal(map['backup-data'], 'backup.data.view')
assert.equal(map['backup-attachments'], 'backup.attachments.view')
assert.equal(map['backup-server-config'], 'backup.server_config.view')
assert.equal(map.db, undefined, 'legacy database capabilities must not expose a navigation entry')

const canSectionBody = panel.match(/const canSection = \(section: AdminSectionKey\) => \{([\s\S]*?)\n\}/)?.[1]
assert.ok(canSectionBody, 'navigation access predicate must be available for behavioral coverage')
const evaluateSection = new Function('section', 'isAdmin', 'isPrimaryAdmin', 'sectionCapabilities', 'can', canSectionBody)
const allowed = (section, grants = [], primary = false, admin = true) => evaluateSection(section, { value: admin }, { value: primary }, map, capability => primary || grants.includes(capability))
for (const section of ['storage', 'backup-data', 'backup-attachments']) {
  assert.equal(allowed(section), false, `${section} requires its own view capability`)
  assert.equal(allowed(section, [map[section]]), true, `${section} is accessible with its view capability`)
  assert.equal(allowed(section, [map[section]], false, false), false, `${section} remains an admin context`)
}
for (const section of ['cloud-backup', 'backup-server-config']) {
  assert.equal(allowed(section, Object.values(map)), false, `${section} must remain hidden even with every delegated grant`)
  assert.equal(allowed(section, [], true), true, `${section} is available to the primary administrator`)
}
for (const section of ['backup-data', 'backup-attachments']) {
  for (const action of ['create', 'download', 'restore', 'plan', 'history']) {
    assert.equal(allowed(section, [map[section].replace(/view$/, action)]), false, `${action} cannot bypass the ${section} view prerequisite`)
  }
}
