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
assert.match(homePage, /<HomeSidebarPager\s+:context-key="activeTab"/, '切换分页界面时必须重置未提交的目标页输入，不能跨界面残留')
assert.doesNotMatch(homePage, /<HomeSidebarPager\s+:key="activeTab"/, '常驻分页器不能在切换界面时重新挂载')
assert.doesNotMatch(homePage, /<UCard\s+v-if="activeSidebarPager\.visible"[^>]*left-widget-pager-card/, '侧栏分页卡必须常驻，不能随页面状态反复挂载和卸载')
assert.match(homePage, /:disabled="!isSidebarPagerInteractive"/, '无分页界面必须把整个侧栏分页器切换为不可交互状态')
assert.match(homePage, /currentPage:\s*0,[\s\S]*?totalPages:\s*latestTotalPages\.value/, '无分页界面必须显示第 0 页，并沿用最新板块总页数')
assert.match(homePage, /postRequest<[^>]*>\('messages\/page',[\s\S]*?pageSize:\s*15[\s\S]*?excludeId:/, '最新板块总页数必须按当前会话可见数据读取，并排除留言板消息')
assert.match(homePage, /onMounted\(async \(\) => \{[\s\S]*?await loadGuestbookTarget\(\)[\s\S]*?if \(!isSidebarPagerInteractive\.value\) await loadLatestTotalPages\(\)/, '直接进入任意无分页界面（包括访客个人页）时都必须读取最新板块总页数')

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
assert.match(sidebarPager, /disabled\?:\s*boolean/, '紧凑分页器必须支持整体禁用态')
assert.match(sidebarPager, /Number\.isFinite\(Number\(page\)\)[^\n]*Number\(page\)[^\n]*:\s*0/, '无分页界面的目标页输入必须保留数值 0，不能回退成第 1 页')
assert.ok((sidebarPager.match(/:disabled="disabled \|\| loading/g) || []).length >= 4, '数字输入、微调和跳转控件必须在无分页界面全部禁用')
assert.match(sidebarPager, /\.home-sidebar-pager__jump\s*\{[^}]*width:\s*36px;[^}]*height:\s*28px;/, '跳转按钮必须保持紧凑宽度，并与两侧翻页按钮、数字输入组统一为 28px 高')
assert.match(sidebarPager, /\.home-sidebar-pager__nav\s*\{[^}]*height:\s*28px;/, '两侧翻页按钮高度必须保持 28px')
assert.match(sidebarPager, /\.home-sidebar-pager__number-control\s*\{[^}]*height:\s*28px;/, '数字输入组高度必须保持 28px')
assert.equal((sidebarPager.match(/class="home-sidebar-pager__step nw-action-btn"/g) || []).length, 2, '两个页码微调键必须复用全站交互按钮体系')
assert.match(messageList, /\.pager-stepper-btn\s*\{[^}]*width:\s*24px;[^}]*height:\s*16px;[^}]*border-radius:\s*0;/, '页面底部分页微调键是左侧微调键的尺寸与圆角基准')
const sidebarStepRule = sidebarPager.match(/\.home-sidebar-pager__step\s*\{([\s\S]*?)\n\}/)?.[1] || ''
assert.match(sidebarStepRule, /border:\s*0;/, '左侧微调键必须保持与页面底部分页栏一致的无外框结构')
assert.match(sidebarStepRule, /border-radius:\s*0;/, '左侧微调键必须与页面底部分页栏一致地取消单键圆角')
assert.doesNotMatch(sidebarPager, /\.home-sidebar-pager__step:not\(:disabled\):(hover|focus-visible)/, '左侧微调键不得再用局部亮暗悬浮规则覆盖底部分页栏的全站按钮状态')
assert.doesNotMatch(sidebarStepRule, /(?:background|color|opacity|--nw-action-hover-)/, '左侧微调键必须继承全站亮暗与悬浮变量，不能固定为另一套颜色或透明度')

console.log('home sidebar pager contract checks passed')
