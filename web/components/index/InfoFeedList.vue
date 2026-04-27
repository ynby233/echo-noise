<template>
  <div :class="['feed-list-wrap', wrapThemeClass]">
    <div v-if="loading" class="feed-empty feed-loading-text">信息流加载中...</div>
    <div v-else-if="errorText" class="feed-empty">{{ errorText }}</div>
    <div v-else-if="allItems.length === 0" class="feed-empty">暂无信息流内容</div>
    <div v-else :class="['feed-grid', gridClass]">
      <article
        v-for="item in pageItems"
        :key="`${item.link}-${item.timestamp}`"
        :class="['feed-card', listThemeClass, contentTheme === 'dark' ? 'feed-card-dark' : 'feed-card-light']"
      >
        <div class="feed-card-head author-row">
          <div class="feed-source-user">
            <img
              v-if="getAvatarUrl(item)"
              :src="getAvatarUrl(item)"
              :alt="getDisplayName(item)"
              class="feed-avatar"
              loading="lazy"
              decoding="async"
              @error="markAvatarBroken(item)"
            />
            <UIcon v-else :name="getHeadIcon(item)" class="w-4 h-4 opacity-70" />
            <span>{{ getDisplayName(item) }}</span>
          </div>
          <div class="feed-time">{{ formatDate(item) }}</div>
        </div>
        <h3 v-if="shouldShowTitle(item)" class="feed-title">{{ item.title }}</h3>
        <div v-if="getDisplayRaw(item)" class="feed-summary-block content-container" :data-feed-id="getFeedItemId(item)">
          <div
            :ref="(el) => setFeedSummaryRef(getFeedItemId(item), el)"
            class="feed-summary-body overflow-y-hidden relative"
            :style="shouldShowExpandButton[getFeedItemId(item)] && !isExpanded[getFeedItemId(item)] ? { maxHeight: `${collapsedContentHeight}px` } : undefined"
          >
            <div class="feed-summary-markdown">
              <MarkdownRenderer
                :content="getDisplayRaw(item)"
                :enable-github-card="enableGithubCard"
                :theme-mode="contentTheme"
                @rendered="checkContentHeights"
              />
            </div>
            <div v-if="shouldShowStandaloneImage(item)" class="feed-image-wrap feed-image-wrap-inline">
              <button
                type="button"
                class="feed-image-btn"
                :aria-label="`查看大图：${item.title || '图片'}`"
                @click="openImagePreview(item.imageURL)"
              >
                <img :src="item.imageURL" :alt="item.title || 'image'" class="feed-image" loading="lazy" />
              </button>
            </div>
            <div
              v-if="shouldShowExpandButton[getFeedItemId(item)] && !isExpanded[getFeedItemId(item)]"
              :class="['absolute bottom-0 left-0 right-0 h-14 bg-gradient-to-t backdrop-blur-sm pointer-events-none content-fade-mask', gradientClass]"
              style="z-index:20"
            />
          </div>
          <div
            v-if="shouldShowExpandButton[getFeedItemId(item)]"
            :class="['relative left-0 right-0 flex justify-center z-30', isExpanded[getFeedItemId(item)] ? 'mb-1' : '-mt-2 mb-1']"
          >
            <div class="expand-button-container px-4 py-1.5 rounded-full backdrop-blur-sm">
              <button
                type="button"
                class="expand-toggle-btn text-sm inline-flex items-center justify-center gap-1"
                :aria-label="isExpanded[getFeedItemId(item)] ? '收起全文' : '展开全文'"
                @click="toggleExpand(getFeedItemId(item))"
              >
                {{ isExpanded[getFeedItemId(item)] ? '收起全文' : '展开全文' }}
                <UIcon :name="isExpanded[getFeedItemId(item)] ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" class="w-4 h-4 flex-shrink-0" />
              </button>
            </div>
          </div>
        </div>
        <div v-if="shouldShowStandaloneImage(item) && !getDisplayRaw(item)" class="feed-image-wrap">
          <button
            type="button"
            class="feed-image-btn"
            :aria-label="`查看大图：${item.title || '图片'}`"
            @click="openImagePreview(item.imageURL)"
          >
            <img :src="item.imageURL" :alt="item.title || 'image'" class="feed-image" loading="lazy" />
          </button>
        </div>
        <div class="feed-footer">
          <div class="feed-domain" :title="item.link || ''">
            <UIcon name="i-heroicons-link" class="w-4 h-4 opacity-70" />
            <span>{{ item.link ? getLinkHost(item.link) : '-' }}</span>
          </div>
          <div class="feed-actions">
            <UTooltip text="阅读原文" :popper="{ placement: 'top' }">
              <a
                v-if="item.link"
                class="feed-icon-btn"
                :href="item.link"
                target="_blank"
                rel="noopener noreferrer"
                aria-label="阅读原文"
              >
                <UIcon name="i-heroicons-arrow-top-right-on-square" class="w-4 h-4" />
              </a>
            </UTooltip>
            <UTooltip :text="copiedLink === item.link ? '已复制' : '复制链接'" :popper="{ placement: 'top' }">
              <button
                v-if="item.link"
                type="button"
                :class="['feed-icon-btn', copiedLink === item.link ? 'is-success' : '']"
                :aria-label="copiedLink === item.link ? '已复制链接' : '复制链接'"
                @click="copyLink(item.link)"
              >
                <UIcon :name="copiedLink === item.link ? 'i-heroicons-check' : 'i-heroicons-clipboard-document'" class="w-4 h-4" />
              </button>
            </UTooltip>
          </div>
        </div>
      </article>
    </div>
    <div v-if="!loading && !errorText && allItems.length > 0" class="pager-shell">
      <div class="pager-main">
        <UButton
          v-if="currentPage > 1"
          color="gray"
          variant="solid"
          size="xs"
          class="rounded-full px-4 py-1.5 shadow-lg hover:shadow-xl transition-all duration-300 pager-btn"
          @click="goPrevPage"
        >
          <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-left" class="w-4 h-4 pager-icon" /></span>
          上一页
        </UButton>
        <UButton
          v-if="currentPage < totalPages"
          color="gray"
          variant="solid"
          size="xs"
          class="rounded-full px-4 py-1.5 shadow-lg hover:shadow-xl transition-all duration-300 pager-btn"
          @click="goNextPage"
        >
          下一页
          <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-right" class="w-4 h-4 pager-icon" /></span>
        </UButton>
      </div>
      <div class="pager-meta">第 {{ currentPage }} / {{ totalPages }} 页</div>
    </div>
    <UModal v-model="previewOpen">
      <div class="feed-preview-modal">
        <img
          v-if="previewImageURL"
          :src="previewImageURL"
          alt="预览图片"
          class="feed-preview-image"
        />
      </div>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
// @ts-ignore Vetur 对 .vue 默认导出识别不稳定，这里与项目内其他组件保持一致
import MarkdownRenderer from "~/components/index/MarkdownRenderer.vue";

const FEED_CACHE_PREFIX = 'ech0-noise:feed-cache:v1'
const feedMemoryCache = new Map<string, { ts: number; items: FeedItem[] }>()

type FeedItem = {
  title: string
  link: string
  content: string
  summary: string
  imageURL: string
  source: string
  author?: string
  avatarURL?: string
  type?: string
  publishedAt: string
  timestamp: number
}

const props = defineProps<{
  layoutState: 'three' | 'two' | 'single'
  limit?: number
  refreshSeconds?: number
  active?: boolean
  baseApi?: string
  enableGithubCard?: boolean
}>()
const emit = defineEmits<{
  (e: 'count-change', count: number): void
}>()

const loading = ref(false)
const allItems = ref<FeedItem[]>([])
const errorText = ref('')
const requestInFlight = ref(false)
const copiedLink = ref('')
const copiedTimer = ref<number | null>(null)
const currentPage = ref(1)
const previewOpen = ref(false)
const previewImageURL = ref('')
const brokenAvatarSet = ref<Set<string>>(new Set())
const collapsedContentHeight = 820
const isExpanded = ref<Record<string, boolean>>({})
const hasUserToggled = ref<Record<string, boolean>>({})
const shouldShowExpandButton = ref<Record<string, boolean>>({})
const measureTimer = ref<number | null>(null)
const feedSummaryRefs = ref<Record<string, HTMLElement | null>>({})
const feedResizeObservers = new Map<string, ResizeObserver>()
const cacheKey = computed(() => {
  const apiBase = String(props.baseApi || '/api').replace(/\/$/, '')
  return `${FEED_CACHE_PREFIX}:${apiBase}:${maxItems.value ?? 'all'}`
})

const gridClass = computed(() => {
  if (props.layoutState === 'single') return 'feed-grid-single'
  if (props.layoutState === 'two') return 'feed-grid-two'
  return 'feed-grid-three'
})

// 与 MessageList 同源的内容主题，保证卡片背景一致
const contentTheme = inject('contentTheme', ref<string>(typeof window !== 'undefined' ? (localStorage.getItem('contentTheme') || 'dark') : 'dark'))
const listThemeClass = computed(() => contentTheme.value === 'dark' ? 'bg-[var(--home-surface-dark)] text-white' : 'bg-white text-black')
const wrapThemeClass = computed(() => contentTheme.value === 'dark' ? 'feed-wrap-dark' : 'feed-wrap-light')
const enableGithubCard = computed(() => props.enableGithubCard === true)
const gradientClass = computed(() => contentTheme.value === 'dark'
  ? 'from-[var(--home-surface-dark)] via-[rgba(32,42,54,0.82)] to-transparent'
  : 'from-[rgba(255,255,255,1)] via-[rgba(255,255,255,0.8)] to-transparent')

const maxItems = computed<number | null>(() => {
  const value = Number(props.limit)
  if (!Number.isFinite(value) || value <= 0) return null
  return Math.max(1, Math.min(100, Math.floor(value)))
})
const pageSize = computed(() => {
  if (props.layoutState === 'single') return 8
  if (props.layoutState === 'two') return 10
  return 12
})
const totalPages = computed(() => {
  const total = Math.ceil(allItems.value.length / pageSize.value)
  return total > 0 ? total : 1
})
const pageItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return allItems.value.slice(start, start + pageSize.value)
})

const feedKeyOf = (item: FeedItem) => `${String(item.link || '').trim()}|${String(item.title || '').trim()}|${String(item.publishedAt || '').trim()}|${Number(item.timestamp || 0)}`

const getFeedItemId = (item: FeedItem) => {
  const key = feedKeyOf(item)
  let hash = 0
  for (let i = 0; i < key.length; i += 1) {
    hash = ((hash << 5) - hash) + key.charCodeAt(i)
    hash |= 0
  }
  return `feed-${Math.abs(hash)}`
}

const getItemTimestamp = (item: FeedItem) => {
  const ts = Number(item?.timestamp || 0)
  if (Number.isFinite(ts) && ts > 0) return ts
  const publishedAt = String(item?.publishedAt || '').trim()
  if (!publishedAt) return 0
  const parsed = new Date(publishedAt)
  if (Number.isNaN(parsed.getTime())) return 0
  return Math.floor(parsed.getTime() / 1000)
}

const sortFeedItems = (rows: FeedItem[]) => {
  const list = Array.isArray(rows) ? [...rows] : []
  list.sort((a, b) => {
    const bTime = getItemTimestamp(b)
    const aTime = getItemTimestamp(a)
    if (bTime !== aTime) return bTime - aTime
    return String(b.title || '').localeCompare(String(a.title || ''), 'zh-Hans-CN')
  })
  return list
}

const clampPage = () => {
  if (currentPage.value < 1) currentPage.value = 1
  if (currentPage.value > totalPages.value) currentPage.value = totalPages.value
}

const goPrevPage = () => {
  if (currentPage.value <= 1) return
  currentPage.value -= 1
  deferMeasure()
}

const goNextPage = () => {
  if (currentPage.value >= totalPages.value) return
  currentPage.value += 1
  deferMeasure()
}

const toggleExpand = (feedId: string) => {
  hasUserToggled.value[feedId] = true
  isExpanded.value[feedId] = !isExpanded.value[feedId]
}

const cleanupFeedSummaryObserver = (feedId: string) => {
  const observer = feedResizeObservers.get(feedId)
  if (!observer) return
  observer.disconnect()
  feedResizeObservers.delete(feedId)
}

const setFeedSummaryRef = (feedId: string, el: any) => {
  const currentEl = feedSummaryRefs.value[feedId]
  const nextEl = el instanceof HTMLElement ? el : null
  if (currentEl === nextEl) return
  cleanupFeedSummaryObserver(feedId)
  if (!nextEl) {
    delete feedSummaryRefs.value[feedId]
    return
  }
  feedSummaryRefs.value[feedId] = nextEl
  if (typeof window !== 'undefined' && typeof window.ResizeObserver !== 'undefined') {
    const observer = new window.ResizeObserver(() => {
      deferMeasure()
    })
    observer.observe(nextEl)
    feedResizeObservers.set(feedId, observer)
  }
}

const deferMeasure = () => {
  if (typeof window === 'undefined') return
  if (measureTimer.value) {
    window.clearTimeout(measureTimer.value)
  }
  measureTimer.value = window.setTimeout(() => {
    checkContentHeights()
    measureTimer.value = null
  }, 60)
}

const checkContentHeights = () => {
  nextTick(() => {
    pageItems.value.forEach((item) => {
      const feedId = getFeedItemId(item)
      const contentEl = feedSummaryRefs.value[feedId]
      if (!contentEl) return
      const fullHeight = contentEl.scrollHeight
      if (fullHeight > collapsedContentHeight) {
        shouldShowExpandButton.value[feedId] = true
        if (!hasUserToggled.value[feedId]) {
          isExpanded.value[feedId] = false
        }
      } else {
        shouldShowExpandButton.value[feedId] = false
        isExpanded.value[feedId] = true
        hasUserToggled.value[feedId] = false
      }
    })
  })
}

const applyFeedItems = (items: FeedItem[]) => {
  const sortedItems = sortFeedItems(items)
  allItems.value = typeof maxItems.value === 'number' ? sortedItems.slice(0, maxItems.value) : sortedItems
  isExpanded.value = {}
  hasUserToggled.value = {}
  shouldShowExpandButton.value = {}
  clampPage()
  emit('count-change', allItems.value.length)
  deferMeasure()
}

const readCachedFeed = () => {
  const key = cacheKey.value
  const memoryCached = feedMemoryCache.get(key)
  if (memoryCached && Array.isArray(memoryCached.items) && memoryCached.items.length > 0) {
    return memoryCached
  }
  if (typeof window === 'undefined') return null
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (parsed && Array.isArray(parsed.items) && parsed.items.length > 0) {
      const payload = {
        ts: Number(parsed.ts || 0),
        items: parsed.items as FeedItem[]
      }
      feedMemoryCache.set(key, payload)
      return payload
    }
  } catch {}
  return null
}

const hydrateFeedCache = () => {
  const cached = readCachedFeed()
  if (!cached) return false
  applyFeedItems(cached.items)
  return true
}

const persistFeedCache = (items: FeedItem[]) => {
  const sortedItems = sortFeedItems(items)
  const payload = {
    ts: Date.now(),
    items: typeof maxItems.value === 'number' ? sortedItems.slice(0, maxItems.value) : sortedItems
  }
  const key = cacheKey.value
  feedMemoryCache.set(key, payload)
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(key, JSON.stringify(payload))
  } catch {}
}

const loadFeed = async () => {
  if (requestInFlight.value) return
  requestInFlight.value = true
  const hasVisibleItems = allItems.value.length > 0
  loading.value = !hasVisibleItems
  errorText.value = ''
  try {
    const limit = maxItems.value
    const apiBase = String(props.baseApi || '/api').replace(/\/$/, '')
    const query = typeof limit === 'number' ? `?limit=${limit}` : ''
    const resp = await fetch(`${apiBase}/feed/items${query}`, {
      credentials: 'include',
      headers: { Accept: 'application/json' }
    })
    const data = await resp.json()
    if (data?.code !== 1) {
      throw new Error(data?.msg || '加载失败')
    }
    const list = Array.isArray(data?.data?.items)
      ? data.data.items
      : (Array.isArray(data?.data) ? data.data : [])
    applyFeedItems(list)
    persistFeedCache(list)
  } catch (err: any) {
    errorText.value = err?.message || '信息流加载失败'
    if (!allItems.value.length) {
      allItems.value = []
      currentPage.value = 1
      emit('count-change', 0)
    }
  } finally {
    loading.value = false
    requestInFlight.value = false
  }
}

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

const parseFeedDate = (item: FeedItem) => {
  const ts = Number(item?.timestamp || 0)
  if (Number.isFinite(ts) && ts > 0) {
    return new Date(ts * 1000)
  }
  const text = String(item?.publishedAt || '').trim()
  if (!text) return null
  const hasTimezone = /(?:Z|[+\-]\d{2}:?\d{2})$/i.test(text)
  const normalized = hasTimezone ? text : `${text.replace(' ', 'T')}Z`
  const d = new Date(normalized)
  if (Number.isNaN(d.getTime())) return null
  return d
}

const formatDate = (item: FeedItem) => {
  const d = parseFeedDate(item)
  if (!d) return String(item?.publishedAt || '').trim() || '-'
  const parts = shanghaiDateTimeFormatter.formatToParts(d)
  const pick = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value || ''
  return `${pick('year')}/${pick('month')}/${pick('day')} ${pick('hour')}:${pick('minute')}:${pick('second')}`
}

const getLinkHost = (url: string) => {
  const raw = String(url || '').trim()
  if (!raw) return '打开原文'
  try {
    return new URL(raw).host
  } catch {
    return raw
  }
}

const isBuiltinSourceName = (name: string) => {
  const normalized = String(name || '').trim().toLowerCase()
  return ['ech0', '同部署项目', '说说笔记', '本项目api', '本项目 api', 'memos', 'mastodon', '信息流'].includes(normalized)
}

const getDisplayName = (item: FeedItem) => {
  if (isRSSItem(item)) return String(item.source || '信息流').trim() || '信息流'
  const author = String(item.author || '').trim()
  if (author) return author
  const source = String(item.source || '').trim()
  if (source && !isBuiltinSourceName(source)) return source
  return '匿名'
}

const getHeadIcon = (item: FeedItem) => isRSSItem(item) ? 'i-heroicons-rss' : 'i-heroicons-user-circle'

const getAvatarUrl = (item: FeedItem) => {
  if (isRSSItem(item)) return ''
  const raw = String(item.avatarURL || '').trim()
  if (!raw) return ''
  if (brokenAvatarSet.value.has(raw)) return ''
  return raw
}

const markAvatarBroken = (item: FeedItem) => {
  const raw = String(item.avatarURL || '').trim()
  if (!raw) return
  brokenAvatarSet.value.add(raw)
}

const normalizeContent = (value: string) => {
  const raw = String(value || '').trim()
  if (!raw) return ''
  let text = raw
  if (/<[a-z][\s\S]*>/i.test(raw)) {
    if (typeof window !== 'undefined' && typeof window.DOMParser !== 'undefined') {
      const doc = new window.DOMParser().parseFromString(raw, 'text/html')
      text = String(doc.body?.textContent || '')
    } else {
      text = raw.replace(/<[^>]*>/g, ' ')
    }
  }
  return text
    .replace(/\u00A0/g, ' ')
    .replace(/\r\n/g, '\n')
    .replace(/[ \t]+\n/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

const extractComparableText = (value: string) => {
  const raw = String(value || '').trim()
  if (!raw) return ''
  if (!/<[a-z][\s\S]*>/i.test(raw)) return normalizeContent(raw)
  return cleanComparableMarkup(raw)
}

const cleanComparableMarkup = (value: string) => {
  return String(value || '')
    .replace(/<img[\s\S]*?>/gi, ' ')
    .replace(/<video[\s\S]*?>[\s\S]*?<\/video>/gi, ' ')
    .replace(/<source[\s\S]*?>/gi, ' ')
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<\/(p|div|li|blockquote|h[1-6])>/gi, '\n')
    .replace(/<[^>]*>/g, ' ')
    .replace(/\u00A0/g, ' ')
    .replace(/[ \t]+\n/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .replace(/[ \t]{2,}/g, ' ')
    .trim()
}

const toComparable = (value: string) => extractComparableText(value)
  .replace(/[#*_`~>|[\](){}]+/g, '')
  .replace(/[：:，,。.!！?？\-\s]+/g, '')
  .toLowerCase()

const isEch0Item = (item: FeedItem) => {
  return String(item.type || '').toLowerCase() === 'ech0'
}

const getDisplayRaw = (item: FeedItem) => {
  const isRSS = isRSSItem(item)
  const title = normalizeContent(item.title || '')
  const summaryRaw = String(item.summary || '').trim()
  const contentRaw = String(item.content || '').trim()
  // 优先使用后端保留的原始内容，确保 Markdown/媒体卡片可被正确渲染。
  let text = contentRaw || summaryRaw
  if (!text) return ''
  // RSS 与 Ech0 正文都可能把第一段作为真实内容的一部分，不能按标题首行去重裁剪。
  if (isRSS || isEch0Item(item)) return text
  if (!title) return text
  const titleComparable = toComparable(title)
  const textComparable = toComparable(text)
  if (!titleComparable) return text
  // 避免把单行正文直接清空导致“只有标题”。
  if (textComparable === titleComparable) return text
  const lines = text
    .replace(/\r\n/g, '\n')
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
  if (lines.length > 1 && toComparable(lines[0]) === titleComparable) {
    lines.shift()
    text = lines.join('\n').trim()
  }
  return text
}

const isRSSItem = (item: FeedItem) => String(item.type || '').toLowerCase() === 'rss'

const hasInlineMediaInContent = (item: FeedItem) => {
  const raw = getDisplayRaw(item)
  if (!raw) return false
  return /!\[[^\]]*]\(([^)]+)\)/i.test(raw) || /<img[\s\S]*?>/i.test(raw)
}

const shouldShowStandaloneImage = (item: FeedItem) => {
  const hasImage = String(item.imageURL || '').trim().length > 0
  if (!hasImage) return false
  if (isRSSItem(item)) return true
  return !hasInlineMediaInContent(item)
}

const shouldShowTitle = (item: FeedItem) => {
  const title = normalizeContent(item.title || '')
  if (!title) return false
  // RSS 一直展示标题；其他源在正文为空时展示标题，避免空白卡片。
  if (isRSSItem(item)) return true
  return !getDisplayRaw(item)
}

const openImagePreview = (url: string) => {
  const raw = String(url || '').trim()
  if (!raw) return
  previewImageURL.value = raw
  previewOpen.value = true
}

const copyLink = async (url: string) => {
  const text = String(url || '').trim()
  if (!text || typeof window === 'undefined') return
  let copied = false
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      copied = true
    }
  } catch {}
  if (!copied) {
    const input = document.createElement('input')
    input.value = text
    document.body.appendChild(input)
    input.select()
    copied = document.execCommand('copy')
    document.body.removeChild(input)
  }
  if (!copied) return
  copiedLink.value = text
  if (copiedTimer.value) {
    window.clearTimeout(copiedTimer.value)
  }
  copiedTimer.value = window.setTimeout(() => {
    copiedLink.value = ''
    copiedTimer.value = null
  }, 1600)
}

watch(() => props.active, (v) => {
  if (v) {
    hydrateFeedCache()
    void loadFeed()
    return
  }
})

watch(() => props.limit, () => {
  currentPage.value = 1
  hydrateFeedCache()
  if (props.active) void loadFeed()
})

watch(() => props.layoutState, () => {
  currentPage.value = 1
  deferMeasure()
})

watch(pageItems, () => {
  deferMeasure()
})

onMounted(() => {
  hydrateFeedCache()
  if (props.active !== false) {
    void loadFeed()
  }
})

onUnmounted(() => {
  Array.from(feedResizeObservers.keys()).forEach((feedId) => cleanupFeedSummaryObserver(feedId))
  if (copiedTimer.value) {
    window.clearTimeout(copiedTimer.value)
    copiedTimer.value = null
  }
  if (measureTimer.value) {
    window.clearTimeout(measureTimer.value)
    measureTimer.value = null
  }
})
</script>

<style scoped>
.feed-list-wrap {
  width: 100%;
  margin-top: 0;
  background: transparent !important;
}

.feed-empty {
  padding: 24px 0;
  text-align: center;
  opacity: 0.95;
  color: #f8fafc;
}

.feed-loading-text {
  color: #f8fafc !important;
}

.feed-wrap-light .feed-empty {
  color: #0f172a;
}

.feed-wrap-dark .feed-empty {
  color: #f8fafc;
}

.feed-wrap-light .feed-loading-text,
.feed-wrap-dark .feed-loading-text {
  color: #f8fafc !important;
}

.feed-grid {
  display: grid;
  gap: 14px !important;
  background: transparent !important;
  row-gap: 14px !important;
  align-items: start;
}

.feed-grid-three,
.feed-grid-two,
.feed-grid-single {
  grid-template-columns: 1fr;
}

.feed-card {
  padding: 10px;
  border-radius: 12px;
  transition: none;
  margin: 0 !important;
  display: flex;
  flex-direction: column;
  gap: 9px;
  width: 100%;
  box-sizing: border-box;
  position: relative;
  overflow: hidden;
  align-self: start;
  height: auto;
}

.feed-card + .feed-card {
  margin-top: 0 !important;
}

.feed-card-light {
  border: 1px solid rgba(15, 23, 42, 0.14);
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.08);
}

.feed-card-dark {
  border: 1px solid rgba(255, 255, 255, 0.14);
}

.feed-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.author-row {
  line-height: 1.1;
  position: relative;
}

.feed-source-user {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 14px;
  font-weight: 700;
  opacity: 0.92;
}

.feed-avatar {
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  border-radius: 999px;
  object-fit: cover;
  border: 1px solid rgba(148, 163, 184, 0.45);
}

.feed-source-user span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.15;
}

.feed-time {
  flex: 0 0 auto;
  font-size: clamp(11px, 0.9vw, 12px);
  line-height: 1.2;
  opacity: 0.72;
  white-space: nowrap;
  text-align: right;
}

.feed-title {
  font-size: 18px;
  font-weight: 700;
  line-height: 1.45;
  margin: 0;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.feed-summary {
  white-space: pre-wrap;
  line-height: 1.7;
  font-size: 15px;
  color: #334155;
}

.feed-summary-markdown :deep(.markdown-preview) {
  font-size: 15px;
  line-height: 1.7;
}

.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview .vditor-reset),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview p),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview li),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview span),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview code),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview blockquote) {
  color: #0f172a !important;
  opacity: 1 !important;
}

.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview .vditor-reset),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview p),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview li),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview span),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview code),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview blockquote) {
  color: #f8fafc !important;
  opacity: 1 !important;
}

.feed-card-dark .feed-summary-markdown :deep(.markdown-preview),
.feed-card-dark .feed-summary-markdown :deep(.markdown-preview p),
.feed-card-dark .feed-summary-markdown :deep(.markdown-preview li),
.feed-card-dark .feed-summary-markdown :deep(.markdown-preview span) {
  color: #f8fafc !important;
}

.feed-summary-markdown :deep(.markdown-preview p) {
  margin: 4px 0 !important;
  text-shadow: none !important;
}

.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview a),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview a span),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview .vditor-reset a) {
  color: #2563eb !important;
  text-decoration: underline !important;
  text-underline-offset: 2px;
}

.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview a:hover),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview a:hover span),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview .vditor-reset a:hover) {
  color: #1d4ed8 !important;
}

.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview a),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview a span),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview .vditor-reset a) {
  color: #60a5fa !important;
  text-decoration: underline !important;
  text-underline-offset: 2px;
}

.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview a:hover),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview a:hover span),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview .vditor-reset a:hover) {
  color: #93c5fd !important;
}

.feed-summary-markdown :deep(.markdown-preview ul),
.feed-summary-markdown :deep(.markdown-preview ol) {
  margin: 6px 0;
}

.feed-summary-markdown :deep(.markdown-preview .image-grid) {
  margin-top: 8px;
  margin-bottom: 8px;
}

.feed-summary-block {
  max-height: none;
  overflow: visible;
  padding-right: 0;
}

.feed-summary-body {
  transition: max-height 0.3s ease-in-out;
  z-index: 1;
}

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

.expand-toggle-btn:hover {
  transform: scale(1.02);
}

:global(html.dark) .expand-toggle-btn {
  color: #fff;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
}

:global(html.dark) .expand-toggle-btn:hover {
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.5);
}

:global(html:not(.dark)) .expand-toggle-btn {
  color: #111827;
  text-shadow: 0 1px 2px rgba(255, 255, 255, 0.5);
}

:global(html:not(.dark)) .expand-toggle-btn:hover {
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
}

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

:global(html.dark) .feed-summary {
  color: #fff;
}

.feed-card-dark .feed-summary,
.feed-card-dark .feed-title,
.feed-card-dark .feed-time,
.feed-card-dark .feed-source-user,
.feed-card-dark .feed-footer,
.feed-card-dark .feed-domain,
.feed-card-dark .pager-meta {
  color: #fff !important;
}

.feed-card-dark .feed-time {
  opacity: 0.88;
}

.feed-image-wrap {
  border-radius: 12px;
  overflow: hidden;
  background: rgba(0, 0, 0, 0.04);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 6px;
}

:global(html.dark) .feed-image-wrap {
  background: rgba(255, 255, 255, 0.06);
}

.feed-image {
  width: 100%;
  max-height: 640px;
  object-fit: contain;
  height: auto;
  display: block;
  transition: transform .18s ease, box-shadow .18s ease, filter .18s ease;
}

.feed-image-btn {
  width: 100%;
  display: flex;
  justify-content: center;
  border: 0;
  background: transparent;
  padding: 0;
  cursor: zoom-in;
}

.feed-image:hover {
  transform: translate3d(0, 0, 0) scale(1.02);
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.28);
  filter: saturate(1.06) contrast(1.02);
}

.feed-preview-modal {
  padding: 8px;
  display: flex;
  justify-content: center;
  align-items: center;
}

.feed-preview-image {
  width: 100%;
  max-width: min(1200px, 92vw);
  max-height: 86vh;
  object-fit: contain;
}

.feed-footer {
  margin-top: auto;
  padding-top: 10px;
  border-top: 1px solid rgba(148, 163, 184, 0.32);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: #64748b;
  font-size: 12px;
  font-weight: 600;
  background: transparent !important;
}

:global(html.dark) .feed-footer {
  border-top-color: rgba(100, 116, 139, 0.45);
  color: rgba(226, 232, 240, 0.78);
}

.feed-domain {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.feed-domain span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 与 MessageList 的链接配色保持一致 */
.feed-card a:not(.feed-icon-btn),
.feed-domain,
.feed-domain span {
  color: #0366d6 !important;
  text-decoration: none;
  font-weight: 500;
}

.feed-card a:not(.feed-icon-btn):hover,
.feed-domain:hover,
.feed-domain:hover span {
  color: #1d4ed8 !important;
  text-decoration: underline;
}

.feed-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.feed-icon-btn {
  width: 30px;
  height: 30px;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(100, 116, 139, 0.35);
  color: #334155;
  background: transparent;
  transition: transform .12s ease, box-shadow .12s ease, color .12s ease, border-color .12s ease, background-color .12s ease;
  cursor: pointer;
}

.feed-icon-btn:hover {
  transform: translate3d(0, 0, 0) scale(1.06);
  box-shadow: 0 6px 16px rgba(15, 23, 42, 0.16);
  color: #0f172a;
  background: rgba(15, 23, 42, 0.04);
}

.feed-icon-btn:focus-visible {
  outline: 2px solid rgba(14, 165, 233, 0.65);
  outline-offset: 1px;
}

.feed-icon-btn:active {
  transform: scale(0.96);
}

.feed-icon-btn.is-success {
  border-color: rgba(34, 197, 94, 0.48);
  color: #16a34a;
  background: rgba(34, 197, 94, 0.1);
}

.pager-shell {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 12px;
  margin: 10px 0 4px;
  flex-wrap: wrap;
}

.pager-main {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pager-meta {
  font-size: 13px;
  color: #64748b;
}

.pager-icon-wrap {
  display: inline-flex;
  align-items: center;
}

:global(html.dark) .pager-meta {
  color: rgba(226, 232, 240, 0.8);
}

:global(html.dark) .feed-icon-btn {
  border-color: rgba(255, 255, 255, 0.48);
  color: #fff;
  background: rgba(255, 255, 255, 0.06);
}

:global(html.dark) .feed-icon-btn:hover {
  box-shadow: 0 8px 18px rgba(255, 255, 255, 0.2);
  color: #ffffff;
  background: rgba(255, 255, 255, 0.14);
}

.feed-card-dark .feed-icon-btn,
.feed-card-dark .feed-icon-btn :deep(svg),
.feed-card-dark .feed-icon-btn :deep(path),
.feed-card-dark .feed-icon-btn :deep(i),
.feed-card-dark .feed-icon-btn :deep(span) {
  color: #fff !important;
}

:global(html.dark) .feed-icon-btn.is-success {
  border-color: rgba(74, 222, 128, 0.52);
  color: #86efac;
  background: rgba(34, 197, 94, 0.14);
}

@media (max-width: 1024px) {
  .feed-title { font-size: 17px; }
  .feed-summary { font-size: 14px; }
}

@media (max-width: 768px) {
  .feed-card {
    margin: 0 !important;
    padding: 6px;
    box-shadow: none;
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }
  .feed-card + .feed-card {
    margin-top: 0 !important;
  }
  .feed-title { font-size: 16px; }
  .feed-summary { font-size: 14px; }
  .feed-source-user {
    gap: 7px;
    font-size: 13px;
  }
  .feed-avatar {
    width: 32px;
    height: 32px;
    flex-basis: 32px;
  }
  .feed-time {
    font-size: 11px;
  }
  .feed-footer {
    font-size: 12px;
    gap: 8px;
  }
}
</style>
