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
  homePage.includes("v-else-if=\"activeTab==='comment'\" class=\"comment-page\"") &&
    homePage.includes("v-else-if=\"activeTab==='notifications'\" class=\"notification-page\"") &&
    homePage.includes('<div class="notification-shell-card mb-3 p-5 md:p-6">') &&
    homePage.includes('.notification-shell-card,') &&
    homePage.includes('html.dark .notification-shell-card') &&
    homePage.includes('background: transparent;') &&
    homePage.includes('border: 0;') &&
    notificationCenter.includes('notification-feed-panel notification-board-wrap') &&
    notificationCenter.includes('.notification-title { display:block; margin:0 0 14px;') &&
    notificationCenter.includes('.notification-subtitle { max-width:42rem; margin:2px auto 20px;') &&
    notificationCenter.includes('.notification-count-title { margin:0 0 8px;') &&
    notificationCenter.includes('.notification-board-wrap { box-sizing:border-box; max-width:48rem; margin:0 auto 8px; padding:8px; }') &&
    notificationCenter.includes('--notice-card: rgba(255,255,255,.74);') &&
    notificationCenter.includes('--notice-card: rgba(15,23,42,.44);') &&
    notificationCenter.includes('.notification-target-card { width:100%; margin-top:10px; padding:0; border:0; border-radius:0; background:transparent;') &&
    notificationCenter.includes('background:linear-gradient(0deg, rgba(59,130,246,.10), rgba(59,130,246,.10)), var(--notice-card);'),
  'notification page must keep the guestbook content rhythm, preserve item backgrounds, and avoid an extra nested target-card background'
)

assert(
  messageList.includes('margin: 16px 0 72px;') &&
    homePage.includes('padding-bottom: calc(96px + env(safe-area-inset-bottom, 0px));') &&
    homePage.includes('scroll-padding-bottom: calc(160px + env(safe-area-inset-bottom, 0px));'),
  'pagination must keep explicit bottom clearance from the browser viewport'
)

console.log('frontend layout edge cases checks passed')