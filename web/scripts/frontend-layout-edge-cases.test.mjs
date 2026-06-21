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
const audioRecorder = read('components/index/AudioRecorder.vue')
const vditorEditor = read('components/index/VditorEditor.vue')
const messageList = read('components/index/MessageList.vue')
const messageStore = read('store/message.ts')
const notificationCenter = read('components/index/UserNotificationCenter.vue')
const searchMode = read('components/index/Searchmode.vue')
const authLogin = read('pages/auth/login.vue')
const authRegister = read('pages/auth/register.vue')
const homePage = read('pages/index.vue')
const builtinComments = read('components/comments/BuiltinComments.vue')
const infoFeedList = read('components/index/InfoFeedList.vue')
const markdownRenderer = read('components/index/MarkdownRenderer.vue')
const calendarWidget = read('components/widgets/CalendarWidget.vue')
const mediaUpload = read('utils/media-upload.ts')
const floatingCss = read('assets/css/tailwind.css')
const backendRouter = read('../internal/routers/routers.go')

assert(
  addForm.includes('.editor-toolbar') &&
    addForm.includes('position: relative;') &&
    !addForm.includes('position: sticky; bottom: 0;') &&
    backendRouter.includes('filePath := filepath.Join("./public", strings.TrimPrefix(cleanPath, "/"))') &&
    backendRouter.includes('c.File("./public/index.html")') &&
    !backendRouter.includes('static.Serve("/", static.LocalFile("./public", true))'),
  'publish toolbar must stay in normal flow after editor resize, and backend static serving must not intercept SPA refresh fallback'
)

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
  homePage.includes('<UCard class="search-card mb-3" :ui="{ body: { padding: \'p-5 md:p-6\' } }">\n              <UserNotificationCenter') &&
    notificationCenter.includes('notification-feed-panel notification-board-wrap') &&
    notificationCenter.includes('margin:0;') &&
    notificationCenter.includes('padding:0;') &&
    notificationCenter.includes('border:0;') &&
    notificationCenter.includes('border-radius:0;') &&
    notificationCenter.includes('background:transparent;') &&
    notificationCenter.includes('box-shadow:none;') &&
    !notificationCenter.includes('--notice-frame-line') &&
    !notificationCenter.includes('.notification-center::before {') &&
    !notificationCenter.includes('box-shadow:0 14px 28px rgba(2,6,23,.45);') &&
    notificationCenter.includes('.notification-title-row { position:relative; display:block;') &&
    notificationCenter.includes('.notification-title { display:block; margin:0 0 14px;') &&
    notificationCenter.includes('.notification-subtitle { max-width:42rem; margin:2px auto 20px;') &&
    notificationCenter.includes('.notification-count-title { margin:0 0 8px;') &&
    notificationCenter.includes('.notification-board-wrap { box-sizing:border-box; max-width:48rem; margin:0 auto 8px; padding:8px;'),
  'notification page must use the same UCard.search-card outer frame as guestbook, with notification content no longer drawing its own frame'
)

assert(
  notificationCenter.includes('--notice-card: #ffffff;') &&
    notificationCenter.includes('--notice-card: rgba(15,23,42,.52);') &&
    notificationCenter.includes('--notice-card-shadow: 0 14px 30px rgba(15,23,42,.12);') &&
    notificationCenter.includes('--notice-card-shadow: 0 16px 32px rgba(2,6,23,.52);') &&
    notificationCenter.includes('通知 ({{ total }})') &&
    notificationCenter.includes('.notification-feed-item { position:relative; display:flex; gap:12px; padding:12px; border:1px solid var(--notice-border); border-radius:12px; background:var(--notice-card); color:var(--notice-text); box-shadow:var(--notice-card-shadow);') &&
    notificationCenter.includes('background:linear-gradient(0deg, rgba(59,130,246,.10), rgba(59,130,246,.10)), var(--notice-card);') &&
    notificationCenter.includes('background:linear-gradient(0deg, rgba(59,130,246,.14), rgba(59,130,246,.14)), var(--notice-card); box-shadow:inset 3px 0 0 rgba(59,130,246,.72), var(--notice-card-shadow);'),
  'notification cards must keep their own visible surface and framed shadow in light and dark modes, including unread and highlighted states'
)

assert(
  notificationCenter.includes('class="notification-refresh-button nw-action-btn nw-tooltip-anchor"') &&
  notificationCenter.includes('class="notification-text-button nw-action-btn nw-action-btn--label" :disabled="markingAll || unreadCount === 0"') &&
  notificationCenter.includes('class="reply-toggle nw-action-btn nw-action-btn--label"') &&
  notificationCenter.includes('.notification-refresh-button,\n.notification-text-button,\n.reply-toggle {') &&
  notificationCenter.includes('height:28px;') &&
  notificationCenter.includes('font-size:12px;') &&
  notificationCenter.includes('font-weight:650;') &&
  notificationCenter.includes('.notification-text-button,\n.reply-toggle { min-width:max-content; padding:0 8px; }') &&
  notificationCenter.includes('--nw-action-bg:rgba(51,65,85,.96);') &&
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
  homePage.includes('rounded bg-blue-500 text-white text-[10px]">{{ isLoggedIn ? \'用户\' : \'访客\' }}</span>') &&
    homePage.includes('<UButton variant="ghost" color="blue" class="auth-btn" aria-label="登录"') &&
    homePage.includes('.stats-login-prompt:hover { color: #2563eb; }') &&
    homePage.includes('html.dark .stats-login-prompt { color: #60a5fa; }') &&
    !homePage.includes('rounded bg-indigo-500 text-white text-[10px]">{{ isLoggedIn ? \'用户\' : \'访客\' }}</span>') &&
    !homePage.includes('<UButton variant="ghost" color="indigo" class="auth-btn" aria-label="登录"') &&
    !homePage.includes('color: rgba(79, 70, 229, 0.92);') &&
    !homePage.includes('.stats-login-prompt:hover { color: #4338ca; }'),
  'sidebar guest/user badge, login button, and login-required stats prompt must use the same blue family as the comment submit button'
)

assert(
  messageList.includes('class="search-results-title">搜索</div>') &&
    messageList.includes('搜索内容：{{ activeFilterContent }}') &&
    messageList.includes('class="search-results-back nw-action-btn nw-action-btn--label"') &&
    messageList.includes('>笔记 ({{ filteredResultCount }})</div>') &&
    messageList.includes('v-if="props.pageReady && hasActiveFilters && !isPageLoading && !isDisplayQueryPending && displayMessages.length" class="search-results-count"') &&
    messageList.includes('v-if="props.pageReady && hasActiveFilters && (isPageLoading || isDisplayQueryPending || !displayMessages.length)" class="search-results-empty"') &&
    messageList.includes('v-if="isPageLoading || isDisplayQueryPending"') &&
    messageList.includes('v-if="!props.pageReady || !hasActiveFilters || (!isDisplayQueryPending && displayMessages.length)"') &&
    messageList.includes("(e: 'loading-change', loading: boolean): void") &&
    messageList.includes('const setPageLoading = (loading: boolean) => {') &&
    messageList.includes("emit('loading-change', loading)") &&
    messageList.includes('setPageLoading(true)') &&
    messageList.includes('setPageLoading(false)') &&
    !messageList.includes('listStabilityStyle') &&
    !messageList.includes('lockListHeight()') &&
    !messageList.includes('releaseListHeight()') &&
    messageList.includes('const stableDisplayMessages = ref<any[]>([])') &&
    messageList.includes("const currentDisplayQueryKey = computed(() => message.listQueryKey(pageQueryFor(1)))") &&
    messageList.includes('const isDisplayQueryPending = computed(() => Boolean(message.currentListQueryKey && message.currentListQueryKey !== currentDisplayQueryKey.value))') &&
    messageList.includes('if (isDisplayQueryPending.value) return []') &&
    messageList.includes('if (isPageLoading.value && stableDisplayQueryKey.value === currentDisplayQueryKey.value && stableDisplayMessages.value.length) return stableDisplayMessages.value') &&
    messageList.includes('v-if="showPager" ref="prefetchSentinel"') &&
    messageList.includes('const showPager = computed(() => {') &&
    messageList.includes('class="search-results-empty-icon"') &&
    messageList.includes(":is=\"props.pageReady && hasActiveFilters ? resolveComponent('UCard') : 'div'\"") &&
    messageList.includes("import { resolveComponent } from 'vue'") &&
    messageList.includes("['search-card', 'search-results-panel', 'mb-3', { 'is-dark': isContentDark }]") &&
    messageList.includes("ui: { body: { padding: 'p-5 md:p-6' } }") &&
    messageList.includes("props.pageReady && hasActiveFilters ? 'search-results-list' : 'my-4'") &&
    messageList.includes('.search-results-list {') &&
    messageList.includes('width: calc(100% + 2rem + 2px);') &&
    messageList.includes('max-width: calc(56rem + 2px);') &&
    messageList.includes('margin: 0 calc(-1rem - 1px);') &&
    messageList.includes("class=\"w-full h-auto overflow-hidden flex flex-col justify-between\"") &&
    messageList.includes("['content-container', innerContainerClass, listThemeClass]") &&
    !messageList.includes('search-result-note-card') &&
    !messageList.includes('search-result-note-frame') &&
    !messageList.includes('search-result-note-frame--bounded') &&
    !messageList.includes("props.pageReady && hasActiveFilters ? innerContainerClass : ''") &&
    messageList.includes("return filtering ? 'flex-grow w-full' : 'flex-grow w-full px-1 sm:px-2'") &&
    !messageList.includes("['search-results-list', innerContainerClass]") &&
    messageList.includes('.search-card {') &&
    messageList.includes('border: 1px solid #e5e7eb;') &&
    messageList.includes('border-radius: var(--home-radius-panel);') &&
    messageList.includes('background: var(--home-surface-light);') &&
    messageList.includes('margin-top: 20px;') &&
    homePage.includes('.moments-header {\n  margin-bottom: 20px;\n}') &&
    messageList.includes('.search-card.is-dark {') &&
    messageList.includes('background: linear-gradient(180deg, rgba(30, 41, 59, 0.48) 0%, rgba(15, 23, 42, 0.82) 100%);') &&
    messageList.includes('box-shadow: 0 14px 28px rgba(2, 6, 23, 0.45);') &&
    messageList.includes('backdrop-filter: blur(8px) saturate(118%);') &&
    !messageList.includes('margin: 20px 0 16px;') &&
    !messageList.includes('padding: 24px;') &&
    !messageList.includes('padding: 20px;') &&
    !messageList.includes('--search-panel-frame-line') &&
    !messageList.includes('.search-results-panel::before {') &&
    messageList.includes('right: 17px;') &&
    messageList.includes('height: 28px;') &&
    messageList.includes('padding: 0 8px;') &&
    messageList.includes('font-size: 12px;') &&
    messageList.includes('font-weight: 650;') &&
    messageList.includes('.search-results-panel.is-dark .search-results-back {') &&
    messageList.includes('--nw-action-bg: rgba(51, 65, 85, .96);') &&
    messageList.includes('.search-results-list > .w-full,') &&
    !messageList.includes('max-width: none !important;') &&
    messageList.includes('overflow: visible !important;') &&
    messageList.includes('.search-results-list > .w-full > .p-0 > .content-container {') &&
    messageList.includes('.search-results-panel.is-dark .search-results-list > .w-full > .p-0 > .content-container.content-container {') &&
    messageList.includes('background: rgba(15, 23, 42, .52) !important;') &&
    messageList.includes('background-color: rgba(15, 23, 42, .52) !important;') &&
    messageList.includes('background-image: none !important;') &&
    messageList.includes('border-color: rgba(255, 255, 255, .12);') &&
    messageList.includes('box-shadow: 0 16px 32px rgba(2, 6, 23, .52) !important;') &&
    homePage.includes('background: linear-gradient(180deg, rgba(30, 41, 59, 0.48) 0%, rgba(15, 23, 42, 0.82) 100%);') &&
    builtinComments.includes("const rootCardClass = computed(() => 'comment-card-frame comment-card-root')") &&
    builtinComments.includes("const childCardClass = computed(() => 'comment-card-frame comment-card-child')") &&
    builtinComments.includes('box-shadow: 0 16px 32px rgba(2, 6, 23, .52);') &&
    builtinComments.includes('class="comment-load-btn nw-action-btn nw-action-btn--label"') &&
    builtinComments.includes('<UIcon :name="visibilityTag(c.visibility).icon" class="w-4 h-4" />') &&
    builtinComments.includes('<UIcon :name="visibilityTag(child.visibility).icon" class="w-4 h-4" />') &&
    !builtinComments.includes(':global(html.dark) .comment-item.child { background: rgba(255,255,255,0.06);') &&
    !builtinComments.includes(':global(html:not(.dark)) .comment-item.child { background: rgba(0,0,0,0.04);') &&
    !messageList.includes('.search-results-panel > .content-container {') &&
    !messageList.includes('.search-result-note-card') &&
    !messageList.includes('.search-result-note-frame--bounded {') &&
    !messageList.includes('max-width: calc(56rem + 26px);') &&
    !messageList.includes('.search-result-note-frame .content-container') &&
    !messageList.includes('max-width: 48rem;\n  margin: 20px auto 16px;') &&
    messageList.includes('font-weight: 400;') &&
    messageList.includes('line-height: 20px;') &&
    !messageList.includes('.content-container.search-result-note-card') &&
    !messageList.includes('.search-results-panel.is-dark .content-container.search-result-note-card') &&
    !messageList.includes('background: rgba(255, 255, 255, 0.72);') &&
    !messageList.includes('class="date-filter-bar"') &&
    !messageList.includes('筛选结果：'),
  'filtered search results must keep guestbook/notification-aligned panel width, spacing, count typography, original note width, direct note-card framing, and empty state without a 0-count heading'
)

assert(
  addForm.includes('class="tb-btn nw-action-btn state-toggle-btn full-image-btn') &&
    addForm.indexOf('<AudioRecorder') < addForm.indexOf('data-tooltip="上传图片"') &&
    addForm.includes('class="tb-btn nw-action-btn state-toggle-btn notify-btn') &&
    addForm.indexOf('data-tooltip="上传图片"') < addForm.indexOf('<VideoUpload') &&
    addForm.indexOf('<VideoUpload') < addForm.indexOf('data-tooltip="图床上传"') &&
    addForm.includes(':data-tooltip="`全图显示：${fullImageAttachments ? \'已开启\' : \'已关闭\'}`"') &&
    addForm.includes(':data-tooltip="`推送：${enableNotify ? \'已开启\' : \'已关闭\'}`"') &&
    addForm.includes(":aria-pressed=\"fullImageAttachments\"") &&
    addForm.includes(":aria-pressed=\"enableNotify\"") &&
    addForm.includes("const FULL_IMAGE_ATTACHMENTS_MARKER = '<!-- noise-full-image-attachments -->'") &&
    addForm.includes('fullImageAttachments: !!fullImageAttachments.value') &&
    addForm.includes('content: buildPublishContent(MessageContent.value)') &&
    addForm.includes("applyImageGridHTML(raw, keepImagesFullSize)") &&
    addForm.includes('.editor-preview :deep(.full-image-attachment img)') &&
    markdownRenderer.includes('const FULL_IMAGE_ATTACHMENTS_MARKER_RE = /<!--\\s*noise-full-image-attachments\\s*-->\\s*/gi') &&
    markdownRenderer.includes('const keepImagesFullSize = hasFullImageAttachmentsMarker(markdown ?? \'\')') &&
    markdownRenderer.includes("wrapper.className = 'full-image-attachment'") &&
    markdownRenderer.includes('wrapper.appendChild(ensureImageAnchor(node, group))') &&
    markdownRenderer.includes('applyImageGrid(keepImagesFullSize)') &&
    markdownRenderer.includes('.markdown-preview :deep(.full-image-attachment img)') &&
    markdownRenderer.includes('height: auto !important;') &&
    markdownRenderer.includes('object-fit: contain !important;'),
  'publish composer must keep upload image before upload video, expose icon-only full-image and push toggles with current-state tooltips, persist the hidden marker, and render marked image attachments as responsive full images with Fancybox links'
)

assert(
  audioRecorder.includes('<div class="audio-recorder-control">') &&
  audioRecorder.includes('ref="triggerRef"\n      type="button"') &&
  !audioRecorder.includes('<div ref="triggerRef" class="audio-recorder-control">') &&
  audioRecorder.includes("positionFloatingMenu(triggerRef.value, menuRef.value, menuStyle, 292, 'above-align-left')") &&
  audioRecorder.includes("const canPause = computed(() => (isRecording.value || isPaused.value) && !!recorder && !isProcessing.value)") &&
  audioRecorder.includes("const canStop = computed(() => (isRecording.value || isPaused.value) && !!recorder && !isProcessing.value)") &&
  audioRecorder.includes('class="floating-action-btn clear-action-btn nw-action-btn nw-action-btn--label nw-action-btn--danger"') &&
  audioRecorder.includes('analyser.getByteTimeDomainData(raw)') &&
  audioRecorder.includes('spectrumLevels[i] = spectrumLevels[i] * 0.68 + target * 0.32') &&
  audioRecorder.includes('drawRoundRect(x, y, barWidth, barHeight, 999)') &&
  !audioRecorder.includes('analyser.getByteFrequencyData(raw)') &&
  markdownRenderer.includes("audio.style.border = 'none';") &&
  messageList.includes(':global(html.dark) .content-container :deep(audio) {\n  background-color: var(--home-surface-dark) !important;\n  border: none !important;'),
  'audio recorder must left-align its floating menu to the real button, keep pause/stop reactive, keep stop as a persistent danger button, draw smoothed time-domain bars, and render published audio without an extra frame'
)

assert(
  mediaUpload.includes("return `\\n[${label}：${sanitizeAttachmentName(name, url)}](${url})\\n`") &&
    mediaUpload.includes("createAttachmentMarkdown('image', url, name)") &&
    mediaUpload.includes("createAttachmentMarkdown('video', url, name)") &&
    mediaUpload.includes("createAttachmentMarkdown('audio', url, name)") &&
    markdownRenderer.includes('const ATTACHMENT_LINK_REG = /\\[(图片附件|视频附件|音频附件)：([^\\]]+)\\]\\(([^)\\s]+)\\)/g') &&
    markdownRenderer.includes('buildAttachmentHtml(kindLabel, name, url)') &&
    markdownRenderer.includes('noise-attachment-audio') &&
    !markdownRenderer.includes('noise-attachment-render--audio') &&
    markdownRenderer.includes('noise-attachment-render--video') &&
    markdownRenderer.includes('noise-attachment-paragraph') &&
    markdownRenderer.includes('noise-attachment-image') &&
    addForm.includes('replaceAttachmentMarkersForPreview') &&
    addForm.includes('const ATTACHMENT_LINK_REG = /\\[(图片附件|视频附件|音频附件)：([^\\]]+)\\]\\(([^)\\s]+)\\)/g') &&
    vditorEditor.includes('setupAttachmentPreview()') &&
    vditorEditor.includes('editor-attachment-preview') &&
    vditorEditor.includes('refreshAttachmentLinks') &&
    vditorEditor.includes('editor-attachment-link') &&
    vditorEditor.includes('attachmentInfoFromIrNode') &&
    vditorEditor.includes('attachmentInfoFromIrLabel') &&
    vditorEditor.includes("root.querySelectorAll('[data-type=\"a\"]')") &&
    vditorEditor.includes("target.closest<HTMLElement>('.vditor-ir__link.editor-attachment-link')") &&
    vditorEditor.includes("target.closest('a.editor-attachment-link')") &&
    vditorEditor.includes('showAttachmentGallery(getAttachmentInfosByType(info.type), info)') &&
    !vditorEditor.includes('buildVideoFancyboxHtml') &&
    vditorEditor.includes('buildAttachmentPreviewHtml') &&
    vditorEditor.includes('transform: transformAttachmentPreviewHtml') &&
    vditorEditor.includes('const showImageInProjectViewer = (info: EditorAttachmentInfo) => showAttachmentGallery([info], info)') &&
    vditorEditor.includes('Fancybox.show') &&
    vditorEditor.includes('getAttachmentImageFancyboxOptions') &&
    vditorEditor.includes('getAttachmentVideoFancyboxOptions') &&
    vditorEditor.includes("left: ['infobar']") &&
    vditorEditor.includes("right: ['iterateZoom', 'slideshow', 'fullscreen', 'thumbs', 'close']") &&
    !vditorEditor.includes("right: ['slideshow', 'fullscreen', 'thumbs', 'close']") &&
    vditorEditor.includes('Html: {') &&
    vditorEditor.includes('videoAutoplay: false') &&
    vditorEditor.includes('getVideoFirstFrameThumbnail') &&
    vditorEditor.includes("await Promise.all(galleryItems.map((item) => getVideoFirstFrameThumbnail(item.url)))") &&
    vditorEditor.includes("return { src: item.url, type: 'html5video', thumbSrc: thumb, poster: thumb }") &&
    !vditorEditor.includes('caption: item.name') &&
    vditorEditor.includes('autoStart: true') &&
    vditorEditor.includes('Images: {') &&
    vditorEditor.includes("mainClass: 'noise-media-fancybox'") &&
    !vditorEditor.includes('editor-attachment-image-fancybox') &&
    !vditorEditor.includes('editor-attachment-video-fancybox') &&
    !vditorEditor.includes("type: 'video'") &&
    vditorEditor.includes('Thumbs: {') &&
    vditorEditor.includes('Carousel: { infinite: true }') &&
    !vditorEditor.includes("backdropClick: 'close'") &&
    vditorEditor.includes('closeButton: false') &&
    vditorEditor.includes('middle: []') &&
    vditorEditor.includes('top: max(0px, env(safe-area-inset-top, 0px));') &&
    vditorEditor.includes('right: max(0px, env(safe-area-inset-right, 0px));') &&
    vditorEditor.includes('width: min(300px, 100%);') &&
    vditorEditor.includes('TABLE_SIZE_LIMIT = 10') &&
    vditorEditor.includes('Array.from({ length: TABLE_SIZE_LIMIT * TABLE_SIZE_LIMIT }') &&
    vditorEditor.includes("positionFloatingMenu(tableTrigger.value, tableMenuRef.value, tableMenuStyle, 324, 'above-align-left')") &&
    vditorEditor.includes('width: 324px !important;') &&
    vditorEditor.includes('min-width: 324px !important;') &&
    vditorEditor.includes('grid-template-columns: repeat(10, 24px);') &&
    vditorEditor.includes('width: 24px !important;') &&
    !vditorEditor.includes('.vditor-reset table th {\n  background:') &&
    !vditorEditor.includes('html.dark .vditor-reset table th { background:') &&
    vditorEditor.includes("const header = Array.from({ length: colCount }, () => ' ')") &&
    !vditorEditor.includes('`列 ${index + 1}`') &&
    vditorEditor.includes('getCurrentEditorTableCell') &&
    vditorEditor.includes('normalizeTableCellInsertion') &&
    vditorEditor.includes('insertValueIntoCurrentTableCell') &&
    vditorEditor.includes("range.insertNode(textNode)") &&
    vditorEditor.includes("inputType: 'insertText'") &&
    vditorEditor.includes('if (insertValueIntoCurrentTableCell(val)) return') &&
    vditorEditor.includes('enhanceEditorTables(root)') &&
    vditorEditor.includes('deleteSelectedEditorTable') &&
    vditorEditor.includes('showTableDeleteButton') &&
    vditorEditor.includes('tableDeleteButtonStyle') &&
    vditorEditor.includes("Teleport to=\"body\"") &&
    vditorEditor.includes('editor-table-delete-button') &&
    vditorEditor.includes('confirm(\'确定要删除该表格吗？\')') &&
    vditorEditor.includes("root.addEventListener('pointermove', onTablePointerMove, true)") &&
    vditorEditor.includes("root.addEventListener('pointerout', onTablePointerOut, true)") &&
    !vditorEditor.includes('editor-table-select-handle') &&
    vditorEditor.includes('.vditor-reset table.editor-table-selected') &&
    markdownRenderer.includes('.markdown-preview :deep(.noise-attachment-audio)') &&
    markdownRenderer.includes('.markdown-preview :deep(.noise-attachment-audio--table)') &&
    markdownRenderer.includes('width: min(300px, 100%) !important;') &&
    markdownRenderer.includes('max-width: 300px;') &&
    markdownRenderer.includes('min-width: min(220px, 100%);') &&
    messageList.includes("mainClass: 'noise-media-fancybox'") &&
    messageList.includes("left: ['infobar']") &&
    messageList.includes("right: ['iterateZoom', 'slideshow', 'fullscreen', 'thumbs', 'close']") &&
    messageList.includes('Images: {') &&
    messageList.includes('Html: { videoAutoplay: false }') &&
    messageList.includes("Thumbs: { type: 'classic', autoStart: true }") &&
    !/\bImage:\s*\{/.test(messageList) &&
    !messageList.includes('window.Fancybox.destroy()') &&
    messageList.includes(':deep(.noise-media-fancybox .fancybox__toolbar)') &&
    !messageList.includes(':deep(.noise-media-fancybox .fancybox__infobar)') &&
    vditorEditor.includes('collapseIrAttachmentChrome') &&
    vditorEditor.includes('scheduleCollapseIrAttachmentChrome') &&
    vditorEditor.includes('scheduleRefreshAttachmentLinks') &&
    vditorEditor.includes('normalizeAttachmentInsertValue') &&
    vditorEditor.includes('normalizeEditorAttachmentSource') &&
    vditorEditor.includes('normalizeAdjacentAttachmentMarkers') &&
    vditorEditor.includes('ADJACENT_ATTACHMENT_MARKER_RE') &&
    vditorEditor.includes("return `\\n\\n${normalized}\\n\\n`") &&
    vditorEditor.includes("marker.setAttribute('contenteditable', 'false')") &&
    vditorEditor.includes('onPlainTextEnterKeydown') &&
    vditorEditor.includes("document.createTextNode('\\n')") &&
    !vditorEditor.includes("document.execCommand('insertLineBreak')") &&
    !vditorEditor.includes("inputType: 'insertLineBreak'") &&
    vditorEditor.includes("marker.classList.remove('vditor-ir__node--expand')") &&
    vditorEditor.includes('previewObserver.observe(root, { childList: true, subtree: true })') &&
    !vditorEditor.includes('characterData: true') &&
    markdownRenderer.includes('initializeMediaViewer') &&
    markdownRenderer.includes('Fancybox.bind(root, \'[data-fancybox]\'') &&
    markdownRenderer.includes("right: ['iterateZoom', 'slideshow', 'fullscreen', 'thumbs', 'close']") &&
    markdownRenderer.includes("mainClass: 'noise-media-fancybox'") &&
    markdownRenderer.includes('Html: { videoAutoplay: false }') &&
    markdownRenderer.includes("Thumbs: { type: 'classic', autoStart: true }") &&
    markdownRenderer.includes("video.dataset.type = 'html5video'") &&
    !markdownRenderer.includes("video.dataset.type = 'video'") &&
    homePage.includes('projectFancyboxOptions') &&
    homePage.includes("Fancybox?.bind?.('[data-fancybox]', projectFancyboxOptions)") &&
    homePage.includes("mainClass: 'noise-media-fancybox'") &&
    !homePage.includes("Fancybox?.bind?.('[data-fancybox]', {})") &&
    !markdownRenderer.includes('mediumZoom(') &&
    vditorEditor.includes('stopImmediatePropagation') &&
    vditorEditor.includes("root.addEventListener('pointerdown', preventAttachmentNavigation, true)") &&
    vditorEditor.includes("root.addEventListener('mousedown', preventAttachmentNavigation, true)") &&
    vditorEditor.includes("document.addEventListener('selectionchange', onEditorSelectionChange, true)") &&
    vditorEditor.includes("root.addEventListener('keydown', onPlainTextEnterKeydown, true)") &&
    vditorEditor.includes("root.addEventListener('keydown', onAttachmentKeydown, true)") &&
    vditorEditor.includes('.editor-attachment-node .vditor-ir__marker--link') &&
    vditorEditor.includes('.editor-attachment-node .vditor-ir__marker--paren') &&
    !vditorEditor.includes('suppressIrAttachmentChrome') &&
    !vditorEditor.includes('irAttachmentNodeNearPointer') &&
    !vditorEditor.includes('editor-attachment-marker-block') &&
    !vditorEditor.includes('lineFromPoint(event, root)') &&
    !vditorEditor.includes('is-hovering-attachment-line') &&
    !vditorEditor.includes("root.addEventListener('mousemove', onAttachmentMouseMove") &&
    !vditorEditor.includes('editor-attachment-preview editor-attachment-preview--image'),
  'inserted image/video/audio attachments must use stable attachment links in the editor, render to media components in published markdown, use image viewer controls for image markers, keep video side navigation without top prev/next controls, preview only from marker text, and collapse raw HTML expansion around attachment markers without blocking text editing'
)

assert(
  audioRecorder.includes('let recordingStartedAt = 0') &&
    audioRecorder.includes('const recordingFileName = (type: string) => {') &&
    audioRecorder.includes('const userPart = safeNameSegment(user?.userid ?? user?.id ?? user?.username ?? \'user\')') &&
    !audioRecorder.includes('Math.random().toString(36).slice(2, 8)') &&
    audioRecorder.includes('new File([blob], recordingFileName(type), { type })'),
  'recorded audio filenames must include date and user identity, without an unnecessary random suffix'
)

assert(
  messageList.includes("const measureEl = (el.querySelector('.markdown-preview') as HTMLElement | null) || el;") &&
    messageList.includes("measureEl.querySelectorAll('img, video, audio')") &&
    messageList.includes("item.addEventListener('loadedmetadata', schedule)") &&
    messageList.includes("item.addEventListener('loadeddata', schedule)") &&
    messageList.includes("item.addEventListener('canplay', schedule)") &&
    messageList.includes('const schedule = () => deferMeasure();') &&
    messageList.includes('new ResizeObserver(() => deferMeasure())') &&
    messageList.includes('measuredMessageHeights') &&
    messageList.includes('const needsExpand = fullHeight > 708;') &&
    !messageList.includes('setTimeout(() => deferMeasure(), 420)'),
  'message expand measurement must observe rendered markdown content and re-check media metadata without threshold-edge layout churn'
)

assert(
  messageList.includes('margin: 16px 0 72px;') &&
    infoFeedList.includes('class="pager-btn nw-action-btn nw-action-btn--label"') &&
    infoFeedList.includes('class="pager-nav-group"') &&
    infoFeedList.includes('class="pager-jump-group"') &&
    infoFeedList.includes('class="pager-page-text"') &&
    infoFeedList.includes(":class=\"{ 'is-dark': contentTheme === 'dark' }\"") &&
    infoFeedList.includes('border-radius: 999px;') &&
    infoFeedList.includes('margin: 16px 0 72px;') &&
    infoFeedList.includes('box-shadow: 0 8px 22px rgba(15, 23, 42, 0.10);') &&
    !infoFeedList.includes('class="pager-main"') &&
    !infoFeedList.includes('class="pager-meta"') &&
    !infoFeedList.includes('<UButton\n          v-if="currentPage') &&
    homePage.includes('padding-bottom: calc(96px + env(safe-area-inset-bottom, 0px));') &&
    homePage.includes('scroll-padding-bottom: calc(160px + env(safe-area-inset-bottom, 0px));'),
  'pagination must keep explicit bottom clearance from the browser viewport and the info feed pager must use the same shared shell/action-button template as notes'
)

assert(
  infoFeedList.includes('const measureFrame = ref<number | null>(null)') &&
    infoFeedList.includes('window.requestAnimationFrame(() => {') &&
    infoFeedList.includes('setFeedExpansionState(feedId, fullHeight > collapsedContentHeight + 8)') &&
    infoFeedList.includes('window.cancelAnimationFrame(measureFrame.value)') &&
    !infoFeedList.includes('fullHeight > collapsedContentHeight) {'),
  'info feed expand measurement must batch ResizeObserver work by animation frame and avoid threshold-edge layout churn while full-size media loads'
)

assert(
  homePage.includes('class="content-wrapper gpu-accelerated"') &&
    !homePage.includes('center-navigation-resetting') &&
    !homePage.includes('visibility: hidden;') &&
    !homePage.includes('centerStabilityStyle') &&
    !homePage.includes('centerHeightHoldCount') &&
    !homePage.includes('holdCenterHeight') &&
    !homePage.includes('releaseCenterHeightHold') &&
    !homePage.includes('@loading-change="handleMessageListLoadingChange"') &&
    !homePage.includes('usesCenterColumnScroll') &&
    !homePage.includes('.content-wrapper {\n    overflow-y: hidden;') &&
    !homePage.includes('overflow-y: auto !important;') &&
    homePage.includes('const getMainScrollElement = () => {') &&
    homePage.includes("return contentWrapper.value || document.querySelector('.content-wrapper') as HTMLElement | null") &&
    homePage.includes('const resetContentScrollInstant = () => {') &&
    homePage.includes('const el = getMainScrollElement()') &&
    homePage.includes('el.scrollTop = 0') &&
    homePage.includes('const runCenterNavigationReset = async (mutate: () => void) => {') &&
    homePage.includes('mutate()\n  await nextTick()\n  resetContentScrollInstant()') &&
    !homePage.includes('waitForNextFrame') &&
    !homePage.includes('requestAnimationFrame(() => resolve())') &&
    messageList.includes('const getAppScrollContainer = (target?: HTMLElement | null) => {') &&
    messageList.includes('const candidates = [') &&
    messageList.includes("target?.closest('.content-wrapper') as HTMLElement | null") &&
    messageList.includes('return candidates.find(isScrollableY) || candidates.find(Boolean) || null') &&
    builtinComments.includes("document.querySelector('.center-col') as HTMLElement | null") &&
    homePage.includes('const switchActiveTab = async (tab: string, options: { resetScroll?: boolean } = {}) => {') &&
    homePage.includes('await runCenterNavigationReset(() => { activeTab.value = tab })') &&
    homePage.includes('@click="switchActiveTab(t.key, { resetScroll: true })"') &&
    homePage.includes("switchActiveTab('comment', { resetScroll: true })") &&
    homePage.includes("switchActiveTab('notifications', { resetScroll: true })") &&
    homePage.includes('await runCenterNavigationReset(() => {\n    ensureMessageTab()\n    searchKeyword.value = String(keyword || \'\').trim()') &&
    homePage.includes('await runCenterNavigationReset(() => {\n    ensureMessageTab()\n    selectedTag.value = selectedTag.value === normalizedTag ? \'\' : normalizedTag'),
  'center navigation must keep the shared content-wrapper as the desktop/tablet scroll container, reset it after DOM patch, and avoid hiding or height-locking the center column'
)

assert(
  homePage.indexOf('const frontendConfig = ref<any>({})') > -1 &&
    homePage.indexOf('const frontendConfig = ref<any>({})') < homePage.indexOf('const isFeedEnabled = computed(() =>') &&
    homePage.includes('Object.assign(frontendConfig.value, {') &&
    !homePage.includes('const frontendConfig = ref<any>({\n'),
  'homepage frontendConfig must be initialized before computed values read it, avoiding production TDZ startup errors'
)

assert(
  markdownRenderer.includes('const updateTaskListContent = (content: string, taskIndex: number, checked: boolean) => {') &&
    markdownRenderer.includes('const persistTaskListChange = async (input: HTMLInputElement, taskIndex: number, checked: boolean) => {') &&
    markdownRenderer.includes('const response = await messageStore.updateMessage(Number(props.messageId), nextContent)') &&
    markdownRenderer.includes('if (!response) throw new Error') &&
    markdownRenderer.includes('input.disabled = !props.taskListEditable') &&
    markdownRenderer.includes("input.style.pointerEvents = props.taskListEditable ? 'auto' : 'none'") &&
    markdownRenderer.includes("return Array.from(root.querySelectorAll<HTMLInputElement>('input[type=\"checkbox\"]'))") &&
    markdownRenderer.includes('const taskCheckedInContent = (content: string, taskIndex: number) => {') &&
    markdownRenderer.includes('const resetTaskCheckbox = (input: HTMLInputElement, taskIndex = taskIndexForInput(input)) => {') &&
    markdownRenderer.includes('input.onclick = (event) => {') &&
    markdownRenderer.includes('input.onchange = async (event) => {') &&
    markdownRenderer.includes("previewElement.value?.addEventListener('click', onTaskListClick, true)") &&
    markdownRenderer.includes("previewElement.value?.addEventListener('change', onTaskListChange, true)") &&
    markdownRenderer.includes('taskListObserver = new MutationObserver(() => scheduleTaskListEnhance())') &&
    markdownRenderer.includes('.markdown-preview[data-task-list-editable="false"] input[type="checkbox"]') &&
    messageList.includes(':task-list-editable="canEditMessageTasks(msg)"') &&
    messageList.includes(':message-id="Number(msg.id)"') &&
    messageList.includes('const canEditMessageTasks = (msg: any) =>') &&
    messageList.includes('userStore.isLogin && (currentUserIsAdmin.value || isCurrentUserMessage(msg))'),
  'published markdown task lists must persist checkbox changes through the message update API and only allow authors/admins to interact'
)

assert(
  messageList.includes('const result = await fetchListPage(pageQueryFor(targetPage));') &&
    !messageList.includes('listRefreshController') &&
    !messageList.includes('message.getMessages(pageQueryFor('),
  'message pagination must reuse the unified page loader and must not keep stale local AbortController references'
)

assert(
  homePage.includes('transition: background-color .15s ease, border-color .15s ease, color .15s ease;') &&
    homePage.includes('.layout-container.grid-3 {\n  display: grid;\n  grid-template-columns: var(--sidebar-width, 320px) minmax(0, 1fr) var(--sidebar-width, 320px);') &&
    homePage.includes('.layout-container.grid-2 {\n  display: grid;\n  grid-template-columns: var(--sidebar-width, 320px) minmax(0, 1fr);') &&
    homePage.includes('.left-col, .right-col { position: sticky; top: 0; align-self: start; height: fit-content; width: 100%; min-width: 0; box-sizing: border-box; }') &&
    homePage.includes('scrollbar-gutter: stable;') &&
    homePage.includes('.right-col > * {\n  width: 100%;\n  min-width: 0;\n  box-sizing: border-box;\n}') &&
    calendarWidget.includes('.calendar-widget {\n  width: 100%;\n  min-width: 0;\n  box-sizing: border-box;') &&
    homePage.includes('.auth-btn:hover { background: transparent !important; transform: none !important; }') &&
    homePage.includes('.avatar-lg:hover { transform: none; }') &&
    homePage.includes('.social-item:hover { transform: none; transition: none; }') &&
    homePage.includes('.right-col :where(.avatar-lg, .auth-btn, .social-item, .calendar-card button, .recommend-image-box, .ad-image)') &&
    homePage.includes('.right-col :deep(*:hover),') &&
    homePage.includes('transition-property: background-color, border-color, color, opacity, box-shadow, filter !important;') &&
    infoFeedList.includes('.expand-toggle-btn:hover {\n  transform: none;\n}') &&
    !homePage.includes('grid-template-columns: minmax(260px, var(--sidebar-width, 320px))') &&
    !homePage.includes('transform: scale(1.02);') &&
    !homePage.includes('transform: translateY(-1px);'),
  'right sidebar width and hover states must stay stable after calendar filtering or feed measurement'
)

assert(
  messageStore.includes('const currentListQueryKey = ref("")') &&
    messageStore.includes('const listQueryKey = (query: PageQuery) => JSON.stringify({') &&
    messageStore.includes('const requestListKey = listQueryKey(query);') &&
    messageStore.includes('currentListQueryKey.value = requestListKey;') &&
    messageStore.includes('messages.value = response.data.items;') &&
    messageStore.includes('listQueryKey,'),
  'message store must expose the active list query key so filtered views can hide stale results while a new query is loading'
)

assert(
  homePage.includes('<div class="header-subtitle" :style="activeHeaderTextStyle.subtitle">{{ frontendConfig.subtitleText || \'\' }}</div>') &&
    !homePage.includes('subtitleEl') &&
    !homePage.includes('startTypeEffect') &&
    !homePage.includes('textContent = frontendConfig.value.subtitleText') &&
    !homePage.includes('setInterval(() => {\n    if (!subtitleEl.value)'),
  'home subtitle must render as stable text instead of a looping typewriter effect that mutates the shared center header during navigation'
)

console.log('frontend layout edge cases checks passed')
