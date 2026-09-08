import { computed, ref, watch, onMounted, onUnmounted, type Ref, type ComponentPublicInstance } from 'vue'
import { useRoute } from '#imports'
import { savePagePosition, type HomePagePosition } from '~/utils/home-page-position'
import { getMessageIdFromRouteHash } from '~/utils/message-route-hash'

// Owns pager refs and reload persistence. The page supplies tab/layout/filter
// state; hidden or filtered views never overwrite an ordinary page position.
type HomePagerState = {
  visible: boolean
  currentPage: number
  totalPages: number
  loading: boolean
  canPrevious: boolean
  canNext: boolean
}
export type HomePagerController = ComponentPublicInstance & {
  sidebarPagerState: HomePagerState
  previousPage: () => void | Promise<void>
  nextPage: () => void | Promise<void>
  goToPage: (page: string | number) => void | Promise<void>
}
type MessageListExpose = HomePagerController & {
  refreshList: () => Promise<void>
}
type InfoFeedListExpose = HomePagerController & {
  refreshFeed: () => Promise<void>
  footerPagerState: HomePagerState & { targetPage: string }
  setTargetPage: (page: string) => void
  adjustTargetPage: (delta: number) => void
  jumpToTargetPage: () => void
}
export type CommentThreadExpose = HomePagerController & {
  focusCommentById: (commentId: number) => Promise<boolean>
}
type AnnouncementCenterExpose = HomePagerController & { refresh: () => void | Promise<void> }
type AnnouncementModalExpose = { refresh: () => void | Promise<void> }
export const useHomePager = (activeTab: Ref<string>, isMasonry: Ref<boolean>, filters: {
  selectedCalendarDate: Ref<string>, searchKeyword: Ref<string>, selectedTag: Ref<string>
}, reloadPosition: HomePagePosition | null) => {
  const route = useRoute()
  const { selectedCalendarDate, searchKeyword, selectedTag } = filters
  const feedRefreshing = ref(false)
  const messageList = ref<MessageListExpose | null>(null)
  const infoFeedList = ref<InfoFeedListExpose | null>(null)
  const guestbookCommentsRef = ref<CommentThreadExpose | null>(null)
  const notificationCenter = ref<HomePagerController | null>(null)
  const announcementCenter = ref<AnnouncementCenterExpose | null>(null)
  const announcementModal = ref<AnnouncementModalExpose | null>(null)
  const latestTotalPages = ref(1)
  const activeSidebarPagerController = computed<HomePagerController | null>(() => {
    if (activeTab.value === 'feed') return infoFeedList.value
    if (activeTab.value === 'comment') return guestbookCommentsRef.value
    if (activeTab.value === 'notifications') return notificationCenter.value
    if (activeTab.value === 'announcements') return announcementCenter.value
    if (activeTab.value === 'latest' || activeTab.value === 'personal') return messageList.value
    return null
  })
  const activeSidebarPagerState = computed<HomePagerState | null>(() => activeSidebarPagerController.value?.sidebarPagerState || null)
  const isFeedLoading = computed(() => infoFeedList.value?.sidebarPagerState.loading === true)
  const feedPagerState = computed<HomePagerState & { targetPage: string }>(() => infoFeedList.value?.footerPagerState || {
    visible: false,
    currentPage: 1,
    totalPages: 1,
    targetPage: '1',
    loading: false,
    canPrevious: false,
    canNext: false,
  })
  const setFeedTargetPage = (event: Event) => {
    const input = event.currentTarget as HTMLInputElement | null
    infoFeedList.value?.setTargetPage(input?.value || '')
  }
  const refreshInfoFeed = async () => {
    if (feedRefreshing.value || isFeedLoading.value) return
    feedRefreshing.value = true
    try {
      await infoFeedList.value?.refreshFeed()
    } finally {
      window.setTimeout(() => {
        feedRefreshing.value = false
      }, 300)
    }
  }
  const isSidebarPagerInteractive = computed(() => activeSidebarPagerState.value?.visible === true)
  const disabledSidebarPager = computed<HomePagerState>(() => ({
    visible: true,
    currentPage: 0,
    totalPages: latestTotalPages.value,
    loading: false,
    canPrevious: false,
    canNext: false
  }))
  const activeSidebarPager = computed<HomePagerState>(() => isSidebarPagerInteractive.value
    ? (activeSidebarPagerState.value as HomePagerState)
    : disabledSidebarPager.value)
  const handleSidebarPagerPrevious = () => {
    if (isSidebarPagerInteractive.value) activeSidebarPagerController.value?.previousPage()
  }
  const handleSidebarPagerNext = () => {
    if (isSidebarPagerInteractive.value) activeSidebarPagerController.value?.nextPage()
  }
  const handleSidebarPagerJump = (page: string) => {
    if (isSidebarPagerInteractive.value) activeSidebarPagerController.value?.goToPage(page)
  }
  watch(() => [
    activeTab.value,
    messageList.value?.sidebarPagerState?.totalPages,
    selectedCalendarDate.value,
    searchKeyword.value,
    selectedTag.value
  ], ([tab, pages, date, keyword, tag]) => {
    if (tab !== 'latest' || date || keyword || tag) return
    const next = Number(pages)
    if (Number.isFinite(next) && next > 0) latestTotalPages.value = Math.max(1, Math.floor(next))
  })
  const initialPage = computed(() => !isMasonry.value && reloadPosition?.tab === activeTab.value ? reloadPosition.page : 1)
  const rememberPagePosition = () => {
    const pager = activeSidebarPager.value
    const eligible = !isMasonry.value && ['latest', 'personal', 'feed'].includes(activeTab.value) && !selectedCalendarDate.value && !searchKeyword.value && !selectedTag.value && !getMessageIdFromRouteHash(route.hash) && !route.query.message_id
    savePagePosition(eligible ? { tab: activeTab.value as HomePagePosition['tab'], page: Math.max(1, pager.currentPage) } : null)
  }
  onMounted(() => window.addEventListener('pagehide', rememberPagePosition))
  onUnmounted(() => window.removeEventListener('pagehide', rememberPagePosition))
  return { messageList, infoFeedList, guestbookCommentsRef, notificationCenter, announcementCenter, announcementModal, latestTotalPages, isFeedLoading, feedPagerState, setFeedTargetPage, refreshInfoFeed, activeSidebarPager, handleSidebarPagerPrevious, handleSidebarPagerNext, handleSidebarPagerJump, initialPage, feedRefreshing, isSidebarPagerInteractive }
}
