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
  'a PWA user must be able to copy the complete diagnostic without transcribing a screenshot',
)
for (const detail of ['注册错误', '缓存接口']) {
  assert.ok(component.includes(detail), `the copied diagnostic must expose ${detail} for cross-platform triage`)
}
assert.match(
  component,
  /iPhone 或 iPad.*设置.*通知.*允许通知/s,
  'Apple mobile permission guidance must cover both iPhone and iPad without assuming one device',
)
assert.match(
  component,
  /系统或浏览器.*设备的通知设置.*浏览器权限设置/s,
  'non-Apple guidance must cover both installed desktop PWAs and ordinary browser tabs',
)
assert.match(
  component,
  /:disabled="pushBusy"/,
  'a denied permission must not permanently disable the action that rechecks a restored system setting',
)
assert.equal(
  component.includes(':disabled="pushBusy || permissionDenied"'),
  false,
  'a stale denied state must not trap the user after changing system settings',
)
assert.ok(
  component.includes('独立应用模式：'),
  'cross-platform diagnostics must describe standalone app mode instead of assuming a Home Screen device',
)
assert.ok(
  component.includes('测试通知已加入发送队列'),
  'the test action must report the durable queue state that the API has actually confirmed',
)
assert.equal(
  component.includes("title: '测试通知已发送'"),
  false,
  'the UI must not claim provider delivery before the durable dispatcher has completed',
)

console.log('PWA push error surface test passed')
