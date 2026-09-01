import assert from 'node:assert/strict'
import fs from 'node:fs'

const componentURL = new URL('../components/index/PwaPushSettings.vue', import.meta.url)
const component = fs.readFileSync(componentURL, 'utf8')

assert.match(
  component,
  /v-if="actionError"[^>]*class="push-message is-error push-action-error"[^>]*role="alert"/,
  'a failed push action must remain visible in the settings panel instead of only appearing in a transient toast',
)
assert.match(
  component,
  /actionError\.value\s*=\s*message/,
  'the exact staged Push API failure must be retained for real-device diagnosis',
)
assert.match(
  component,
  /if \(actionError\.value\) return `失败详情：\$\{actionError\.value\}`/,
  'the staged failure must also replace the generic status description so it cannot be overlooked',
)
assert.match(
  component,
  /复制错误详情[\s\S]*navigator\.clipboard\.writeText\(diagnosticText\.value\)/,
  'an iPad user must be able to copy the complete diagnostic without transcribing a screenshot',
)
for (const detail of ['注册错误', '缓存接口']) {
  assert.ok(component.includes(detail), `the copied diagnostic must expose ${detail} for iPadOS Lockdown Mode triage`)
}
assert.match(
  component,
  /iPad 设置.*通知.*允许通知.*主屏幕/s,
  'an iPad permission denial must point to iPadOS settings rather than nonexistent browser address-bar controls',
)

console.log('PWA push error surface test passed')
