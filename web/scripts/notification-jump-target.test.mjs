import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const notificationCenter = await readFile(join(root, 'components/index/UserNotificationCenter.vue'), 'utf8')
const messageList = await readFile(join(root, 'components/index/MessageList.vue'), 'utf8')
const comments = await readFile(join(root, 'components/comments/BuiltinComments.vue'), 'utf8')

assert.match(
  notificationCenter,
  /const\s+targetCommentId\s*=\s*\(item:\s*UserNotification\)\s*=>\s*\{[\s\S]*?return\s+Number\(item\.comment_id\s*\|\|\s*item\.comment\?\.id\s*\|\|\s*0\)/,
  'notification jumps should target the concrete comment/reply id instead of the parent thread id'
)

assert.match(
  notificationCenter,
  /const\s+replyCommentId\s*=\s*\(item:\s*UserNotification\)\s*=>\s*\{[\s\S]*?item\.type\s*===\s*'reply'[\s\S]*?Number\(item\.parent_comment_id\s*\|\|\s*item\.parent_comment\?\.id\s*\|\|\s*item\.comment\?\.parent_id/,
  'inline reply boxes should reply to the parent comment for reply notifications'
)

assert.match(
  messageList,
  /expandedCommentsMap\.value\[messageId\]\s*=\s*true[\s\S]*?focusBuiltinTargetComment\(messageId,\s*commentId\)/,
  'message notifications should open comments and delegate exact comment focusing to BuiltinComments'
)

assert.match(
  messageList,
  /const\s+waitForNotificationMedia\s*=\s*async\s*\(messageId:\s*number[\s\S]*?querySelectorAll\('img, video'\)[\s\S]*?loadedmetadata/,
  'notification jumps should wait for images and video metadata inside the target message before final alignment'
)

assert.match(
  messageList,
  /await\s+stabilizeNotificationTargetScroll\(commentEl,\s*messageId\)[\s\S]*?commentEl\.classList\.add\('notification-comment-highlight'\)/,
  'comment and reply notifications should perform the final stabilized scroll on the concrete target element'
)

assert.match(
  messageList,
  /if\s*\(!commentId\)\s*\{[\s\S]*?await\s+stabilizeNotificationTargetScroll\(targetElement,\s*messageId\)/,
  'message-only notifications should also stabilize after target attachments load'
)

assert.match(
  comments,
  /const\s+revealComment\s*=\s*async\s*\(commentId:\s*number\)\s*=>\s*\{[\s\S]*?visibleCount\.value\s*=\s*rootIndex\s*\+\s*1[\s\S]*?visibleChildrenCount\.value\[rootId\]\s*=\s*childIndex\s*\+\s*1/,
  'comment focusing should reveal hidden root comments and folded child replies before highlighting'
)

console.log('notification jump target tests passed')