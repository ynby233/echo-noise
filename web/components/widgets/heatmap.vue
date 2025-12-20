<template>
  <div class="calendar-wrapper">
    <div class="calendar-container" ref="calendarContainer" :class="isDark ? 'heatmap-dark' : 'heatmap-light'">
      <div class="heatmap-grid">
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
      <div v-if="tooltip.visible" class="heatmap-tooltip" :class="isDark ? 'heatmap-tooltip-dark' : 'heatmap-tooltip-light'" :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px' }">{{ tooltip.text }}</div>
    </div>
  </div>
</template>
  
<script setup lang="ts">
import { ref, onMounted, computed, nextTick, inject } from 'vue'

interface HeatItem { date: string; count: number }
interface CalendarDay { date: string; count: number; level: number }
const rawData = ref<HeatItem[]>([])
const calendarData = ref<CalendarDay[][]>([])
const calendarContainer = ref<HTMLElement | null>(null)
const tooltip = ref({ visible: false, text: '', x: 0, y: 0 })

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
  const fetchHeatmapData = async () => {
    try {
      const response = await fetch('/api/messages/calendar')
      const data = await response.json()
      
      if (data && data.code === 1 && data.data && data.data.length > 0) {
        rawData.value = data.data
        generateCalendarData()
      } else {
        generateTestData()
      }
    } catch (error) {
      console.error('获取热力图数据失败:', error)
      generateTestData()
    }
  }
  const showTooltip = (e: MouseEvent, day: any) => {
    tooltip.value.text = `${day.date} · ${day.count || 0} 条`
    const target = e.target as HTMLElement
    const tRect = target.getBoundingClientRect()
    const cRect = (calendarContainer.value as HTMLElement)?.getBoundingClientRect()
    const x = tRect.left - (cRect?.left || 0) + tRect.width / 2
    const y = tRect.top - (cRect?.top || 0) - 6
    tooltip.value.x = x
    tooltip.value.y = y
    tooltip.value.visible = true
  }
  const moveTooltip = (e: MouseEvent) => {
    const target = e.target as HTMLElement
    const tRect = target.getBoundingClientRect()
    const cRect = (calendarContainer.value as HTMLElement)?.getBoundingClientRect()
    const x = tRect.left - (cRect?.left || 0) + tRect.width / 2
    const y = tRect.top - (cRect?.top || 0) - 6
    tooltip.value.x = x
    tooltip.value.y = y
  }
  const hideTooltip = () => {
    tooltip.value.visible = false
  }
  
  const generateTestData = () => {
    const today = new Date()
    const startDate = new Date(today)
    startDate.setMonth(today.getMonth() - 11) // 从11个月前开始
    startDate.setDate(1) // 从月初开始
    
    const testData: HeatItem[] = []
    let currentDate = new Date(startDate)
    
    while (currentDate <= today) {
      const count = Math.random() > 0.7 ? Math.floor(Math.random() * 10) + 1 : 0
      testData.push({
        date: formatDate(currentDate),
        count: count
      })
      currentDate.setDate(currentDate.getDate() + 1)
    }
    
    rawData.value = testData
    generateCalendarData()
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
  const requiredColumns = () => {
    const w = calendarContainer.value?.clientWidth || 0
    const isMobile = typeof window !== 'undefined' && window.matchMedia('(max-width: 1024px)').matches
    const daySize = isMobile ? 8 : 12
    const gap = 3
    if (!w) return 0
    return Math.max(0, Math.floor((w + gap) / (daySize + gap)))
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

  onMounted(() => {
    generateEmptyCalendar()
    fetchHeatmapData()
  })
  </script>
  
  <style scoped>
  .calendar-wrapper {
    position: relative;
    overflow: visible;
    margin: 0;
    padding: 0;
    width: 100%;
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
    padding-bottom: 0;
    scroll-behavior: smooth;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: thin;
    width: 100%;
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
    border-radius: 2px;
    transition: all 0.2s ease;
    border: 1px solid transparent;
  }
  .heatmap-light .heatmap-day { border-color: #cbd5e1; }
  .heatmap-dark .heatmap-day { border-color: rgba(255,255,255,0.12); }
  
  .heatmap-day:hover {
    transform: scale(1.2);
  }
  .heatmap-tooltip {
    position: absolute;
    transform: translate(-50%, -100%);
    padding: 4px 8px;
    border-radius: 6px;
    font-size: 11px;
    line-height: 1.2;
    white-space: nowrap;
    pointer-events: none;
    z-index: 1000;
  }
  .heatmap-tooltip-light {
    background: rgba(255,255,255,0.95);
    color: #111827;
    border: 1px solid rgba(0,0,0,0.18);
    box-shadow: 0 4px 12px rgba(0,0,0,0.12);
  }
  .heatmap-tooltip-dark {
    background: rgba(36,43,50,0.92);
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
