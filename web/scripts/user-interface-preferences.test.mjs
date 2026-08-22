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

assert.match(models, /type UserFrontendPreference struct \{[\s\S]*?HitokotoEnabled\s+\*bool[\s\S]*?HomeStatsEnabled\s+\*bool[\s\S]*?HeatmapEnabled\s+\*bool/, 'all account widget preferences must use nullable per-user fields')
assert.ok((migrate.match(/&UserFrontendPreference\{\}/g) || []).length >= 3, 'all database migrations must include user frontend preferences')
assert.match(service, /userFrontendPreferenceSettingKeys[\s\S]*?"hitokotoEnabled"/, 'daily quote must be an allowed ordinary-user preference')
assert.match(service, /func IsUserFrontendSettingsOnly/, 'ordinary-user settings payloads must be restricted to personal interface fields')
assert.match(service, /type WidgetVisibilitySettings struct[\s\S]*?LatestGalleryEnabled[\s\S]*?HeatmapEnabled/, 'the seven widgets must share one visibility value object')
assert.match(service, /func resolveWidgetVisibilitySettings[\s\S]*?DefaultAccountWidgetVisibilitySettings/, 'frontend config must resolve the current viewer widget preferences')
assert.match(controllers, /func UpdateWidgetPreferences[\s\S]*?UpdateUserWidgetPreferences\(user\.ID/, 'personal widget saves must target only the authenticated user')
assert.match(controllers, /func UpdateGuestWidgetPreferences[\s\S]*?requirePrimaryAdmin/, 'guest defaults must be protected by the fixed primary-admin identity')
assert.ok(statusPanel.indexOf('const userStore = useUserStore()') < statusPanel.indexOf('const adminNavGroups = computed'), 'user store must be initialized before navigation computed state is evaluated')
assert.ok(statusPanel.indexOf('const isAdmin = computed') < statusPanel.indexOf('const adminNavGroups = computed'), 'admin role state must be initialized before navigation computed state is evaluated')
assert.ok(statusPanel.indexOf('const frontendConfig = reactive') < statusPanel.indexOf('() => !!(frontendConfig as any).notifyEnabled'), 'frontend config must be initialized before its watcher is evaluated')
assert.match(statusPanel, /\{ key: 'widgets', label: '小组件'/, 'all logged-in users must have the unified widget entry')
assert.match(statusPanel, /我的小组件[\s\S]*?访客默认/, 'primary-admin UI must separate personal widgets from guest defaults')
assert.match(statusPanel, /\/admin\/guest-widget-preferences[\s\S]*?\/user\/widget-preferences/, 'frontend must use separate personal and guest-default save routes')
assert.match(home, /v-if="frontendConfig\.hitokotoEnabled"[^>]*left-widget-hitokoto-card/, 'home page must render daily quote from the viewer-resolved setting')

console.log('user interface preference contract checks passed')
