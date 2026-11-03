import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'

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

// Execute the component's actual helpers, including TypeScript annotations.
const jumpHelpers = notificationCenter.slice(
  notificationCenter.indexOf('const parseTargetUrlNumber ='),
  notificationCenter.indexOf('const scrollInlineReplyIntoView =')
)
const compiledHelpers = ts.transpileModule(jumpHelpers, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None },
}).outputText
const { targetCommentId, canReply, jumpPayload } = new Function('isTargetUnavailable', `${compiledHelpers}; return { targetCommentId, canReply, jumpPayload }`)(item => Boolean(item.target_status && item.target_status !== 'available'))

for (const source of [
  { message_id: 42, comment_id: 7 },
  { message: { id: 42 }, comment: { id: 8 } },
  { target_url: '/?message_id=42&comment_id=9' },
]) {
  const item = { ...source, type: 'author_subscription', message: { can_interact: true, ...source.message } }
  assert.equal(targetCommentId(item), 0, 'subscription notifications must ignore every comment id source')
  assert.equal(canReply(item), false, 'subscription notifications must never offer inline replies')
  assert.equal(jumpPayload(item).target_message_id, 42)
  assert.equal(jumpPayload(item).target_comment_id, null, 'subscription jumps must only focus the note')
}
const reply = { type: 'reply', message_id: 42, comment_id: 8, parent_comment_id: 7, message: { can_interact: true } }
assert.equal(jumpPayload(reply).target_comment_id, 8, 'reply jumps must still focus the concrete reply')
assert.equal(canReply(reply), true)
assert.equal(canReply({ ...reply, target_status: 'deleted' }), false)
assert.equal(canReply({ ...reply, message: { can_interact: false } }), false)
assert.equal(targetCommentId({ ...reply, type: 'like' }), 0)
assert.equal(targetCommentId({ type: 'comment', target_url: '/?message_id=42&comment_id=9' }), 9)

console.log('notification jump target tests passed')
