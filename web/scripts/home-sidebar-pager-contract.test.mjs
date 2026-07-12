import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const homePath = fileURLToPath(new URL('../pages/index.vue', import.meta.url))
const sidebarPagerPath = fileURLToPath(new URL('../components/index/HomeSidebarPager.vue', import.meta.url))
const messageListPath = fileURLToPath(new URL('../components/index/MessageList.vue', import.meta.url))
const infoFeedPath = fileURLToPath(new URL('../components/index/InfoFeedList.vue', import.meta.url))
const commentsPath = fileURLToPath(new URL('../components/comments/BuiltinComments.vue', import.meta.url))
const notificationsPath = fileURLToPath(new URL('../components/index/UserNotificationCenter.vue', import.meta.url))
const sources = await Promise.all([homePath, sidebarPagerPath, messageListPath, infoFeedPath, commentsPath, notificationsPath].map((path) => readFile(path, 'utf8')))
const [homePage, sidebarPager, messageList, infoFeed, comments, notifications] = sources.map((source) => source.replace(/\r\n?/g, '\n'))

const socialCardIndex = homePage.indexOf('left-widget-social-card')
const sidebarPagerIndex = homePage.indexOf('<HomeSidebarPager')
const clockCardIndex = homePage.indexOf('left-widget-clock-card')

assert.ok(socialCardIndex >= 0, '首页左栏应包含社交链接卡片')
assert.ok(sidebarPagerIndex > socialCardIndex, '侧栏分页器必须位于社交链接栏之后')
assert.ok(sidebarPagerIndex < clockCardIndex, '侧栏分页器必须位于时间栏之前')
assert.match(homePage, /<HomeSidebarPager\s+:key="activeTab"/, '切换分页界面时必须重置未提交的目标页输入，不能跨界面残留')

assert.match(homePage, /<InfoFeedList[\s\S]*?ref="infoFeedList"/)
assert.match(homePage, /<UserNotificationCenter[\s\S]*?ref="notificationCenter"/)
assert.match(homePage, /activeTab\.value === 'feed'[\s\S]*?infoFeedList\.value/)
assert.match(homePage, /activeTab\.value === 'comment'[\s\S]*?guestbookCommentsRef\.value/)
assert.match(homePage, /activeTab\.value === 'notifications'[\s\S]*?notificationCenter\.value/)

for (const [name, source] of [
  ['笔记列表', messageList],
  ['信息流', infoFeed],
  ['留言板', comments],
  ['通知中心', notifications],
]) {
  assert.match(source, /const sidebarPagerState = computed\(/, `${name}必须公开响应式分页状态`)
  assert.match(source, /defineExpose\(\{[\s\S]*?sidebarPagerState[\s\S]*?previousPage[\s\S]*?nextPage[\s\S]*?goToPage[\s\S]*?\}\)/, `${name}必须实现统一侧栏分页接口`)
}

assert.match(sidebarPager, /grid-template-columns:\s*28px minmax\(0, 1fr\) 28px;/, '上一页和下一页按钮必须固定在组件左右两端')
assert.match(sidebarPager, /white-space:\s*nowrap;/, '侧栏分页所有内容必须强制保持一行')
assert.doesNotMatch(sidebarPager, /<span[^>]*>\s*(?:上一页|下一页)\s*<\/span>/, '侧栏翻页按钮不得显示上一页或下一页文本')
assert.match(sidebarPager, /aria-label="上一页"[\s\S]*?i-heroicons-arrow-left/)
assert.match(sidebarPager, /aria-label="下一页"[\s\S]*?i-heroicons-arrow-right/)

console.log('home sidebar pager contract checks passed')
