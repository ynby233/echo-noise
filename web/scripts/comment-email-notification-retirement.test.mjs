import assert from 'node:assert/strict'
import { access, readFile, readdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const repoRoot = dirname(webRoot)
const read = (path) => readFile(join(repoRoot, path), 'utf8')

const runtimeFiles = {
  statusPanel: await read('web/components/index/StatusPanel.vue'),
  indexPage: await read('web/pages/index.vue'),
  settingsService: await read('internal/services/setting_service.go'),
  seedService: await read('internal/services/seed_service.go'),
  models: await read('internal/models/models.go'),
  controllers: await read('internal/controllers/controllers.go'),
}

const retiredRuntimePattern = /commentEmail|CommentEmail|commentLoginRequired|CommentLoginRequired|CommentsSettings/
for (const [name, source] of Object.entries(runtimeFiles)) {
  assert.doesNotMatch(source, retiredRuntimePattern, `${name} must not compile the archived comment-email or retired login-toggle runtime`)
}

assert.doesNotMatch(
  runtimeFiles.statusPanel,
  /评论系统[\s\S]*?管理员全站|邮件通知[\s\S]*?saveCommentConfig/,
  'interaction management must not render the archived configuration panel'
)
assert.match(runtimeFiles.statusPanel, /<CommentManager\s+v-if="can\('comments\.view'\)"/, 'interaction management itself must remain available')
assert.match(runtimeFiles.controllers, /CreateNotificationsForComment/, 'in-app interaction notifications must remain active')
assert.match(runtimeFiles.models, /SitePublicURL\s+string/, 'shared public links must use a neutral site-level field')

for (const path of [
  'extras/comment-email-notifications/README.md',
  'extras/comment-email-notifications/frontend/CommentsSettings.vue.disabled',
  'extras/comment-email-notifications/backend/comment_email_notifications.go.disabled',
  'extras/comment-email-notifications/backend/post_comment_email_delivery.fragment.go.disabled',
]) {
  await access(join(repoRoot, path))
}

const archiveReadme = await read('extras/comment-email-notifications/README.md')
const archiveDelivery = await read('extras/comment-email-notifications/backend/post_comment_email_delivery.fragment.go.disabled')
const archiveFrontend = await read('extras/comment-email-notifications/frontend/CommentsSettings.vue.disabled')
assert.match(archiveReadme, /不参与.*编译|不会进入.*构建/s, 'archive must explicitly stay outside every runtime build')
assert.match(archiveReadme, /后续版本.*重新设计|未来版本.*重新设计/s, 'archive must document that restoration requires a later redesign')
assert.match(archiveDelivery, /SendEmailHTMLWithFrom/, 'archive must retain the original backend delivery source fragment')
assert.match(archiveFrontend, /commentEmailSiteURL/, 'archive must retain the original frontend configuration component')

const readme = await read('README.md')
assert.match(readme, /互动邮件通知.*暂时封存/, 'README must disclose the temporary retirement')
assert.match(readme, /站内通知.*继续可用/, 'README must distinguish in-app notifications from archived email delivery')

const generatedFiles = []
const walkGenerated = async (directory) => {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) await walkGenerated(path)
    else if (/\.(?:html|js)$/i.test(entry.name)) generatedFiles.push(path)
  }
}
await walkGenerated(join(repoRoot, 'public'))
assert.ok(generatedFiles.length > 0, 'server-distributed frontend assets must exist')
for (const path of generatedFiles) {
  const source = await readFile(path, 'utf8')
  assert.doesNotMatch(source, /commentLoginRequired|commentEmailAdminNotifyAll|commentEmailEnabled|管理员全站|仅登录/, `${path} must not ship the archived panel`)
}

console.log('comment email notification retirement contract passed')
