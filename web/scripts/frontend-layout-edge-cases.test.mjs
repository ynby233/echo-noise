import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const root = resolve(process.cwd())
const read = (path) => readFileSync(resolve(root, path), 'utf8')
const assert = (condition, message) => {
  if (!condition) {
    console.error(message)
    process.exit(1)
  }
}

const addForm = read('components/index/AddForm.vue')
const messageList = read('components/index/MessageList.vue')
const notificationCenter = read('components/index/UserNotificationCenter.vue')
const searchMode = read('components/index/Searchmode.vue')
const authLogin = read('pages/auth/login.vue')
const authRegister = read('pages/auth/register.vue')
const homePage = read('pages/index.vue')
const floatingCss = read('assets/css/tailwind.css')

assert(
  addForm.includes("class=\"publish-time-option\"") &&
    addForm.includes("'is-current': hour === publishCurrentHour") &&
    addForm.includes("'is-current': minute === publishCurrentMinute"),
  'publish time picker must keep the shared current-time classes'
)

assert(
  messageList.includes("class=\"publish-time-option\"") &&
    messageList.includes("'is-current': hour === editPublishCurrentHour") &&
    messageList.includes("'is-current': minute === editPublishCurrentMinute"),
  'edit time picker must use the same shared current-time classes as publish'
)

assert(
  floatingCss.includes('.publish-datetime-menu .publish-time-option.is-current:not(.is-selected)') &&
    floatingCss.indexOf('.publish-datetime-menu .publish-time-option.is-current:not(.is-selected)') <
      floatingCss.indexOf('.publish-datetime-menu .publish-time-option.is-current.is-selected'),
  'current time state must be blue unless the option is selected, where selected orange wins'
)

assert(
  notificationCenter.includes('notification-feed-panel notification-board-wrap') &&
    notificationCenter.includes('margin-top:1px;') &&
    notificationCenter.includes('padding:24px;') &&
    notificationCenter.includes('background:var(--notice-surface);') &&
    notificationCenter.includes('.notification-center { padding:20px; }') &&
    notificationCenter.includes('.notification-title-row { position:relative; display:block;') &&
    notificationCenter.includes('.notification-title { display:block; margin:0 0 14px;') &&
    notificationCenter.includes('.notification-subtitle { max-width:42rem; margin:2px auto 20px;') &&
    notificationCenter.includes('.notification-count-title { margin:0 0 8px;') &&
    notificationCenter.includes('.notification-board-wrap { box-sizing:border-box; max-width:48rem; margin:0 auto 8px; padding:8px;'),
  'notification title, description, count, and first-card spacing must mirror the measured guestbook rhythm while keeping the outer surface'
)

assert(
  notificationCenter.includes('--notice-card: rgba(255,255,255,.72);') &&
    notificationCenter.includes('--notice-card: rgba(15,23,42,.52);') &&
    notificationCenter.includes('.notification-feed-item { position:relative; display:flex; gap:12px; padding:12px; border:1px solid var(--notice-border); border-radius:12px; background:var(--notice-card);') &&
    notificationCenter.includes('background:linear-gradient(0deg, rgba(59,130,246,.10), rgba(59,130,246,.10)), var(--notice-card);') &&
    notificationCenter.includes('background:linear-gradient(0deg, rgba(59,130,246,.14), rgba(59,130,246,.14)), var(--notice-card);'),
  'notification cards must keep their own visible surface in light and dark modes, including unread and highlighted states'
)

assert(
  notificationCenter.includes('class="notification-refresh-button nw-action-btn nw-tooltip-anchor"') &&
    notificationCenter.includes('class="notification-text-button nw-action-btn nw-action-btn--label" :disabled="markingAll || unreadCount === 0"') &&
    notificationCenter.includes('class="reply-toggle nw-action-btn nw-action-btn--label"') &&
    notificationCenter.includes('.notification-refresh-button,\n.notification-text-button,\n.reply-toggle { --nw-action-bg:var(--notice-card);') &&
    !notificationCenter.includes('class="icon-action') &&
    !notificationCenter.includes('class="text-action') &&
    !notificationCenter.includes('.icon-action') &&
    !notificationCenter.includes('.text-action'),
  'notification refresh, mark-all-read, and reply buttons must use the shared general action button template'
)

assert(
  searchMode.includes('class="nw-modal-card search-modal-card"') &&
    searchMode.includes("background: 'bg-transparent dark:bg-transparent'") &&
    searchMode.includes('color="orange"') &&
    searchMode.includes('class="search-input w-full"') &&
    searchMode.includes('class="search-modal-button nw-action-btn nw-action-btn--label"') &&
    !searchMode.includes('nw-action-btn--accent') &&
    searchMode.includes('.search-input :deep(input:focus)') &&
    searchMode.includes('rgba(249, 115, 22, .92)') &&
    !searchMode.includes('<UButton variant="ghost" color="gray" @click="closeModal">取消</UButton>') &&
    !searchMode.includes('<UButton color="orange" @click="handleSearch">搜索</UButton>'),
  'site search modal must use orange input focus and shared general action-button templates for cancel/search'
)

assert(
  floatingCss.includes('.nw-modal-card {') &&
    floatingCss.includes('--nw-modal-bg: #111827;') &&
    floatingCss.includes('.nw-modal-card :where(input:focus, input:focus-visible, textarea:focus, textarea:focus-visible)') &&
    !floatingCss.includes('.nw-action-btn--accent {') &&
    !floatingCss.includes('--nw-modal-bg: var(--home-surface-dark-elevated') &&
    homePage.includes('class="nw-modal-card"') &&
    !homePage.includes('class="search-card nw-modal-card"') &&
    authLogin.includes('class="nw-modal-card is-dark auth-page-card') &&
    authRegister.includes('class="nw-modal-card is-dark auth-page-card') &&
    authLogin.includes("background: 'bg-transparent dark:bg-transparent'"),
  'search, login, and register dialog cards must share the single solid modal shell without default modal corner artifacts'
)

assert(
  homePage.includes('color="orange" placeholder="用户名"') &&
    homePage.includes(':color="remaining>0 ? \'orange\' : \'red\'"') &&
    homePage.includes('color="orange" @click="showForgot = false"') &&
    authLogin.includes('color="orange" placeholder="请输入用户名或已绑定邮箱"') &&
    authRegister.includes('color="orange" placeholder="请输入验证码"') &&
    !homePage.includes("color: 'green'") &&
    !authLogin.includes("color: 'green'") &&
    !authRegister.includes("color: 'green'"),
  'auth and forgot-password dialogs must use orange controls instead of the green primary palette'
)

assert(
  messageList.includes('margin: 16px 0 72px;') &&
    homePage.includes('padding-bottom: calc(96px + env(safe-area-inset-bottom, 0px));') &&
    homePage.includes('scroll-padding-bottom: calc(160px + env(safe-area-inset-bottom, 0px));'),
  'pagination must keep explicit bottom clearance from the browser viewport'
)

console.log('frontend layout edge cases checks passed')