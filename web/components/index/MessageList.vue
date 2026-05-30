<template>
  <div>
    <div class="min-h-screen flex flex-col">
    <!-- 空状态显示 -->
    <div v-if="props.pageReady && !displayMessages.length" class="text-center text-gray-500 py-8">
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
      <!-- 搜索模式提示 -->
      <div 
        v-if="isSearchMode" 
        class="flex justify-between items-center mb-4 p-4 rounded-lg"
      >
        <p class="text-gray-400">搜索结果 ({{ searchResults.length }} 条)</p>
        <UButton
          size="sm"
          variant="ghost"
          class="text-gray-400 hover:text-orange-500"
          icon="i-heroicons-arrow-left"
          @click="resetList"
        >
          返回完整列表
        </UButton>
      </div>
      <!-- 消息列表 -->
      <div class="my-4">
         <!-- 无搜索结果提示 -->
  <div v-if="isSearchMode && searchResults.length === 0" class="text-center text-gray-500 py-8">
    <UIcon name="i-heroicons-magnifying-glass" class="w-12 h-12 mx-auto mb-4" />
    <p>未找到相关内容</p>
  </div>
        <!-- 消息列表内容 -->
        <div v-for="(msg, idx) in displayMessages" :key="msg.id" class="w-full h-auto overflow-hidden flex flex-col justify-between">

          <div class="p-0">
            <div :class="['content-container', innerContainerClass, listThemeClass]" :data-msg-id="msg.id">
              <div class="flex items-center gap-2 mb-1 author-row">
                <img :src="authorAvatar(msg)" alt="avatar" class="avatar-img w-9 h-9 rounded-full object-cover" @error="authorAvatarOnError($event, msg.username || '匿名')" @mouseenter="showAuthorCard($event, msg)" @mouseleave="hideAuthorCard" @click="toggleAuthorCard($event, msg)" />
                <div v-if="openAuthorId === msg.id" class="noise-author-card bg-white text-black dark:bg-[var(--home-surface-dark-elevated)] dark:text-white" :style="openAuthorStyle">
                  <div class="noise-author-card-header">
                    <img :src="authorProfileAvatar(msg)" class="avatar-img w-10 h-10 rounded-full object-cover" />
                    <div class="font-semibold leading-tight text-[14px]">{{ msg.username }}</div>
                  </div>
                  <div class="noise-author-card-body">
                    <div class="noise-author-card-sign"><span :class="['noise-author-card-scroll', { 'center': !authorSignShouldScroll(msg) }]">{{ authorProfileDesc(msg) }}</span></div>
                    <div class="author-card-muted text-[12px] whitespace-nowrap">笔记 {{ authorProfileCount(msg) }}</div>
                  </div>
                </div>
                <div class="min-w-0">
                  <div class="text-sm font-semibold leading-tight">{{ msg.username || siteConfig.username || '匿名' }}</div>
                  <div class="flex items-center gap-2">
                    <span class="text-xs opacity-70">{{ formatDate(msg.created_at) }}</span>
                  </div>
                </div>
                <div class="ml-auto flex items-center gap-2 text-xs opacity-80">
                  <UIcon v-if="msg.private" name="i-mdi-lock-outline" class="w-4 h-4" />
                  <UIcon v-if="msg.pinned" name="i-mdi-pin" class="w-4 h-4" />
                </div>
              </div>
              
              <!-- 图片内容（支持放大预览 + 悬停效果） -->
              <a v-if="msg.image_url" :href="resolveMediaUrl(msg.image_url)" :data-fancybox="`message-image-${msg.id}`" :class="['message-image-wrap', messageImageAR[msg.id] || '']">
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
              <div class="overflow-y-hidden relative" :class="[{ 'max-h-[700px]': !isExpanded[msg.id] && !hasGrid[msg.id] }, listThemeTextClass]" :style="contentStyle(idx)">
                <MarkdownRenderer :content="msg.content" :enableGithubCard="siteConfig?.enableGithubCard === true" @tagClick="handleTagClick" @rendered="checkContentHeight" link-target="_blank"/>
                <div v-if="shouldShowExpandButton[msg.id] && !isExpanded[msg.id]"
    :class="['absolute bottom-0 left-0 right-0 h-14 bg-gradient-to-t backdrop-blur-sm pointer-events-none content-fade-mask', gradientClass]" style="z-index:20"></div>
              </div>
              
              <!-- 展开按钮 - 放在分割线上方 -->
              <div v-if="shouldShowExpandButton[msg.id]"
                :class="['relative left-0 right-0 flex justify-center z-30', isExpanded[msg.id] ? 'mb-1' : '-mt-2 mb-1']"
              >
                <div 
                  class="expand-button-container px-4 py-1.5 rounded-full backdrop-blur-sm"
                >
                  <button
                    class="expand-toggle-btn text-sm inline-flex items-center justify-center gap-1"
                    @click="toggleExpand(msg.id)"
                    aria-label="toggle-expand"
                  >
                    {{ isExpanded[msg.id] ? '收起全文' : '展开全文' }}
                    <UIcon :name="isExpanded[msg.id] ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" class="w-4 h-4 flex-shrink-0" />
                  </button>
                </div>
              </div>
              <div class="border-t border-gray-300 dark:border-gray-700 my-3"></div>
              <div class="message-socialbar">
                <button class="social-item" @click="like(msg.id)" :title="'点赞'">
                  <UIcon
                    :name="(likedMap[msg.id] ? 'i-mdi-heart' : 'i-mdi-heart-outline')"
                    class="social-icon"
                    :class="[likedMap[msg.id] ? 'text-red-500' : '']"
                  />
                  <span :class="['opacity-80', isMobile ? 'text-xs' : 'text-sm']">{{ likesMap[msg.id] ?? (msg.like_count || 0) }}</span>
                </button>
                <button v-if="!isGuestbookMessage(msg)" class="social-item" @click="toggleComment(msg.id)" :title="'评论'">
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
                  <UButton size="xs" color="gray" variant="ghost" :ui="{ base: 'rounded-full' }" class="tool-open-btn" @click="toggleToolbox(msg.id)" title="展开工具">
                    <UIcon name="i-heroicons-ellipsis-horizontal" style="font-size: 16px; line-height: 1;" />
                  </UButton>
                  <div class="message-toolbox overlay" v-show="openToolboxId === msg.id">
                    <div class="tool-icons">
                      <div v-if="canEdit(msg)" class="tool-icon" :data-label="(msg.private ? '设为公开' : '设为私密')" @click="togglePrivate(msg)"><UIcon :name="msg.private ? 'i-mdi-lock-outline' : 'i-mdi-lock-open-outline'" /></div>
                      <div v-else-if="msg.private" class="tool-icon" data-label="私密"><UIcon name="i-mdi-lock-outline" /></div>
                      <div v-if="canPin(msg)" class="tool-icon" :data-label="(msg.pinned ? '取消置顶' : '置顶内容')" @click="togglePin(msg)"><UIcon :name="msg.pinned ? 'i-mdi-pin' : 'i-mdi-pin-outline'" /></div>
                      <div v-if="isLogin" class="tool-icon" data-label="编辑" @click="editMessage(msg)"><UIcon name="i-mdi-pencil-outline" /></div>
                      <div class="tool-icon" data-label="复制" @click="copyContent(msg.content)"><UIcon name="i-mdi-content-copy" /></div>
                      <div v-if="isLogin" class="tool-icon" data-label="删除" @click="deleteMsg(msg.id)"><UIcon name="i-mdi-trash-can-outline" /></div>
                  </div>
                  </div>
                </div>
              </div>
              <div v-if="(expandedCommentsMap[msg.id] || activeCommentId === msg.id) && isCommentEnabled && !isGuestbookMessage(msg)" :id="`comment-container-${msg.id}`" class="mt-2" style="position: relative;">
                <BuiltinComments v-if="isBuiltin && apiReachable" :key="(commentRefreshKey[msg.id] || 0)" :message-id="msg.id" :site-config="siteConfig" :show-input="activeCommentId === msg.id" @cancel="handleCancel(msg.id, $event)" />
                <div v-else-if="useWaline && apiReachable" :id="`waline-${msg.id}`"></div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <!-- 预取下一页哨兵 -->
      <div v-if="!isSearchMode && !isPersonalGuest" ref="prefetchSentinel" style="height:1px"></div>
      <!-- 分页控制区域 -->
      <div v-if="!isSearchMode && !isPersonalGuest" class="pager-shell flex justify-center items-center space-x-4 w-full my-4 flex-wrap md:flex-nowrap">
  <div class="flex justify-center items-center space-x-4 w-full md:w-auto">
    <UButton 
      v-if="message.page > 1"
      color="gray" 
      variant="solid" 
      size="xs" 
      class="rounded-full px-4 py-1.5 shadow-lg hover:shadow-xl transition-all duration-300 pager-btn"
      @click="loadPreviousPage"
      :disabled="isPageLoading"
    >
      <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-left" class="w-4 h-4 pager-icon" /></span>
      上一页
    </UButton>

    <UButton 
      v-if="message.hasMore"
      color="gray" 
      variant="solid" 
      size="xs" 
      class="rounded-full px-4 py-1.5 shadow-lg hover:shadow-xl transition-all duration-300 pager-btn"
      @click="loadNextPage"
      :disabled="isPageLoading"
    >
      下一页
      <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-right" class="w-4 h-4 pager-icon" /></span>
    </UButton>
    <span v-if="isPageLoading" class="ml-2 text-orange-400">加载中...</span>
  </div>

  <!-- 页码显示和跳转 -->
  <div class="flex items-center justify-center space-x-2 w-full md:w-auto mt-3 md:mt-0">
    <span class="text-gray-500 text-shadow-sm text-sm">第 {{ message.page }} 页</span> 
    <UInput
      v-model="targetPage"
      type="number"
      min="1"
      :max="totalPages"
      class="w-12 text-center text-sm" 
      placeholder="#"
      @keyup.enter="jumpToPage"
    />
    <UButton
      size="xs" 
      color="gray"
      variant="ghost"
      class="text-gray-400 hover:text-orange-500 text-sm pager-jump-btn"  
      @click="jumpToPage"
    >
      跳转
    </UButton>
  </div>
</div>
      <!-- 加载完毕提示 -->
      <div v-if="!isSearchMode && message.messages.length > 0 && !message.hasMore" class="text-center text-gray-500 mt-4">
        <UIcon name="i-fluent-emoji-flat-confetti-ball" size="lg" />
        加载完毕~
      </div>
    </div>
    
</div>
  <!-- 编辑对话框 -->
  <UModal v-model="showEditModal" :ui="{ width: 'sm:max-w-2xl' }">
    <UCard>
      <template #header>
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-medium">编辑内容</h3>
          <UButton color="gray" variant="ghost" icon="i-mdi-close" class="-my-1" @click="showEditModal = false" />
        </div>
      </template>
      <div class="flex flex-col space-y-4">
        <UTextarea
          v-model="editingContent"
          placeholder="编辑内容..."
          rows="10"
          class="font-mono text-sm"
        />
        <div class="border-t border-gray-200 my-2 pt-2">
          <div class="text-sm text-gray-500 mb-2">预览：</div>
          <div class="p-4 rounded-lg overflow-auto max-h-[300px] bg-white dark:bg-[var(--home-surface-dark-elevated)]">
            <div class="text-black dark:text-white">
              <MarkdownRenderer :content="editingContent" :enableGithubCard="siteConfig?.enableGithubCard === true" />
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end space-x-2">
          <UButton color="gray" variant="outline" @click="showEditModal = false" class="text-white">
            取消
          </UButton>
          <UButton color="orange" @click="saveEditedMessage" :loading="isSaving" class="text-white">
            保存
          </UButton>
        </div>
      </template>
    </UCard>
  </UModal>
  </div>
</template>

<script setup lang="ts">
import { useMessageStore } from "~/store/message";
import { useUserStore } from "~/store/user";
import MarkdownRenderer from "~/components/index/MarkdownRenderer.vue";
import BuiltinComments from '../comments/BuiltinComments.vue'
import { writeClipboardText } from '~/utils/clipboard'
import { useRuntimeConfig } from '#imports'
import { useToast } from '#ui/composables/useToast'
const config = useRuntimeConfig()
const BASE_API = config.public.baseApi || '/api'

const resolveMediaUrl = (s: string) => {
  if (!s) return ''
  if (/^https?:\/\//i.test(s)) return s
  
  const base = (BASE_API || '').replace(/\/$/, '')
  const path = String(s || '')
  const p = path.startsWith('/') ? path : `/${path}`

  // 特殊处理: 如果路径以 /api/ 开头且 base 以 /api 结尾，避免重复
  if (p.startsWith('/api/') && base.endsWith('/api')) {
    return `${base.substring(0, base.length - 4)}${p}`
  }
  
  // 如果路径以 /images/ 或 /video/ 开头，且 base 包含 /api，可能需要注意
  // 但通常 /images/ 是相对于 /api 的? 不，gin router 是 /api/images
  
  return `${base}${p}`
}

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
  if (img) img.src = fallback
}
// 主题切换改为纯 CSS（html.dark）控制，避免组件重渲染导致媒体刷新

const contentStyle = (index: number) => {
  return index < 5 ? '' : 'content-visibility:auto;contain-intrinsic-size:700px';
}

// 内容工具栏折叠与样式
const openToolboxId = ref<number | null>(null)
const toggleToolbox = (id: number) => {
  openToolboxId.value = openToolboxId.value === id ? null : id
}
const closeToolboxIfOutside = (e: Event) => {
  const target = e.target as HTMLElement
  if (!target) { openToolboxId.value = null; return }
  const inPanel = !!target.closest('.message-toolbox')
  const onToggle = !!target.closest('.tool-open-btn')
  if (!inPanel && !onToggle) openToolboxId.value = null
}
onMounted(() => {
  window.addEventListener('scroll', () => { openToolboxId.value = null })
  document.addEventListener('click', closeToolboxIfOutside, true)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', closeToolboxIfOutside, true)
})
// 工具箱主题同样由 CSS 控制

// 点赞与评论计数
const likesMap = ref<Record<number, number>>({})
const likedMap = ref<Record<number, boolean>>({})
const commentCountMap = ref<Record<number, number>>({})
const fetchedCommentIds = ref<Record<number, boolean>>({})
const pendingCommentIds = ref<number[]>([])
let activeCommentBatch = false
const batchSize = 20
let io: IntersectionObserver | null = null
const fetchCommentCountsBatch = async (ids: number[]) => {
  if (!ids.length) return
  try {
    const resp = await fetch(`${BASE_API}/messages/comments/counts`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
      body: JSON.stringify({ ids })
    })
    if (!resp.ok) return
    const js = await resp.json()
    const arr = Array.isArray(js?.data) ? js.data : []
    arr.forEach((row: any) => {
      const id = Number(row?.id || 0)
      const count = Number(row?.count || 0)
      if (!id) return
      commentCountMap.value[id] = count
      fetchedCommentIds.value[id] = true
      if (isBuiltin.value && count > 0) expandedCommentsMap.value[id] = true
    })
  } catch {}
}
const runCommentQueue = () => {
  if (activeCommentBatch) return
  if (!pendingCommentIds.value.length) return
  const uniq: number[] = []
  const seen = new Set<number>()
  while (uniq.length < batchSize && pendingCommentIds.value.length) {
    const id = pendingCommentIds.value.shift() as number
    if (seen.has(id)) continue
    seen.add(id)
    if (!fetchedCommentIds.value[id]) uniq.push(id)
  }
  if (!uniq.length) return
  activeCommentBatch = true
  fetchCommentCountsBatch(uniq).finally(() => { activeCommentBatch = false; runCommentQueue() })
}
const scheduleCommentFetch = (id: number) => {
  if (fetchedCommentIds.value[id]) return
  pendingCommentIds.value.push(id)
  runCommentQueue()
}
const isMobile = (typeof window !== 'undefined') && window.matchMedia('(max-width: 1024px)').matches
const observeContainers = () => {
  if (!io) return
  const nodes = document.querySelectorAll('.content-container')
  nodes.forEach((node) => {
    const idAttr = (node as HTMLElement).getAttribute('data-msg-id') || '0'
    const id = Number(idAttr)
    const m = getMessageById(id)
    if (id && m && !isGuestbookMessage(m)) io!.observe(node)
  })
}
onMounted(() => {
  try {
    if (isMobile) return
    io = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          const el = entry.target as HTMLElement
          const id = Number(el.getAttribute('data-msg-id') || '0')
          if (id) scheduleCommentFetch(id)
          io && io.unobserve(el)
        }
      })
    }, { rootMargin: '256px 0px' })
    observeContainers()
  } catch {}
})
onBeforeUnmount(() => { try { io && io.disconnect() } catch {} })
const like = async (id: number) => {
  try {
    const resp = await fetch(`${BASE_API}/messages/${id}/like/toggle`, { method: 'POST', credentials: 'include', headers: { 'Accept': 'application/json' } })
    if (!resp.ok) throw new Error('点赞失败')
    const js = await resp.json()
    const count = js?.data?.like_count ?? (likesMap.value[id] || 0)
    const liked = !!js?.data?.liked
    likesMap.value[id] = count
    likedMap.value[id] = liked
  } catch (e) {
    useToast().add({ title: '点赞失败', color: 'red', timeout: 2000 })
  }
}

const targetPage = ref('');
const totalPages = computed(() => Math.ceil(message.total / 15));
const jumpToPage = async () => {
  const page = parseInt(targetPage.value);
  if (!page || page < 1 || page > totalPages.value || message.loading) {
    useToast().add({
      title: '页码无效',
      description: `请输入 1-${totalPages.value} 之间的数字`,
      color: 'orange',
      timeout: 2000
    });
    return;
  }

  try {
    const sc = document.querySelector('.content-wrapper') as HTMLElement | null;
    const prevY = sc ? sc.scrollTop : window.scrollY;
    const result = await message.getMessages(pageQueryFor(page));
    
    if (!result) {
      throw new Error('跳转页面失败');
    }
    
    const nonPinned = result.items.filter((m: any) => !m.pinned);
    message.messages = [...pinnedTopItems.value, ...nonPinned];
    message.page = result.page || page;
    
    targetPage.value = '';
    await nextTick();
    if (sc) sc.scrollTo({ top: prevY, behavior: 'instant' }); else window.scrollTo({ top: prevY, behavior: 'instant' });
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
  siteConfig: {
    type: Object,
    required: true
  },
  targetMessageId: {
    type: String,
    default: null
  },
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
  }
});
const outerContainerClass = computed(() => props.wide ? 'flex-grow w-full px-1 sm:px-2' : 'flex-grow w-full px-1 sm:px-2')
const innerContainerClass = computed(() => props.wide ? '' : 'mx-auto sm:max-w-4xl')
// 独立的内容主题（与页面主题解耦）
const contentTheme = inject('contentTheme', ref<string>(typeof window !== 'undefined' ? (localStorage.getItem('contentTheme') || 'dark') : 'dark'))
const listThemeClass = computed(() => contentTheme.value === 'dark' ? 'bg-[var(--home-surface-dark)] text-white' : 'bg-white text-black')
const listThemeTextClass = computed(() => contentTheme.value === 'dark' ? 'text-white' : 'text-black')
const gradientClass = computed(() => contentTheme.value === 'dark' ? 'from-[var(--home-surface-dark)] via-[rgba(32,42,54,0.82)] to-transparent' : 'from-[rgba(255,255,255,1)] via-[rgba(255,255,255,0.8)] to-transparent')
const useWaline = computed(() => {
  return false
})
// 添加监听器
watch(() => props.targetMessageId, async (newId) => {
  if (!newId) return;
  
  await nextTick();
  const targetElement = document.querySelector(`.content-container[data-msg-id="${newId}"]`);
  if (targetElement) {
    targetElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
    // 添加高亮效果
    targetElement.classList.add('highlight-message');
    setTimeout(() => {
      targetElement.classList.remove('highlight-message');
    }, 2000);
  }
}, { immediate: true });

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

const prefetchAuthorProfilesForList = () => {
  const names = Array.from(new Set((message.messages || []).map((m: any) => String(m?.username || '').trim()).filter((n) => !!n)))
  names.forEach((n) => fetchAuthorProfile(n))
}
watch(() => message.messages, () => { prefetchAuthorProfilesForList() }, { deep: false, immediate: true })

const activeCommentId = ref<number | null>(null);
const commentRefreshKey = ref<Record<number, number>>({});
const expandedCommentsMap = ref<Record<number, boolean>>({});
const isCommentEnabled = computed(() => {
  const v: any = (props.siteConfig as any)?.commentEnabled
  return v === true || v === 'true'
})
const isBuiltin = computed(() => {
  return true
})
const guestbookId = ref<number | null>(null)
const isGuestbookMessage = (m: any) => {
  if (!m) return false
  if (guestbookId.value && m.id === guestbookId.value) return true
  const text = String(m.content || '').toLowerCase()
  return /#guestbook|#留言|留言板/.test(text)
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
  const getMessageById = (id: number) => (message.messages || []).find((m: any) => m.id === id)
  const userStore = useUserStore();
  const isLogin = computed(() => userStore.isLogin);
  const isPersonalTab = computed(() => props.activeTab === 'personal')
  const isPersonalGuest = computed(() => isPersonalTab.value && !userStore.isLogin)
  const currentUserId = computed(() => Number((userStore.user as any)?.userid || (userStore.user as any)?.id || (userStore.user as any)?.user_id || 0))
  const currentUsername = computed(() => String((userStore.user as any)?.username || '').trim())
  const pageQueryFor = (pageNumber: number) => {
    const query: any = { page: pageNumber, pageSize: 15 }
    if (isPersonalTab.value && currentUserId.value) query.authorId = currentUserId.value
    return query
  }
  const isCurrentUserMessage = (msg: any) => {
    if (!msg || !userStore.isLogin) return false
    const msgUserId = Number(msg?.user_id || msg?.userId || 0)
    if (currentUserId.value && msgUserId) return msgUserId === currentUserId.value
    return !!currentUsername.value && String(msg?.username || '').trim() === currentUsername.value
  }
  const isContentEmpty = (m: any) => {
    const img = String(m?.image_url || '').trim()
    const c0 = String(m?.content || '')
    const c = c0.replace(/\s|&nbsp;|\u00A0/gi, '').trim()
    return img === '' && c.length === 0
  }
const openInNewTab = (url: string) => {
  window.open(url, '_blank', 'noopener,noreferrer');
};
// 修改标签点击处理函数
const handleTagClick = async (tag: string) => {
  try {
    const encodedTag = encodeURIComponent(tag.trim());
    const response = await fetch(`${BASE_API}/messages/tags/${encodedTag}`, {
      credentials: 'include',
      headers: {
        'Accept': 'application/json'
      }
    });
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    const data = await response.json();
    if (data.code === 1 && Array.isArray(data.data)) {
      isSearchMode.value = true;
      searchResults.value = data.data;
      await nextTick();
      deferMeasure();
      deferInitFancybox();
    } else {
      throw new Error(data.msg || '获取标签内容失败');
    }
  } catch (error: any) {
    console.error('获取标签消息失败:', error);
    useToast().add({
      title: '获取标签消息失败',
      description: error.message || '服务器错误，请稍后重试',
      color: 'red',
      timeout: 3000
    });
  }
};
// 修改重置搜索函数名称，使其更通用
// 修改 resetList 函数
const resetList = async () => {
  searchResults.value = [];
  isSearchMode.value = false;
  
  // 重新获取当前视图消息列表
  await message.getMessages(pageQueryFor(1));
  
  await nextTick();
  deferMeasure();
  deferInitFancybox();
};

const deleteMsg = async (id: number) => {
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

const initFancybox = () => {
  if (window.Fancybox) {
    window.Fancybox.destroy();
    const fancyboxOptions = {
      Carousel: {
        infinite: false,
      },
      Toolbar: {
        display: [
          { id: "prev", position: "center" },
          { id: "counter", position: "center" },
          { id: "next", position: "center" },
          "zoom",
          "slideshow",
          "fullscreen",
          "close",
        ],
      },
      Image: {
        zoom: true,
        click: true,
        wheel: "slide",
      },
    };

    const mdImages = document.querySelectorAll(".markdown-preview img");
    mdImages.forEach((img) => {
      const src = img.getAttribute("src") || "";
      if (img.closest('.image-grid-item')) return;
      const parent = img.parentElement;
      if (parent && parent.tagName === "A") {
        parent.setAttribute("data-fancybox", "uploaded-image");
        const href = parent.getAttribute('href') || '';
        const isImageHref = /\.(png|jpe?g|gif|webp|bmp|svg)(\?.*)?$/i.test(href) || href.startsWith('data:') || href.startsWith('blob:');
        if (!href || href === '#' || href.startsWith('javascript:') || !isImageHref) {
          parent.setAttribute('href', src);
        }
      } else {
        const wrapper = document.createElement("a");
        wrapper.href = src;
        wrapper.setAttribute("data-fancybox", "uploaded-image");
        wrapper.style.display = "block";
        img.parentNode.insertBefore(wrapper, img);
        wrapper.appendChild(img);
      }
    });

    window.Fancybox.bind("[data-fancybox]", fancyboxOptions);
  }
};

let fancyboxScheduled = false
const deferInitFancybox = () => {
  if (fancyboxScheduled) return
  fancyboxScheduled = true
  const run = () => { try { initFancybox() } finally { fancyboxScheduled = false } }
  try {
    const w: any = window
    if (w && typeof w.requestIdleCallback === 'function') w.requestIdleCallback(run)
    else setTimeout(run, 0)
  } catch { setTimeout(run, 0) }
}

let measureScheduled = false
const deferMeasure = () => {
  if (measureScheduled) return
  measureScheduled = true
  const run = () => { try { checkContentHeight() } finally { measureScheduled = false } }
  try {
    const w: any = window
    if (w && typeof w.requestIdleCallback === 'function') w.requestIdleCallback(run)
    else requestAnimationFrame(run)
  } catch { setTimeout(run, 0) }
}

const scrollToCommentInput = async (msgId: number) => {
  await nextTick()
  const container = document.querySelector(`#comment-container-${msgId}`) as HTMLElement | null
  const input = container?.querySelector('.comment-input-card textarea') as HTMLTextAreaElement | null
  const target = (input?.closest('.comment-input-card') as HTMLElement | null) || input || container
  target?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  try {
    input?.focus({ preventScroll: true })
  } catch {
    input?.focus?.()
  }
}

const toggleComment = async (msgId: number) => {
  const m = getMessageById(msgId)
  if (isGuestbookMessage(m)) return
  const isShown = !!(expandedCommentsMap.value[msgId] || activeCommentId.value === msgId)
  if (isShown) {
    expandedCommentsMap.value[msgId] = false
    if (activeCommentId.value === msgId) activeCommentId.value = null
    return
  }
  activeCommentId.value = msgId
  commentRefreshKey.value[msgId] = (commentRefreshKey.value[msgId] || 0) + 1;
  expandedCommentsMap.value[msgId] = true;
  if ((props.siteConfig?.commentSystem || 'waline') === 'builtin') {
    await nextTick();
    window.dispatchEvent(new Event(`refresh-comments-${msgId}`));
    await scrollToCommentInput(msgId)
    return;
  }
  if (useWaline.value) {
    await loadWalineAssets();
    if (!window.Waline) return;
    const el = document.querySelector(`#waline-${msgId}`);
    if (el) {
      window.Waline.init({
        el: `#waline-${msgId}`,
        serverURL: props.siteConfig.walineServerURL,
        path: `messages/${msgId}`,
        reaction: false,
        pageview: true,
        search: false,
        wordLimit: 200,
        pageSize: 5,
        emoji: ["https://unpkg.com/@waline/emojis@1.2.0/tieba"],
        imageUploader: false,
        copyright: false,
        dark: 'html[class="dark"]',
      });
    } else {
      console.error(`评论容器 #waline-${msgId} 未找到`);
    }
  }
};

const handleCancel = (msgId: number, payload?: { empty?: boolean }) => {
  if (payload && payload.empty === true) {
    toggleComment(msgId); // 与点击评论图标行为一致（当前显示则关闭）
    return;
  }
  if (activeCommentId.value === msgId) activeCommentId.value = null
  commentRefreshKey.value[msgId] = (commentRefreshKey.value[msgId] || 0) + 1
}

// 置顶权限：作者或管理员
  const canPin = (msg: any) => {
  if (!isLogin.value) return false;
  const user = userStore.user as any;
  if (!user) return false;
  const isAdmin = !!(user.IsAdmin || user.is_admin);
  const isAuthor = (user.ID || user.userid) === msg.user_id;
  return isAdmin || isAuthor;
  };

  const canEdit = (msg: any) => {
    if (!isLogin.value) return false;
    const user = userStore.user as any;
    if (!user) return false;
    const isAdmin = !!(user.IsAdmin || user.is_admin);
    const isAuthor = (user.ID || user.userid) === msg.user_id;
    return isAdmin || isAuthor;
  };

const pinnedTopItems = ref<any[]>([]);

  const togglePin = async (msg: any) => {
  try {
    const next = !msg.pinned;
    const res = await message.setPinned(msg.id, next);
    if (res) {
      if (next) {
        if (!pinnedTopItems.value.some((m: any) => m.id === msg.id)) {
          pinnedTopItems.value = [msg, ...pinnedTopItems.value];
        }
      } else {
        pinnedTopItems.value = pinnedTopItems.value.filter((m: any) => m.id !== msg.id);
      }
      useToast().add({ title: next ? '已置顶' : '已取消置顶', color: 'green', timeout: 1500 });
    }
  } catch (e) {
    useToast().add({ title: '操作失败', color: 'red', timeout: 2000 });
  }
  };

  const togglePrivate = async (msg: any) => {
    try {
      const next = !msg.private;
      const res = await message.setPrivate(msg.id, next);
      if (res) {
        useToast().add({ title: next ? '已设为私密' : '已设为公开', color: 'green', timeout: 1500 });
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
// 添加展开状态管理
const isExpanded = ref<{ [key: number]: boolean }>({});
const shouldShowExpandButton = ref<{ [key: number]: boolean }>({});
const hasGrid = ref<{ [key: number]: boolean }>({});

// 添加展开/折叠切换函数
const toggleExpand = (msgId: number) => {
  isExpanded.value[msgId] = !isExpanded.value[msgId];
};

// 修改检查内容高度的函数
// 修改检查内容高度的函数
const checkContentHeight = () => {
  nextTick(() => {
    // 获取当前显示的消息列表（可能是普通列表或搜索结果）
    const currentMessages = isSearchMode.value ? searchResults.value : message.messages;
    
    // 检查每条消息的内容高度
    currentMessages.forEach((msg) => {
      const contentEl = document.querySelector(
        `.content-container[data-msg-id="${msg.id}"] .overflow-y-hidden`
      );
      if (!contentEl) return;
      const el = contentEl as HTMLElement;
      const prevCV = (el.style as any).contentVisibility;
      const prevCIS = (el.style as any).containIntrinsicSize;
      try {
        if (prevCV) (el.style as any).contentVisibility = 'visible';
        if (prevCIS) (el.style as any).containIntrinsicSize = '';
      } catch {}
      const hasImageGrid = !!document.querySelector(`.content-container[data-msg-id="${msg.id}"] .image-grid`);
      hasGrid.value[msg.id] = hasImageGrid;
      if (hasImageGrid) {
        shouldShowExpandButton.value[msg.id] = false;
        isExpanded.value[msg.id] = true;
        return;
      }
      const fullHeight = (contentEl as HTMLElement).scrollHeight;
      if (fullHeight > 700) {
        shouldShowExpandButton.value[msg.id] = true;
        if (isExpanded.value[msg.id] === undefined) {
          isExpanded.value[msg.id] = false;
        }
      } else {
        shouldShowExpandButton.value[msg.id] = false;
      }
      try {
        if (prevCV) (el.style as any).contentVisibility = prevCV;
        if (prevCIS) (el.style as any).containIntrinsicSize = prevCIS;
      } catch {}

      const imgs = Array.from(el.querySelectorAll('img')) as HTMLImageElement[];
      imgs.forEach((img) => {
        const flag = (img as any).__measureAttached;
        if (!flag) {
          (img as any).__measureAttached = true;
          img.addEventListener('load', () => deferMeasure());
          img.addEventListener('error', () => deferMeasure());
        }
      });
    });
    deferInitFancybox();
  });
};

// 确保在内容变化时重新检查高度
watch(() => message.messages, () => {
  // 如果是单条消息查看模式，不执行滚动
  if (route.hash.includes('/messages/')) {
    return;
  }
  nextTick(() => {
    deferMeasure();
    deferInitFancybox();
  });
}, { deep: true });
// 添加路由相关
const route = useRoute();
const loadWalineAssets = async () => {
  if (useWaline.value && !window.Waline) {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = "https://unpkg.com/@waline/client@v2/dist/waline.css";
    document.head.appendChild(link);

    await new Promise((resolve, reject) => {
      const script = document.createElement("script");
      script.src = "https://unpkg.com/@waline/client@v2/dist/waline.js";
      script.crossOrigin = "anonymous";
      script.onload = resolve;
      script.onerror = reject;
      document.head.appendChild(script);
    });
  }
}
onMounted(async () => {
  try {
    isPageLoading.value = true
    await checkApi()
    await fetchGuestbookId()
    // 获取路由中的消息ID
    const messageId = route.hash.split('/messages/').pop();
    
    loadWalineAssets().catch(() => {})

    // 根据是否有消息ID来决定加载方式
    if (messageId) {
    const response = await fetch(`${BASE_API}/messages/${messageId}`, {
      credentials: 'include',
      headers: {
        'Accept': 'application/json'
      }
    });
    if (!response.ok) throw new Error('消息加载失败');
    const data = await response.json();
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
      if (!route.hash.includes('/messages/')) {
        const response = await fetch(`${BASE_API}/messages/page`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({ page: 1, pageSize: 15 })
        });
        if (response.ok) {
          const data = await response.json();
          if (data.code === 1 && data.data) {
            const items = (data.data.items || []).filter((m: any) => !isGuestbookMessage(m));
            message.messages = items;
            const totalRaw = data.data.total || 0;
            const adjustedTotal = totalRaw - (guestbookId.value ? 1 : 0);
            message.total = Math.max(0, adjustedTotal);
            const lastPage = Math.max(1, Math.ceil((message.total || 0) / 15));
            message.page = 1;
            message.hasMore = message.page < lastPage;
          }
        }
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
          if (isBuiltin.value && count > 0) expandedCommentsMap.value[m.id] = true;
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
    isPageLoading.value = false
  }
});

// 修改路由监听
watch(() => route.hash, async (newHash) => {
  const messageId = newHash.split('/messages/').pop();
  
  // 如果没有消息ID且不是从消息详情页返回，则保持当前状态，不重新加载
  if (!messageId && !route.hash.includes('/messages/')) {
    // 如果当前已有消息，不做任何操作，保持滚动位置
    if (message.messages && message.messages.length > 0) {
      return;
    }
    
    // 只有在首次加载且没有消息时才加载第一页
    const response = await fetch(`${BASE_API}/messages/page`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ page: 1, pageSize: 15 })
    });
    if (response.ok) {
      const data = await response.json();
      if (data.code === 1 && data.data) {
        const items = (data.data.items || []).filter((m: any) => !isGuestbookMessage(m));
        message.messages = items;
        const totalRaw = data.data.total || 0;
        const adjustedTotal = totalRaw - (guestbookId.value ? 1 : 0);
        message.total = Math.max(0, adjustedTotal);
        const lastPage = Math.max(1, Math.ceil((message.total || 0) / 15));
        message.page = 1;
        message.hasMore = message.page < lastPage;
        expandedCommentsMap.value = {};
        try {
          const tasks = (message.messages || []).filter((m: any) => !isGuestbookMessage(m)).map(async (m: any) => {
            try {
              const resp = await fetch(`${BASE_API}/messages/${m.id}/comments`, { credentials: 'include', headers: { 'Accept': 'application/json' } });
              if (resp.ok) {
                const js = await resp.json();
                const count = Array.isArray(js.data) ? js.data.length : 0;
                commentCountMap.value[m.id] = count;
                if (isBuiltin.value && count > 0) expandedCommentsMap.value[m.id] = true;
              }
            } catch {}
          });
          await Promise.allSettled(tasks);
        } catch {}
      }
    }
    return;
  }
  
  try {
    const response = await fetch(`${BASE_API}/messages/${messageId}`, {
      credentials: 'include',
      headers: {
        'Accept': 'application/json'
      }
    });
    if (!response.ok) throw new Error('消息加载失败');
    const data = await response.json();
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
const isPageLoading = ref(false);

watch(
  [() => props.activeTab, () => userStore.isLogin, () => currentUserId.value],
  async () => {
    if (route.hash.includes('/messages/')) return
    searchResults.value = []
    isSearchMode.value = false
    if (isPersonalGuest.value) {
      message.reset()
      return
    }
    isPageLoading.value = true
    try {
      await message.getMessages(pageQueryFor(1))
      expandedCommentsMap.value = {}
      await nextTick()
      deferMeasure()
      deferInitFancybox()
    } finally {
      isPageLoading.value = false
    }
  }
)

const loadPreviousPage = async () => {
  if (isPageLoading.value || message.page <= 1) return;
  isPageLoading.value = true;
  try {
    const sc = document.querySelector('.content-wrapper') as HTMLElement | null;
    const prevY = sc ? sc.scrollTop : window.scrollY;
    const targetPage = message.page - 1;
    const result = await message.getMessages(pageQueryFor(targetPage));
    if (result && Array.isArray(result.items)) {
      const nonPinned = result.items.filter((m: any) => !m.pinned && !isGuestbookMessage(m));
      message.messages = [...pinnedTopItems.value, ...nonPinned];
      const totalRaw = (result as any).total || message.total || 0;
      const adjustedTotal = totalRaw - (guestbookId.value ? 1 : 0);
      message.total = Math.max(0, adjustedTotal);
      message.page = (result as any).page || targetPage;
      const lastPage = Math.max(1, Math.ceil((message.total || 0) / 15));
      message.hasMore = message.page < lastPage;
    } else {
      message.page = targetPage;
    }
    await nextTick();
    if (sc) sc.scrollTo({ top: prevY, behavior: 'instant' }); else window.scrollTo({ top: prevY, behavior: 'instant' });
  } catch (error) {
    useToast().add({
      title: '加载失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    isPageLoading.value = false;
  }
};

const loadNextPage = async () => {
  if (isPageLoading.value || !message.hasMore) return;
  isPageLoading.value = true;
  try {
    const sc = document.querySelector('.content-wrapper') as HTMLElement | null;
    const prevY = sc ? sc.scrollTop : window.scrollY;
    const targetPage = message.page + 1;
    const result = await message.getMessages(pageQueryFor(targetPage));
    if (result && Array.isArray(result.items)) {
      const nonPinned = result.items.filter((m: any) => !m.pinned && !isGuestbookMessage(m));
      message.messages = [...pinnedTopItems.value, ...nonPinned];
      const totalRaw = (result as any).total || message.total || 0;
      const adjustedTotal = totalRaw - (guestbookId.value ? 1 : 0);
      message.total = Math.max(0, adjustedTotal);
      message.page = (result as any).page || targetPage;
      const lastPage = Math.max(1, Math.ceil((message.total || 0) / 15));
      message.hasMore = message.page < lastPage;
    } else {
      message.page = targetPage;
    }
    await nextTick();
    if (sc) sc.scrollTo({ top: prevY, behavior: 'instant' }); else window.scrollTo({ top: prevY, behavior: 'instant' });
  } catch (error) {
    useToast().add({
      title: '加载失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    isPageLoading.value = false;
  }
};
// 添加登录状态变化监听
watch(
  () => userStore.isLogin,
  (newVal) => {
    if (newVal && !isPersonalTab.value) {
      // 用户登录后的处理
      message.getMessages(pageQueryFor(1));
    }
  }
);

// 监听消息变化
watch(
  () => message.messages,
  async () => {
    try {
      if (message.page === 1) {
        const pins = (message.messages || []).filter((m: any) => m.pinned && !isGuestbookMessage(m));
        const unique = pins.filter((m: any, i: number, arr: any[]) => arr.findIndex((x: any) => x.id === m.id) === i);
        pinnedTopItems.value = unique;
      }
      await nextTick();
      observeContainers();
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
onBeforeUnmount(() => {
  if (window.Fancybox) {
    window.Fancybox.destroy();
  }
});
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
// 添加编辑功能
const showEditModal = ref(false);
const editingContent = ref('');
const editingMessageId = ref<number | null>(null);
const isSaving = ref(false);

const editMessage = (msg: any) => {
  editingMessageId.value = msg.id;
  
  // 保存原始内容，不包含附件图片
  editingContent.value = msg.content;
  
  // 如果存在附件图片，添加到编辑器中以便用户可以看到和编辑
  if (msg.image_url) {
    const imageMarkdown = `\n\n<!-- 附件图片(编辑时可删除) -->\n![附件图片](${BASE_API}${msg.image_url})\n<!-- 附件图片结束 -->`;
    editingContent.value += imageMarkdown;
  }
  
  showEditModal.value = true;
};

const saveEditedMessage = async () => {
  if (!editingMessageId.value) return;
  
  isSaving.value = true;
  try {
    // 获取当前编辑的消息
    const currentMsg = message.messages.find(msg => msg.id === editingMessageId.value);
    if (!currentMsg) return;

    // 处理编辑内容，移除附件图片的 Markdown 标记
    let processedContent = editingContent.value;
    
    // 移除附件图片的 Markdown 标记
    processedContent = processedContent.replace(/\n*<!-- 附件图片\(编辑时可删除\) -->\n!\[附件图片\]\(.*?\)\n<!-- 附件图片结束 -->\n*/g, '');
    
    // 检查内容是否有修改
    if (processedContent === currentMsg.content) {
      useToast().add({
        title: '内容未修改',
        description: '请修改内容后再保存',
        color: 'orange',
        timeout: 2000
      });
      isSaving.value = false;
      return;
    }
    // 直接使用编辑器中的内容，不做任何修改
    const response = await fetch(`${BASE_API}/messages/${editingMessageId.value}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      },
      credentials: 'include',
      body: JSON.stringify({
        content: processedContent,
        image_url: currentMsg.image_url
      })
    });

    if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

    const data = await response.json();
    if (data.code === 1) {
      const index = message.messages.findIndex(msg => msg.id === editingMessageId.value);
      if (index !== -1) {
        message.messages[index] = {
          ...message.messages[index],
          content: editingContent.value,  // 修正：使用 editingContent.value 替代 pureContent
          image_url: currentMsg.image_url  // 修正：使用 currentMsg.image_url 替代 imageUrl
        };
      }
      showEditModal.value = false;
      useToast().add({
        title: '更新成功',
        color: 'green',
        timeout: 2000
      });
    } else {
      throw new Error(data.msg || '保存失败');
    }
  } catch (error) {
    console.error('更新消息失败:', error);
    useToast().add({
      title: '更新失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    isSaving.value = false;
  }
};
// 添加搜索相关变量
const isSearchMode = ref(false);
const searchResults = ref([]);

// 添加搜索结果处理函数
const handleSearchResult = async (results: any) => {
  try {
    // 如果当前不是搜索模式，记录滚动位置
    const scrollPosition = !isSearchMode.value ? window.scrollY : null;
    
    console.debug('API返回的原始数据:', results);
    
    if (!results) {
      throw new Error('API返回数据为空');
    }
    
    let items = [];
    let total = 0;
    
    // 统一数据处理逻辑
    if (results.code === 1) {
      if (Array.isArray(results.data)) {
        items = results.data;
      } else if (results.data?.items) {
        items = results.data.items;
      }
    } else if (Array.isArray(results)) {
      items = results;
    }
    
    if (!Array.isArray(items)) {
      throw new Error('无效的数据格式');
    }
    
    // 排除留言板消息
    items = items.filter((m: any) => !isGuestbookMessage(m))
    total = items.length;
    
    // 更新搜索状态和结果
    isSearchMode.value = true;
    searchResults.value = items;
    
    // 显示结果提示
    if (total === 0) {
      useToast().add({
        title: '未找到相关内容',
        color: 'orange',
        timeout: 2000
      });
    } else {
      useToast().add({
        title: `找到 ${total} 条结果`,
        color: 'green',
        timeout: 2000
      });
    }
    
    // 如果是从非搜索模式切换来的，滚动到顶部
    if (scrollPosition !== null) {
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }
    
    await nextTick();
    checkContentHeight();
    deferInitFancybox();
    
  } catch (error: any) {
    console.error('处理搜索结果时出错:', error);
    useToast().add({
      title: '搜索失败',
      description: error.message || '处理搜索结果时发生错误',
      color: 'red',
      timeout: 2000
    });
    resetSearch();
  }
};
// 添加重置搜索函数
const resetSearch = () => {
  // 先清空结果数组
  searchResults.value = [];
  // 再关闭搜索模式
  isSearchMode.value = false;
  
  console.log('重置搜索 - searchResults:', searchResults.value);
  console.log('重置搜索 - isSearchMode:', isSearchMode.value);
  
  // 重置后更新UI
  nextTick(() => {
    checkContentHeight();
    deferInitFancybox();
  });
};

// 修改displayMessages计算属性以支持搜索模式和个人视图
const displayMessages = computed(() => {
  const filterPersonal = (items: any[]) => isPersonalTab.value ? items.filter(isCurrentUserMessage) : items
  if (isSearchMode.value && Array.isArray(searchResults.value)) {
    return filterPersonal(searchResults.value || []);
  }
  const base = (message.messages || []).filter((m: any) => !isGuestbookMessage(m));
  const pinned = (pinnedTopItems.value || []).filter((m: any) => !isGuestbookMessage(m));
  if (!pinned.length) return filterPersonal(base);
  const rest = base.filter((m: any) => !pinned.some((p: any) => p.id === m.id));
  return filterPersonal([...pinned, ...rest]);
});

// 添加事件监听
defineExpose({
  handleSearchResult
});

// 添加watch监听searchResults变化
watch(searchResults, (newVal) => {
  console.log('searchResults变化:', newVal);
  // 强制更新内容高度检查
  nextTick(() => {
    checkContentHeight();
    initFancybox();
  });
}, { deep: true, immediate: true });

// 添加watch监听isSearchMode变化
watch(isSearchMode, (newVal) => {
  console.log('isSearchMode变化:', newVal);
  // 强制更新内容高度检查
  nextTick(() => {
    checkContentHeight();
    initFancybox();
  });
});
const onCommentCountUpdated = (e: any) => {
  try {
    const d = e?.detail || {}
    const id = Number(d?.messageId || 0)
    const cnt = Number(d?.count || 0)
    if (id) commentCountMap.value[id] = cnt
  } catch {}
}
onMounted(() => { try { window.addEventListener('comment-count-updated', onCommentCountUpdated) } catch {} })
onBeforeUnmount(() => { try { window.removeEventListener('comment-count-updated', onCommentCountUpdated) } catch {} })
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
// 确保在模板中使用正确的配置数据
const footerConfig = computed(() => ({
  cardFooterTitle: props.siteConfig.cardFooterTitle,
  cardFooterSubtitle: props.siteConfig.cardFooterSubtitle,
  pageFooterHTML: props.siteConfig.pageFooterHTML,
  walineServerURL: props.siteConfig.walineServerURL
}));

// 下一页预取（靠近底部时触发）
const prefetchSentinel = ref<HTMLElement | null>(null)
let prefetchObservedPage = 0
onMounted(() => {
  try {
    const io2 = new IntersectionObserver((entries) => {
      entries.forEach(async (entry) => {
        if (!entry.isIntersecting) return
        if (isSearchMode.value) return
        const nextPage = (message.page || 1) + 1
        if (!message.hasMore) return
        if (prefetchObservedPage === nextPage) return
        prefetchObservedPage = nextPage
        const anyMsg = message as any
        if (anyMsg && typeof anyMsg.prefetchPage === 'function') {
          await anyMsg.prefetchPage(nextPage)
        }
      })
    }, { rootMargin: '512px 0px' })
    if (prefetchSentinel.value) io2.observe(prefetchSentinel.value)
  } catch {}
})

</script>

<style scoped>
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
  height: auto;
  border-radius: 12px;
  display: block;
  object-fit: contain;
  transition: transform .18s ease, box-shadow .18s ease, filter .18s ease;
  box-shadow: 0 1px 2px rgba(0,0,0,0.10);
}
.message-image-wrap {
  display: block;
  width: 100%;
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
  padding: 6px 10px; 
  border-radius: 12px; 
  background: var(--toolbox-bg) !important;
  color: var(--toolbox-fg) !important;
  opacity: 1 !important;
  backdrop-filter: none !important;
  -webkit-backdrop-filter: none !important;
}
.tool-icons { display: flex; align-items: center; gap: 8px; padding: 6px 8px; }
.tool-icon { 
  width: 28px; 
  height: 28px; 
  display:flex; 
  align-items:center; 
  justify-content:center; 
  cursor:pointer; 
  opacity:1; 
  font-size:18px; 
  line-height:1; 
  border-radius: 9999px; 
  position: relative; 
  transition: all 0.2s ease;
}

:global(html:not(.dark)) .tool-icon { 
  background: #ffffff; 
  color: #111827; 
  border: 1px solid rgba(0,0,0,0.12); 
  box-shadow: 0 1px 6px rgba(0,0,0,0.08); 
}
:global(html.dark) .tool-icon { 
  background: var(--home-surface-dark-elevated); 
  color: #ffffff; 
  border: 1px solid rgba(255,255,255,0.12); 
  box-shadow: 0 1px 6px rgba(255,255,255,0.06); 
}

.tool-icon:hover { 
  opacity: 1; 
  transform: translate3d(0,0,0) scale(1.06); 
  transition: transform .12s ease, box-shadow .12s ease; 
}

:global(html:not(.dark)) .tool-icon:hover { 
  box-shadow: 0 6px 18px rgba(0,0,0,0.20); 
}
:global(html.dark) .tool-icon:hover { 
  box-shadow: 0 8px 22px rgba(255,255,255,0.12); 
}

.tool-icon > * { color: currentColor; }
.tool-icon::after { content: attr(data-label); position: absolute; left: 50%; top: calc(100% + 6px); transform: translateX(-50%); white-space: nowrap; font-size: 12px; padding: 2px 8px; border-radius: 9999px; opacity: 0; transition: opacity .12s ease; pointer-events: none; }
:global(html:not(.dark)) .tool-icon::after { background: #ffffff; color: #111827; border: 1px solid rgba(0,0,0,0.12); box-shadow: 0 2px 8px rgba(0,0,0,0.08); }
:global(html.dark) .tool-icon::after { background: var(--home-surface-dark-elevated); color: #ffffff; border: 1px solid rgba(255,255,255,0.16); box-shadow: 0 2px 8px rgba(255,255,255,0.06); }
.tool-icon:hover::after { opacity: 1; }
.toolbox-dark { background: var(--home-surface-dark-elevated); border: 1px solid rgba(255,255,255,0.16); }
.toolbox-light { background: #fff; border: 1px solid rgba(0,0,0,0.08); }

/* 工具栏主题色（变量在全局定义，避免 scoped 优先级问题） */
:global(html) {
  --toolbox-bg: #ffffff;
  --toolbox-fg: #111827;
  --toolbox-border: rgba(100,116,139,0.40);
  --toolbox-shadow: 0 8px 22px rgba(0,0,0,0.15);
}
:global(html.dark),
:global(body.dark),
:global(.dark) {
  --toolbox-bg: var(--home-surface-dark-elevated);
  --toolbox-fg: #ffffff;
  --toolbox-border: rgba(148,163,184,0.50);
  --toolbox-shadow: 0 8px 22px rgba(255,255,255,0.12);
}

.message-toolbox.overlay {
  border: 1px solid var(--toolbox-border) !important;
  box-shadow: var(--toolbox-shadow) !important;
}

.message-toolbox.overlay .tool-icons {
  background: var(--toolbox-bg) !important;
  color: var(--toolbox-fg) !important;
}

.message-toolbox.overlay .tool-icon {
  color: inherit;
}

/* 参考图的边缘描边效果（双层细描边） */
.message-toolbox.overlay::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  pointer-events: none;
}
:global(html.dark) .message-toolbox.overlay::before {
  box-shadow: inset 0 0 0 1px rgba(255,255,255,0.15) !important;
}
:global(html:not(.dark)) .message-toolbox.overlay::before {
  box-shadow: inset 0 0 0 1px rgba(0,0,0,0.15) !important;
}

.message-toolbox.overlay::after {
  content: '';
  position: absolute;
  left: -28px;
  top: 50%;
  transform: translateY(-50%);
  width: 24px;
  height: 1px;
  pointer-events: none;
  border-radius: 1px;
}
:global(html.dark) .message-toolbox.overlay::after { background-color: rgba(148,163,184,0.50) !important; }
:global(html:not(.dark)) .message-toolbox.overlay::after { background-color: rgba(100,116,139,0.40) !important; }

.message-toolbox.overlay::after {
  content: '';
  position: absolute;
  left: -28px;
  top: 50%;
  transform: translateY(-50%);
  width: 24px;
  height: 1px;
  pointer-events: none;
  border-radius: 1px;
}
:global(html.dark) .message-toolbox.overlay::after { background-color: rgba(148,163,184,0.50) !important; }
:global(html:not(.dark)) .message-toolbox.overlay::after { background-color: rgba(100,116,139,0.40) !important; }
.author-row { line-height: 1.1; position: relative; }
.message-socialbar { display:flex; align-items:center; gap:12px; padding:0; margin-top:6px; }
.social-item { display:flex; align-items:center; gap:6px; opacity:.85; cursor:pointer; }
.social-item:hover { opacity:1; }
@media (max-width: 640px) {
  .tool-icons { gap:10px; padding:6px 8px; }
  .tool-icon { width:22px; height:22px; font-size:18px; }
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

/* 添加展开/折叠按钮容器样式 */
.expand-toggle-btn {
  border: none;
  background: transparent;
  color: inherit;
  font-weight: 600;
  font-size: 14px;
  padding: 4px 8px;
  transition: all 0.2s ease;
  display: flex;
  align-items: center;
  justify-content: center;
  white-space: nowrap;
  cursor: pointer;
}

/* 按钮容器样式 - 用于提供背景和轮廓 */
.expand-toggle-btn:hover {
  transform: scale(1.02);
}

/* 按钮容器样式 - 用于提供背景和轮廓 */
.expand-toggle-btn:hover {
  transform: scale(1.02);
}

/* 暗黑模式按钮容器样式 */
:global(html.dark) .expand-toggle-btn {
  color: #fff;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
}
:global(html.dark) .expand-toggle-btn:hover {
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.5);
}

/* 白天模式按钮容器样式 */
:global(html:not(.dark)) .expand-toggle-btn {
  color: #111827;
  text-shadow: 0 1px 2px rgba(255, 255, 255, 0.5);
}
:global(html:not(.dark)) .expand-toggle-btn:hover {
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
}

/* 暗黑模式按钮容器（父元素）样式 */
:global(html.dark) .expand-button-container {
  background: rgba(39, 50, 66, 0.92) !important;
  border: 1px solid rgba(255, 255, 255, 0.2) !important;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2) !important;
  backdrop-filter: blur(4px) !important;
}
:global(html.dark) .expand-button-container:hover {
  background: rgba(47, 59, 76, 0.96) !important;
  border-color: rgba(255, 255, 255, 0.24) !important;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3) !important;
}

/* 白天模式按钮容器（父元素）样式 */
:global(html:not(.dark)) .expand-button-container {
  background: rgba(255, 255, 255, 0.9) !important;
  border: 1px solid rgba(251, 146, 60, 0.5) !important;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1) !important;
  backdrop-filter: blur(4px) !important;
}
:global(html:not(.dark)) .expand-button-container:hover {
  background: rgba(255, 255, 255, 0.95) !important;
  border-color: rgba(251, 146, 60, 0.7) !important;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15) !important;
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
:global(html:not(.dark)) .content-container :deep(.markdown-preview *:not(pre):not(code)) {
  color: #111 !important;
  opacity: 1 !important;
}
/* 彻底取消白天模式灰度，所有元素不透明 */
:global(html:not(.dark)) .content-container :deep(.markdown-preview *) { opacity: 1 !important; }
:global(html:not(.dark)) .content-container :deep(.markdown-preview p),
:global(html:not(.dark)) .content-container :deep(.markdown-preview li),
:global(html:not(.dark)) .content-container :deep(.markdown-preview span),
:global(html:not(.dark)) .content-container :deep(.markdown-preview em),
:global(html:not(.dark)) .content-container :deep(.markdown-preview strong),
:global(html:not(.dark)) .content-container :deep(.markdown-preview blockquote),
:global(html:not(.dark)) .content-container :deep(.markdown-preview code) { opacity: 1 !important; }

/* 确保所有模式下链接颜色都是蓝色 */
:global(html:not(.dark)) .content-container :deep(.markdown-preview a),
:global(html.dark) .content-container :deep(.markdown-preview a),
.content-container :deep(.markdown-preview a) { 
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
:global(html:not(.dark)) .content-container :deep(.markdown-preview a:hover),
:global(html.dark) .content-container :deep(.markdown-preview a:hover),
.content-container :deep(.markdown-preview a:hover) { 
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
  border: 1px solid #e5e7eb !important;
  border-radius: 8px !important;
}

:global(html.dark) .content-container :deep(video) {
  background-color: var(--home-surface-dark) !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}

:global(html.dark) .content-container :deep(audio) {
  background-color: var(--home-surface-dark) !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
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
.noise-author-card { position: absolute; top: -28px; left: 36px; z-index: 2147483647; border-radius: 12px; padding: 10px 12px; min-width: 300px; box-shadow: 0 8px 24px rgba(0,0,0,0.25); border: 1px solid rgba(0,0,0,0.08); transform: translate3d(0,0,0); isolation: isolate; backdrop-filter: none; -webkit-backdrop-filter: none; overflow: visible; }
.noise-author-card::after { content: ''; position: absolute; left: -10px; top: 27px; width: 0; height: 0; border-top: 10px solid transparent; border-bottom: 10px solid transparent; z-index: 1; filter: drop-shadow(0 2px 4px rgba(0,0,0,0.25)); }
:global(html.dark) .noise-author-card { --home-surface-dark-elevated: rgb(15, 24, 39); background: var(--home-surface-dark-elevated) !important; border-color: rgba(255,255,255,0.14); box-shadow: 0 10px 24px rgba(0,0,0,0.38); }
:global(html.dark) .noise-author-card::after { border-right: 8px solid var(--home-surface-dark-elevated); }
:global(html:not(.dark)) .noise-author-card::after { border-right: 8px solid #ffffff; }
.noise-author-card-header { display: flex; gap: 10px; align-items: center; margin-bottom: 8px; pointer-events: auto; }
.noise-author-card-body { display: flex; gap: 10px; align-items: center; justify-content: flex-end; }
.noise-author-card-sign { overflow: hidden; font-size: 12px; line-height: 16px; white-space: nowrap; flex: 1; text-align: center; }
.noise-author-card-scroll { display: inline-block; white-space: nowrap; will-change: transform; animation: author-sign-scroll 12s linear infinite; }
.noise-author-card-scroll.center { animation: none; }
.author-card-muted { color: #7a7f85 }
@keyframes author-sign-scroll { 0% { transform: translateX(100%); } 100% { transform: translateX(-100%); } }
.author-card-muted { color: #7a7f85 }
@media (max-width: 640px) { .noise-author-card { position: fixed; left: 12px; right: 12px; top: auto; bottom: auto; min-width: auto; z-index: 2147483647; } .noise-author-card::after { display: none; } }
.pager-shell {
  padding: 10px 14px;
  border-radius: 999px;
  backdrop-filter: blur(8px);
}
.pager-icon-wrap {
  width: 1.45rem;
  height: 1.45rem;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.pager-icon {
  line-height: 1;
}
.pager-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
}
.pager-jump-btn {
  border-radius: 999px;
  padding: 0.35rem 0.8rem;
}
:global(html.dark) .pager-shell {
  background: rgba(18, 24, 34, 0.56) !important;
  border: 1px solid rgba(255, 255, 255, 0.12) !important;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.3) !important;
}
:global(html.dark) .pager-btn {
  background: rgba(39, 50, 66, 0.58) !important;
  color: #f8fafc !important;
  border: 1px solid rgba(255, 255, 255, 0.22) !important;
}
:global(html.dark) .pager-btn:hover {
  background: rgba(50, 62, 82, 0.72) !important;
}
:global(html.dark) .pager-icon-wrap {
  background: rgba(255, 255, 255, 0.14) !important;
}
:global(html.dark) .pager-jump-btn {
  background: rgba(39, 50, 66, 0.45) !important;
  border: 1px solid rgba(255, 255, 255, 0.18) !important;
}
:global(html:not(.dark)) .pager-shell {
  background: rgba(255, 255, 255, 0.52) !important;
  border: 1px solid rgba(15, 23, 42, 0.1) !important;
  box-shadow: 0 6px 18px rgba(15, 23, 42, 0.12) !important;
}
:global(html:not(.dark)) .pager-btn {
  background: rgba(255, 255, 255, 0.64) !important;
  color: #0f172a !important;
  border: 1px solid rgba(15, 23, 42, 0.16) !important;
}
:global(html:not(.dark)) .pager-btn:hover {
  background: rgba(255, 255, 255, 0.8) !important;
}
:global(html:not(.dark)) .pager-icon-wrap {
  background: rgba(15, 23, 42, 0.1) !important;
}
:global(html:not(.dark)) .pager-jump-btn {
  background: rgba(255, 255, 255, 0.58) !important;
  border: 1px solid rgba(15, 23, 42, 0.16) !important;
}
</style>
