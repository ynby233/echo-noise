import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const panel = fs.readFileSync(path.join(root, 'components/index/StatusPanel.vue'), 'utf8')

assert.match(panel, /v-if="canManageVoceChatConfig"[^>]*class="rounded-lg p-4 space-y-4"/, 'the runtime and VoceChat control panel must only render for the primary administrator')
assert.match(panel, /getRequest<any>\('admin\/runtime-policy'/, 'the panel must load the authoritative runtime policy endpoint')
assert.match(panel, /putRequest<any>\('admin\/runtime-policy\/mode'/, 'mode changes must use the dedicated primary-only endpoint')
assert.match(panel, /switchRuntimeMode\('local'\)/, 'the panel must offer an explicit local-mode action')
assert.match(panel, /switchRuntimeMode\('vocechat'\)/, 'the panel must offer an explicit VoceChat-mode action')
assert.match(panel, /runtimePolicy\.configuredMode/, 'the panel must display the configured mode')
assert.match(panel, /runtimePolicy\.runtimeState/, 'the panel must display the derived runtime state')

for (const retiredLabel of ['启用 VoceChat 集成', '登录校验', '本地备用登录', '联系人可见性', '通知推送']) {
  assert.ok(!panel.includes(`>${retiredLabel}</span>`), `retired independent switch label must be removed: ${retiredLabel}`)
}

const payloadStart = panel.indexOf('const buildVoceChatConfigPayload')
const payloadEnd = panel.indexOf('const saveVoceChatConfig', payloadStart)
assert.ok(payloadStart >= 0 && payloadEnd > payloadStart, 'VoceChat configuration payload builder must exist')
const payloadBuilder = panel.slice(payloadStart, payloadEnd)
for (const retiredKey of ['enabled:', 'loginVerificationEnabled:', 'localFallbackEnabled:', 'contactsEnabled:', 'notificationEnabled:']) {
  assert.ok(!payloadBuilder.includes(retiredKey), `credential/config save must not write the retired mode switch key ${retiredKey}`)
}

console.log('runtime policy panel contract tests passed')
