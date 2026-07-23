import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))))
const read = (path) => readFile(join(repoRoot, path), 'utf8')
const [models, migrate, service, controllers, statusPanel, home] = await Promise.all([
  read('internal/models/models.go'),
  read('internal/models/migrate.go'),
  read('internal/services/setting_service.go'),
  read('internal/controllers/controllers.go'),
  read('web/components/index/StatusPanel.vue'),
  read('web/pages/index.vue'),
])

assert.match(models, /type UserFrontendPreference struct \{[\s\S]*?UserID\s+uint[\s\S]*?HitokotoEnabled\s+\*bool/, 'daily quote preference must use a nullable per-user record')
assert.ok((migrate.match(/&UserFrontendPreference\{\}/g) || []).length >= 3, 'all database migrations must include user frontend preferences')
assert.match(service, /userFrontendPreferenceSettingKeys[\s\S]*?"hitokotoEnabled"/, 'daily quote must be an allowed ordinary-user preference')
assert.match(service, /func IsUserFrontendSettingsOnly/, 'ordinary-user settings payloads must be restricted to personal interface fields')
assert.match(service, /func UpdateUserFrontendPreferenceConfig/, 'daily quote must have a user-scoped persistence path')
assert.match(service, /hitokotoEnabled := resolveHitokotoEnabled\(db, viewerUserID, config\)/, 'frontend config must resolve the current viewer daily quote preference')
assert.match(controllers, /!services\.IsUserFrontendSettingsOnly\(frontendSettings\)/, 'ordinary users must still be blocked from site-wide settings')
assert.match(controllers, /services\.UpdateUserFrontendPreferenceConfig\(user\.ID, frontendSettings\)/, 'ordinary-user daily quote saves must target their own preference')
assert.ok(statusPanel.indexOf('const userStore = useUserStore()') < statusPanel.indexOf('const adminNavGroups = computed'), 'user store must be initialized before navigation computed state is evaluated')
assert.ok(statusPanel.indexOf('const isAdmin = computed') < statusPanel.indexOf('const adminNavGroups = computed'), 'admin role state must be initialized before navigation computed state is evaluated')
assert.ok(statusPanel.indexOf('const frontendConfig = reactive') < statusPanel.indexOf('() => !!(frontendConfig as any).notifyEnabled'), 'frontend config must be initialized before its watcher is evaluated')
assert.match(statusPanel, /\{ key: 'hitokoto', label: '随机一言'/, 'ordinary-user backend navigation must expose the daily quote page')
assert.match(statusPanel, /!isAdmin[\s\S]*?isSectionVisible\('hitokoto'\)/, 'daily quote panel must be reachable in the ordinary-user backend')
assert.match(statusPanel, /仅控制当前账号访问首页时是否显示随机一言/, 'ordinary-user UI must explain that the setting is personal')
assert.match(home, /v-if="frontendConfig\.hitokotoEnabled"[^>]*left-widget-hitokoto-card/, 'home page must render daily quote from the viewer-resolved setting')

console.log('user interface preference contract checks passed')
