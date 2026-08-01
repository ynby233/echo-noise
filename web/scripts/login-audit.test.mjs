import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const repoRoot = dirname(webRoot)

const component = await readFile(join(webRoot, 'components/index/StatusPanel.vue'), 'utf8')
const securityController = await readFile(join(repoRoot, 'internal/controllers/security.go'), 'utf8')
const loginController = await readFile(join(repoRoot, 'internal/controllers/controllers.go'), 'utf8')
const routes = await readFile(join(repoRoot, 'internal/routers/routers.go'), 'utf8')
const securityModel = await readFile(join(repoRoot, 'internal/models/security.go'), 'utf8')
const migrate = await readFile(join(repoRoot, 'internal/models/migrate.go'), 'utf8')

assert.match(
  securityModel,
  /type\s+SecurityLoginAudit\s+struct\s*\{[\s\S]*?UserID\s+uint[\s\S]*?Username\s+string[\s\S]*?Action\s+string[\s\S]*?IP\s+string[\s\S]*?UserAgent\s+string[\s\S]*?\}/,
  'login audit model must store user id, username, action, IP, and user agent'
)

assert.match(
  migrate,
  /AutoMigrate\([\s\S]*?&SecurityLoginAudit\{\}[\s\S]*?\)/,
  'login audit table must be included in database migration'
)

assert.match(
  securityController,
  /func\s+recordUserLoginAudit\(c \*gin\.Context, user \*models\.User, action string\) error \{[\s\S]*?user\s*==\s*nil\s*\|\|\s*user\.ID\s*==\s*0\s*\|\|\s*user\.IsAdmin[\s\S]*?Action:\s*normalizeLoginAuditAction\(action\)[\s\S]*?c\.ClientIP\(\)[\s\S]*?db\.Create\(&audit\)/,
  'login audit recording must normalize the action, skip administrators, and use gin ClientIP() for ordinary users'
)

assert.match(
  loginController,
  /session\.Save\(\)[\s\S]*?_\s*=\s*recordUserLoginAudit\(c,\s*user,\s*loginAuditActionLogin\)[\s\S]*?c\.JSON\(http\.StatusOK,\s*dto\.OK\(user,\s*"登录成功"\)\)/,
  'successful password login should record the login audit after the session is saved and before responding'
)

const auditRecordCalls = loginController.match(/recordUserLoginAudit\(c,\s*\*?user,\s*loginAuditActionLogin\)/g) || []
assert.ok(
  auditRecordCalls.length >= 2,
  'password login and GitHub callback should both record successful ordinary-user login audits'
)

assert.match(
  loginController,
  /func\s+Logout\(c \*gin\.Context\)[\s\S]*?recordSessionLogoutAudit\(c,\s*session\)[\s\S]*?session\.Clear\(\)/,
  'logout should record the audit before clearing the session'
)

assert.match(
  loginController,
  /recordUserLoginAudit\(c,\s*&user,\s*loginAuditActionLogout\)/,
  'logout should record ordinary-user logout audits with the logout action'
)

assert.match(
  securityController,
  /func\s+GetLoginAudits\(c \*gin\.Context\)[\s\S]*?Limit\(limit\)[\s\S]*?Query\("username"\)[\s\S]*?Query\("ip"\)[\s\S]*?Query\("action"\)[\s\S]*?Find\(&audits\)/,
  'admin login audit API should list recent audits and support username/IP/action filters'
)

assert.match(
  routes,
  /security\.GET\("\/login-audits",\s*middleware\.(?:AdminAuthMiddleware\(\)|RequireCapability\(authorization\.CapabilityLoginAuditsView\)),\s*controllers\.GetLoginAudits\)/,
  'login audit API must live under the existing capability-protected security route group'
)

assert.match(
  component,
  /type\s+AdminSectionKey\s*=\s*[\s\S]*?'login-audits'/,
  'admin status panel must include login-audits as an admin section key'
)

assert.match(
  component,
  /\{ key: 'admin-users', label: '用户管理'[\s\S]*?\{ key: 'login-audits', label: '登录审计'[\s\S]*?\{ key: 'security', label: '安全防护'/,
  'login audit navigation must sit between user management and security protection'
)

assert.match(
  component,
  /id="login-audits-section"\s+v-if="isAdmin && isSectionVisible\('login-audits'\)"/,
  'login audit page must render only in the administrator backend section'
)

assert.match(
  component,
  /getRequest<any>\('security\/login-audits',\s*params,\s*\{ credentials: 'include', silent: true \}\)/,
  'login audit page should call the admin security audit endpoint with credentials'
)

assert.match(
  component,
  /watch\(\(\) => activeSection\.value,[\s\S]*?section === 'login-audits'[\s\S]*?refreshLoginAudits\(\)/,
  'login audit list should refresh when the admin opens the audit section'
)

console.log('login audit tests passed')
