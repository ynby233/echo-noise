import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const repoRoot = dirname(webRoot)
const statusPanel = await readFile(join(webRoot, 'components/index/StatusPanel.vue'), 'utf8')
const settingDTO = await readFile(join(repoRoot, 'internal/dto/setting.go'), 'utf8')
const settingController = await readFile(join(repoRoot, 'internal/controllers/controllers.go'), 'utf8')

for (const configuredFlag of [
  'githubClientSecretConfigured',
  'smtpUserConfigured',
  'smtpPassConfigured',
  'accessKeyConfigured',
  'secretKeyConfigured'
]) {
  assert.match(statusPanel, new RegExp(configuredFlag), `admin credential UI must consume ${configuredFlag}`)
}

for (const clearFlag of [
  'clearGithubClientSecret',
  'clearSmtpUser',
  'clearSmtpPass',
  'clearAccessKey',
  'clearSecretKey'
]) {
  assert.match(statusPanel, new RegExp(clearFlag), `admin credential UI must submit ${clearFlag}`)
}

assert.match(statusPanel, /已配置；留空将保持不变/, 'redacted credentials must explain blank-save preservation')
assert.match(statusPanel, /清除现有 Secret/, 'GitHub and storage secrets must have an explicit clear action')
assert.match(statusPanel, /清除现有用户名/, 'SMTP username must have an explicit clear action')
assert.match(statusPanel, /清除现有密码/, 'SMTP password must have an explicit clear action')
assert.match(statusPanel, /await loadSmtp()/, 'SMTP save must refresh configured-state flags')
assert.match(statusPanel, /await loadStorageConfig()/, 'backup storage save must refresh configured-state flags')
assert.match(statusPanel, /await loadAttachmentStorageConfig()/, 'attachment storage save must refresh configured-state flags')
assert.ok(
  statusPanel.includes('const hasUser = (!!smtp.user || smtp.userConfigured) && !smtp.clearUser') &&
    statusPanel.includes('const hasPass = (!!smtp.pass || smtp.passConfigured) && !smtp.clearPass'),
  'SMTP testing must accept redacted credentials that are already configured'
)
assert.match(settingDTO, /json:"clearSmtpUser"/, 'SMTP username clear intent must survive JSON binding')
assert.match(settingDTO, /json:"clearSmtpPass"/, 'SMTP password clear intent must survive JSON binding')
assert.match(settingController, /settingMap\["clearSmtpUser"\]/, 'SMTP username clear intent must reach the settings service')
assert.match(settingController, /settingMap\["clearSmtpPass"\]/, 'SMTP password clear intent must reach the settings service')

console.log('credential redaction admin UI checks passed')
