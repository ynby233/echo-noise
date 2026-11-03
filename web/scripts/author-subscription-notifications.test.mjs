import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

const read = path => readFile(new URL(path, import.meta.url), 'utf8')
const [center, settings, plugin] = await Promise.all([
  read('../components/index/UserNotificationCenter.vue'),
  read('../components/index/PwaPushSettings.vue'),
  read('../plugins/pwa.client.ts'),
])

// Select real declarations with the TypeScript parser so these tests execute
// production logic without mounting unrelated UI or browser services.
const declaration = (source, name) => {
  const script = source.includes('<script setup') ? source.split('<script setup lang="ts">')[1].split('</script>')[0] : source
  const file = ts.createSourceFile('fixture.ts', script, ts.ScriptTarget.Latest, true)
  let result
  const visit = node => {
    if (ts.isVariableDeclaration(node) && node.name.getText(file) === name) result = `const ${node.getText(file)}`
    ts.forEachChild(node, visit)
  }
  visit(file)
  assert.ok(result, `production declaration ${name} must exist`)
  return result
}
const load = (source, names, dependencies = {}) => {
  const code = names.map(name => declaration(source, name)).join('\n')
  const compiled = ts.transpileModule(code, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None },
  }).outputText
  return new Function(...Object.keys(dependencies), `${compiled}; return { ${names.join(', ')} }`)(...Object.values(dependencies))
}

const copy = load(center, ['normalizeText', 'truncate', 'actorName', 'isTargetUnavailable', 'actorContent', 'targetText', 'targetFallbackText', 'targetAriaLabel', 'unavailableTitle'])
const note = { type: 'author_subscription', actor: { username: '作者甲' }, message: { content: '<p>新的笔记</p>' }, comment: { content: '无关评论' } }
assert.equal(copy.actorName(note), '作者甲')
assert.equal(copy.actorName({ type: 'author_subscription' }), '订阅的作者')
assert.equal(copy.actorContent(note), '你订阅的作者发布了新笔记')
assert.equal(copy.targetText(note), '新的笔记')
assert.equal(copy.targetFallbackText(note), '查看订阅作者的新笔记')
assert.equal(copy.targetAriaLabel(note), '查看订阅作者的新笔记位置')
assert.equal(copy.unavailableTitle(note), '订阅作者的新笔记暂不可用')
assert.equal(copy.actorContent({ ...note, target_status: 'deleted' }), '')
assert.match(center, /v-if="item.type === 'author_subscription'"[^>]*>发布了新笔记<\/span>/)

const preferences = { value: {} }
let savedResponse = {}
let requestBody
const { defaultPreferences, savePreferences } = load(plugin, ['defaultPreferences', 'savePreferences'], {
  preferences,
  fetchJSON: async (url, options) => {
    assert.equal(url, '/api/web-push/preferences')
    assert.equal(options.method, 'PUT')
    requestBody = JSON.parse(options.body)
    return savedResponse
  },
})
assert.equal(defaultPreferences().subscription_enabled, true)
await savePreferences({ comment_enabled: false })
assert.equal(requestBody.subscription_enabled, true, 'older callers must serialize the new preference as enabled')
assert.equal(requestBody.comment_enabled, false)
assert.equal(preferences.value.subscription_enabled, true, 'older server responses must default to enabled')
savedResponse = { subscription_enabled: false }
await savePreferences({ subscription_enabled: false })
assert.equal(requestBody.subscription_enabled, false, 'explicit opt-out must survive serialization')
assert.equal(preferences.value.subscription_enabled, false, 'explicit server opt-out must survive loading')

// Execute the actual config preference assignment for older and current servers.
const assignment = plugin.match(/preferences\.value = .*data\?\.preferences.*$/m)?.[0]
assert.ok(assignment, 'push configuration must load server preferences')
const applyConfig = new Function('data', 'preferences', 'defaultPreferences', assignment)
for (const data of [{}, { preferences: {} }, { preferences: { subscription_enabled: false } }, { preferences: { subscription_enabled: true } }]) {
  applyConfig(data, preferences, defaultPreferences)
  assert.equal(preferences.value.subscription_enabled, data.preferences?.subscription_enabled ?? true)
}

const pwa = { preferences: { value: {} } }
const ui = load(settings, ['draft', 'preferenceOptions', 'changed', 'copyPreferences'], {
  pwa,
  reactive: value => value,
  computed: getter => ({ get value() { return getter() } }),
})
// Keep the old plugin's other preferences in sync while omitting the new key.
pwa.preferences.value = { ...ui.draft }
delete pwa.preferences.value.subscription_enabled
ui.copyPreferences()
assert.equal(ui.draft.subscription_enabled, true)
assert.equal(ui.changed.value, false, 'a missing plugin preference must not create a phantom unsaved change')
ui.draft.subscription_enabled = false
assert.equal(ui.changed.value, true)
ui.copyPreferences()
assert.equal(ui.draft.subscription_enabled, true, 'copying an old plugin must reset a previous opt-out to the default')
pwa.preferences.value.subscription_enabled = false
ui.copyPreferences()
assert.equal(ui.draft.subscription_enabled, false)
assert.equal(ui.changed.value, false)
const option = ui.preferenceOptions.find(option => option.key === 'subscription_enabled')
assert.equal(option?.label, '订阅作者的新笔记')
assert.match(option.description, /仅停止浏览器推送/)
assert.match(option.description, /保留作者订阅和站内通知/)

console.log('author subscription notification tests passed')
