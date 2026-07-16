<template>
  <div ref="feedListRoot" :class="['feed-list-wrap', wrapThemeClass]">
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
            class="feed-summary-body relative"
            :class="{ 'feed-summary-body--clipped': shouldShowExpandButton[getFeedItemId(item)] && !isExpanded[getFeedItemId(item)] }"
            :style="shouldShowExpandButton[getFeedItemId(item)] && !isExpanded[getFeedItemId(item)] ? { maxHeight: `${collapsedContentHeight}px` } : undefined"
          >
            <div class="feed-summary-markdown feed-summary-measure">
              <MarkdownRenderer
                :content="getDisplayRaw(item)"
                :enable-github-card="enableGithubCard"
                :theme-mode="contentTheme"
                @rendered="deferMeasure"
              />
            </div>
            <div v-if="shouldShowStandaloneImage(item)" class="feed-image-wrap feed-image-wrap-inline">
              <button
                type="button"
                class="feed-image-btn"
                :aria-label="`查看大图：${item.title || '图片'}`"
                @click="openImagePreview(item.imageURL)"
              >
                <img :src="item.imageURL" :alt="item.title || 'image'" class="feed-image" loading="lazy" decoding="async" @load="deferMeasure" @error="deferMeasure" />
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
            <img :src="item.imageURL" :alt="item.title || 'image'" class="feed-image" loading="lazy" decoding="async" @load="deferMeasure" @error="deferMeasure" />
          </button>
        </div>
        <div class="feed-footer">
          <div class="feed-domain nw-tooltip-anchor" :data-tooltip="item.link || ''">
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
    <div
      v-if="!loading && !errorText && allItems.length > 0"
      class="pager-shell"
      :class="{ 'is-dark': contentTheme === 'dark' }"
    >
      <div class="pager-nav-group">
        <button
          v-if="currentPage > 1"
          type="button"
          class="pager-btn nw-action-btn nw-action-btn--label"
          @click="goPrevPage"
          :disabled="loading"
        >
          <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-left" class="w-4 h-4 pager-icon" /></span>
          <span>上一页</span>
        </button>
        <button
          v-if="currentPage < totalPages"
          type="button"
          class="pager-btn nw-action-btn nw-action-btn--label"
          @click="goNextPage"
          :disabled="loading"
        >
          <span>下一页</span>
          <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-right" class="w-4 h-4 pager-icon" /></span>
        </button>
        <span v-if="loading" class="pager-status-text">加载中...</span>
      </div>
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
              :disabled="loading"
              @click="adjustTargetPage(1)"
            >
              <UIcon name="i-heroicons-chevron-up-20-solid" class="w-3 h-3" />
            </button>
            <button
              type="button"
              class="pager-stepper-btn nw-action-btn"
              aria-label="页码减一"
              :disabled="loading"
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
          :disabled="loading"
        >
          跳转
        </button>
      </div>
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
import { writeClipboardText } from '~/utils/clipboard'

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

const props = withDefaults(defineProps<{
  layoutState: 'three' | 'two' | 'single'
  limit?: number
  refreshSeconds?: number
  active?: boolean
  baseApi?: string
  enableGithubCard?: boolean
}>(), {
  enableGithubCard: false
})
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
const targetPage = ref('1')
const feedListRoot = ref<HTMLElement | null>(null)
const previewOpen = ref(false)
const previewImageURL = ref('')
const brokenAvatarSet = ref<Set<string>>(new Set())
const collapsedContentHeight = 820
const isExpanded = ref<Record<string, boolean>>({})
const hasUserToggled = ref<Record<string, boolean>>({})
const shouldShowExpandButton = ref<Record<string, boolean>>({})
const measuredContentHeights = ref<Record<string, number>>({})
const measureTimer = ref<number | null>(null)
const measureFrame = ref<number | null>(null)
const pageTopScrollTimer = ref<number | null>(null)
const feedSummaryRefs = ref<Record<string, HTMLElement | null>>({})
const feedResizeObservers = new Map<string, ResizeObserver>()
const observedFeedMedia = new WeakSet<Element>()
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
const syncTargetPageToCurrent = () => {
  const page = Math.min(Math.max(Number(currentPage.value) || 1, 1), totalPages.value)
  targetPage.value = String(page)
}
const normalizeTargetPage = (fallback = currentPage.value) => {
  const parsed = Number.parseInt(targetPage.value.trim() || '', 10)
  const next = Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
  return Math.min(Math.max(next, 1), totalPages.value)
}
const adjustTargetPage = (delta: number) => {
  targetPage.value = String(normalizeTargetPage(currentPage.value) + delta)
  targetPage.value = String(normalizeTargetPage(currentPage.value))
}
const pageItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return allItems.value.slice(start, start + pageSize.value)
})

const isScrollableY = (el: HTMLElement | null) => {
  if (!el || typeof window === 'undefined') return false
  const style = window.getComputedStyle(el)
  return /(auto|scroll|overlay)/.test(`${style.overflowY || ''} ${style.overflow || ''}`) && el.scrollHeight > el.clientHeight
}
const getAppScrollContainer = (target?: HTMLElement | null) => {
  if (typeof document === 'undefined') return null as HTMLElement | null
  const candidates = [
    target?.closest('.center-col') as HTMLElement | null,
    target?.closest('.content-wrapper') as HTMLElement | null,
    document.querySelector('.content-wrapper') as HTMLElement | null,
    document.querySelector('.center-col') as HTMLElement | null,
  ]
  return candidates.find(isScrollableY) || null
}
const scrollFeedFirstBlockToTop = async () => {
  await nextTick()
  if (typeof window === 'undefined') return
  if (pageTopScrollTimer.value) {
    window.clearTimeout(pageTopScrollTimer.value)
    pageTopScrollTimer.value = null
  }
  const target = feedListRoot.value?.querySelector<HTMLElement>('.feed-card') || feedListRoot.value
  if (!target) return
  const sc = getAppScrollContainer(target)
  const targetRect = target.getBoundingClientRect()
  if (sc) {
    const scRect = sc.getBoundingClientRect()
    const nextTop = sc.scrollTop + targetRect.top - scRect.top
    sc.scrollTo({ top: Math.max(0, nextTop), behavior: 'instant' })
    pageTopScrollTimer.value = window.setTimeout(() => {
      pageTopScrollTimer.value = null
      const rect = target.getBoundingClientRect()
      const rootRect = sc.getBoundingClientRect()
      sc.scrollTo({ top: Math.max(0, sc.scrollTop + rect.top - rootRect.top), behavior: 'instant' })
    }, 120)
    return
  }
  window.scrollTo({
    top: Math.max(0, (window.scrollY || window.pageYOffset || 0) + targetRect.top),
    left: window.scrollX || window.pageXOffset || 0,
    behavior: 'instant',
  })
}

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

watch(currentPage, syncTargetPageToCurrent, { immediate: true })
watch(totalPages, () => {
  clampPage()
  syncTargetPageToCurrent()
})

const goPrevPage = () => {
  if (currentPage.value <= 1) return
  currentPage.value -= 1
  deferMeasure()
  void scrollFeedFirstBlockToTop()
}

const goNextPage = () => {
  if (currentPage.value >= totalPages.value) return
  currentPage.value += 1
  deferMeasure()
  void scrollFeedFirstBlockToTop()
}

const jumpToPage = () => {
  const page = Number.parseInt(targetPage.value.trim() || '', 10)
  if (!page || page < 1 || page > totalPages.value || loading.value) {
    useToast().add({
      title: '页码无效',
      description: `请输入 1-${totalPages.value} 之间的数字`,
      color: 'orange',
      timeout: 2000
    })
    return
  }
  currentPage.value = page
  syncTargetPageToCurrent()
  deferMeasure()
  void scrollFeedFirstBlockToTop()
}

const goToPage = (page: string | number) => {
  targetPage.value = String(page)
  jumpToPage()
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

const bindSummaryMediaEvents = (container: HTMLElement) => {
  const mediaList = container.querySelectorAll('img, video, iframe')
  mediaList.forEach((media) => {
    if (observedFeedMedia.has(media)) return
    observedFeedMedia.add(media)
    media.addEventListener('load', deferMeasure, { passive: true })
    media.addEventListener('error', deferMeasure, { passive: true })
    media.addEventListener('loadedmetadata', deferMeasure, { passive: true })
    const image = media as HTMLImageElement
    if (image.complete || image.naturalHeight > 0) deferMeasure()
  })
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
  const measuredEl = (nextEl.querySelector('.feed-summary-measure') as HTMLElement | null) || nextEl
  bindSummaryMediaEvents(measuredEl)
  if (typeof window !== 'undefined' && typeof window.ResizeObserver !== 'undefined') {
    const observer = new window.ResizeObserver(() => {
      deferMeasure()
    })
    observer.observe(measuredEl)
    feedResizeObservers.set(feedId, observer)
  }
}

const deferMeasure = () => {
  if (typeof window === 'undefined') return
  if (measureFrame.value) return
  measureFrame.value = window.requestAnimationFrame(() => {
    measureFrame.value = null
    if (measureTimer.value) {
      window.clearTimeout(measureTimer.value)
    }
    measureTimer.value = window.setTimeout(() => {
      checkContentHeights()
      measureTimer.value = null
    }, 80)
  })
}

const setFeedExpansionState = (feedId: string, needsExpand: boolean) => {
  if (shouldShowExpandButton.value[feedId] !== needsExpand) {
    shouldShowExpandButton.value[feedId] = needsExpand
  }
  if (needsExpand) {
    if (!hasUserToggled.value[feedId] && isExpanded.value[feedId] !== false) {
      isExpanded.value[feedId] = false
    }
    return
  }
  if (isExpanded.value[feedId] !== true) isExpanded.value[feedId] = true
  if (hasUserToggled.value[feedId] !== false) hasUserToggled.value[feedId] = false
}

const checkContentHeights = () => {
  nextTick(() => {
    pageItems.value.forEach((item) => {
      const feedId = getFeedItemId(item)
      const contentEl = feedSummaryRefs.value[feedId]
      if (!contentEl) return
      bindSummaryMediaEvents(contentEl)
      const measuredEl = contentEl.querySelector('.feed-summary-measure') as HTMLElement | null
      const fullHeight = measuredEl?.scrollHeight || contentEl.scrollHeight
      const prevHeight = measuredContentHeights.value[feedId]
      const needsExpand = fullHeight > collapsedContentHeight + 8
      if (typeof prevHeight === 'number'
        && Math.abs(fullHeight - prevHeight) <= 8
        && shouldShowExpandButton.value[feedId] === needsExpand) {
        return
      }
      measuredContentHeights.value[feedId] = fullHeight
      setFeedExpansionState(feedId, fullHeight > collapsedContentHeight + 8)
    })
  })
}

const applyFeedItems = (items: FeedItem[]) => {
  const sortedItems = sortFeedItems(items)
  allItems.value = typeof maxItems.value === 'number' ? sortedItems.slice(0, maxItems.value) : sortedItems
  isExpanded.value = {}
  hasUserToggled.value = {}
  shouldShowExpandButton.value = {}
  measuredContentHeights.value = {}
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

const isMemosItem = (item: FeedItem) => {
  return String(item.type || '').toLowerCase() === 'memos'
}

const isNoteItem = (item: FeedItem) => {
  return String(item.type || '').toLowerCase() === 'note'
}

const isMastodonItem = (item: FeedItem) => {
  return String(item.type || '').toLowerCase() === 'mastodon'
}

const splitNormalizedLines = (raw: string) => {
  return String(raw || '')
    .replace(/\r\n/g, '\n')
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
}

const dedupeTitleFromFirstLine = (rawText: string, rawTitle: string) => {
  const text = String(rawText || '').trim()
  const title = normalizeContent(rawTitle || '')
  if (!text || !title) return text
  const textComparable = toComparable(text)
  const titleComparable = toComparable(title)
  if (!textComparable || !titleComparable) return text
  if (textComparable === titleComparable) return ''
  const lines = splitNormalizedLines(text)
  if (lines.length > 1 && toComparable(lines[0]) === titleComparable) {
    return lines.slice(1).join('\n').trim()
  }
  return text
}

const getDisplayRaw = (item: FeedItem) => {
  const summaryRaw = String(item.summary || '').trim()
  const contentRaw = String(item.content || '').trim()
  const text = contentRaw || summaryRaw
  if (!text) return ''

  // RSS：常见问题是正文首行重复标题，做有条件去重；避免标题+正文双重显示。
  if (isRSSItem(item)) {
    return dedupeTitleFromFirstLine(text, item.title || '')
  }

  // Memos：保留原始正文（含首行标签、代码块、引用等），不做裁剪。
  if (isMemosItem(item)) {
    return text
  }

  // Ech0/说说：保留原始正文，避免误删首段内容。
  if (isEch0Item(item)) {
    return text
  }

  // Note：仅做轻量首行去重，避免“标题+第一行同文”重复。
  if (isNoteItem(item)) {
    return dedupeTitleFromFirstLine(text, item.title || '')
  }

  // Mastodon：保留原始正文（含 CW/卡片文本/标签），避免误裁剪造成信息丢失。
  if (isMastodonItem(item)) {
    return text
  }

  // 其它未知源默认原样显示，降低误伤风险。
  return text
}

const isRSSItem = (item: FeedItem) => String(item.type || '').toLowerCase() === 'rss'

const hasInlineMediaInContent = (item: FeedItem) => {
  const raw = getDisplayRaw(item)
  if (!raw) return false
  return (
    /!\[[^\]]*]\((?:<)?[^)\s>]+(?:>)?\)/i.test(raw) ||
    /<img[\s\S]*?>/i.test(raw) ||
    /<picture[\s\S]*?>/i.test(raw) ||
    /<video[\s\S]*?>/i.test(raw) ||
    /\[[^\]]*]\((?:https?:\/\/)?[^\s)]+\.(?:jpg|jpeg|png|gif|webp|bmp|svg|avif|heic|heif)(?:\?[^\s)]*)?\)/i.test(raw) ||
    /(?:^|[\s(（[{【<])https?:\/\/[^\s<>()]+\.(?:jpg|jpeg|png|gif|webp|bmp|svg|avif|heic|heif)(?:\?[^\s<>()]*)?(?:$|[\s)）\]}>，,。.!！?？])/i.test(raw)
  )
}

const shouldShowStandaloneImage = (item: FeedItem) => {
  const hasImage = String(item.imageURL || '').trim().length > 0
  if (!hasImage) return false
  // RSS 若正文中已包含内联图片/媒体，不再额外展示独立图片，避免重复渲染。
  if (isRSSItem(item)) {
    const hasContent = String(getDisplayRaw(item) || '').trim().length > 0
    if (hasInlineMediaInContent(item)) return false
    // RSS 存在正文时优先相信正文，不额外补独立图，避免“正文图 + 独立图”重复。
    if (hasContent) return false
    return true
  }
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
  try {
    await writeClipboardText(text)
  } catch {
    return
  }
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

const sidebarPagerState = computed(() => ({
  visible: !errorText.value && allItems.value.length > 0,
  currentPage: currentPage.value,
  totalPages: totalPages.value,
  loading: loading.value,
  canPrevious: !loading.value && currentPage.value > 1,
  canNext: !loading.value && currentPage.value < totalPages.value
}))

defineExpose({
  sidebarPagerState,
  previousPage: goPrevPage,
  nextPage: goNextPage,
  goToPage
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
  if (measureFrame.value) {
    window.cancelAnimationFrame(measureFrame.value)
    measureFrame.value = null
  }
  if (pageTopScrollTimer.value) {
    window.clearTimeout(pageTopScrollTimer.value)
    pageTopScrollTimer.value = null
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
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview code),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview blockquote) {
  color: #0f172a !important;
  opacity: 1 !important;
}

.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview .vditor-reset),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview p),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview li),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview code),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview blockquote) {
  color: #f8fafc !important;
  opacity: 1 !important;
}

.feed-card-dark .feed-summary-markdown :deep(.markdown-preview),
.feed-card-dark .feed-summary-markdown :deep(.markdown-preview p),
.feed-card-dark .feed-summary-markdown :deep(.markdown-preview li) {
  color: #f8fafc !important;
}

.feed-summary-markdown :deep(.markdown-preview p) {
  margin: 4px 0 !important;
  text-shadow: none !important;
}

.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview a:not(.noise-attachment-file)),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview a:not(.noise-attachment-file) span),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview .vditor-reset a:not(.noise-attachment-file)) {
  color: #2563eb !important;
  text-decoration: underline !important;
  text-underline-offset: 2px;
}

.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview a:not(.noise-attachment-file):hover),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview a:not(.noise-attachment-file):hover span),
.feed-wrap-light .feed-summary-markdown :deep(.markdown-preview .vditor-reset a:not(.noise-attachment-file):hover) {
  color: #1d4ed8 !important;
}

.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview a:not(.noise-attachment-file)),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview a:not(.noise-attachment-file) span),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview .vditor-reset a:not(.noise-attachment-file)) {
  color: #60a5fa !important;
  text-decoration: underline !important;
  text-underline-offset: 2px;
}

.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview a:not(.noise-attachment-file):hover),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview a:not(.noise-attachment-file):hover span),
.feed-wrap-dark .feed-summary-markdown :deep(.markdown-preview .vditor-reset a:not(.noise-attachment-file):hover) {
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
  overflow: visible;
  transition: none;
  z-index: 1;
}

.feed-summary-body--clipped {
  overflow-y: hidden;
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
  transform: none;
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
  aspect-ratio: 16 / 9;
  max-height: 640px;
}

:global(html.dark) .feed-image-wrap {
  background: rgba(255, 255, 255, 0.06);
}

.feed-image {
  width: 100%;
  height: 100%;
  max-height: 640px;
  object-fit: contain;
  display: block;
  transition: transform .18s ease, box-shadow .18s ease, filter .18s ease;
}

.feed-image-btn {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
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
  --pager-shell-bg: rgba(255, 255, 255, 0.85);
  --pager-shell-border: rgba(15, 23, 42, 0.12);
  --pager-shell-text: #334155;
  --pager-shell-muted: #64748b;
  --pager-input-bg: rgba(255, 255, 255, 0.92);
  --pager-input-border: rgba(15, 23, 42, 0.16);
  --pager-input-text: #0f172a;
  --pager-input-placeholder: rgba(15, 23, 42, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  width: 100%;
  margin: 16px 0 72px;
  padding: 10px 14px;
  border: 1px solid var(--pager-shell-border);
  border-radius: 999px;
  background: var(--pager-shell-bg);
  color: var(--pager-shell-text);
  box-shadow: 0 8px 22px rgba(15, 23, 42, 0.10);
  flex-wrap: wrap;
}

.pager-shell.is-dark {
  --pager-shell-bg: rgba(39, 50, 66, 0.68);
  --pager-shell-border: rgba(255, 255, 255, 0.16);
  --pager-shell-text: #e2e8f0;
  --pager-shell-muted: #cbd5e1;
  --pager-input-bg: rgba(17, 24, 39, 0.58);
  --pager-input-border: rgba(255, 255, 255, 0.18);
  --pager-input-text: #f8fafc;
  --pager-input-placeholder: rgba(226, 232, 240, 0.58);
  box-shadow: 0 8px 22px rgba(0, 0, 0, 0.24);
}

.pager-nav-group,
.pager-jump-group {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  flex-wrap: wrap;
}

.pager-btn,
.pager-jump-btn {
  min-height: 34px;
  padding-inline: 14px;
  font-size: 13px;
  font-weight: 700;
  --nw-action-bg: rgba(15, 23, 42, .06);
  --nw-action-text: var(--pager-shell-text);
  --nw-action-border: var(--pager-shell-border);
}

.pager-page-text,
.pager-status-text,
.pager-done-text {
  color: var(--pager-shell-muted);
  font-size: 13px;
  font-weight: 650;
  text-shadow: none;
}

.pager-icon-wrap {
  width: 1.35rem;
  height: 1.35rem;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--nw-action-text) 10%, transparent);
}

.pager-icon {
  line-height: 1;
}

.pager-number-control {
  display: inline-flex;
  align-items: stretch;
  min-height: 34px;
  border: 1px solid var(--pager-input-border);
  border-radius: 12px;
  background: var(--pager-input-bg);
  color: var(--pager-input-text);
  overflow: hidden;
  transition: border-color .15s ease, box-shadow .15s ease, background-color .15s ease;
}

.pager-number-control:focus-within {
  border-color: rgba(249, 115, 22, 0.72);
  box-shadow: 0 0 0 2px rgba(249, 115, 22, 0.18);
}

.pager-page-input {
  width: 42px;
  min-height: 32px;
  padding: 0 6px;
  border: 0;
  outline: none;
  background: transparent;
  color: var(--pager-input-text);
  font-size: 14px;
  font-weight: 700;
  text-align: center;
  appearance: textfield;
}

.pager-page-input::placeholder {
  color: var(--pager-input-placeholder);
}

.pager-stepper {
  display: grid;
  grid-template-rows: 1fr 1fr;
  width: 24px;
  border-left: 1px solid var(--pager-input-border);
}

.pager-stepper-btn {
  width: 24px;
  min-width: 24px;
  height: 16px;
  min-height: 16px;
  padding: 0;
  border: 0;
  border-radius: 0;
}

.pager-stepper-btn + .pager-stepper-btn {
  border-top: 1px solid var(--pager-input-border);
}

.pager-stepper-btn svg {
  width: 12px;
  height: 12px;
}
.pager-shell.is-dark .pager-btn,
.pager-shell.is-dark .pager-jump-btn {
  --nw-action-bg: rgba(51, 65, 85, .96);
  --nw-action-text: #cbd5e1;
  --nw-action-border: rgba(148, 163, 184, .28);
}

@media (max-width: 640px) {
  .pager-shell {
    border-radius: 18px;
    gap: 10px;
  }

  .pager-nav-group,
  .pager-jump-group {
    width: 100%;
  }
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
