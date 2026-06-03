import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const repoRoot = dirname(webRoot)
const heatmap = await readFile(join(webRoot, 'components/widgets/heatmap.vue'), 'utf8')
const indexPage = await readFile(join(webRoot, 'pages/index.vue'), 'utf8')
const calendarController = await readFile(join(repoRoot, 'internal/controllers/controllers.go'), 'utf8')
const messageService = await readFile(join(repoRoot, 'internal/services/message_service.go'), 'utf8')

assert.match(
  heatmap,
  /defineProps<\{\s*activeTab\?:\s*string\s*\}>\(\)/,
  'heatmap must accept the active tab so it can distinguish latest and personal scopes'
)
assert.match(
  heatmap,
  /props\.activeTab\s*===\s*'personal'/,
  'heatmap must explicitly detect the personal tab'
)
assert.match(
  heatmap,
  /params\.set\('authorId',\s*String\(currentUserId\.value\)\)/,
  'personal heatmap requests must pass the current user as authorId'
)
assert.match(
  heatmap,
  /fetch\(calendarRequestURL\.value,\s*\{\s*credentials:\s*'include'\s*\}\)/,
  'heatmap requests must include credentials so the backend can apply the viewer visibility scope'
)
assert.match(
  heatmap,
  /if\s*\(isPersonalScope\.value\s*&&\s*\(!userStore\.isLogin\s*\|\|\s*currentUserId\.value\s*<=\s*0\)\)\s*\{[\s\S]*?generateEmptyCalendar\(\)/,
  'guest personal heatmaps must render an empty calendar instead of global activity'
)
assert.doesNotMatch(
  heatmap,
  /generateTestData/,
  'heatmap must not synthesize random activity when the API has no matching rows'
)
assert.doesNotMatch(
  heatmap,
  /fetch\('\/api\/messages\/calendar'/,
  'heatmap must not call the unscoped calendar endpoint directly'
)

const heatmapInstances = indexPage.match(/<HeatmapWidget\s+:active-tab="activeTab"\s*\/>/g) || []
assert.equal(heatmapInstances.length, 3, 'all home-page heatmap instances must receive activeTab')
assert.doesNotMatch(indexPage, /<HeatmapWidget\s*\/>/, 'home page must not keep unscoped heatmap instances')

assert.match(
  calendarController,
  /commentAuthUserID\(c\)[\s\S]*?commentAuthIsAdmin\(c\)[\s\S]*?c\.Query\("authorId"\)[\s\S]*?GetMessagesGroupByDate\(currentUserID,\s*isAdmin,\s*authorID\)/,
  'calendar controller must pass viewer and author scope to the service'
)
assert.match(
  messageService,
  /func\s+GetMessagesGroupByDate\(userID \*uint,\s*isAdmin bool,\s*authorID \*uint\)/,
  'calendar service must accept viewer and author scope parameters'
)
assert.match(
  messageService,
  /authorID != nil[\s\S]*?q = q\.Where\("user_id = \?", \*authorID\)[\s\S]*?q = ApplyMessageVisibilityScope\(q, userID, isAdmin\)[\s\S]*?Select\("created_at"\)/,
  'calendar service must apply author filtering before the shared four-state visibility scope'
)

console.log('heatmap scope tests passed')
