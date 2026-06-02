import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const repoRoot = dirname(webRoot)

const component = await readFile(join(webRoot, 'components/index/StatusPanel.vue'), 'utf8')
const models = await readFile(join(repoRoot, 'internal/models/models.go'), 'utf8')
const settingsService = await readFile(join(repoRoot, 'internal/services/setting_service.go'), 'utf8')
const loginController = await readFile(join(repoRoot, 'internal/controllers/controllers.go'), 'utf8')
const authMiddleware = await readFile(join(repoRoot, 'internal/middleware/auth.go'), 'utf8')

assert.match(
  models,
  /LoginExpireDays\s+int\s+`gorm:"default:3"`[\s\S]*?LoginExpireHours\s+int\s+`gorm:"default:0"`/,
  'site config model must persist login expiry as days plus hours'
)

assert.match(
  settingsService,
  /"loginExpireDays"[\s\S]*?normalizeLoginExpireConfig\(config\.LoginExpireDays, config\.LoginExpireHours\)[\s\S]*?"loginExpireHours"/,
  'frontend config API must return normalized loginExpireDays and loginExpireHours'
)

assert.match(
  settingsService,
  /parsePositiveIntSetting\(frontendSettings\["loginExpireDays"\]\)[\s\S]*?parsePositiveIntSetting\(frontendSettings\["loginExpireHours"\]\)[\s\S]*?normalizeLoginExpireConfig\(loginExpireDays, loginExpireHours\)/,
  'settings save path must accept and normalize both login expiry fields'
)

assert.match(
  loginController,
  /func\s+getLoginExpireDuration\(\)\s+time\.Duration\s+\{[\s\S]*?normalizeLoginExpireConfig\(cfg\.LoginExpireDays, cfg\.LoginExpireHours\)[\s\S]*?time\.Duration\(days\)\*24 \+ time\.Duration\(hours\)/,
  'login session expiry duration must be calculated from days plus hours'
)

assert.match(
  loginController,
  /func\s+applyLoginSessionExpire\(session sessions\.Session, user \*models\.User\)[\s\S]*?user\.IsAdmin[\s\S]*?login_expire_at", int64\(0\)[\s\S]*?time\.Now\(\)\.Add\(getLoginExpireDuration\(\)\)/,
  'ordinary users should receive a fixed login expiry while administrators are exempt'
)

assert.match(
  authMiddleware,
  /sessionExpired := userID != nil && !isSessionAdmin && expireAt > 0 && now > expireAt/,
  'session expiry must only clear ordinary user sessions'
)
assert.match(
  authMiddleware,
  /Bearer Token 回退仅保留给管理员[\s\S]*?authenticateAdminBearerToken\(ctx\)/,
  'session middleware must restrict bearer fallback to administrators'
)
assert.match(
  authMiddleware,
  /token = strings\.TrimSpace\(token\)[\s\S]*?!user\.IsAdmin/,
  'bearer fallback helper must reject ordinary user tokens'
)

assert.match(
  component,
  /v-model\.number="frontendConfig\.loginExpireDays"[\s\S]*?>天<[\s\S]*?v-model\.number="frontendConfig\.loginExpireHours"[\s\S]*?>小时</,
  'admin UI must expose both days and hours inputs'
)
assert.match(
  component,
  /loginExpirePresetOptions\s*=\s*\[[\s\S]*?label: '1 小时'[\s\S]*?label: '12 小时'[\s\S]*?label: '7 天'/,
  'login expiry quick presets must include hour-level and day-level options'
)
assert.match(
  component,
  /key === 'loginExpireDays'[\s\S]*?normalizeLoginExpireConfig\([\s\S]*?loginExpireDays: \(frontendConfig as any\)\.loginExpireDays[\s\S]*?loginExpireHours: \(frontendConfig as any\)\.loginExpireHours/,
  'saving login expiry must send both days and hours'
)
assert.match(
  component,
  /const loginExpire = normalizeLoginExpireConfig\([\s\S]*?loginExpireDays: loginExpire\.days[\s\S]*?loginExpireHours: loginExpire\.hours/,
  'saving the full site config must keep both login expiry fields normalized'
)

console.log('login expire tests passed')
