import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const notificationCenter = await readFile(join(root, 'components/index/UserNotificationCenter.vue'), 'utf8')
const messageList = await readFile(join(root, 'components/index/MessageList.vue'), 'utf8')
const navigation = await readFile(join(root, 'utils/message-target-navigation.ts'), 'utf8')
const engagement = await readFile(join(root, 'utils/message-list-engagement.ts'), 'utf8')
const comments = await readFile(join(root, 'components/comments/BuiltinComments.vue'), 'utf8')

assert.match(
  notificationCenter,
  /const\s+parseTargetUrlNumber\s*=\s*\(item:\s*UserNotification,\s*key:\s*'message_id'\s*\|\s*'comment_id'\)[\s\S]*?new URL\(raw[\s\S]*?url\.searchParams\.get\(key\)/,
  'notification jumps should parse target_url as a fallback source for message/comment ids'
)

assert.match(
  notificationCenter,
  /const\s+targetCommentId\s*=\s*\(item:\s*UserNotification\)\s*=>\s*\{[\s\S]*?return\s+Number\(item\.comment_id\s*\|\|\s*item\.comment\?\.id\s*\|\|\s*parseTargetUrlNumber\(item,\s*'comment_id'\)\s*\|\|\s*0\)/,
  'notification jumps should target the concrete comment/reply id instead of the parent thread id'
)

assert.match(
  notificationCenter,
  /const\s+replyCommentId\s*=\s*\(item:\s*UserNotification\)\s*=>\s*\{[\s\S]*?item\.type\s*===\s*'reply'[\s\S]*?Number\(item\.parent_comment_id\s*\|\|\s*item\.parent_comment\?\.id\s*\|\|\s*item\.comment\?\.parent_id/,
  'inline reply boxes should reply to the parent comment for reply notifications'
)

assert.match(
  messageList,
  /expandComments: \(messageId\) => \{ expandedCommentsMap\.value\[messageId\] = true \}[\s\S]*?focusComment: focusBuiltinTargetComment/,
  'message notifications should open comments and delegate exact comment focusing to BuiltinComments'
)

assert.match(
  engagement,
  /comments\.focusCommentById\(commentId,\s*\{\s*scroll:\s*false\s*\}\)/,
  'notification jumps should let MessageList own the final scroll instead of stacking BuiltinComments smooth scrolling'
)

assert.match(
  navigation,
  /await waitForStableLayout\(element, currentGeneration\)[\s\S]*?if \(Math\.abs\(focusDistance\(element\)\) > 2\) scrollToFocus\(element, behavior\)/,
  'notification target stabilization should wait for layout quietly and scroll only to the final target'
)

assert.doesNotMatch(
  navigation,
  /const stabilizeScroll[\s\S]*?scrollToFocus\(element, 'instant'\)[\s\S]*?await waitForMedia\(messageId\)/,
  'notification target stabilization should not visibly snap to the target before media/layout settle'
)

assert.match(
  navigation,
  /const waitForMediaElement[\s\S]*?loadeddata[\s\S]*?element\.decode\(\)[\s\S]*?const waitForMedia[\s\S]*?querySelectorAll\('img, video'\)/,
  'notification jumps should wait for image decode and video data inside the target message before final alignment'
)

assert.match(
  navigation,
  /if \(messageElement\) \{[\s\S]*?if \(!commentId\) scrollToFocus\(messageElement, 'instant'\)/,
  'comment/reply notification jumps should not first snap to the message card before the concrete comment target is ready'
)

assert.match(
  navigation,
  /await stabilizeScroll\(commentElement, messageId, currentGeneration\)[\s\S]*?commentElement\.classList\.add\('notification-comment-highlight'\)/,
  'comment and reply notifications should perform the final stabilized scroll on the concrete target element'
)

assert.match(
  navigation,
  /if \(!commentId\) \{[\s\S]*?await stabilizeScroll\(messageElement, messageId, currentGeneration\)/,
  'message-only notifications should also stabilize after target attachments load'
)

assert.match(
  comments,
  /const\s+revealComment\s*=\s*async\s*\(commentId:\s*number\)\s*=>\s*\{[\s\S]*?visibleCount\.value\s*=\s*rootIndex\s*\+\s*1[\s\S]*?visibleChildrenCount\.value\[rootId\]\s*=\s*childIndex\s*\+\s*1/,
  'comment focusing should reveal hidden root comments and folded child replies before highlighting'
)

console.log('notification jump target tests passed')
