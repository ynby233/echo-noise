import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const panel = fs.readFileSync(path.join(root, 'components/index/StatusPanel.vue'), 'utf8')

assert.match(panel, /v-if="canManageVoceChatConfig"[^>]*class="[^"]*\badmin-subcard\b[^"]*"/, 'the runtime and VoceChat control panel must only render for the primary administrator')
assert.match(panel, /getRequest<any>\('admin\/runtime-policy'/, 'the panel must load the authoritative runtime policy endpoint')
assert.match(panel, /putRequest<any>\('admin\/runtime-policy\/mode'/, 'mode changes must use the dedicated primary-only endpoint')
assert.match(panel, /switchRuntimeMode\('local'\)/, 'the panel must offer an explicit local-mode action')
assert.match(panel, /switchRuntimeMode\('vocechat'\)/, 'the panel must offer an explicit VoceChat-mode action')
assert.match(panel, /runtimePolicy\.configuredMode/, 'the panel must display the configured mode')
assert.match(panel, /runtimePolicy\.runtimeState/, 'the panel must display the derived runtime state')
assert.match(panel, /postRequest<any>\(`admin\/runtime-policy\/provisioning\/\$\{command\}`/, 'the primary-only panel must call the dedicated provisioning commands')
assert.match(panel, /开始补建\/同步/, 'the panel must provide an explicit manual provisioning action')
assert.match(panel, /重试失败项/, 'the panel must provide an explicit failed-task retry action')
assert.match(panel, /待创建邮箱：\{\{ task\.candidate_email \}\}/, 'the panel must label persistent candidates as pending creation rather than bound email')
assert.match(panel, /runtimePolicy\.provisioningTasks/, 'the panel must render persistent per-user provisioning progress')
assert.match(panel, /runtimePolicy\.runtimeState !== 'vocechat_normal'/, 'provisioning actions must remain disabled outside normal VoceChat runtime')
assert.match(panel, /runtimePolicy\.provisioningRun\?\.status !== 'running'/, 'the panel must poll persistent progress only while a provisioning run is active')

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
