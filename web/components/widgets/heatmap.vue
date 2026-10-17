<template>
  <div class="calendar-wrapper" :class="{ 'heatmap-compact': props.compact }">
    <div class="calendar-container" ref="calendarContainer" :class="isDark ? 'heatmap-dark' : 'heatmap-light'">
      <div ref="calendarGrid" class="heatmap-grid" :style="{ width: gridViewportWidth }" @scroll.passive="rememberCalendarScroll">
        <div v-for="(week, i) in calendarData" :key="i" class="heatmap-week">
          <div 
            v-for="(day, j) in week" 
            :key="j" 
            class="heatmap-day"
            :style="{ backgroundColor: getBackgroundColor(day) }"
            @mouseenter="showTooltip($event, day)"
            @mouseleave="hideTooltip"
            @mousemove="moveTooltip"
          ></div>
        </div>
      </div>
    </div>
    <Teleport to="body">
      <div v-if="tooltip.visible" ref="heatmapTooltip" class="heatmap-tooltip nw-tooltip" :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px' }">{{ tooltip.text }}</div>
    </Teleport>
  </div>
</template>
  
<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed, watch, inject, nextTick } from 'vue'
import { useUserStore } from '~/store/user'
import { generateHeatmapCalendar, type HeatItem, type CalendarDay } from '~/utils/heatmapCalendar'

const props = withDefaults(defineProps<{ activeTab?: string; compact?: boolean }>(), { activeTab: 'latest', compact: false })
const userStore = useUserStore()
const rawData = ref<HeatItem[]>([])
const calendarData = ref<CalendarDay[][]>([])
const calendarContainer = ref<HTMLElement | null>(null)
const calendarGrid = ref<HTMLElement | null>(null)
const heatmapTooltip = ref<HTMLElement | null>(null)
const tooltip = ref({ visible: false, text: '', x: 0, y: 0 })
const gridViewportWidth = ref('100%')
let followLatest = true
let savedScrollLeft = 0
let scrollRevision = 0
let layoutPending = false
let programmedScrollLeft: number | undefined
let layoutObserver: ResizeObserver | undefined

const rememberCalendarScroll = () => {
  const grid = calendarGrid.value
  // Hidden elements and layout changes must not overwrite the user's position.
  if (!grid || !grid.clientWidth || layoutPending || grid.scrollWidth <= grid.clientWidth) return
  if (programmedScrollLeft !== undefined && Math.abs(grid.scrollLeft - programmedScrollLeft) <= 1) {
    programmedScrollLeft = undefined
    return
  }
  programmedScrollLeft = undefined
  savedScrollLeft = grid.scrollLeft
  followLatest = grid.scrollWidth - grid.clientWidth - grid.scrollLeft <= 2
}

const scheduleCalendarScroll = async () => {
  const revision = ++scrollRevision
  layoutPending = true
  await nextTick()
  if (revision !== scrollRevision) return
  const grid = calendarGrid.value
  if (grid && grid.clientWidth > 0) {
    // ResizeObserver retries when a hidden widget becomes measurable again.
    const right = Math.max(0, grid.scrollWidth - grid.clientWidth)
    programmedScrollLeft = followLatest ? right : Math.min(savedScrollLeft, right)
    grid.scrollTo({ left: programmedScrollLeft, behavior: 'instant' })
  }
  layoutPending = false
}

// 主题注入与样式类
const injectedTheme = inject('contentTheme', ref('light')) as any
const isDark = computed(() => String((injectedTheme && injectedTheme.value !== undefined) ? injectedTheme.value : injectedTheme) === 'dark')
const mutedTextClass = computed(() => (isDark.value ? 'text-white/70' : 'text-black/60'))
  
  // 生成月份标签
  const monthLabels = computed(() => {
  if (!rawData.value.length) return Array(12).fill('').map((_, i) => `${i + 1}月`);
  
  const dates = rawData.value.map(item => new Date(item.date).getTime());
  const firstDate = new Date(Math.min(...dates));
  
  const labels = [];
  let currentDate = new Date(firstDate);
  currentDate.setDate(1);
  
  let currentYear = currentDate.getFullYear();
  
  // 生成12个月的标签
  for (let i = 0; i < 12; i++) {
    const month = currentDate.getMonth();
    const year = currentDate.getFullYear();
    
    // 只在年份变化时或第一个月显示年份
    if (year !== currentYear || i === 0) {
      labels.push(`${year}年${month + 1}月`);
      currentYear = year;
    } else {
      labels.push(`${month + 1}月`);
    }
    
    currentDate.setMonth(month + 1);
  }
  
  return labels;
})
  
  // 中文星期
  const weekdays = ['日', '一', '二', '三', '四', '五', '六']
  
const getColor = (level: number) => {
  // 统一为主题友好的橙色梯度，和站内交互强调色保持一致
  const lightColors = ['#fed7aa', '#fdba74', '#fb923c', '#f97316', '#c2410c']
  const darkColors = [
    'rgba(249, 115, 22, 0.24)',
    'rgba(249, 115, 22, 0.42)',
    'rgba(249, 115, 22, 0.60)',
    'rgba(249, 115, 22, 0.78)',
    'rgba(249, 115, 22, 0.96)'
  ]
  const arr = isDark.value ? darkColors : lightColors
  return arr[Math.min(Math.max(level - 1, 0), 4)] || (isDark.value ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.06)')
}
  // 优化颜色计算
const getBackgroundColor = (day: { count: number; level: number }) => {
  // 空格子颜色统一且对比适中
  if (!day.count) return isDark.value ? 'rgba(255, 255, 255, 0.10)' : '#e5e7eb'
  return getColor(day.level)
}
const currentUserId = computed(() => Number((userStore.user as any)?.userid || (userStore.user as any)?.id || (userStore.user as any)?.user_id || 0))
const isPersonalScope = computed(() => props.activeTab === 'personal')
const calendarRequestURL = computed(() => {
  const params = new URLSearchParams()
  if (isPersonalScope.value && currentUserId.value > 0) {
    params.set('authorId', String(currentUserId.value))
  }
  const query = params.toString()
  return `/api/messages/calendar${query ? `?${query}` : ''}`
})
const fetchHeatmapData = async () => {
  if (isPersonalScope.value && (!userStore.isLogin || currentUserId.value <= 0)) {
    rawData.value = []
    generateEmptyCalendar()
    return
  }
  try {
    const response = await fetch(calendarRequestURL.value, { credentials: 'include' })
    const data = await response.json()
    if (data && data.code === 1 && Array.isArray(data.data) && data.data.length > 0) {
      rawData.value = data.data
      generateCalendarData()
    } else {
      rawData.value = []
      generateEmptyCalendar()
    }
  } catch (error) {
    console.error('获取热力图数据失败:', error)
    rawData.value = []
    generateEmptyCalendar()
  }
}
  const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), Math.max(min, max))

  const getFixedCoordinateScale = () => {
    if (typeof window === 'undefined') return 1
    const zoom = Number.parseFloat(window.getComputedStyle(document.body).zoom || '1')
    return Number.isFinite(zoom) && zoom > 0 ? zoom : 1
  }

  const getFixedViewport = (scale: number) => {
    const viewport = window.visualViewport
    const left = (viewport?.offsetLeft || 0) / scale
    const top = (viewport?.offsetTop || 0) / scale
    const width = (viewport?.width || window.innerWidth) / scale
    const height = (viewport?.height || window.innerHeight) / scale
    return { left, top, right: left + width, bottom: top + height }
  }

  const getFixedRect = (element: HTMLElement, scale: number) => {
    const rect = element.getBoundingClientRect()
    return {
      left: rect.left / scale,
      right: rect.right / scale,
      top: rect.top / scale,
      bottom: rect.bottom / scale,
      width: rect.width / scale,
      height: rect.height / scale
    }
  }

  const placeTooltip = (target: HTMLElement) => {
    const scale = getFixedCoordinateScale()
    const rect = getFixedRect(target, scale)
    const viewport = getFixedViewport(scale)
    const tip = heatmapTooltip.value
    const tooltipWidth = tip?.offsetWidth || 112
    const tooltipHeight = tip?.offsetHeight || 24
    const pad = 8
    const gap = 4
    const minLeft = viewport.left + pad
    const maxLeft = Math.max(minLeft, viewport.right - tooltipWidth - pad)
    const idealLeft = rect.left + rect.width / 2 - tooltipWidth / 2
    const left = clamp(idealLeft, minLeft, maxLeft)
    const aboveTop = rect.top - tooltipHeight - gap
    const belowTop = rect.bottom + gap
    const minTop = viewport.top + pad
    const maxTop = Math.max(minTop, viewport.bottom - tooltipHeight - pad)
    const centerTop = clamp(rect.top + rect.height / 2 - tooltipHeight / 2, minTop, maxTop)
    const horizontalDrift = Math.abs(left + tooltipWidth / 2 - (rect.left + rect.width / 2))
    const canPlaceRight = rect.right + gap + tooltipWidth <= viewport.right - pad
    const canPlaceLeft = rect.left - gap - tooltipWidth >= viewport.left + pad

    if (horizontalDrift > Math.max(18, tooltipWidth * 0.22) && (canPlaceRight || canPlaceLeft)) {
      const sideLeft = canPlaceRight ? rect.right + gap : rect.left - tooltipWidth - gap
      tooltip.value.x = clamp(sideLeft, minLeft, maxLeft)
      tooltip.value.y = centerTop
      return
    }

    const preferBelow = aboveTop < minTop && belowTop + tooltipHeight <= viewport.bottom - pad
    const rawTop = preferBelow ? belowTop : aboveTop
    tooltip.value.x = left
    tooltip.value.y = clamp(rawTop, minTop, maxTop)
  }

  const showTooltip = (e: MouseEvent, day: any) => {
    tooltip.value.text = `${day.date} · ${day.count || 0} 条`
    const target = e.currentTarget as HTMLElement
    placeTooltip(target)
    tooltip.value.visible = true
    nextTick(() => {
      placeTooltip(target)
      if (typeof window !== 'undefined') window.requestAnimationFrame(() => placeTooltip(target))
    })
  }
  const moveTooltip = (e: MouseEvent) => {
    placeTooltip(e.currentTarget as HTMLElement)
  }
  const hideTooltip = () => {
    tooltip.value.visible = false
  }
  
  const getHeatmapSizing = () => {
    const isMobile = typeof window !== 'undefined' && window.matchMedia('(max-width: 1024px)').matches
    return {
      daySize: props.compact ? (isMobile ? 9 : 10) : (isMobile ? 8 : 12),
      gap: props.compact ? 2 : 3,
      endGutter: props.compact ? 8 : 0
    }
  }
  const visibleColumnCount = () => {
    const w = calendarContainer.value?.clientWidth || 0
    const { daySize, gap, endGutter } = getHeatmapSizing()
    const availableWidth = Math.max(0, w - endGutter)
    if (!availableWidth) return 0
    return Math.max(0, Math.floor((availableWidth + gap) / (daySize + gap)))
  }
  const syncGridViewportWidth = () => {
    const columns = visibleColumnCount()
    if (!columns) {
      gridViewportWidth.value = '100%'
      return
    }
    const { daySize, gap } = getHeatmapSizing()
    const width = columns * daySize + Math.max(0, columns - 1) * gap
    gridViewportWidth.value = `${width}px`
  }
  const generateCalendarData = () => {
    calendarData.value = generateHeatmapCalendar(rawData.value)
    scheduleCalendarScroll()
  }

  const generateEmptyCalendar = () => {
    calendarData.value = generateHeatmapCalendar([])
    scheduleCalendarScroll()
  }

  const refreshCalendarLayout = () => {
    syncGridViewportWidth()
    if (rawData.value.length) {
      generateCalendarData()
    } else {
      generateEmptyCalendar()
    }
  }

  watch(
    [() => props.activeTab, () => userStore.isLogin, () => currentUserId.value],
    () => { fetchHeatmapData() }
  )

  watch(() => props.compact, refreshCalendarLayout)

  onMounted(() => {
    generateEmptyCalendar()
    syncGridViewportWidth()
    window.addEventListener('resize', refreshCalendarLayout)
    if (typeof ResizeObserver !== 'undefined' && calendarContainer.value) {
      layoutObserver = new ResizeObserver(refreshCalendarLayout)
      layoutObserver.observe(calendarContainer.value)
    }
    fetchHeatmapData()
  })

  onBeforeUnmount(() => {
    scrollRevision++
    layoutObserver?.disconnect()
    window.removeEventListener('resize', refreshCalendarLayout)
  })
  </script>
  
  <style scoped>
  .calendar-wrapper {
    position: relative;
    overflow: visible;
    margin: 0;
    padding: 0;
    width: 100%;
    box-sizing: border-box;
  }

  .calendar-wrapper.heatmap-compact {
    padding: 0;
  }

  .calendar-container {
    position: relative;
    padding: 0;
    overflow: visible;
    width: 100%;
  }

  
  .calendar-container {
    position: relative;
    padding-top: 0;
    padding-left: 0;
  }
  
  .month-label {
  flex: 1;
  text-align: center;
  font-size: 11px;
  padding: 0 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
  
  .month-labels { display: none; }
  
  .weekday-labels { display: none; }
  
  .weekday-label {
    height: 12px;
    line-height: 12px;
    font-size: 10px;
    text-align: right;
    padding-right: 4px;
  }
  
  .heatmap-grid {
    display: flex;
    gap: 3px;
    overflow-x: auto;
    overflow-y: visible;
    padding-bottom: 8px;
    scroll-behavior: smooth;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: thin;
    width: 100%;
    max-width: 100%;
    margin-inline: auto;
  }
  .heatmap-grid::-webkit-scrollbar { height: 6px; }
  .heatmap-light .heatmap-grid::-webkit-scrollbar-track { background: rgba(0, 0, 0, 0.08); border-radius: 3px; }
  .heatmap-light .heatmap-grid::-webkit-scrollbar-thumb { background: rgba(0, 0, 0, 0.25); border-radius: 3px; }
  .heatmap-dark .heatmap-grid::-webkit-scrollbar-track { background: rgba(255, 255, 255, 0.08); border-radius: 3px; }
  .heatmap-dark .heatmap-grid::-webkit-scrollbar-thumb { background: rgba(255, 255, 255, 0.28); border-radius: 3px; }
  .heatmap-week {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  
  .heatmap-day {
    width: 12px;
    height: 12px;
    box-sizing: border-box;
    border-radius: 2px;
    transition: background-color 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease;
    border: 1px solid transparent;
  }

  .heatmap-compact .heatmap-grid {
    gap: 2px;
    padding-bottom: 0;
    scrollbar-width: none;
  }

  .heatmap-compact .heatmap-grid::-webkit-scrollbar { display: none; height: 0; }

  .heatmap-compact .heatmap-week {
    gap: 2px;
  }

  .heatmap-compact .heatmap-day {
    width: 10px;
    height: 10px;
    border-radius: 2px;
  }

  .heatmap-compact .heatmap-day:hover {
    box-shadow: 0 0 0 2px rgba(249, 115, 22, 0.32);
  }
  .heatmap-light .heatmap-day { border-color: #cbd5e1; }
  .heatmap-dark .heatmap-day { border-color: rgba(255,255,255,0.12); }
  
  .heatmap-day:hover {
    border-color: rgba(249, 115, 22, 0.72);
    box-shadow: 0 0 0 2px rgba(249, 115, 22, 0.24);
  }
  .heatmap-tooltip {
    z-index: 5006;
  }
  
  @media screen and (max-width: 1024px) {
    .heatmap-day {
      width: 8px;
      height: 8px;
    }

    .heatmap-compact .heatmap-day {
      width: 9px;
      height: 9px;
    }
    .month-label {
      font-size: 10px;
    }
    
    .weekday-label {
      font-size: 8px;
      height: 8px;
      line-height: 8px;
      padding-right: 2px;
    }
    .weekday-labels {
  position: absolute;
  left: 0;
  top: 20px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-right: 10px;
  z-index: 1;
}
  }
  </style>
<style scoped>
</style>
