<template>
  <div class="calendar-wrapper" :class="{ 'heatmap-compact': props.compact }">
    <div class="calendar-container" ref="calendarContainer" :class="isDark ? 'heatmap-dark' : 'heatmap-light'">
      <div class="heatmap-grid" :style="{ width: gridViewportWidth }">
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
      <div v-if="tooltip.visible" ref="heatmapTooltip" class="heatmap-tooltip" :class="isDark ? 'heatmap-tooltip-dark' : 'heatmap-tooltip-light'" :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px' }">{{ tooltip.text }}</div>
    </Teleport>
  </div>
</template>
  
<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed, watch, inject, nextTick } from 'vue'
import { useUserStore } from '~/store/user'

interface HeatItem { date: string; count: number }
interface CalendarDay { date: string; count: number; level: number }
const props = withDefaults(defineProps<{ activeTab?: string; compact?: boolean }>(), { activeTab: 'latest', compact: false })
const userStore = useUserStore()
const rawData = ref<HeatItem[]>([])
const calendarData = ref<CalendarDay[][]>([])
const calendarContainer = ref<HTMLElement | null>(null)
const heatmapTooltip = ref<HTMLElement | null>(null)
const tooltip = ref({ visible: false, text: '', x: 0, y: 0 })
const gridViewportWidth = ref('100%')

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
  // 统一为主题友好的绿色梯度（亮：GitHub风格；暗：降低饱和度提高层次）
  const lightColors = ['#9be9a8', '#40c463', '#30a14e', '#216e39', '#0e4429']
  const darkColors = [
    'rgba(16, 185, 129, 0.28)',
    'rgba(16, 185, 129, 0.45)',
    'rgba(16, 185, 129, 0.62)',
    'rgba(16, 185, 129, 0.80)',
    'rgba(16, 185, 129, 0.98)'
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
  const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), max)

  const placeTooltip = (target: HTMLElement) => {
    const rect = target.getBoundingClientRect()
    const viewportWidth = typeof window !== 'undefined' ? window.innerWidth : rect.right
    const viewportHeight = typeof window !== 'undefined' ? window.innerHeight : rect.bottom
    const tip = heatmapTooltip.value
    const tooltipWidth = tip?.offsetWidth || 112
    const tooltipHeight = tip?.offsetHeight || 24
    const pad = 8
    const gap = 4
    const maxLeft = Math.max(pad, viewportWidth - tooltipWidth - pad)
    const idealLeft = rect.left + rect.width / 2 - tooltipWidth / 2
    const left = clamp(idealLeft, pad, maxLeft)
    const aboveTop = rect.top - tooltipHeight - gap
    const belowTop = rect.bottom + gap
    const maxTop = Math.max(pad, viewportHeight - tooltipHeight - pad)
    const centerTop = clamp(rect.top + rect.height / 2 - tooltipHeight / 2, pad, maxTop)
    const horizontalDrift = Math.abs(left + tooltipWidth / 2 - (rect.left + rect.width / 2))
    const canPlaceRight = rect.right + gap + tooltipWidth <= viewportWidth - pad
    const canPlaceLeft = rect.left - gap - tooltipWidth >= pad

    if (horizontalDrift > Math.max(18, tooltipWidth * 0.22) && (canPlaceRight || canPlaceLeft)) {
      const sideLeft = canPlaceRight ? rect.right + gap : rect.left - tooltipWidth - gap
      tooltip.value.x = clamp(sideLeft, pad, maxLeft)
      tooltip.value.y = centerTop
      return
    }

    const preferBelow = aboveTop < pad && belowTop + tooltipHeight <= viewportHeight - pad
    const rawTop = preferBelow ? belowTop : aboveTop
    tooltip.value.x = left
    tooltip.value.y = clamp(rawTop, pad, maxTop)
  }

  const showTooltip = (e: MouseEvent, day: any) => {
    tooltip.value.text = `${day.date} · ${day.count || 0} 条`
    const target = e.currentTarget as HTMLElement
    placeTooltip(target)
    tooltip.value.visible = true
    nextTick(() => placeTooltip(target))
  }
  const moveTooltip = (e: MouseEvent) => {
    placeTooltip(e.currentTarget as HTMLElement)
  }
  const hideTooltip = () => {
    tooltip.value.visible = false
  }
  
  const formatDate = (date: Date) => {
    const year = date.getFullYear()
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    return `${year}-${month}-${day}`
  }
  const parseDate = (s: string) => {
    const [y, m, d] = s.split('-').map(n => parseInt(n))
    const dt = new Date(y, (m || 1) - 1, d || 1)
    return dt
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
  const requiredColumns = () => {
    return visibleColumnCount()
  }
  const ensureFillColumns = (calendar: CalendarDay[][], dateMap: Map<string, number>) => {
    const need = requiredColumns()
    if (!need) return
    if (calendar.length >= need) return
    let deficit = need - calendar.length
    while (deficit > 0) {
      const firstWeek = calendar[0]
      const firstDateStr = firstWeek?.[0]?.date
      const start = firstDateStr ? parseDate(firstDateStr) : new Date()
      start.setDate(start.getDate() - 7)
      const weekData: CalendarDay[] = []
      for (let day = 0; day < 7; day++) {
        const ds = formatDate(start)
        const count = dateMap.get(ds) || 0
        weekData.push({ date: ds, count, level: count ? Math.min(Math.ceil(count / 2), 5) : 0 })
        start.setDate(start.getDate() + 1)
      }
      calendar.unshift(weekData)
      deficit--
    }
  }
  
  const generateCalendarData = () => {
    if (!rawData.value.length) return;
    
    // 获取最早和最新的日期
    const times = rawData.value.map(item => new Date(item.date).getTime());
    const firstDate = new Date(Math.min(...times));
    const lastDate = new Date(Math.max(...times));
    
    // 从最早发布的月份起始开始，右侧对齐到最新日期所在周的结束，确保近期日期完整可见
    const startDate = new Date(firstDate);
    startDate.setDate(1);
    const endDate = new Date(lastDate);
    endDate.setDate(endDate.getDate() + (6 - endDate.getDay()));
    
    // 创建日期映射
    const dateMap = new Map<string, number>();
    rawData.value.forEach(item => {
      if (item && item.date && item.count !== undefined) {
        dateMap.set(item.date, item.count);
      }
    });
    
    // 生成日历网格
    const calendar: CalendarDay[][] = [];
    
    // 从周日开始填充
    let currentDate = new Date(startDate);
    currentDate.setDate(currentDate.getDate() - currentDate.getDay());
    
    while (currentDate <= endDate) {
      const weekData: CalendarDay[] = [];
      for (let day = 0; day < 7; day++) {
        const dateStr = formatDate(currentDate);
        const count = dateMap.get(dateStr) || 0;
        weekData.push({
          date: dateStr,
          count: count,
          level: count ? Math.min(Math.ceil(count / 2), 5) : 0
        });
        currentDate.setDate(currentDate.getDate() + 1);
      }
      calendar.push(weekData);
    }
    ensureFillColumns(calendar, dateMap)
    calendarData.value = calendar;
  }
  
  const generateEmptyCalendar = () => {
    const today = new Date()
    const startDate = new Date(today)
    startDate.setMonth(today.getMonth() - 11)
    startDate.setDate(1)
    const endDate = new Date(startDate)
    endDate.setMonth(startDate.getMonth() + 11)
    endDate.setDate(endDate.getDate() + (6 - endDate.getDay()))

    const calendar: CalendarDay[][] = []
    let currentDate = new Date(startDate)
    currentDate.setDate(currentDate.getDate() - currentDate.getDay())

    while (currentDate <= endDate) {
      const weekData: CalendarDay[] = []
      for (let day = 0; day < 7; day++) {
        weekData.push({ date: formatDate(currentDate), count: 0, level: 0 })
        currentDate.setDate(currentDate.getDate() + 1)
      }
      calendar.push(weekData)
    }
    ensureFillColumns(calendar, new Map<string, number>())
    calendarData.value = calendar
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

  onMounted(() => {
    generateEmptyCalendar()
    syncGridViewportWidth()
    window.addEventListener('resize', refreshCalendarLayout)
    fetchHeatmapData()
  })

  onBeforeUnmount(() => {
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
    box-shadow: 0 0 0 2px rgba(96, 165, 250, 0.32);
  }
  .heatmap-light .heatmap-day { border-color: #cbd5e1; }
  .heatmap-dark .heatmap-day { border-color: rgba(255,255,255,0.12); }
  
  .heatmap-day:hover {
    border-color: rgba(96, 165, 250, 0.72);
    box-shadow: 0 0 0 2px rgba(96, 165, 250, 0.24);
  }
  .heatmap-tooltip {
    position: fixed;
    padding: 4px 8px;
    border-radius: 6px;
    font-size: 11px;
    line-height: 1.2;
    white-space: nowrap;
    pointer-events: none;
    z-index: 5000;
    will-change: left, top;
  }
  .heatmap-tooltip-light {
    background: rgba(255,255,255,0.95);
    color: #111827;
    border: 1px solid rgba(0,0,0,0.18);
    box-shadow: 0 4px 12px rgba(0,0,0,0.12);
  }
  .heatmap-tooltip-dark {
    background: rgba(12, 17, 23, 0.92);
    color: #ffffff;
    border: 1px solid rgba(255,255,255,0.20);
    box-shadow: 0 6px 16px rgba(0,0,0,0.35);
    backdrop-filter: blur(6px);
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
