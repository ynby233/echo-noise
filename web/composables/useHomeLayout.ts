import { computed, onMounted, onUnmounted, ref, watch, type Ref } from 'vue'

export const normalizeLayoutMode = (raw: any): 'three' | 'two' | 'single' | 'masonry' => {
  const val = String(raw || '').trim()
  return (val === 'three' || val === 'two' || val === 'single' || val === 'masonry') ? val : 'three'
}

// Layout preferences belong to the device; unsupported tabs exit masonry exactly
// as before. This module owns the media-query listener and releases it on unmount.
export const useHomeLayout = (activeTab: Ref<string>) => {
  let desktopLayoutDefault: 'three' | 'two' | 'single' | 'masonry' = 'three'
  const initialLayout = ((): 'three' | 'two' | 'single' | 'masonry' => {
    if (typeof window === 'undefined') return 'three'
    const isMobileInit = window.matchMedia('(max-width: 1024px)').matches
    const saved = localStorage.getItem(isMobileInit ? 'homeLayoutMobile' : 'homeLayoutDesktop') as any
    if (saved) return normalizeLayoutMode(saved)
    return isMobileInit ? 'single' : desktopLayoutDefault
  })()
  const layoutState = ref<'three' | 'two' | 'single' | 'masonry'>(initialLayout)
  const mq = typeof window !== 'undefined' ? window.matchMedia('(max-width: 1024px)') : null
  const isMobile = ref<boolean>(!!mq?.matches)
  const cycleLayout = () => {
    if (isMobile.value) return
    layoutState.value = layoutState.value === 'three' ? 'two' : (layoutState.value === 'two' ? 'single' : (layoutState.value === 'single' && supportsMasonry.value ? 'masonry' : 'three'))
    if (typeof window !== 'undefined') localStorage.setItem('homeLayoutDesktop', layoutState.value)
  }
  const handleLayoutMediaChange = (e: MediaQueryListEvent) => {
    isMobile.value = e.matches
    if (isMobile.value) {
      layoutState.value = 'single'
      localStorage.setItem('homeLayoutMobile', 'single')
      return
    }
    const saved = localStorage.getItem('homeLayoutDesktop') as any
    layoutState.value = normalizeLayoutMode(saved || desktopLayoutDefault)
  }
  onMounted(() => {
    mq?.addEventListener?.('change', handleLayoutMediaChange)
  })
  onUnmounted(() => {
    mq?.removeEventListener?.('change', handleLayoutMediaChange)
  })
  const gridModeClass = computed(() => isMasonry.value ? 'grid-masonry' : (layoutState.value === 'three' ? 'grid-3' : (layoutState.value === 'two' ? 'grid-2' : 'grid-1')))
  const layoutIcon = computed(() => isMasonry.value ? 'i-mdi-view-dashboard' : (layoutState.value === 'three' ? 'i-mdi-view-grid' : (layoutState.value === 'two' ? 'i-mdi-view-column' : 'i-mdi-view-stream')))
  const centerContainerClass = computed(() => (
    (layoutState.value === 'two' || isMasonry.value)
      ? 'w-full max-w-none'
      : (layoutState.value === 'single'
          ? 'mx-auto w-full max-w-[640px] sm:max-w-3xl'
          : 'mx-auto w-full sm:max-w-4xl')
  ))
  const supportsMasonry = computed(() => ['latest', 'personal', 'feed'].includes(activeTab.value))
  const isMasonry = computed(() => !isMobile.value && layoutState.value === 'masonry' && supportsMasonry.value)
  watch([activeTab, layoutState], () => {
    if (layoutState.value === 'masonry' && !supportsMasonry.value) {
      layoutState.value = 'three'
      if (typeof window !== 'undefined') localStorage.setItem('homeLayoutDesktop', 'three')
    }
  }, { flush: 'sync' })

  const applyDefaultLayout = (layout: ReturnType<typeof normalizeLayoutMode>) => {
    desktopLayoutDefault = layout
    if (typeof window !== 'undefined' && !isMobile.value && !localStorage.getItem('homeLayoutDesktop')) layoutState.value = layout
  }
  return { layoutState, isMobile, cycleLayout, gridModeClass, layoutIcon, centerContainerClass, supportsMasonry, isMasonry, applyDefaultLayout }
}
