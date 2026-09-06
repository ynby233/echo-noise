<template>
  <div class="calendar-widget" :class="{ 'calendar-theme-dark': isDarkTheme }">
    <div class="calendar-head">
      <button type="button" class="calendar-nav" aria-label="上个月" @click="moveMonth(-1)">
        <UIcon name="i-heroicons-chevron-left" class="w-4 h-4" />
      </button>
      <button type="button" class="calendar-today" @click="goToday">今天</button>
      <div class="calendar-picker">
        <UIcon name="i-heroicons-calendar-days" class="w-4 h-4 calendar-picker-icon" />
        <button
          ref="yearPickerButton"
          type="button"
          class="calendar-select calendar-select-button year-select"
          aria-label="选择年份"
          aria-haspopup="listbox"
          :aria-expanded="openPicker === 'year'"
          @click="togglePicker('year')"
        >
          <span>{{ currentYear }}年</span>
          <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
        </button>
        <button
          ref="monthPickerButton"
          type="button"
          class="calendar-select calendar-select-button month-select"
          aria-label="选择月份"
          aria-haspopup="listbox"
          :aria-expanded="openPicker === 'month'"
          @click="togglePicker('month')"
        >
          <span>{{ currentMonthNumber }}月</span>
          <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
        </button>
      </div>
      <span class="calendar-scope">{{ scopeLabel }}</span>
      <button type="button" class="calendar-nav" aria-label="下个月" @click="moveMonth(1)">
        <UIcon name="i-heroicons-chevron-right" class="w-4 h-4" />
      </button>
    </div>

    <div class="calendar-weekdays">
      <span v-for="label in weekLabels" :key="label">{{ label }}</span>
    </div>

    <div class="calendar-grid" :aria-busy="loading ? 'true' : 'false'">
      <button
        v-for="day in calendarDays"
        :key="day.key"
        type="button"
        class="calendar-day nw-tooltip-anchor"
        :class="{
          'is-muted': !day.inMonth,
          'is-today': day.isToday,
          'is-selected': day.selected,
          'has-count': day.count > 0
        }"
        :aria-label="`${day.date}，${day.count} 条笔记`"
        :data-tooltip="`${day.count} 条笔记`"
        :style="day.count > 0 ? { '--calendar-count-bg': `rgba(234, 88, 12, ${0.14 + 0.66 * day.count / (day.count + 5)})` } : undefined"
        :aria-pressed="day.selected"
        @click="selectDay(day)"
      >
        <span class="day-number">{{ day.day }}</span>
      </button>
    </div>

    <div v-if="selectedDate" class="calendar-foot">
      <button type="button" class="calendar-clear" @click="emit('select-date', '')">清除筛选</button>
    </div>

    <Teleport to="body">
      <div
        v-if="openPicker"
        ref="pickerMenu"
        class="calendar-floating-menu nw-floating-menu"
        :class="[`is-${openPicker}`, { 'is-dark': isDarkTheme }]"
        :style="pickerMenuStyle"
        role="listbox"
        @mousedown.stop
      >
        <button
          v-for="option in pickerOptions"
          :key="option.value"
          type="button"
          class="calendar-floating-option nw-floating-option"
          :class="{ 'is-selected': option.selected }"
          role="option"
          :aria-selected="option.selected"
          @click="selectPickerValue(option.value)"
        >
          {{ option.label }}
        </button>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRuntimeConfig } from '#imports'
import { useUserStore } from '~/store/user'

type CalendarCountRow = {
  date?: string
  Date?: string
  count?: number
  Count?: number
}

type CalendarDay = {
  key: string
  date: string
  day: number
  inMonth: boolean
  isToday: boolean
  selected: boolean
  count: number
}

const props = defineProps({
  activeTab: {
    type: String,
    default: 'latest'
  },
  selectedDate: {
    type: String,
    default: ''
  }
})

const emit = defineEmits<{
  (e: 'select-date', date: string): void
}>()

const baseApi = (useRuntimeConfig().public.baseApi || '/api').replace(/\/$/, '')
const userStore = useUserStore()
const contentTheme = inject('contentTheme', ref('light')) as { value: string }
const isDarkTheme = computed(() => contentTheme.value === 'dark')
const weekLabels = ['一', '二', '三', '四', '五', '六', '日']
const loading = ref(false)
const countMap = ref<Record<string, number>>({})

const startOfMonth = (date: Date) => new Date(date.getFullYear(), date.getMonth(), 1)
const currentMonth = ref(startOfMonth(new Date()))

const pad2 = (value: number) => String(value).padStart(2, '0')
const formatLocalDate = (date: Date) => `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
const parseLocalDate = (value: string) => {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(String(value || ''))
  if (!match) return null
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]))
  return Number.isNaN(date.getTime()) ? null : date
}

const today = computed(() => formatLocalDate(new Date()))
const currentUserId = computed(() => Number((userStore.user as any)?.userid || (userStore.user as any)?.id || (userStore.user as any)?.user_id || 0))
const personalActive = computed(() => props.activeTab === 'personal')
const scopeLabel = computed(() => personalActive.value ? '个人' : '全部')
const currentYear = computed(() => currentMonth.value.getFullYear())
const currentMonthNumber = computed(() => currentMonth.value.getMonth() + 1)
type PickerType = 'year' | 'month'

const CALENDAR_MIN_YEAR = 1971
const CALENDAR_MAX_YEAR = 2099
const monthOptions = Array.from({ length: 12 }, (_, index) => index + 1)
const openPicker = ref<PickerType | ''>('')
const yearPickerButton = ref<HTMLElement | null>(null)
const monthPickerButton = ref<HTMLElement | null>(null)
const pickerMenu = ref<HTMLElement | null>(null)
const pickerMenuStyle = ref<Record<string, string>>({})
const yearOptions = computed(() => Array.from({ length: CALENDAR_MAX_YEAR - CALENDAR_MIN_YEAR + 1 }, (_, index) => CALENDAR_MIN_YEAR + index))

const pickerOptions = computed(() => {
  if (openPicker.value === 'year') {
    return yearOptions.value.map((year) => ({ value: year, label: `${year}年`, selected: year === currentYear.value }))
  }
  if (openPicker.value === 'month') {
    return monthOptions.map((month) => ({ value: month, label: `${month}月`, selected: month === currentMonthNumber.value }))
  }
  return []
})

const calendarDays = computed<CalendarDay[]>(() => {
  const first = currentMonth.value
  const startOffset = (first.getDay() + 6) % 7
  const days: CalendarDay[] = []
  for (let i = 0; i < 42; i += 1) {
    const date = new Date(first.getFullYear(), first.getMonth(), 1 - startOffset + i)
    const value = formatLocalDate(date)
    days.push({
      key: value,
      date: value,
      day: date.getDate(),
      inMonth: date.getMonth() === first.getMonth(),
      isToday: value === today.value,
      selected: value === props.selectedDate,
      count: countMap.value[value] || 0
    })
  }
  return days
})

const fetchCounts = async () => {
  if (personalActive.value && (!userStore.isLogin || !currentUserId.value)) {
    countMap.value = {}
    return
  }
  loading.value = true
  try {
    const query = new URLSearchParams()
    if (personalActive.value && currentUserId.value) query.set('authorId', String(currentUserId.value))
    const suffix = query.toString() ? `?${query.toString()}` : ''
    const response = await fetch(`${baseApi}/messages/calendar${suffix}`, {
      credentials: 'include',
      headers: { Accept: 'application/json' }
    })
    const data = await response.json().catch(() => null)
    if (!response.ok || data?.code !== 1 || !Array.isArray(data?.data)) {
      countMap.value = {}
      return
    }
    const next: Record<string, number> = {}
    for (const row of data.data as CalendarCountRow[]) {
      const date = String(row.date || row.Date || '').trim()
      const count = Number(row.count ?? row.Count ?? 0)
      if (date) next[date] = Number.isFinite(count) ? count : 0
    }
    countMap.value = next
  } catch {
    countMap.value = {}
  } finally {
    loading.value = false
  }
}

const moveMonth = (delta: number) => {
  currentMonth.value = new Date(currentMonth.value.getFullYear(), currentMonth.value.getMonth() + delta, 1)
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
  const viewport = window.visualViewport
  const offsetLeft = viewport?.offsetLeft || 0
  const offsetTop = viewport?.offsetTop || 0
  return {
    left: (rect.left + offsetLeft) / scale,
    right: (rect.right + offsetLeft) / scale,
    top: (rect.top + offsetTop) / scale,
    bottom: (rect.bottom + offsetTop) / scale,
    width: rect.width / scale,
    height: rect.height / scale
  }
}

const updatePickerPosition = () => {
  if (!openPicker.value || typeof window === 'undefined') return
  const trigger = openPicker.value === 'year' ? yearPickerButton.value : monthPickerButton.value
  if (!trigger) return
  const pickerType = openPicker.value
  const scale = getFixedCoordinateScale()
  const rect = getFixedRect(trigger, scale)
  const viewport = getFixedViewport(scale)
  const menu = pickerMenu.value
  const menuWidth = Math.ceil(rect.width)
  const menuHeight = menu?.offsetHeight || (pickerType === 'year' ? 204 : 167)
  const pad = 8
  const gap = 4
  const minLeft = viewport.left + pad
  const maxLeft = Math.max(minLeft, viewport.right - menuWidth - pad)
  const idealLeft = rect.left + rect.width / 2 - menuWidth / 2
  const minTop = viewport.top + pad
  const maxTop = Math.max(minTop, viewport.bottom - menuHeight - pad)
  const belowTop = rect.bottom + gap
  const aboveTop = rect.top - menuHeight - gap
  const idealTop = belowTop + menuHeight <= viewport.bottom - pad
    ? belowTop
    : (aboveTop >= minTop ? aboveTop : belowTop)
  pickerMenuStyle.value = {
    position: 'fixed',
    left: `${clamp(idealLeft, minLeft, maxLeft)}px`,
    top: `${clamp(idealTop, minTop, maxTop)}px`,
    right: 'auto',
    bottom: 'auto',
    transform: 'none',
    width: `${menuWidth}px`,
    minWidth: `${menuWidth}px`,
    visibility: 'visible'
  }
}

const schedulePickerPosition = () => {
  updatePickerPosition()
  if (typeof window !== 'undefined') {
    window.requestAnimationFrame(() => {
      updatePickerPosition()
      window.requestAnimationFrame(updatePickerPosition)
    })
  }
}

const scrollSelectedPickerOptionToTop = () => {
  const menu = pickerMenu.value
  const selected = menu?.querySelector<HTMLElement>('.calendar-floating-option.is-selected')
  if (!menu || !selected || typeof window === 'undefined') return
  const options = Array.from(menu.querySelectorAll<HTMLElement>('.calendar-floating-option'))
  const selectedIndex = options.indexOf(selected)
  if (selectedIndex < 0) return
  const style = window.getComputedStyle(menu)
  const gap = Number.parseFloat(style.rowGap || style.gap || '0')
  const paddingTop = Number.parseFloat(style.paddingTop || '0')
  const step = selected.offsetHeight + (Number.isFinite(gap) ? gap : 0)
  const maxScrollTop = Math.max(0, menu.scrollHeight - menu.clientHeight)
  menu.scrollTop = clamp(paddingTop + selectedIndex * step, 0, maxScrollTop)
}

const togglePicker = async (type: PickerType) => {
  openPicker.value = openPicker.value === type ? '' : type
  if (openPicker.value) {
    pickerMenuStyle.value = {
      position: 'fixed',
      left: '0px',
      top: '0px',
      right: 'auto',
      bottom: 'auto',
      visibility: 'hidden'
    }
    await nextTick()
    scrollSelectedPickerOptionToTop()
    schedulePickerPosition()
  }
}

const closePicker = () => {
  openPicker.value = ''
}

const selectPickerValue = (value: number) => {
  if (openPicker.value === 'year' && Number.isFinite(value)) {
    currentMonth.value = new Date(value, currentMonth.value.getMonth(), 1)
  } else if (openPicker.value === 'month' && Number.isFinite(value)) {
    currentMonth.value = new Date(currentMonth.value.getFullYear(), value - 1, 1)
  }
  closePicker()
}

const selectDay = (day: CalendarDay) => {
  const parsed = parseLocalDate(day.date)
  if (parsed && !day.inMonth) currentMonth.value = startOfMonth(parsed)
  emit('select-date', props.selectedDate === day.date ? '' : day.date)
}

const goToday = () => {
  const now = new Date()
  currentMonth.value = startOfMonth(now)
  emit('select-date', formatLocalDate(now))
}

const handleDocumentPointerDown = (event: MouseEvent) => {
  if (!openPicker.value) return
  const target = event.target as Node | null
  if (!target) return
  if (pickerMenu.value?.contains(target)) return
  if (yearPickerButton.value?.contains(target)) return
  if (monthPickerButton.value?.contains(target)) return
  closePicker()
}

const handleViewportChange = () => {
  if (openPicker.value) updatePickerPosition()
}

watch([() => props.activeTab, () => userStore.isLogin, () => currentUserId.value], fetchCounts)
watch(() => props.selectedDate, (value) => {
  const parsed = parseLocalDate(value)
  if (parsed) currentMonth.value = startOfMonth(parsed)
})

onMounted(() => {
  fetchCounts()
  document.addEventListener('mousedown', handleDocumentPointerDown)
  window.addEventListener('resize', handleViewportChange)
  window.addEventListener('scroll', handleViewportChange, true)
  window.visualViewport?.addEventListener('resize', handleViewportChange)
  window.visualViewport?.addEventListener('scroll', handleViewportChange)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', handleDocumentPointerDown)
  window.removeEventListener('resize', handleViewportChange)
  window.removeEventListener('scroll', handleViewportChange, true)
  window.visualViewport?.removeEventListener('resize', handleViewportChange)
  window.visualViewport?.removeEventListener('scroll', handleViewportChange)
})
</script>

<style scoped>
.calendar-widget {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
  --calendar-section-gap: 5px;
  padding: 10px 9px;
  --calendar-control-bg: rgba(148, 163, 184, 0.08);
  --calendar-control-bg-hover: rgba(249, 115, 22, 0.12);
  --calendar-control-border: rgba(15, 23, 42, 0.10);
  --calendar-control-border-hover: rgba(249, 115, 22, 0.34);
  --calendar-control-text: rgba(30, 41, 59, 0.88);
  --calendar-weekday-text: rgba(71, 85, 105, 0.86);
  --calendar-day-text: rgba(30, 41, 59, 0.88);
  --calendar-muted-day-text: rgba(100, 116, 139, 0.62);
  --calendar-option-bg: #1f2937;
}

.calendar-head,
.calendar-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 3px;
}

.calendar-head {
  margin-bottom: var(--calendar-section-gap);
}

.calendar-widget.calendar-theme-dark,
:global(html.dark) .calendar-widget {
  --calendar-control-bg: rgba(255, 255, 255, 0.075);
  --calendar-control-bg-hover: rgba(249, 115, 22, 0.20);
  --calendar-control-border: rgba(255, 255, 255, 0.14);
  --calendar-control-border-hover: rgba(249, 115, 22, 0.50);
  --calendar-control-text: rgba(248, 250, 252, 0.86);
  --calendar-weekday-text: rgba(226, 232, 240, 0.72);
  --calendar-day-text: rgba(248, 250, 252, 0.88);
  --calendar-muted-day-text: rgba(203, 213, 225, 0.50);
}

.calendar-widget.calendar-theme-dark .calendar-today,
.calendar-widget.calendar-theme-dark .calendar-clear,
.calendar-widget.calendar-theme-dark .calendar-scope,
.calendar-widget.calendar-theme-dark .calendar-select,
.calendar-widget.calendar-theme-dark .calendar-nav,
.calendar-widget.calendar-theme-dark .calendar-weekdays,
.calendar-widget.calendar-theme-dark .calendar-day,
:global(html.dark) .calendar-widget .calendar-today,
:global(html.dark) .calendar-widget .calendar-clear,
:global(html.dark) .calendar-widget .calendar-scope,
:global(html.dark) .calendar-widget .calendar-select,
:global(html.dark) .calendar-widget .calendar-nav,
:global(html.dark) .calendar-widget .calendar-weekdays,
:global(html.dark) .calendar-widget .calendar-day {
  color: var(--calendar-control-text) !important;
}

.calendar-widget.calendar-theme-dark .calendar-day.is-muted,
:global(html.dark) .calendar-widget .calendar-day.is-muted {
  color: var(--calendar-muted-day-text) !important;
}

.calendar-picker {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 3px;
  min-width: 0;
  flex: 0 0 auto;
}

.calendar-picker-icon {
  display: none;
}

.calendar-select {
  height: 24px;
  min-width: 0;
  padding: 0 7px;
  border-radius: 8px;
  border: 1px solid var(--calendar-control-border);
  background: var(--calendar-control-bg);
  color: var(--calendar-control-text);
  font-size: 11px;
  font-weight: 650;
  line-height: 1;
  outline: none;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.calendar-select-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 3px;
}

.calendar-select-button svg {
  opacity: 0.72;
}

.calendar-select:hover,
.calendar-select:focus-visible {
  background: var(--calendar-control-bg-hover);
  border-color: var(--calendar-control-border-hover);
}

.year-select {
  width: 75px;
}

.month-select {
  width: 55px;
}

.calendar-nav {
  width: 22px;
  height: 22px;
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  border: 1px solid var(--calendar-control-border);
  background: var(--calendar-control-bg);
  color: var(--calendar-control-text);
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.calendar-nav:hover {
  background: var(--calendar-control-bg-hover);
  border-color: var(--calendar-control-border-hover);
}

.calendar-grid,
.calendar-weekdays {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: 2px;
}

.calendar-weekdays {
  margin-bottom: 3px;
  color: var(--calendar-weekday-text);
  font-size: 10px;
  font-weight: 600;
  opacity: 0.78;
  text-align: center;
}

.calendar-day {
  position: relative;
  height: 18px;
  min-width: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  border: 1px solid transparent;
  background: var(--calendar-count-bg, rgba(148, 163, 184, 0.08));
  color: var(--calendar-day-text);
  font-size: 11px;
  line-height: 1;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.calendar-day:hover {
  border-color: rgba(249, 115, 22, 0.28);
}

.calendar-day:focus-visible {
  outline: 2px solid rgb(249, 115, 22);
  outline-offset: 1px;
}

.calendar-day.is-muted {
  color: var(--calendar-muted-day-text);
}

.calendar-day.is-today {
  border-color: rgba(37, 99, 235, 0.86);
}

.calendar-day.is-selected {
  border-color: rgba(249, 115, 22, 1);
  box-shadow: inset 0 0 0 1px rgba(249, 115, 22, 0.85);
}

.day-number {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 999px;
}

.calendar-day.is-today .day-number {
  background: rgb(37, 99, 235);
  color: #fff;
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.22);
}

.calendar-foot {
  min-height: 18px;
  margin-top: var(--calendar-section-gap);
  font-size: 11px;
  justify-content: center;
}

.calendar-today,
.calendar-clear {
  height: 24px;
  padding: 0 7px;
  border-radius: 8px;
  border: 1px solid var(--calendar-control-border);
  background: var(--calendar-control-bg);
  color: var(--calendar-control-text);
  font-size: 11px;
  font-weight: 650;
  line-height: 1;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.calendar-today:hover,
.calendar-clear:hover {
  background: var(--calendar-control-bg-hover);
  border-color: var(--calendar-control-border-hover);
}

.calendar-scope {
  flex: 0 0 auto;
  min-width: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 1px;
  border: 0;
  background: transparent;
  color: var(--calendar-control-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  font-weight: 650;
}
</style>

<style>
.calendar-floating-menu {
  position: fixed;
  z-index: 5002;
  box-sizing: border-box;
  display: grid;
  gap: 4px;
  max-height: 204px;
  overflow-y: auto;
  padding: 8px;
  border-radius: 10px;
  border: 1px solid var(--nw-floating-border);
  background: var(--nw-floating-bg);
  color: var(--nw-floating-text);
  box-shadow: var(--nw-floating-shadow);
  scrollbar-width: none;
}

.calendar-floating-menu.is-dark {
  --nw-floating-bg: rgba(15, 23, 42, 0.96);
  --nw-floating-text: #f8fafc;
  --nw-floating-border: rgba(255, 255, 255, 0.16);
  --nw-floating-hover-bg: rgba(249, 115, 22, 0.26);
  --nw-floating-hover-border: rgba(249, 115, 22, 0.58);
  --nw-floating-selected-bg: rgba(249, 115, 22, 0.24);
  --nw-floating-selected-border: rgba(251, 146, 60, 0.52);
  --nw-floating-shadow: 0 18px 38px rgba(0, 0, 0, 0.42);
}

.calendar-floating-menu::-webkit-scrollbar {
  width: 0;
  height: 0;
}

.calendar-floating-menu.is-year {
  grid-template-columns: minmax(0, 1fr);
}

.calendar-floating-menu.is-month {
  grid-template-columns: minmax(0, 1fr);
  gap: 3px;
  max-height: 167px;
  padding: 4px;
}

.calendar-floating-menu.is-month .calendar-floating-option {
  min-height: 24px;
  padding: 0 4px;
}

.calendar-floating-option {
  box-sizing: border-box;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 28px;
  min-width: 0;
  width: 100%;
  padding: 0 6px;
  border-radius: 8px;
  border: 1px solid transparent;
  color: inherit;
  font-size: 12px;
  font-weight: 650;
  line-height: 1;
  text-align: center;
  white-space: nowrap;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.calendar-floating-option:hover,
.calendar-floating-option:focus-visible {
  border-color: var(--nw-floating-hover-border);
  background: var(--nw-floating-hover-bg);
  outline: none;
}

.calendar-floating-option.is-selected {
  border-color: var(--nw-floating-selected-border);
  background: var(--nw-floating-selected-bg);
  color: var(--nw-floating-text);
}
</style>
