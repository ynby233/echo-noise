<!-- Message-list orchestrator: owns paging, item identity and permission-aware
actions. Editing, media enhancement, engagement state, target navigation and
toolbox listeners are delegated to modules with their own cleanup. -->
<template>
  <div ref="messageListRoot" :class="{ 'message-list-wide': props.wide, 'message-list-masonry': props.masonry }">
    <div class="min-h-screen flex flex-col">
      <!-- 空状态显示 -->
      <div v-if="props.pageReady && !hasActiveFilters && !displayMessages.length" class="text-center text-gray-500 py-8">
        <div v-if="isPageLoading">
          <p>加载中...</p>
        </div>
        <div v-else-if="isPersonalGuest">
          <UIcon name="i-heroicons-user-circle" class="w-12 h-12 mx-auto mb-4" />
          <p>请先登录查看个人笔记</p>
          <p class="text-xs mt-2 opacity-70">登录后这里只显示你自己发表的内容</p>
        </div>
        <div v-else>
          <UIcon :name="isPersonalTab ? 'i-heroicons-document-text' : 'i-heroicons-inbox'" class="w-12 h-12 mx-auto mb-4" />
          <p>{{ isPersonalTab ? '暂无个人笔记' : '暂无消息内容' }}</p>
        </div>
      </div>

      <div :class="outerContainerClass">
        <component
          :is="props.pageReady && hasActiveFilters ? resolveComponent('UCard') : 'div'"
          :class="props.pageReady && hasActiveFilters ? ['search-card', 'search-results-panel', 'nw-content-panel-surface', 'mb-3', { 'is-dark': isContentDark }] : ''"
          v-bind="props.pageReady && hasActiveFilters ? { ui: { body: { padding: 'p-5 md:p-6' } } } : {}"
        >
          <div v-if="props.pageReady && hasActiveFilters" class="search-results-head nw-content-panel-head">
            <div class="search-results-heading nw-content-panel-heading">
              <div class="search-results-title nw-content-panel-title">搜索</div>
              <div class="search-results-summary nw-content-panel-summary">搜索内容：{{ activeFilterContent }}</div>
            </div>
          </div>
          <div v-if="props.pageReady && hasActiveFilters" class="search-results-board-head nw-content-panel-toolbar">
            <div
              v-if="!isPageLoading && !isDisplayQueryPending && displayMessages.length"
              class="search-results-count nw-content-panel-count"
            >
              笔记 ({{ filteredResultCount }})
            </div>
            <div v-else class="search-results-count nw-content-panel-count nw-content-panel-count-placeholder search-results-count-placeholder" aria-hidden="true"></div>
            <div class="search-results-actions nw-content-panel-actions">
              <button
                type="button"
                class="search-results-refresh nw-content-panel-action nw-content-panel-action--icon nw-action-btn nw-tooltip-anchor"
                data-tooltip="刷新"
                aria-label="刷新"
                :disabled="searchResultsRefreshing || isPageLoading || isDisplayQueryPending"
                @click="refreshSearchResults"
              >
                <UIcon name="i-mdi-refresh" class="w-4 h-4" :class="{ 'animate-spin': searchResultsRefreshing }" />
              </button>
              <button type="button" class="search-results-back nw-content-panel-action nw-action-btn nw-action-btn--label" @click="resetList">
                <UIcon name="i-heroicons-x-mark" class="w-4 h-4" />
                <span>返回完整列表</span>
              </button>
            </div>
          </div>
          <div v-if="props.pageReady && hasActiveFilters && (isPageLoading || isDisplayQueryPending || !displayMessages.length)" class="search-results-empty">
            <div v-if="isPageLoading || isDisplayQueryPending">
              <p>加载中...</p>
            </div>
            <div v-else>
              <UIcon name="i-heroicons-inbox" class="search-results-empty-icon" />
              <p>暂无消息内容</p>
            </div>
          </div>
          <!-- 消息列表 -->
          <div v-if="!props.pageReady || !hasActiveFilters || (!isDisplayQueryPending && displayMessages.length)" v-masonry="props.masonry" :class="[props.pageReady && hasActiveFilters ? 'search-results-list' : 'my-4', { 'masonry-grid': props.masonry }]">
        <!-- 消息列表内容 -->
        <div
          v-for="(msg, idx) in displayMessages"
          :key="msg.id"
          :class="['message-list-item w-full h-auto overflow-hidden flex flex-col justify-between', { 'file-attachment-shadow-open': isFileAttachmentShadowOpen(msg.id) }]"
        >

          <div class="p-0">
            <div :class="['content-container', innerContainerClass, listThemeClass, { 'is-dark': isContentDark, 'file-attachment-shadow-open': isFileAttachmentShadowOpen(msg.id), 'file-attachment-shadow-clip': isFileAttachmentShadowClipped(msg.id) }]" :data-msg-id="msg.id">
              <div class="flex items-center gap-2 mb-1 author-row">
                <span v-if="msg.is_tombstone" class="message-tombstone-icon"><UIcon name="i-heroicons-archive-box-x-mark" /></span>
                <img v-else :src="authorAvatar(msg)" alt="avatar" class="avatar-img w-9 h-9 rounded-full object-cover" @error="authorAvatarOnError($event, msg.username || '匿名')" @mouseenter="showAuthorCard($event, msg)" @mouseleave="hideAuthorCard" @click="toggleAuthorCard($event, msg)" />
                <div v-if="!msg.is_tombstone && openAuthorId === msg.id" class="site-author-card bg-white text-black dark:bg-[var(--home-surface-dark-elevated)] dark:text-white" :style="openAuthorStyle">
                  <div class="site-author-card-header">
                    <img :src="authorProfileAvatar(msg)" class="avatar-img w-10 h-10 rounded-full object-cover" />
                    <div class="font-semibold leading-tight text-[14px]">{{ msg.username }}</div>
                  </div>
                  <div class="site-author-card-body">
                    <div class="site-author-card-sign"><span :class="['site-author-card-scroll', { 'center': !authorSignShouldScroll(msg) }]">{{ authorProfileDesc(msg) }}</span></div>
                    <div class="author-card-muted text-[12px] whitespace-nowrap">笔记 {{ authorProfileCount(msg) }}</div>
                  </div>
                </div>
                <div class="min-w-0">
                  <div class="text-sm font-semibold leading-tight">{{ msg.is_tombstone ? '原笔记已永久删除' : (msg.username || siteConfig.username || '匿名') }}</div>
                  <div class="flex items-center gap-2">
                    <span class="text-xs opacity-70">{{ formatDate(msg.created_at) }}</span>
                  </div>
                </div>
                <div class="ml-auto flex items-center gap-2 text-xs opacity-80">
                  <span v-if="shouldShowMessageVisibility(msg)" class="visibility-indicator nw-tooltip-anchor" :data-tooltip="messageVisibilityLabel(messageVisibility(msg))" :aria-label="messageVisibilityLabel(messageVisibility(msg))">
                    <UIcon :name="messageVisibilityIcon(messageVisibility(msg))" class="w-4 h-4" />
                  </span>
                  <UIcon v-if="pinActive(msg)" name="i-mdi-pin" class="w-4 h-4 nw-tooltip-anchor" :data-tooltip="pinStatusLabel()" :aria-label="pinStatusLabel()" />
                </div>
              </div>
              
              <!-- 图片内容（支持放大预览 + 悬停效果） -->
              <div v-if="msg.is_tombstone" class="message-tombstone-panel">
                <strong>此处保留互动关系</strong>
                <span>原笔记正文、附件和作者展示信息已清除；仍可见的后代互动继续遵循原可见范围上限。</span>
              </div>
              <a v-if="!msg.is_tombstone && msg.image_url" :href="resolveMediaUrl(msg.image_url)" :data-fancybox="`message-image-${msg.id}`" :class="['message-image-wrap', messageImageAR[msg.id] || '']">
                <img 
                  :src="optimizeImage(resolveMediaUrl(msg.image_url))" 
                  alt="Image" 
                  class="message-image-box"
                  loading="lazy"
                  @load="onMessageImageLoad(msg.id, $event)"
                  :fetchpriority="idx < 3 ? 'high' : 'low'"
                  decoding="async"
                  sizes="(max-width: 640px) 100vw, 800px"
                />
              </a>
              <!-- 分隔线 -->
              <div v-if="msg.image_url && msg.content" class="border-t border-gray-600 my-2"></div>
              <!-- 文本内容区域 -->
              <div v-if="!msg.is_tombstone" class="overflow-y-hidden relative" :class="[{ 'max-h-[700px]': !isExpanded[msg.id] && !hasGrid[msg.id], 'file-attachment-shadow-open': isFileAttachmentShadowOpen(msg.id), 'file-attachment-shadow-clip': isFileAttachmentShadowClipped(msg.id) }, listThemeTextClass]" :style="contentStyle(idx)">
                <MarkdownRenderer
                  :content="msg.content"
                  :enableGithubCard="siteConfig?.enableGithubCard === true"
                  :message-id="Number(msg.id)"
                  :task-list-editable="canEditMessageTasks(msg)"
                  :inherit-font="true"
                  @tagClick="handleTagClick"
                  @rendered="checkContentHeight"
                  link-target="_blank"
                />
                <div v-if="shouldShowExpandButton[msg.id] && !isExpanded[msg.id]"
    :class="['absolute bottom-0 left-0 right-0 h-14 bg-gradient-to-t backdrop-blur-sm pointer-events-none content-fade-mask', gradientClass]" style="z-index:20"></div>
              </div>
              
              <div v-if="shouldShowExpandButton[msg.id]" class="expand-button-row">
                <button
                  type="button"
                  class="expand-toggle-btn nw-action-btn nw-action-btn--label"
                  @click="toggleExpand(msg.id)"
                  :aria-expanded="!!isExpanded[msg.id]"
                  :aria-label="isExpanded[msg.id] ? '收起全文' : '展开全文'"
                >
                  {{ isExpanded[msg.id] ? '收起全文' : '展开全文' }}
                  <UIcon :name="isExpanded[msg.id] ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" class="w-4 h-4 flex-shrink-0" />
                </button>
              </div>
              <div class="message-divider my-3"></div>
              <div v-if="!msg.is_tombstone" class="message-socialbar">
                <button v-if="canInteractWithMessage(msg)" class="social-item nw-tooltip-anchor" data-tooltip="点赞" aria-label="点赞" @click="like(msg.id)">
                  <UIcon
                    :name="(likedMap[msg.id] ? 'i-mdi-heart' : 'i-mdi-heart-outline')"
                    class="social-icon"
                    :class="[likedMap[msg.id] ? 'text-red-500' : '']"
                  />
                  <span :class="['opacity-80', isMobile ? 'text-xs' : 'text-sm']">{{ likesMap[msg.id] ?? (msg.like_count || 0) }}</span>
                </button>
                <button v-if="!isGuestbookMessage(msg) && canInteractWithMessage(msg)" class="social-item nw-tooltip-anchor" data-tooltip="评论" aria-label="评论" @click="toggleComment(msg.id)">
                  <UIcon name="i-mdi-comment-outline" class="social-icon" />
                  <span :class="['opacity-80', isMobile ? 'text-xs' : 'text-sm']">{{ commentCountMap[msg.id] || 0 }}</span>
                </button>
                <div class="flex-1 flex items-center justify-center">
                  <span v-if="isContentEmpty(msg)" class="text-xs text-orange-400 inline-flex items-center relative z-30">
                    <UIcon name="i-heroicons-arrow-path" class="w-4 h-4 animate-spin mr-1" />
                    加载内容中...
                  </span>
                </div>
                <div class="toolbox-anchor">
                  <UButton size="xs" color="gray" variant="ghost" :ui="{ base: 'rounded-full' }" class="tool-open-btn nw-tooltip-anchor" data-tooltip="展开工具" aria-label="展开工具" @click="toggleToolbox(msg.id)">
                    <UIcon name="i-heroicons-ellipsis-horizontal" style="font-size: 16px; line-height: 1;" />
                  </UButton>
                  <div class="message-toolbox overlay" v-show="openToolboxId === msg.id">
                      <div class="tool-icons">
                        <button v-if="canPin(msg)" type="button" class="tool-icon nw-action-btn nw-tooltip-anchor" :data-tooltip="pinTooltip(msg)" :aria-label="pinTooltip(msg)" @click="togglePin(msg)"><UIcon :name="pinActive(msg) ? 'i-mdi-pin' : 'i-mdi-pin-outline'" /></button>
                        <span v-else-if="pinActive(msg)" class="tool-icon nw-tooltip-anchor" :data-tooltip="pinStatusLabel()" :aria-label="pinStatusLabel()"><UIcon name="i-mdi-pin" /></span>
                      <button v-if="canEdit(msg)" type="button" class="tool-icon nw-action-btn nw-tooltip-anchor" data-tooltip="编辑" @click="editMessage(msg)"><UIcon name="i-mdi-pencil-outline" /></button>
                      <button type="button" class="tool-icon nw-action-btn nw-tooltip-anchor" data-tooltip="复制" @click="copyContent(msg.content)"><UIcon name="i-mdi-content-copy" /></button>
                      <button v-if="canDelete(msg)" type="button" class="tool-icon nw-action-btn nw-action-btn--danger nw-tooltip-anchor" data-tooltip="删除" @click="deleteMsg(msg)"><UIcon name="i-mdi-close-octagon-outline" /></button>
                  </div>
                  </div>
                </div>
              </div>
              <div v-if="(msg.is_tombstone || expandedCommentsMap[msg.id] || activeCommentId === msg.id) && isCommentEnabled && !isGuestbookMessage(msg)" :id="`comment-container-${msg.id}`" class="mt-2" style="position: relative;">
                <BuiltinComments
                  v-if="apiReachable"
                  :key="(commentRefreshKey[msg.id] || 0)"
                  :ref="builtinCommentsRefFor(msg.id)"
                  :message-id="msg.id"
                  :message-owner-id="msg.user_id"
                  :message-visibility="msg.visibility"
                  :can-interact="canInteractWithMessage(msg)"
                  :site-config="siteConfig"
                  :show-input="!msg.is_tombstone && activeCommentId === msg.id"
                  auto-scroll-input
                  @cancel="handleCancel(msg.id, $event)"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
        </component>
      <ContinuousLoadTrigger v-if="props.masonry && props.pageReady && !isPersonalGuest" :loading="isPageLoading || continuousLoading || !!props.targetMessageId || !targetListReady" :has-more="!!message.hasMore" :error="continuousError" @load="loadContinuousPage" />
      <!-- 预取下一页哨兵 -->
      <div v-if="showPager" ref="prefetchSentinel" style="height:1px"></div>
      <!-- 分页控制区域 -->
      <div v-if="showPager" class="pager-shell" :class="{ 'is-dark': isContentDark }">
        <div class="pager-nav-group">
          <button
            v-if="message.page > 1"
            type="button"
            class="pager-btn nw-action-btn nw-action-btn--label"
            @click="loadPreviousPage"
            :disabled="isPageLoading"
          >
            <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-left" class="w-4 h-4 pager-icon" /></span>
            <span>上一页</span>
          </button>

          <button
            v-if="message.hasMore"
            type="button"
            class="pager-btn nw-action-btn nw-action-btn--label"
            @click="loadNextPage"
            :disabled="isPageLoading"
          >
            <span>下一页</span>
            <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-right" class="w-4 h-4 pager-icon" /></span>
          </button>
          <span v-if="isPageLoading" class="pager-status-text">加载中...</span>
        </div>

        <!-- 页码显示和跳转 -->
        <div class="pager-jump-group">
          <span class="pager-page-text">第</span>
          <div class="pager-number-control">
            <input
              v-model="targetPage"
              type="text"
              inputmode="numeric"
              pattern="[0-9]*"
              class="pager-page-input"
              placeholder="#"
              aria-label="跳转页码"
              @keyup.enter="jumpToPage"
            />
            <div class="pager-stepper" aria-label="页码增减">
              <button
                type="button"
                class="pager-stepper-btn nw-action-btn"
                aria-label="页码加一"
                :disabled="isPageLoading"
                @click="adjustTargetPage(1)"
              >
                <UIcon name="i-heroicons-chevron-up-20-solid" class="w-3 h-3" />
              </button>
              <button
                type="button"
                class="pager-stepper-btn nw-action-btn"
                aria-label="页码减一"
                :disabled="isPageLoading"
                @click="adjustTargetPage(-1)"
              >
                <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
              </button>
            </div>
          </div>
          <span class="pager-page-text">页 / 共 {{ totalPages }} 页</span>
          <button
            type="button"
            class="pager-jump-btn nw-action-btn nw-action-btn--label"
            @click="jumpToPage"
            :disabled="isPageLoading"
          >
            跳转
          </button>
        </div>
      </div>
      <!-- 加载完毕提示 -->
      <div v-if="!props.masonry && message.messages.length > 0 && !message.hasMore" class="pager-done-wrap">
        <UIcon name="i-fluent-emoji-flat-confetti-ball" size="lg" />
        <span class="pager-done-text">加载完毕~</span>
      </div>
    </div>
    
</div>
  <MessageEditDialog
    ref="messageEditDialogRef"
    :site-config="siteConfig"
    :is-content-dark="isContentDark"
    @saved="applyEditedMessage"
  />

  </div>
</template>

<script setup lang="ts">
import ContinuousLoadTrigger from './ContinuousLoadTrigger.vue'
import { vMasonry } from '~/directives/masonry'
import { resolveComponent } from 'vue'
import { useMessageStore } from "~/store/message";
import { useUserStore } from "~/store/user";
import MarkdownRenderer from "~/components/index/MarkdownRenderer.vue";
import MessageEditDialog from './MessageEditDialog.vue'
import { messageVisibility, messageVisibilityIcon, messageVisibilityLabel } from '~/utils/message-visibility'
import { asyncFeature } from '~/utils/async-feature'
const BuiltinComments = asyncFeature(() => import('../comments/BuiltinComments.vue'), '评论')
import { writeClipboardText } from '~/utils/clipboard'
import { resolveManagedAttachmentURL } from '~/utils/media-url'
import { getMessageIdFromRouteHash } from '~/utils/message-route-hash'
import { createMessageToolbox } from '~/utils/message-toolbox'
import { createMessageContentLayout } from '~/utils/message-content-layout'
import { createMessageListMedia } from '~/utils/message-list-media'
import { createMessageListEngagement } from '~/utils/message-list-engagement'
import { createMessageTargetNavigation, getMessageListScrollContainer, isMessageListScrollable } from '~/utils/message-target-navigation'
import { shouldShowVisibilityBadge } from '~/utils/visibility-badge'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { getRequest, postRequest } from '~/utils/api'
import { useRuntimeConfig } from '#imports'
import { useToast } from '#ui/composables/useToast'
const config = useRuntimeConfig()
const BASE_API = config.public.baseApi || '/api'
const messageListRoot = ref<HTMLElement | null>(null)
const isMobile = typeof window !== 'undefined' && window.matchMedia('(max-width: 1024px)').matches

const resolveMediaUrl = (s: string) => resolveManagedAttachmentURL(BASE_API, s)

const messageImageAR = ref<Record<number, string>>({})
const onMessageImageLoad = (id: number, e: Event) => {
  const img = e.target as HTMLImageElement | null
  if (!img) return
  const w = Number(img.naturalWidth || 0)
  const h = Number(img.naturalHeight || 0)
  if (!w || !h) return
  if (h > w) messageImageAR.value[id] = 'ar-11'
  else if (w > h) messageImageAR.value[id] = 'ar-169'
  else messageImageAR.value[id] = 'ar-11'
}
const authorAvatar = (msg: any) => {
  const msgAvatar = String((msg?.avatar_url || (msg as any)?.AvatarURL || '')).trim()
  if (msgAvatar) return resolveMediaUrl(msgAvatar)
  const unameMsg = String(msg?.username || '').trim()
  const prof = authorProfiles.value[unameMsg]
  const profAvatar = String((prof && prof.avatar_url) || '').trim()
  if (profAvatar) return resolveMediaUrl(profAvatar)
  if (prof && (prof.is_admin || prof.IsAdmin)) {
    const adminFallback = String(((props.siteConfig as any)?.avatarURL || '')).trim()
    if (adminFallback) return resolveMediaUrl(adminFallback)
  }
  const uname = String(((useUserStore().user as any)?.username || '')).trim()
  const uav = String((((useUserStore().user as any)?.avatar_url || (useUserStore().user as any)?.AvatarURL) || '')).trim()
  if (uname && String(msg?.username || '').trim() === uname && uav) return resolveMediaUrl(uav)
  return resolveMediaUrl(String(((props.siteConfig as any)?.avatarURL || (props.siteConfig as any)?.rssFaviconURL || '/favicon.svg')).trim())
}
const authorAvatarOnError = (e: Event, seed: string) => {
  const img = e.target as HTMLImageElement
  const fallback = resolveMediaUrl(String(((props.siteConfig as any)?.avatarURL || (props.siteConfig as any)?.rssFaviconURL || '/favicon.svg')).trim())
  if (!img || !fallback) return
  const fallbackURL = new URL(fallback, window.location.href).href
  if (img.src !== fallbackURL) img.src = fallback
}
// 主题切换改为纯 CSS（html.dark）控制，避免组件重渲染导致媒体刷新

const contentStyle = (index: number) => {
  return index < 5 ? '' : 'content-visibility:auto;contain-intrinsic-size:700px';
}

const toolbox = createMessageToolbox()
const { openId: openToolboxId, toggle: toggleToolbox } = toolbox
onMounted(toolbox.mount)
onBeforeUnmount(toolbox.dispose)

const targetPage = ref<string>('1');
const totalPages = computed(() => Math.max(1, Math.ceil(message.total / 15)));
const syncTargetPageToCurrent = () => {
  const page = Math.min(Math.max(Number(message.page) || 1, 1), totalPages.value);
  targetPage.value = String(page);
};
const normalizeTargetPage = (fallback = message.page) => {
  const parsed = Number.parseInt(targetPage.value.trim() || '', 10);
  const next = Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
  return Math.min(Math.max(next, 1), totalPages.value);
};
const adjustTargetPage = (delta: number) => {
  targetPage.value = String(normalizeTargetPage(message.page) + delta);
  targetPage.value = String(normalizeTargetPage(message.page));
};
const isScrollableY = isMessageListScrollable
const getAppScrollContainer = getMessageListScrollContainer
let pageTopScrollFrame: number | null = null
let pageTopScrollTimers: ReturnType<typeof setTimeout>[] = []
const clearPageTopScrollSchedule = () => {
  if (typeof window !== 'undefined' && pageTopScrollFrame !== null) {
    window.cancelAnimationFrame(pageTopScrollFrame)
  }
  pageTopScrollFrame = null
  pageTopScrollTimers.forEach((timer) => clearTimeout(timer))
  pageTopScrollTimers = []
}
const firstCurrentPageBlock = () => {
  const root = messageListRoot.value
  return root?.querySelector<HTMLElement>('.message-list-item .content-container') || null
}
const scrollElementTopToViewportTop = (target: HTMLElement, behavior: ScrollBehavior = 'instant') => {
  if (typeof window === 'undefined') return
  const sc = getAppScrollContainer(target)
  const targetRect = target.getBoundingClientRect()
  if (sc && isScrollableY(sc)) {
    const scRect = sc.getBoundingClientRect()
    const nextTop = sc.scrollTop + targetRect.top - scRect.top
    sc.scrollTo({ top: Math.max(0, nextTop), behavior })
    return
  }
  window.scrollTo({
    top: Math.max(0, (window.scrollY || window.pageYOffset || 0) + targetRect.top),
    left: window.scrollX || window.pageXOffset || 0,
    behavior,
  })
}
const alignCurrentPageFirstBlockToTop = (behavior: ScrollBehavior = 'instant') => {
  const target = firstCurrentPageBlock() || messageListRoot.value
  if (target) scrollElementTopToViewportTop(target, behavior)
}
const scheduleCurrentPageFirstBlockTopScroll = async () => {
  await nextTick()
  clearPageTopScrollSchedule()
  alignCurrentPageFirstBlockToTop('instant')
  if (typeof window === 'undefined') return
  pageTopScrollFrame = window.requestAnimationFrame(() => {
    pageTopScrollFrame = null
    alignCurrentPageFirstBlockToTop('instant')
  })
  pageTopScrollTimers = [80, 220, 420].map((delay) => setTimeout(() => {
    alignCurrentPageFirstBlockToTop('instant')
  }, delay))
}
const jumpToPage = async () => {
  const page = Number.parseInt(targetPage.value.trim() || '', 10);
  if (!page || page < 1 || page > totalPages.value || isPageLoading.value) {
    useToast().add({
      title: '页码无效',
      description: `请输入 1-${totalPages.value} 之间的数字`,
      color: 'orange',
      timeout: 2000
    });
    return;
  }

  try {
    const result = await fetchListPage(pageQueryFor(page));

    if (!applyPageResult(result, page)) {
      throw new Error('跳转页面失败');
    }

    syncTargetPageToCurrent();
    await scheduleCurrentPageFirstBlockTopScroll();
  } catch (error) {
    console.error('跳转页面失败:', error);
    useToast().add({
      title: '跳转失败',
      color: 'red',
      timeout: 2000
    });
  }
};
// 添加 props 定义
const props = defineProps({
  initialPage: { type: Number, default: 1 },
  siteConfig: {
    type: Object,
    required: true
  },
  targetMessageId: {
    type: String,
    default: null
  },
  targetCommentId: {
    type: Number,
    default: null
  },
  masonry: { type: Boolean, default: false },
  wide: {
    type: Boolean,
    default: false
  },
  pageReady: {
    type: Boolean,
    default: true
  },
  activeTab: {
    type: String,
    default: 'latest'
  },
  calendarDate: {
    type: String,
    default: ''
  },
  searchKeyword: {
    type: String,
    default: ''
  },
  selectedTag: {
    type: String,
    default: ''
  }
});
const emit = defineEmits<{
  (e: 'clear-calendar-date'): void
  (e: 'clear-filters'): void
  (e: 'select-tag', tag: string): void
  (e: 'target-consumed'): void
  (e: 'loading-change', loading: boolean): void
}>()
const isPageLoading = ref(false);
const continuousLoading = ref(false)
const continuousError = ref(false)
const setPageLoading = (loading: boolean) => {
  if (isPageLoading.value === loading) return
  isPageLoading.value = loading
  emit('loading-change', loading)
}
const outerContainerClass = computed(() => {
  const filtering = props.pageReady && Boolean(props.calendarDate || String(props.searchKeyword || '').trim() || String(props.selectedTag || '').trim())
  return filtering ? 'flex-grow w-full' : 'flex-grow w-full px-1 sm:px-2'
})
const innerContainerClass = computed(() => props.wide ? '' : 'mx-auto sm:max-w-4xl')
// 独立的内容主题（与页面主题解耦）
const contentTheme = inject('contentTheme', ref<string>(typeof window !== 'undefined' ? (localStorage.getItem('contentTheme') || 'dark') : 'dark'))
const isContentDark = computed(() => contentTheme.value === 'dark')
const listThemeClass = computed(() => isContentDark.value ? 'bg-[var(--home-surface-dark)] text-white' : 'bg-white text-black')
const listThemeTextClass = computed(() => isContentDark.value ? 'text-white' : 'text-black')
const gradientClass = computed(() => isContentDark.value ? 'from-[var(--home-surface-dark)] via-[rgba(32,42,54,0.82)] to-transparent' : 'from-[rgba(255,255,255,1)] via-[rgba(255,255,255,0.8)] to-transparent')
const authorProfiles = ref<Record<string, any>>({})
const openAuthorId = ref<number | null>(null)
const openAuthorStyle = ref<Record<string, string>>({})
let authorHoverTimer: any = null
let authorLeaveTimer: any = null
const fetchAuthorProfile = async (uname: string) => {
  const key = String(uname || '').trim()
  if (!key || authorProfiles.value[key]) return
  try {
    const resp = await fetch(`${BASE_API}/users/profile?username=${encodeURIComponent(key)}`, { credentials: 'include', headers: { 'Accept': 'application/json' } })
    if (!resp.ok) return
    const js = await resp.json()
    const d = js?.data || {}
    if (d && d.username) authorProfiles.value[key] = d
  } catch {}
}
const showAuthorCard = async (ev: MouseEvent, msg: any) => {
  clearTimeout(authorLeaveTimer)
  authorHoverTimer = setTimeout(async () => {
    openAuthorId.value = msg.id
    try {
      const target = ev.target as HTMLElement
      const rect = target.getBoundingClientRect()
      const top = Math.max(8, rect.top - 32)
      const left = rect.left + rect.width + 8
      openAuthorStyle.value = { position: 'fixed', top: `${top}px`, left: `${left}px`, zIndex: '2147483647' }
    } catch {}
    await fetchAuthorProfile(String(msg?.username || ''))
  }, 120)
}
const hideAuthorCard = () => {
  clearTimeout(authorHoverTimer)
  authorLeaveTimer = setTimeout(() => { openAuthorId.value = null }, 120)
}
const toggleAuthorCard = async (ev: MouseEvent, msg: any) => {
  if (openAuthorId.value === msg.id) { openAuthorId.value = null; return }
  openAuthorId.value = msg.id
  try {
    const target = ev.target as HTMLElement
    const rect = target.getBoundingClientRect()
    const top = Math.max(8, rect.top - 32)
    const left = rect.left + rect.width + 8
    openAuthorStyle.value = { position: 'fixed', top: `${top}px`, left: `${left}px`, zIndex: '2147483647' }
  } catch {}
  await fetchAuthorProfile(String(msg?.username || ''))
}
const authorSignShouldScroll = (msg: any) => {
  const t = String(authorProfileDesc(msg) || '').trim()
  return t.length > 12
}
const authorProfileAvatar = (msg: any) => {
  const uname = String(msg?.username || '').trim()
  const d = authorProfiles.value[uname]
  const url = String((d && d.avatar_url) || '')
  if (!url) return authorAvatar(msg)
  return resolveMediaUrl(url)
}
const authorProfileDesc = (msg: any) => {
  const uname = String(msg?.username || '').trim()
  const d = authorProfiles.value[uname]
  return String((d && d.description) || '') || '—'
}
const authorProfileCount = (msg: any) => {
  const uname = String(msg?.username || '').trim()
  const d = authorProfiles.value[uname]
  return Number((d && d.total_messages) || 0)
}
const apiReachable = ref(true)
const checkApi = async () => {
  try {
    const res = await fetch(`${BASE_API}/status`, { credentials: 'include' })
    apiReachable.value = !!res && res.ok
  } catch {
    apiReachable.value = false
  }
}
const { deleteMessage } = useMessage();
const message = useMessageStore();
watch(() => message.page, syncTargetPageToCurrent, { immediate: true });
watch(totalPages, syncTargetPageToCurrent);

const prefetchAuthorProfilesForList = () => {
  const names = Array.from(new Set((message.messages || []).map((m: any) => String(m?.username || '').trim()).filter((n) => !!n)))
  names.forEach((n) => fetchAuthorProfile(n))
}
watch(() => message.messages, () => { prefetchAuthorProfilesForList() }, { deep: false, immediate: true })

const isCommentEnabled = computed(() => {
  const v: any = (props.siteConfig as any)?.commentEnabled
  return v === true || v === 'true'
})
const guestbookId = ref<number | null>(null)
const targetListReady = ref(false)
const isGuestbookMessage = (m: any) => {
  if (!m) return false
  return !!guestbookId.value && m.id === guestbookId.value
}
const fetchGuestbookId = async () => {
  try {
    const resp = await fetch(`${BASE_API}/guestbook/message`, { credentials: 'include', headers: { 'Accept': 'application/json' } })
    if (resp.ok) {
      const js = await resp.json()
      const id = js?.data?.id
      if (id) guestbookId.value = Number(id)
    }
  } catch {}
}
const getMessageById = (id: number) => (message.messages || []).find((m: any) => Number(m?.id || 0) === Number(id))
const applyPageResult = (result: any, targetPage: number) => {
  if (!result || !Array.isArray(result.items)) return false
  const items = result.items.filter((m: any) => !isGuestbookMessage(m))
  message.messages = items
  message.total = Math.max(0, Number(result.total || 0))
  message.page = Number((result as any).page || targetPage || 1)
  message.pageSize = 15
  const size = Number(message.pageSize || 15)
  const lastPage = Math.max(1, Math.ceil((message.total || 0) / size))
  message.hasMore = message.page < lastPage
  return true
}
const loadTargetMessagePage = async (id: number) => {
  if (!id || !targetListReady.value) return false
  if (getMessageById(id)) return true
  const loadThreadTombstone = async () => {
    const response = await getRequest<any>(`messages/${id}`, undefined, { credentials: 'include', silent: true })
    if (response?.code !== 1 || response?.data?.is_tombstone !== true) return false
    message.messages = [response.data]
    message.total = 1
    message.page = 1
    message.hasMore = false
    await nextTick()
    return true
  }
  try {
    const location = await message.locateMessagePage({ ...pageQueryFor(1), messageId: id })
    const targetPage = Number(location?.page || 0)
    if (targetPage < 1) return await loadThreadTombstone()
    if (props.masonry) {
      const queryKey = currentDisplayQueryKey.value
      for (let page = 1; page <= targetPage; page++) {
        const result = await message.loadMessagePage(pageQueryFor(page), { append: page > 1 })
        if (!result || queryKey !== currentDisplayQueryKey.value || !props.masonry) return false
      }
    } else {
      const result = await message.loadMessagePage(pageQueryFor(targetPage))
      if (!applyPageResult(result, targetPage)) return false
    }
    await nextTick()
    return !!getMessageById(id)
  } catch {
    return await loadThreadTombstone()
  }
}

  const userStore = useUserStore();
  const { can, refreshCapabilities } = useAdminCapabilities()
  const isLogin = computed(() => userStore.isLogin);
  const isPersonalTab = computed(() => props.activeTab === 'personal')
  const isPersonalGuest = computed(() => isPersonalTab.value && !userStore.isLogin)
  const currentUserId = computed(() => Number((userStore.user as any)?.userid || (userStore.user as any)?.id || (userStore.user as any)?.user_id || 0))
  const currentUsername = computed(() => String((userStore.user as any)?.username || '').trim())
  const currentUserIsAdmin = computed(() => !!((userStore.user as any)?.is_admin || (userStore.user as any)?.IsAdmin))
  const personalUserPending = computed(() => isPersonalTab.value && userStore.isLogin && !currentUserId.value && !currentUsername.value)
  const calendarDateLabel = computed(() => {
    const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(String(props.calendarDate || ''))
    if (!match) return ''
    return `${match[1]}年${Number(match[2])}月${Number(match[3])}日`
  })
  const normalizedSearchKeyword = computed(() => String(props.searchKeyword || '').trim())
  const normalizedSelectedTag = computed(() => String(props.selectedTag || '').trim().replace(/^#/, ''))
  const hasActiveFilters = computed(() => Boolean(props.calendarDate || normalizedSearchKeyword.value || normalizedSelectedTag.value))
  const activeFilterContent = computed(() => {
    const filters: string[] = []
    if (calendarDateLabel.value) filters.push(calendarDateLabel.value)
    if (normalizedSearchKeyword.value) filters.push(normalizedSearchKeyword.value)
    if (normalizedSelectedTag.value) filters.push(`#${normalizedSelectedTag.value}`)
    return filters.join(' / ')
  })
  const filteredResultCount = computed(() => {
    const total = Number(message.total)
    const visibleCount = displayMessages.value.length
    if (!Number.isFinite(total) || total < visibleCount) return visibleCount
    return total
  })
  const pageQueryFor = (pageNumber: number) => {
    const query: any = { page: pageNumber, pageSize: 15, pinScope: pinScope.value }
    if (guestbookId.value) query.excludeId = guestbookId.value
    if (isPersonalTab.value && currentUserId.value) query.authorId = currentUserId.value
    if (/^\d{4}-\d{2}-\d{2}$/.test(String(props.calendarDate || ''))) query.date = props.calendarDate
    if (normalizedSearchKeyword.value) query.keyword = normalizedSearchKeyword.value
    if (normalizedSelectedTag.value) query.tag = normalizedSelectedTag.value
    return query
  }
  const isCurrentUserMessage = (msg: any) => {
    if (!msg || !userStore.isLogin) return false
    const msgUserId = Number(msg?.user_id || msg?.userId || msg?.authorId || 0)
    if (currentUserId.value && msgUserId) return msgUserId === currentUserId.value
    return !!currentUsername.value && String(msg?.username || msg?.author || '').trim() === currentUsername.value
  }
  const shouldShowMessageVisibility = (msg: any) => shouldShowVisibilityBadge({
    visibility: messageVisibility(msg),
    isAdmin: currentUserIsAdmin.value,
    isAuthenticated: isLogin.value,
    isOwner: isCurrentUserMessage(msg),
  })
  const isPrimaryOwnedMessage = (msg: any) => Number(msg?.user_id || msg?.userId || msg?.UserID || 0) === 1
  const canInteractWithMessage = (msg: any) => msg?.can_interact === true
  const canManageOtherMessage = (msg: any, capability: string) => isLogin.value && !isCurrentUserMessage(msg) && !isPrimaryOwnedMessage(msg) && can(capability)
  const canEditMessageTasks = (msg: any) => isCurrentUserMessage(msg) || canManageOtherMessage(msg, 'notes.edit')
  const isContentEmpty = (m: any) => {
    const img = String(m?.image_url || '').trim()
    const c0 = String(m?.content || '')
    const c = c0.replace(/\s|&nbsp;|\u00A0/gi, '').trim()
    return img === '' && c.length === 0
  }

const engagement = createMessageListEngagement({
  root: () => messageListRoot.value,
  messages: () => message.messages as any[],
  isGuestbook: isGuestbookMessage,
  isLoggedIn: () => isLogin.value,
  canInteract: canInteractWithMessage,
})
const {
  likes: likesMap,
  liked: likedMap,
  commentCounts: commentCountMap,
  expandedComments: expandedCommentsMap,
  activeCommentId,
  commentRefreshKey,
  hydrate: hydrateMessageEngagement,
  like,
  toggleComments: toggleComment,
  handleCancel,
  commentsRefFor: builtinCommentsRefFor,
  focusComment: focusBuiltinTargetComment,
} = engagement

const targetNavigation = createMessageTargetNavigation({
  root: () => messageListRoot.value,
  targetMessageId: () => Number(props.targetMessageId || 0),
  targetCommentId: () => Number(props.targetCommentId || 0),
  ready: () => targetListReady.value,
  loadMessage: loadTargetMessagePage,
  expandComments: (messageId) => { expandedCommentsMap.value[messageId] = true },
  focusComment: focusBuiltinTargetComment,
  consume: () => emit('target-consumed'),
})
const focusTargetMessageAndComment = targetNavigation.focus
onMounted(() => {
  engagement.mount()
  targetNavigation.mount()
})
onBeforeUnmount(() => {
  targetNavigation.dispose()
  engagement.dispose()
})
watch(() => [props.targetMessageId, props.targetCommentId], targetNavigation.update, { immediate: true })
const openInNewTab = (url: string) => {
  window.open(url, '_blank', 'noopener,noreferrer');
};
// 标签点击处理函数
const handleTagClick = (tag: string) => {
  const normalizedTag = String(tag || '').trim().replace(/^#/, '')
  if (normalizedTag) emit('select-tag', normalizedTag)
}

let listRefreshSeq = 0

const fetchListPage = async (query: any) => {
  const result = await message.loadMessagePage(query)
  if (!props.masonry && result && query.page > 1) {
    const lastPage = Math.max(1, Math.ceil(Number(result.total || 0) / query.pageSize))
    if (query.page > lastPage) {
      const lastResult = await message.loadMessagePage({ ...query, page: lastPage })
      return lastResult ? { ...lastResult, page: lastPage } : lastResult
    }
  }
  return result
}

const clearCurrentList = () => {
  message.messages = []
  message.total = 0
  message.page = 1
  message.pageSize = 15
  message.hasMore = false
}

const refreshList = async () => {
  const requestId = ++listRefreshSeq
  continuousLoading.value = false
  continuousError.value = false
  const targetPage = !targetListReady.value && !props.masonry && !hasActiveFilters.value ? props.initialPage : 1
  const query = pageQueryFor(targetPage)
  const requestQueryKey = message.listQueryKey(query)
  message.currentListQueryKey = requestQueryKey
  setPageLoading(true)
  try {
    const result = await fetchListPage(query)
    if (requestId !== listRefreshSeq || requestQueryKey !== currentDisplayQueryKey.value) return
    applyPageResult(result, targetPage)
    message.currentListQueryKey = requestQueryKey
    await nextTick();
    deferMeasure();
    deferInitFancybox();
  } catch (error: any) {
    if (error?.name === 'AbortError') return
    console.error('刷新消息列表失败:', error)
    useToast().add({ title: '加载失败', color: 'red', timeout: 2000 })
  } finally {
    if (requestId === listRefreshSeq) setPageLoading(false)
  }
}

// 修改重置搜索函数名称，使其更通用
// 修改 resetList 函数
const resetList = async () => {
  emit('clear-filters')
};

const deleteMsg = async (msg: any) => {
  if (!canDelete(msg)) return
  const id = Number(msg?.id || 0)
  if (!id) return
  const confirmDelete = confirm("确定要删除这条消息吗？");
  if (confirmDelete) {
    try {
      await message.deleteMessage(id); // 使用 store 中的方法
      message.messages = message.messages.filter(msg => msg.id !== id);
      useToast().add({
        title: '删除成功',
        color: 'green',
        timeout: 2000
      });
    } catch (error) {
      console.error('删除失败:', error);
      useToast().add({
        title: '删除失败',
        color: 'red',
        timeout: 2000
      });
    }
  }
};

const listMedia = createMessageListMedia({ root: () => messageListRoot.value })
const initFancybox = listMedia.update
const deferInitFancybox = listMedia.update
const contentLayout = createMessageContentLayout({
  root: () => messageListRoot.value,
  messages: () => message.messages as any[],
  afterMeasure: listMedia.update,
})
const deferMeasure = contentLayout.update
const checkContentHeight = contentLayout.update
onMounted(() => {
  listMedia.mount()
  contentLayout.mount()
})
onBeforeUnmount(() => {
  contentLayout.dispose()
  listMedia.dispose()
  clearPageTopScrollSchedule()
  listRefreshSeq += 1
})

const pinScope = computed<'latest' | 'personal'>(() => isPersonalTab.value ? 'personal' : 'latest')
const pinActive = (msg: any) => pinScope.value === 'personal' ? !!msg?.personal_pinned : !!msg?.pinned
const pinStatusLabel = () => pinScope.value === 'personal' ? '个人置顶内容' : '全站置顶内容'
const pinTooltip = (msg: any) => {
  if (pinScope.value === 'personal') return pinActive(msg) ? '取消个人置顶' : '个人置顶'
  return pinActive(msg) ? '取消全站置顶' : '全站置顶'
}
const canGlobalPin = (msg: any) => isLogin.value && canInteractWithMessage(msg) && can('notes.pin_global') && (isCurrentUserMessage(msg) || !isPrimaryOwnedMessage(msg))
const canPin = (msg: any) => pinScope.value === 'personal' ? isCurrentUserMessage(msg) : canGlobalPin(msg)
const canEdit = (msg: any) => isCurrentUserMessage(msg) || canManageOtherMessage(msg, 'notes.edit')
const canDelete = (msg: any) => isCurrentUserMessage(msg) || canManageOtherMessage(msg, 'notes.trash')
const canChangeVisibility = (msg: any) => isCurrentUserMessage(msg) || canManageOtherMessage(msg, 'notes.change_visibility')

const togglePin = async (msg: any) => {
  if (!canPin(msg)) return
  try {
    const next = !pinActive(msg);
    const res = await message.setPin(msg.id, next, pinScope.value);
    if (res) {
      await refreshList()
      useToast().add({ title: next ? (pinScope.value === 'personal' ? '已设为个人置顶' : '已设为全站置顶') : (pinScope.value === 'personal' ? '已取消个人置顶' : '已取消全站置顶'), color: 'green', timeout: 1500 });
    }
  } catch (e) {
    useToast().add({ title: '操作失败', color: 'red', timeout: 2000 });
  }
  };

const shanghaiDateTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false
})

const formatShanghaiDateTime = (date: Date) => {
  const parts = shanghaiDateTimeFormatter.formatToParts(date)
  const pick = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value || ''
  return `${pick('year')}/${pick('month')}/${pick('day')} ${pick('hour')}:${pick('minute')}:${pick('second')}`
}

const formatDate = (dateString: string) => {
  const date = new Date(dateString);
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const diffInDays = Math.floor(diff / (1000 * 60 * 60 * 24));
  const diffInHours = Math.floor(diff / (1000 * 60 * 60));
  const diffInMinutes = Math.floor(diff / (1000 * 60));

  const diffInSeconds = Math.floor(diff / 1000);
  if (diffInSeconds < 60) {
    return "刚刚";
  } else if (diffInMinutes < 60) {
    return `${diffInMinutes}分钟前`;
  } else if (diffInHours < 24) {
    return `${diffInHours}小时前`;
  } else if (diffInDays < 3) {
    return `${diffInDays}天前`;
  } else {
    return formatShanghaiDateTime(date);
  }
};
const {
  expanded: isExpanded,
  showExpandButton: shouldShowExpandButton,
  hasGrid,
  toggle: toggleExpand,
  isAttachmentShadowOpen: isFileAttachmentShadowOpen,
  isAttachmentShadowClipped: isFileAttachmentShadowClipped,
} = contentLayout

// 确保在内容变化时重新检查高度
watch(() => message.messages, () => {
  hydrateMessageEngagement(message.messages as any[])
  // 如果是单条消息查看模式，不执行滚动
  if (getMessageIdFromRouteHash(route.hash)) {
    return;
  }
  nextTick(() => {
    deferMeasure();
    deferInitFancybox();
  });
}, { deep: true });
// 添加路由相关
const route = useRoute();
onMounted(async () => {
  try {
    setPageLoading(true)
    await checkApi()
    await fetchGuestbookId()
    // 获取路由中的消息ID
    const messageId = getMessageIdFromRouteHash(route.hash);
    
    // 根据是否有消息ID来决定加载方式
    if (messageId) {
    const data = await getRequest<any>(`messages/${messageId}`, undefined, { credentials: 'include' });
    if (data.code === 1 && data.data) {
      const item = data.data
      if (!isGuestbookMessage(item)) {
        message.messages = [item];
      } else {
        message.messages = []
      }
      message.hasMore = false;
      message.page = 1;
        
        await nextTick();
        const targetElement = document.querySelector(`.content-container[data-msg-id="${messageId}"]`);
        if (targetElement) {
          targetElement.scrollIntoView({ behavior: 'instant', block: 'start' });
        }
      } else {
        throw new Error('消息不存在');
      }
    } else {
      // 只有在非消息详情页时才加载列表
      if (!getMessageIdFromRouteHash(route.hash)) {
        const startPage = !props.masonry && !hasActiveFilters.value ? props.initialPage : 1
        const result = await fetchListPage(pageQueryFor(startPage))
        if (result) applyPageResult(result, startPage)
      }
    }

    // 初始化视图
    await nextTick();
    deferMeasure();
    deferInitFancybox();

    // 默认仅展开已有评论的消息
    try {
      const tasks = (message.messages || []).filter((m: any) => !isGuestbookMessage(m)).map(async (m: any) => {
        try {
          let js: any = null
          const resp1 = await fetch(`${BASE_API}/messages/${m.id}/comments`, { credentials: 'include', headers: { 'Accept': 'application/json' } });
          if (resp1 && resp1.ok) {
            js = await resp1.json();
          } else {
            const resp2 = await fetch(`http://localhost:1315/api/messages/${m.id}/comments`, { credentials: 'include', headers: { 'Accept': 'application/json' } });
            if (resp2 && resp2.ok) js = await resp2.json();
          }
          const count = js && Array.isArray(js.data) ? js.data.length : 0;
          commentCountMap.value[m.id] = count;
          if (count > 0) expandedCommentsMap.value[m.id] = true;
      } catch {}
    });
    await Promise.allSettled(tasks);
    await nextTick();
    } catch {}
    
  } catch (error) {
    console.error('初始化失败:', error);
    if (error instanceof Error) {
      useToast().add({
        title: '加载失败',
        description: error.message || '请刷新重试',
        color: 'red',
        timeout: 2000
      });
    }
  } finally {
    setPageLoading(false)
    targetListReady.value = true
    await nextTick()
    if (props.targetMessageId) focusTargetMessageAndComment()
  }
});

// 修改路由监听
watch(() => route.hash, async (newHash, oldHash) => {
  const messageId = getMessageIdFromRouteHash(newHash);
  
  // 如果没有消息ID且不是从消息详情页返回，则保持当前状态，不重新加载
  if (!messageId) {
    if (getMessageIdFromRouteHash(oldHash)) {
      await refreshList()
      expandedCommentsMap.value = {}
      return
    }
    // 如果当前已有消息，不做任何操作，保持滚动位置
    if (message.messages && message.messages.length > 0) {
      return;
    }
    
    // 只有在首次加载且没有消息时才加载第一页
    await refreshList()
    expandedCommentsMap.value = {}
    return;
  }
  
  try {
    const data = await getRequest<any>(`messages/${messageId}`, undefined, { credentials: 'include' });
    if (data.code === 1 && data.data) {
          message.messages = isGuestbookMessage(data.data) ? [] : [data.data];
          message.hasMore = false;
          message.page = 1;
      
      await nextTick();
      const targetElement = document.querySelector(`.content-container[data-msg-id="${messageId}"]`);
      if (targetElement) {
        targetElement.scrollIntoView({ 
          behavior: 'instant',
          block: 'start'
        });
      }
    }
  } catch (error) {
    console.error('加载消息失败:', error);
    useToast().add({
      title: '加载失败',
      color: 'red',
      timeout: 2000
    });
  }
}, { immediate: true });

// 修改 loadMore 为 loadNextPage
watch(
  [
    () => props.activeTab,
    () => props.masonry,
    () => props.calendarDate,
    () => props.searchKeyword,
    () => props.selectedTag,
    () => userStore.isLogin,
    () => currentUserId.value,
    () => currentUsername.value
  ],
  async () => {
    if (getMessageIdFromRouteHash(route.hash)) return
    if (Number(props.targetMessageId || 0) > 0) {
      await focusTargetMessageAndComment()
      return
    }
    if (isPersonalGuest.value) {
      listRefreshSeq += 1
      clearCurrentList()
      expandedCommentsMap.value = {}
      setPageLoading(false)
      return
    }
    if (personalUserPending.value) {
      listRefreshSeq += 1
      message.currentListQueryKey = currentDisplayQueryKey.value
      setPageLoading(true)
      return
    }
    await refreshList()
    expandedCommentsMap.value = {}
  }
)

const loadPreviousPage = async () => {
  if (isPageLoading.value || message.page <= 1) return;
  setPageLoading(true);
  try {
    const targetPage = message.page - 1;
    const result = await fetchListPage(pageQueryFor(targetPage));
    if (!applyPageResult(result, targetPage)) throw new Error('加载上一页失败');
    await scheduleCurrentPageFirstBlockTopScroll();
  } catch (error) {
    useToast().add({
      title: '加载失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    setPageLoading(false);
  }
};

const loadContinuousPage = async () => {
  if (!props.masonry || isPageLoading.value || continuousLoading.value || props.targetMessageId || !message.hasMore || !targetListReady.value || isPersonalGuest.value || isDisplayQueryPending.value) return
  const requestId = listRefreshSeq
  const queryKey = currentDisplayQueryKey.value
  continuousLoading.value = true
  continuousError.value = false
  try {
    const result = await message.loadMessagePage(pageQueryFor(message.page + 1), { append: true })
    if (requestId !== listRefreshSeq || queryKey !== currentDisplayQueryKey.value || !props.masonry) return
    if (!result) continuousError.value = true
    else if (!result.items.length) message.hasMore = false
  } catch {
    if (requestId === listRefreshSeq) continuousError.value = true
  } finally {
    if (requestId === listRefreshSeq) continuousLoading.value = false
  }
}

const loadNextPage = async () => {
  if (isPageLoading.value || !message.hasMore) return;
  setPageLoading(true);
  try {
    const targetPage = message.page + 1;
    const result = await fetchListPage(pageQueryFor(targetPage));
    if (!applyPageResult(result, targetPage)) throw new Error('加载下一页失败');
    await scheduleCurrentPageFirstBlockTopScroll();
  } catch (error) {
    useToast().add({
      title: '加载失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    setPageLoading(false);
  }
};
// 监听消息变化
watch(
  () => message.messages,
  async () => {
    try {
      await nextTick();
      engagement.update();
      await nextTick();
      checkContentHeight();
      initFancybox();
    } catch (error) {
      console.error('更新视图失败:', error);
    }
  },
  { deep: true }
);
// 组件卸载时清理
// 添加复制功能
const copyContent = async (content: string) => {
  try {
    await writeClipboardText(content);
    // 可以使用 Nuxt 的 toast 提示复制成功
    useToast().add({
      title: '复制成功',
      color: 'green',
      timeout: 2000
    });
  } catch (err) {
    console.error('复制失败:', err);
    useToast().add({
      title: '复制失败',
      color: 'red',
      timeout: 2000
    });
  }
};
type MessageEditDialogExpose = {
  open: (options: { message: any; canChangeVisibility: boolean; canChangePublishTime: boolean }) => void
}
const messageEditDialogRef = ref<MessageEditDialogExpose | null>(null)
const applyEditedMessage = ({ id, patch }: { id: number; patch: any }) => {
  const index = message.messages.findIndex((item: any) => item.id === id)
  if (index !== -1) message.messages[index] = { ...message.messages[index], ...patch }
}
const editMessage = (item: any) => {
  if (!canEdit(item)) return
  const canChangePublishTime = isCurrentUserMessage(item)
    ? currentUserIsAdmin.value && can('notes.change_publish_time')
    : canManageOtherMessage(item, 'notes.change_publish_time')
  messageEditDialogRef.value?.open({
    message: item,
    canChangeVisibility: canChangeVisibility(item),
    canChangePublishTime,
  })
}
const stableDisplayMessages = ref<any[]>([])
const stableDisplayQueryKey = ref('')
const currentDisplayQueryKey = computed(() => message.listQueryKey(pageQueryFor(1)))
const isDisplayQueryPending = computed(() => Boolean(message.currentListQueryKey && message.currentListQueryKey !== currentDisplayQueryKey.value))
const searchResultsRefreshing = ref(false)
const refreshSearchResults = async () => {
  if (searchResultsRefreshing.value || isPageLoading.value || isDisplayQueryPending.value) return
  searchResultsRefreshing.value = true
  try {
    await refreshList()
  } finally {
    window.setTimeout(() => {
      searchResultsRefreshing.value = false
    }, 300)
  }
}
const getServerDisplayMessages = () => {
  return (message.messages || []).filter((m: any) => !isGuestbookMessage(m))
}

// displayMessages 使用统一分页结果；筛选条件由 pageQueryFor 传给后端
const displayMessages = computed(() => {
  if (isDisplayQueryPending.value) return []
  if (isPageLoading.value && stableDisplayQueryKey.value === currentDisplayQueryKey.value && stableDisplayMessages.value.length) return stableDisplayMessages.value
  return getServerDisplayMessages()
})

const syncStableDisplayMessages = () => {
  stableDisplayMessages.value = getServerDisplayMessages()
  stableDisplayQueryKey.value = currentDisplayQueryKey.value
}

watch(
  [
    () => message.messages,
    () => guestbookId.value,
    () => props.activeTab,
    () => props.calendarDate,
    () => props.searchKeyword,
    () => props.selectedTag,
    () => userStore.isLogin,
    () => currentUserId.value
  ],
  () => {
    if (!isPageLoading.value) syncStableDisplayMessages()
  },
  { deep: true, immediate: true }
)

watch(isPageLoading, (loading) => {
  if (!loading) syncStableDisplayMessages()
})

const showPager = computed(() => {
  if (props.masonry || isPersonalGuest.value) return false
  if (!hasActiveFilters.value) return true
  return !isPageLoading.value && displayMessages.value.length > 0
})

const sidebarPagerState = computed(() => ({
  visible: showPager.value,
  currentPage: Math.max(1, Number(message.page) || 1),
  totalPages: totalPages.value,
  loading: isPageLoading.value,
  canPrevious: !isPageLoading.value && message.page > 1,
  canNext: !isPageLoading.value && !!message.hasMore
}))

const goToPage = async (page: string | number) => {
  targetPage.value = String(page)
  await jumpToPage()
}

defineExpose({
  refreshList,
  sidebarPagerState,
  previousPage: loadPreviousPage,
  nextPage: loadNextPage,
  goToPage
});
// 优化图片加载
const optimizeImage = (url: string) => {
  if (!url) return url;
  // 添加图片压缩参数
  return `${url}?imageView2/2/w/800/q/75&format=webp`;
}

// 添加图片预加载缓存
const imageCache = new Map<string, HTMLImageElement>();

const preloadImage = (src: string): Promise<HTMLImageElement> => {
  return new Promise((resolve, reject) => {
    if (imageCache.has(src)) {
      resolve(imageCache.get(src)!);
      return;
    }

    const img = new Image();
    img.onload = () => {
      imageCache.set(src, img);
      resolve(img);
    };
    img.onerror = reject;
    img.src = src;
  });
};
// 下一页预取（靠近底部时触发）
const prefetchSentinel = ref<HTMLElement | null>(null)
let prefetchObservedPage = 0
onMounted(() => {
  void refreshCapabilities()
  try {
    const io2 = new IntersectionObserver((entries) => {
      entries.forEach(async (entry) => {
        if (!entry.isIntersecting) return
        const nextPage = (message.page || 1) + 1
        if (!message.hasMore) return
        if (prefetchObservedPage === nextPage) return
        prefetchObservedPage = nextPage
        const anyMsg = message as any
        if (anyMsg && typeof anyMsg.prefetchPage === 'function') {
          await anyMsg.prefetchPage(pageQueryFor(nextPage))
        }
      })
    }, { rootMargin: '512px 0px' })
    if (prefetchSentinel.value) io2.observe(prefetchSentinel.value)
  } catch {}
})

</script>

<style scoped>
.message-tombstone-icon{width:36px;height:36px;display:inline-flex;align-items:center;justify-content:center;flex:0 0 auto;border:1px dashed rgba(148,163,184,.6);border-radius:999px;color:#64748b;background:rgba(148,163,184,.1)}
.message-tombstone-icon svg{width:18px;height:18px}.message-tombstone-panel{display:flex;flex-direction:column;gap:5px;margin:10px 0;padding:12px 14px;border:1px dashed rgba(148,163,184,.55);border-radius:11px;background:rgba(148,163,184,.08);color:#64748b;font-size:12px;line-height:1.6}.message-tombstone-panel strong{color:inherit;font-size:13px}
.search-mode-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  margin: 0 0 16px;
  padding: 10px 0;
  color: #111827;
}

.search-mode-title {
  margin: 0;
  min-width: 0;
  color: inherit;
  font-size: 14px;
  font-weight: 650;
  line-height: 1.3;
}

.search-mode-back {
  min-width: max-content;
  height: 34px;
  border-radius: 10px;
}

:global(html.dark) .search-mode-bar {
  color: #f8fafc;
}

.search-card {
  background: var(--home-surface-light);
  color: #111827;
  border: 1px solid #e5e7eb;
  border-radius: var(--home-radius-panel);
}

.search-card.is-dark {
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.48) 0%, rgba(15, 23, 42, 0.82) 100%);
  color: #fff;
  border: 1px solid var(--home-border-dark);
  box-shadow: 0 14px 28px rgba(2, 6, 23, 0.45);
  backdrop-filter: blur(8px) saturate(118%);
  -webkit-backdrop-filter: blur(8px) saturate(118%);
}

.search-results-panel {
  position: relative;
  box-sizing: border-box;
  width: 100%;
  margin-top: 20px;
}

.search-results-panel.is-dark {
  color: #f8fafc;
}

.search-results-head {
  position: relative;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  min-height: 56px;
  padding: 0;
}

.search-results-heading {
  min-width: 0;
  text-align: center;
}

.search-results-title {
  display: block;
  margin: 0 0 14px;
  padding: 0;
  border-radius: 0;
  color: inherit;
  font-size: 18px;
  font-weight: 700;
  line-height: 1.5;
}

.search-results-summary {
  max-width: 42rem;
  margin: 2px auto 20px;
  color: inherit;
  font-size: 13px;
  line-height: 1.7;
  opacity: .8;
  overflow-wrap: anywhere;
}

.search-results-board-head {
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: calc(100% + 2rem + 2px);
  max-width: calc(56rem + 2px);
  min-height: 28px;
  margin: 0 calc(-1rem - 1px) 8px;
  padding: 8px 8px 0;
}

.search-results-actions {
  display: flex;
  align-items: center;
  flex: 0 0 auto;
  gap: 8px;
}

.search-results-refresh,
.search-results-back {
  min-width: max-content;
  height: 28px;
  min-height: 28px;
  padding: 0 8px;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 650;
  line-height: 1;
  --nw-action-bg: rgba(15, 23, 42, .06);
  --nw-action-text: #374151;
  --nw-action-border: rgba(15, 23, 42, .10);
}

.search-results-refresh {
  width: 28px;
  min-width: 28px;
  padding: 0;
}

.search-results-panel.is-dark .search-results-refresh,
.search-results-panel.is-dark .search-results-back {
  --nw-action-bg: rgba(51, 65, 85, .96);
  --nw-action-text: #cbd5e1;
  --nw-action-border: rgba(148, 163, 184, .28);
}

.search-results-count {
  box-sizing: border-box;
  min-width: 0;
  margin: 0;
  padding: 0;
  color: inherit;
  font-size: 14px;
  font-weight: 400;
  line-height: 20px;
  text-align: left;
}

.search-results-count-placeholder {
  visibility: hidden;
}

.search-results-list {
  box-sizing: border-box;
  display: flex;
  width: calc(100% + 2rem + 2px);
  max-width: calc(56rem + 2px);
  flex-direction: column;
  gap: 12px;
  margin: 0 calc(-1rem - 1px);
}

.search-results-list > .w-full,
.search-results-list > .w-full > .p-0 {
  overflow: visible !important;
}

.search-results-panel .search-results-list > :first-child > .p-0 > .content-container.content-container {
  margin-top: 0 !important;
}

.search-results-list > .w-full > .p-0 > .content-container {
  background: rgba(255, 255, 255, .72) !important;
  background-color: rgba(255, 255, 255, .72) !important;
  background-image: none !important;
  border: 1px solid rgba(15, 23, 42, .10);
  box-shadow: 0 14px 30px rgba(15, 23, 42, .12) !important;
}

.search-results-panel.is-dark .search-results-list > .w-full > .p-0 > .content-container.content-container {
  background: rgba(15, 23, 42, .52) !important;
  background-color: rgba(15, 23, 42, .52) !important;
  background-image: none !important;
  border-color: rgba(255, 255, 255, .12);
  box-shadow: 0 16px 32px rgba(2, 6, 23, .52) !important;
}

.search-results-empty {
  display: flex;
  min-height: 260px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 42px 12px 34px;
  color: #9ca3af;
  text-align: center;
}

.search-results-panel.is-dark .search-results-empty {
  color: #cbd5e1;
}

.search-results-empty-icon {
  display: block;
  width: 48px;
  height: 48px;
  margin: 0 auto 4px;
}

@media screen and (max-width: 640px) {
  .search-results-head {
    align-items: center;
    flex-direction: column;
    min-height: 0;
    padding: 0;
  }

  .search-results-summary {
    margin-bottom: 14px;
  }

  .search-results-actions {
    align-self: center;
  }
}

.visibility-indicator {
  width: 1rem;
  height: 1rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

/* 修改内容卡片样式 */
.content-container {
  padding: 10px;
  border-radius: 12px;
  transition: none;
  margin: 4px 0 1.2rem 0;
  width: 100%;
  box-sizing: border-box;
  position: relative;
  overflow: hidden;
}
/* 内容图片 box 效果与悬停预览动画 */
.message-image-box {
  width: 100%;
  height: 100%;
  border-radius: 12px;
  display: block;
  object-fit: cover;
  transition: transform .18s ease, box-shadow .18s ease, filter .18s ease;
  box-shadow: 0 1px 2px rgba(0,0,0,0.10);
}
.message-image-wrap {
  display: block;
  width: var(--inline-image-thumb-size);
  height: var(--inline-image-thumb-size);
  max-width: 100%;
  overflow: hidden;
  border-radius: 12px;
  background: rgba(0,0,0,0.04);
}
.message-image-wrap.ar-11 { aspect-ratio: 1 / 1; }
.message-image-wrap.ar-169 { aspect-ratio: 16 / 9; }
.message-image-wrap.ar-11 .message-image-box,
.message-image-wrap.ar-169 .message-image-box { height: 100%; }
:global(html.dark) .message-image-wrap {
  background: rgba(255,255,255,0.06);
}
.message-image-box:hover {
  transform: translate3d(0,0,0) scale(1.02);
  box-shadow: 0 6px 18px rgba(0,0,0,0.28);
  filter: saturate(1.06) contrast(1.02);
}
@media (prefers-color-scheme: dark) {
  .message-image-box { box-shadow: 0 1px 2px rgba(255,255,255,0.06); }
  .message-image-box:hover { box-shadow: 0 8px 22px rgba(255,255,255,0.12); }
}
/* 优化图片渲染 */
.content-container img:not(.avatar-img) {
  width: 100%;
  height: auto;
  min-height: 150px;
  object-fit: cover;
  border-radius: 12px;
  box-shadow: none;  /* 移除阴影 */
  transform: translate3d(0, 0, 0);  /* 启用硬件加速 */
  /* 优化图片加载性能 */
  content-visibility: auto;
  contain-intrinsic-size: 150px auto;
  will-change: transform;
}

.content-container .message-image-box,
.content-container .inline-image-thumb img {
  width: 100% !important;
  height: 100% !important;
  min-height: 0 !important;
  object-fit: cover !important;
  object-position: center;
  margin: 0 !important;
  contain-intrinsic-size: auto !important;
}

.content-container .inline-image-thumb {
  width: var(--inline-image-thumb-size);
  height: var(--inline-image-thumb-size);
  max-width: 100%;
  margin: 6px 0;
  overflow: hidden;
  border-radius: 10px;
}

.content-container .inline-image-thumb > a,
.content-container .inline-image-thumb > img {
  display: block;
  width: 100% !important;
  height: 100% !important;
}
/* 简化过渡动画 */
.overflow-y-hidden {
  transition: max-height 0.2s ease;  /* 缩短动画时间 */
}
/* 优化移动端滚动 */
@media screen and (max-width: 1024px) {
  html, body {
    -webkit-overflow-scrolling: touch;
    overflow-scrolling: touch;
  }
}
/* 添加移动端适配 */
@media screen and (max-width: 1024px) {
  .content-container {
    margin: 4px 0 0.85rem 0;
    padding: 6px;
    box-shadow: none;
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }
  
  
  .message-list-container {
    transform: translate3d(0, 0, 0);
    -webkit-overflow-scrolling: touch;
  }
  .content-container img:not(.avatar-img) {
    min-height: 100px;
    /* 移动端图片渲染优化 */
    content-visibility: auto;
    contain-intrinsic-size: 100px auto;
  }
  .message-actions > div {
    transition: none;
  }
}
.content-container::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: -1;
  border-radius: inherit;
}

:global(html:not(.dark)) .content-container { background: #fff; }
.content-container .bg-gradient-to-t { pointer-events: none; }

/* 内容区工具栏（融合/可折叠） */
.message-toolbox { 
  margin-top: 10px; 
  border-radius: 16px; 
}
.content-fade-mask { 
  -webkit-mask-image: linear-gradient(to top, rgba(0,0,0,1) 60%, rgba(0,0,0,0) 100%); 
  mask-image: linear-gradient(to top, rgba(0,0,0,1) 60%, rgba(0,0,0,0) 100%); 
}
.toolbox-anchor { position: relative; display: inline-block; }
.message-toolbox.overlay { 
  position:absolute; 
  right:0; 
  bottom:calc(100% + 8px); 
  z-index:100; 
  padding: 8px;
  border-radius: 12px;
  background: var(--toolbox-bg) !important;
  color: var(--toolbox-fg) !important;
  opacity: 1 !important;
  backdrop-filter: none !important;
  -webkit-backdrop-filter: none !important;
}
.tool-icons { display: flex; align-items: center; gap: 6px; padding: 0; }
.tool-icon { 
  width: 36px;
  min-width: 36px;
  height: 36px;
  display:flex; 
  align-items:center; 
  justify-content:center; 
  cursor:pointer; 
  opacity:1; 
  font-size:18px;
  line-height:1; 
  border-radius: 12px;
  position: relative; 
  transition: background-color .18s ease, border-color .18s ease, color .18s ease, transform .18s ease;
}

.tool-icon:hover { 
  opacity: 1; 
  transform: translate3d(0,0,0) scale(1.06); 
}

.tool-icon > * { color: currentColor; }
.toolbox-dark { background: var(--toolbox-bg); border: 1px solid var(--toolbox-border); }
.toolbox-light { background: var(--toolbox-bg); border: 1px solid var(--toolbox-border); }

/* 工具栏主题色（变量在全局定义，避免 scoped 优先级问题） */
:global(html) {
  --toolbox-bg: rgba(243, 244, 246, 0.96);
  --toolbox-fg: #111827;
  --toolbox-border: rgba(15,23,42,0.12);
  --toolbox-shadow: 0 14px 30px rgba(15,23,42,0.18);
}
:global(html.dark),
:global(body.dark),
:global(.dark) {
  --toolbox-bg: rgba(15, 23, 42, 0.94);
  --toolbox-fg: #ffffff;
  --toolbox-border: rgba(255,255,255,0.18);
  --toolbox-shadow: 0 18px 42px rgba(0,0,0,0.42);
}

.message-toolbox.overlay {
  border: 1px solid var(--toolbox-border) !important;
  box-shadow: var(--toolbox-shadow) !important;
}

.message-toolbox.overlay .tool-icons {
  background: var(--toolbox-bg) !important;
  color: var(--toolbox-fg) !important;
}

.message-toolbox.overlay .tool-icon:not(.nw-action-btn--danger) {
  color: inherit;
}

.message-toolbox.overlay .tool-icon.nw-action-btn--danger,
.message-toolbox.overlay .tool-icon.nw-action-btn--danger > * {
  color: #fff !important;
}

.message-toolbox.overlay::before,
.message-toolbox.overlay::after {
  content: none !important;
}
.author-row { line-height: 1.1; position: relative; }
.message-socialbar { display:flex; align-items:center; gap:12px; padding:0; margin-top:6px; }
.message-divider { border-top: 1px solid rgba(15, 23, 42, .10); }
.content-container.is-dark .message-divider { border-top-color: rgba(255, 255, 255, .12); }
.social-item { display:flex; align-items:center; gap:6px; opacity:.85; cursor:pointer; }
.social-item:hover { opacity:1; }
@media (max-width: 640px) {
  .tool-icons { gap:6px; padding:0; }
  .tool-icon { width:36px; min-width:36px; height:36px; font-size:18px; }
  .tool-icon :deep(svg) { width: 19px !important; height: 19px !important; }
  .tool-icon :deep(.iconify) {
    width: 19px !important;
    height: 19px !important;
    font-size: 19px !important;
    --iconify-width: 1em !important;
    --iconify-height: 1em !important;
  }
  .message-socialbar { gap:10px; padding:0; }
  .social-item { gap: 6px; }
  .message-socialbar :deep(.social-icon) {
    width: 19px !important;
    height: 19px !important;
    font-size: 19px !important;
    line-height: 1 !important;
    display: inline-flex !important;
    align-items: center;
    justify-content: center;
  }
  .message-socialbar :deep(.iconify) {
    width: 19px !important;
    height: 19px !important;
    font-size: 19px !important;
    line-height: 1 !important;
    min-width: 19px !important;
    display: inline-flex !important;
    align-items: center;
    justify-content: center;
    vertical-align: middle;
    flex: 0 0 auto;
    --iconify-width: 1em !important;
    --iconify-height: 1em !important;
  }
  .message-socialbar :deep(.social-icon svg) {
    width: 19px !important;
    height: 19px !important;
  }
  .message-socialbar :deep(svg) {
    width: 19px !important;
    height: 19px !important;
  }
}

.tool-open-btn { border: none; background: transparent; box-shadow: none; padding: 0; }

.expand-button-row {
  position: relative;
  z-index: 30;
  display: flex;
  justify-content: center;
  margin: 8px 0 4px;
}

.expand-toggle-btn {
  min-width: 86px;
  font-size: 14px;
  font-weight: 650;
  line-height: 1;
  white-space: nowrap;
}

/* 确保内容区域的层级正确 */
.overflow-y-hidden {
  transition: max-height 0.3s ease-in-out;
  position: relative;
  z-index: 1;
}
.overflow-visible { overflow: visible !important; }
/* 添加内容过渡动画 */
.overflow-y-hidden {
  transition: max-height 0.3s ease-in-out;
}

/* 修正展开状态下的最大高度限制 */
.content-container .overflow-y-hidden:not(.max-h-\[700px\]) {
  max-height: none;
}

.message-list-item.file-attachment-shadow-open,
.content-container.file-attachment-shadow-open,
.content-container .overflow-y-hidden.file-attachment-shadow-open {
  overflow: visible !important;
}

.content-container.file-attachment-shadow-open :deep(.markdown-preview) {
  overflow: visible !important;
}

.content-container.file-attachment-shadow-open :deep(.markdown-preview.vditor-reset) {
  overflow: visible !important;
}

/*
 * 收起态：纵向必须继续裁剪（max-height 折叠靠它生效），横向本不需要裁剪。
 * 但 CSS 规定同级两轴的 overflow 不能一边非 visible、一边 visible/clip：
 * visible 会被算成 auto，clip 会被算成 hidden（clip-margin 一并失效）。
 * 所以这里不去改 overflow，而是把裁剪盒本身向左右各外扩 16px（附件卡片与失败占位块的
 * 最大阴影 blur 为 32px，左右外扩量即 16px），再用等量 padding 把内容还原回原位；
 * box-sizing 是 border-box，max-height 不受影响，折叠行为保持不变。
 */
.content-container .overflow-y-hidden.file-attachment-shadow-clip {
  margin-left: -16px;
  margin-right: -16px;
  padding-left: 16px;
  padding-right: 16px;
}

.content-container.file-attachment-shadow-clip :deep(.markdown-preview),
.content-container.file-attachment-shadow-clip :deep(.markdown-preview.vditor-reset) {
  overflow: visible;
}

/* 添加页脚固定样式 */
:deep(.text-center.text-xs.text-gray-400.py-4) {
  margin-top: auto;
  padding-top: 2rem;
}
/* 评论区样式（按主题自适应） */
/* 暗黑模式 */
:global(html.dark) :deep(.wl-comment) {
  background: var(--home-surface-dark) !important;
  border-radius: 8px;
  padding: 8px !important;
  margin-bottom: 6px !important;
}
:global(html.dark) :deep(.wl-input) {
  color: #ffffff !important;
  background-color: var(--home-surface-dark) !important;
  border-color: rgba(251, 146, 60, 0.3) !important;
}
:global(html.dark) :deep(.wl-input::placeholder) { color: rgba(255, 255, 255, 0.5) !important; }
:global(html.dark) :deep(.wl-editor) { background: var(--home-surface-dark) !important; color: #fff !important; }
:global(html.dark) :deep(.wl-editor textarea) { 
  color: #ffffff !important;
  caret-color: #ffffff !important;
  background-color: rgba(24, 28, 32, 0.95) !important;
}
:global(html.dark) :deep(.wl-content),
:global(html.dark) :deep(.wl-content p),
:global(html.dark) :deep(.wl-content *) { color: #fff !important; }
:global(html.dark) :deep(.wl-comment .wl-meta .wl-like),
:global(html.dark) :deep(.wl-comment .wl-meta .wl-reply) { color: #999 !important; }
:global(html.dark) :deep(.wl-comment .wl-meta .wl-like:hover),
:global(html.dark) :deep(.wl-comment .wl-meta .wl-reply:hover) { color: #fff !important; }
:global(html.dark) :deep(.wl-btn) { background-color: rgba(251, 146, 60, 0.8) !important; color: #fff !important; }
:global(html.dark) :deep(.wl-action) { color: #fff !important; }
:global(html.dark) :deep(.wl-header) { border-bottom: 1px solid rgba(14, 14, 14, 0.2) !important; }
:global(html.dark) :deep(.wl-card),
:global(html.dark) :deep(.wl-panel) { background: var(--home-surface-dark) !important; border: 1px solid rgba(14, 14, 14, 0.2) !important; }

/* 白天模式 */
:global(html:not(.dark)) :deep(.wl-comment) {
  background: #fff !important;
  border-radius: 8px;
  padding: 8px !important;
  margin-bottom: 6px !important;
}
:global(html:not(.dark)) :deep(.wl-input) {
  color: #111 !important;
  background-color: #fff !important;
  border-color: rgba(0, 0, 0, 0.2) !important;
}
:global(html:not(.dark)) :deep(.wl-input::placeholder) { color: rgba(0, 0, 0, 0.5) !important; }
:global(html:not(.dark)) :deep(.wl-editor) { background: #fff !important; color: #111 !important; }
:global(html:not(.dark)) :deep(.wl-content),
:global(html:not(.dark)) :deep(.wl-content p),
:global(html:not(.dark)) :deep(.wl-content *) { color: #111 !important; }
:global(html:not(.dark)) :deep(.wl-comment .wl-content) { color: #111 !important; }
:global(html:not(.dark)) :deep(.wl-comment .wl-meta) { color: #666 !important; }
:global(html:not(.dark)) :deep(.wl-comment .wl-meta > span),
:global(html:not(.dark)) :deep(.wl-comment .wl-meta > a) { color: #666 !important; }
:global(html:not(.dark)) :deep(.wl-comment .wl-meta .wl-like),
:global(html:not(.dark)) :deep(.wl-comment .wl-meta .wl-reply) { color: #666 !important; }
:global(html:not(.dark)) :deep(.wl-comment .wl-meta .wl-like:hover),
:global(html:not(.dark)) :deep(.wl-comment .wl-meta .wl-reply:hover) { color: #fb923c !important; }
:global(html:not(.dark)) :deep(.wl-btn) {
  background-color: #fff !important;
  color: #111 !important;
  border: 1px solid rgba(251, 146, 60, 0.4) !important;
}
:global(html:not(.dark)) :deep(.wl-action) { color: #222 !important; }
:global(html:not(.dark)) :deep(.wl-header) { border-bottom: 1px solid rgba(0, 0, 0, 0.1) !important; }
:global(html:not(.dark)) :deep(.wl-card),
:global(html:not(.dark)) :deep(.wl-panel) { background: #fff !important; border: 1px solid rgba(0,0,0,0.1) !important; }

/* 确保评论区域不会被遮挡 */
.content-container {
  position: relative;
  z-index: 1;
}
/* 缩小回复列表的垂直间距 */
:global(html.dark) :deep(.wl-replies),
:global(html:not(.dark)) :deep(.wl-replies) { margin-top: 6px !important; }
:global(html.dark) :deep(.wl-comment .wl-content),
:global(html:not(.dark)) :deep(.wl-comment .wl-content) { margin-bottom: 6px !important; }
/* 添加评论内容文本颜色 */
:global(html.dark) :deep(.wl-comment .wl-content) {
  color: #fff !important;
}

:global(html.dark) :deep(.wl-comment .wl-meta) {
  color: #fff !important;
}

:global(html.dark) :deep(.wl-comment .wl-meta > span),
:global(html.dark) :deep(.wl-comment .wl-meta > a) {
  color: #fff !important;
}
/* 移除 markdown 图片的 hover 效果 */
:deep(.markdown-preview img) {
  cursor: pointer;
  transform: none !important; /* 移除 hover 时的缩放效果 */
  transition: none !important; /* 移除过渡效果 */
}

:deep(.markdown-preview img:hover) {
  transform: none !important;
}

/* 确保灯箱层级最高 */
:deep(.fancybox__container) {
  --fancybox-bg: rgba(0, 0, 0, 0.9);
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  z-index: 9999 !important;
}

:deep(.fancybox__backdrop) {
  z-index: 9998 !important;
}
/* 按钮组样式 */
.message-actions {
  position: relative;
  z-index: 1;
}

/* 按钮悬停效果 */
.message-actions > div {
  position: relative;
  transition: all 0.3s ease;
}

.message-actions > div:hover {
  transform: translateY(-2px);
}

.message-actions > div:hover .text-gray-400 {
  color: #fb923c;
  filter: drop-shadow(0 0 2px rgba(251, 146, 60, 0.3));
}
.gradient-dot {
  /* 添加明亮色彩的动态渐变动画 */
  background: linear-gradient(
    45deg,
    #ff6b6b,
    #ffd93d,
    #ff9a9e,
    #cd4e67,
    #ffb347,
    #ff7eb3,
    #ffa07a
  );
  background-size: 400% 400%;
  animation: rainbow 10s ease infinite;
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  font-weight: bold;
}

@keyframes rainbow {
  0% {
    background-position: 0% 50%;
  }
  50% {
    background-position: 100% 50%;
  }
  100% {
    background-position: 0% 50%;
  }
}

/* 隐藏滚动条但保持功能 */
.hide-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.hide-scrollbar::-webkit-scrollbar {
  display: none;
}
/* ... 跳转页文本 ... */
.text-shadow-sm {
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.1),
               0 2px 4px rgba(0, 0, 0, 0.1);
  font-weight: 500;
  letter-spacing: 0.5px;
}
/* 添加移动端分页按钮适配 */
@media screen and (max-width: 768px) {
  .UButton {
    font-size: 0.875rem;
    padding: 0.375rem 0.75rem;
  }
  
  .UInput {
    height: 2rem;
    font-size: 0.875rem;
  }
  
  /* 调整按钮间距 */
  .space-x-4 > * + * {
    margin-left: 0.5rem;
  }
  
  /* 优化移动端分页布局 */
  .flex-wrap {
    flex-wrap: wrap;
  }
  
  .mt-3 {
    margin-top: 0.75rem;
  }
}

/* 缩小媒体与文本上下间距 */
.message-image-box { display:block; margin:6px 0 !important; }
:global(.content-container) :deep(video),
:global(.content-container) :deep(audio),
:global(.content-container) :deep(iframe) { margin:6px 0 !important; }

/* 手机端社交按钮尺寸与对齐优化 */
@media (max-width: 640px) {
  .message-socialbar { gap:12px; }
  .social-item { min-height: 32px; }
  .social-item .ml-1 { font-size: 13px !important; }
}
/* 添加高亮动画样式 */
@keyframes highlight {
  0% { background: rgba(251, 146, 60, 0.3); }
  100% { background: var(--home-surface-dark); }
}

.highlight-message {
  animation: highlight 2s ease-out;
}

:global(.notification-comment-highlight) {
  animation: notification-comment-highlight 2.2s ease-out;
  border-color: rgba(37, 99, 235, 0.45) !important;
}

@keyframes notification-comment-highlight {
  0% { background: rgba(59, 130, 246, 0.22); }
  100% { background: transparent; }
}

/* 轻模式覆盖 Markdown 颜色 */
:global(html:not(.dark)) .content-container :deep(.markdown-preview h1),
:global(html:not(.dark)) .content-container :deep(.markdown-preview h2),
:global(html:not(.dark)) .content-container :deep(.markdown-preview h3),
:global(html:not(.dark)) .content-container :deep(.markdown-preview h4),
:global(html:not(.dark)) .content-container :deep(.markdown-preview h5),
:global(html:not(.dark)) .content-container :deep(.markdown-preview h6) {
  color: #111 !important;
}
:global(html:not(.dark)) .content-container :deep(.markdown-preview) { color: #111 !important; }
:global(html.dark) .content-container :deep(.markdown-preview) { color: #fff !important; }
:global(html:not(.dark)) .content-container :deep(.markdown-preview *:not(pre):not(code):not(.site-attachment-file):not(.site-attachment-file *):not(.site-attachment-audio):not(.site-attachment-audio *)) {
  color: #111 !important;
  opacity: 1 !important;
}
/* 彻底取消白天模式灰度，所有元素不透明 */
:global(html:not(.dark)) .content-container :deep(.markdown-preview *:not(.site-attachment-file):not(.site-attachment-file *):not(.site-attachment-audio):not(.site-attachment-audio *)) { opacity: 1 !important; }
:global(html:not(.dark)) .content-container :deep(.markdown-preview p),
:global(html:not(.dark)) .content-container :deep(.markdown-preview li),
:global(html:not(.dark)) .content-container :deep(.markdown-preview span),
:global(html:not(.dark)) .content-container :deep(.markdown-preview em),
:global(html:not(.dark)) .content-container :deep(.markdown-preview strong),
:global(html:not(.dark)) .content-container :deep(.markdown-preview blockquote),
:global(html:not(.dark)) .content-container :deep(.markdown-preview code) { opacity: 1 !important; }

/* 确保所有模式下链接颜色都是蓝色 */
.content-container :deep(.markdown-preview a:not(.site-attachment-file)) {
  color: #0366d6 !important;
  text-decoration: none !important;
  background-color: transparent !important;
  padding: 0 !important;
  border-radius: 0 !important;
  border: none !important;
  text-shadow: none !important;
  opacity: 1 !important;
  font-weight: 500 !important;
}
.content-container :deep(.markdown-preview a:not(.site-attachment-file):hover) {
  color: #1d4ed8 !important;
  text-decoration: underline !important;
}

/* 内容容器内的 GitHub 卡片主题（确保随页面切换） */
:global(html.dark) .content-container :deep(.github-card) { 
  border: 1px solid #30363d !important; 
  background: #161b22 !important; 
  color: #c9d1d9 !important; 
}
:global(html:not(.dark)) .content-container :deep(.github-card) { 
  border: 1px solid #e5e7eb !important; 
  background: #ffffff !important; 
  color: #111827 !important; 
}
:global(html.dark) .content-container :deep(.github-card-title) { color: #58a6ff !important; }
:global(html:not(.dark)) .content-container :deep(.github-card-title) { color: #0366d6 !important; }
:global(html.dark) .content-container :deep(.github-card-desc) { color: #8b949e !important; }
:global(html:not(.dark)) .content-container :deep(.github-card-desc) { color: #6b7280 !important; }
:global(html.dark) .content-container :deep(.github-card-footer) { color: #8b949e !important; }
:global(html:not(.dark)) .content-container :deep(.github-card-footer) { color: #6b7280 !important; }
:global(html.dark) .content-container :deep(.github-card-footer span) { 
  background: rgba(0,0,0,0.35) !important; 
  color: #c9d1d9 !important; 
}
:global(html:not(.dark)) .content-container :deep(.github-card-footer span) { 
  background: rgba(255,255,255,0.65) !important; 
  color: #111827 !important; 
}

/* 内容容器内的 APlayer 主题适配（亮/暗模式） */
:global(html:not(.dark)) .content-container :deep(.aplayer) {
  background: #ffffff !important;
  color: #111111 !important;
  border: 1px solid #e5e7eb !important;
  box-shadow: 0 4px 12px rgba(0,0,0,0.08) !important;
}
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-title),
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-author),
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-lrc p) { color: #1f2937 !important; }
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-bar-wrap .aplayer-bar) { background-color: #e5e7eb !important; }
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-played) { background-color: #3b82f6 !important; }
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-loaded) { background-color: #9ca3af !important; }
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-info) { color: #111827 !important; }
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-icon),
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-list-index) { color: #374151 !important; }

:global(html.dark) .content-container :deep(.aplayer) {
  background: var(--home-surface-dark) !important;
  color: #ffffff !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
  box-shadow: 0 4px 12px rgba(255,255,255,0.08) !important;
}
:global(html.dark) .content-container :deep(.aplayer .aplayer-title),
:global(html.dark) .content-container :deep(.aplayer .aplayer-author),
:global(html.dark) .content-container :deep(.aplayer .aplayer-lrc p) { color: #ffffff !important; }
:global(html.dark) .content-container :deep(.aplayer .aplayer-bar-wrap .aplayer-bar) { background-color: #30363d !important; }
:global(html.dark) .content-container :deep(.aplayer .aplayer-played) { background-color: #60a5fa !important; }
:global(html.dark) .content-container :deep(.aplayer .aplayer-loaded) { background-color: #64748b !important; }
:global(html.dark) .content-container :deep(.aplayer .aplayer-info) { color: #e5e7eb !important; }
:global(html.dark) .content-container :deep(.aplayer .aplayer-icon),
:global(html.dark) .content-container :deep(.aplayer .aplayer-list-index) { color: #e5e7eb !important; }

:global(html:not(.dark)) .content-container :deep(pre) {
  background-color: #f5f5f5 !important;
  border: 1px solid #e5e7eb !important;
  color: #1f2937 !important;
}

:global(html:not(.dark)) .content-container :deep(.hljs) {
  color: #1f2937 !important;
}

/* 视频和音频播放器的主题适配 */
:global(html:not(.dark)) .content-container :deep(video) {
  background-color: #ffffff !important;
  border: 1px solid #e5e7eb !important;
  border-radius: 8px !important;
}

:global(html:not(.dark)) .content-container :deep(audio) {
  background-color: #ffffff !important;
  border: none !important;
  border-radius: 8px !important;
}

:global(html.dark) .content-container :deep(video) {
  background-color: var(--home-surface-dark) !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}

:global(html.dark) .content-container :deep(audio) {
  background-color: var(--home-surface-dark) !important;
  border: none !important;
  border-radius: 8px !important;
}

/* iframe 嵌入内容的主题适配 */
:global(html:not(.dark)) .content-container :deep(iframe) {
  border: 1px solid #e5e7eb !important;
  border-radius: 8px !important;
}

:global(html.dark) .content-container :deep(iframe) {
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}

/* 作者悬停卡片 */
.site-author-card { position: absolute; top: -28px; left: 36px; z-index: 2147483647; border-radius: 12px; padding: 10px 12px; min-width: 300px; box-shadow: 0 8px 24px rgba(0,0,0,0.25); border: 1px solid rgba(0,0,0,0.08); transform: translate3d(0,0,0); isolation: isolate; backdrop-filter: none; -webkit-backdrop-filter: none; overflow: visible; }
.site-author-card::after { content: ''; position: absolute; left: -10px; top: 27px; width: 0; height: 0; border-top: 10px solid transparent; border-bottom: 10px solid transparent; z-index: 1; filter: drop-shadow(0 2px 4px rgba(0,0,0,0.25)); }
:global(html.dark) .site-author-card { --home-surface-dark-elevated: rgb(15, 24, 39); background: var(--home-surface-dark-elevated) !important; border-color: rgba(255,255,255,0.14); box-shadow: 0 10px 24px rgba(0,0,0,0.38); }
:global(html.dark) .site-author-card::after { border-right: 8px solid var(--home-surface-dark-elevated); }
:global(html:not(.dark)) .site-author-card::after { border-right: 8px solid #ffffff; }
.site-author-card-header { display: flex; gap: 10px; align-items: center; margin-bottom: 8px; pointer-events: auto; }
.site-author-card-body { display: flex; gap: 10px; align-items: center; justify-content: flex-end; }
.site-author-card-sign { overflow: hidden; font-size: 12px; line-height: 16px; white-space: nowrap; flex: 1; text-align: center; }
.site-author-card-scroll { display: inline-block; white-space: nowrap; will-change: transform; animation: author-sign-scroll 12s linear infinite; }
.site-author-card-scroll.center { animation: none; }
.author-card-muted { color: #7a7f85 }
@keyframes author-sign-scroll { 0% { transform: translateX(100%); } 100% { transform: translateX(-100%); } }
.author-card-muted { color: #7a7f85 }
@media (max-width: 640px) { .site-author-card { position: fixed; left: 12px; right: 12px; top: auto; bottom: auto; min-width: auto; z-index: 2147483647; } .site-author-card::after { display: none; } }
.pager-done-wrap {
  margin-top: 16px;
  text-align: center;
}
.masonry-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr)); grid-auto-rows: 1px; grid-auto-flow: row dense; --masonry-gap: 8px; column-gap: 12px; row-gap: 0; align-items: start; }
.masonry-grid > .message-list-item { align-self: start; min-width: 0; }
.masonry-grid > .message-list-item > .p-0 > .content-container { margin: 0 !important; }
.masonry-grid { margin-top: 0; }
.search-results-list.masonry-grid { width: 100%; max-width: none; margin: 0; }
.message-list-masonry .search-results-panel { margin-top: 0; }
.message-list-wide .search-results-list, .message-list-wide .search-results-board-head { width: 100%; max-width: none; margin-left: 0; margin-right: 0; }
.message-list-wide .search-results-board-head { padding-left: 0; padding-right: 0; }
</style>
