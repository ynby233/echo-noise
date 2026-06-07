<template>
  <div class="calendar-widget">
    <div class="calendar-head">
      <button type="button" class="calendar-nav" aria-label="上个月" @click="moveMonth(-1)">
        <UIcon name="i-heroicons-chevron-left" class="w-4 h-4" />
      </button>
      <div class="calendar-title">
        <UIcon name="i-heroicons-calendar-days" class="w-4 h-4" />
        <span>{{ monthTitle }}</span>
      </div>
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
        class="calendar-day"
        :class="{
          'is-muted': !day.inMonth,
          'is-today': day.isToday,
          'is-selected': day.selected,
          'has-count': day.count > 0
        }"
        :title="`${day.date}，${day.count} 条笔记`"
        @click="selectDay(day)"
      >
        <span class="day-number">{{ day.day }}</span>
        <span v-if="day.count > 0" class="day-count">{{ day.count > 99 ? '99+' : day.count }}</span>
        <span class="day-tooltip">{{ day.count }} 条笔记</span>
      </button>
    </div>

    <div class="calendar-foot">
      <button type="button" class="calendar-today" @click="goToday">今天</button>
      <button v-if="selectedDate" type="button" class="calendar-clear" @click="emit('select-date', '')">清除筛选</button>
      <span v-else class="calendar-scope">{{ scopeLabel }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
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
const scopeLabel = computed(() => personalActive.value ? '个人笔记' : '全部可见笔记')
const monthTitle = computed(() => `${currentMonth.value.getFullYear()}年${currentMonth.value.getMonth() + 1}月`)

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

watch([() => props.activeTab, () => userStore.isLogin, () => currentUserId.value], fetchCounts)
watch(() => props.selectedDate, (value) => {
  const parsed = parseLocalDate(value)
  if (parsed) currentMonth.value = startOfMonth(parsed)
})

onMounted(() => {
  fetchCounts()
})
</script>

<style scoped>
.calendar-widget {
  padding: 8px;
  min-width: 0;
}

.calendar-head,
.calendar-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.calendar-head {
  margin-bottom: 10px;
}

.calendar-title {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 0;
  font-size: 13px;
  font-weight: 700;
}

.calendar-nav {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  border: 1px solid rgba(148, 163, 184, 0.28);
  background: rgba(148, 163, 184, 0.08);
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

.calendar-nav:hover {
  background: rgba(249, 115, 22, 0.12);
  border-color: rgba(249, 115, 22, 0.34);
}

.calendar-weekdays,
.calendar-grid {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: 4px;
}

.calendar-weekdays {
  margin-bottom: 6px;
  font-size: 11px;
  opacity: 0.66;
  text-align: center;
}

.calendar-day {
  position: relative;
  aspect-ratio: 1 / 1;
  min-width: 0;
  border-radius: 7px;
  border: 1px solid transparent;
  background: rgba(148, 163, 184, 0.08);
  color: inherit;
  font-size: 12px;
  line-height: 1;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.calendar-day:hover {
  background: rgba(249, 115, 22, 0.12);
  border-color: rgba(249, 115, 22, 0.28);
}

.calendar-day.is-muted {
  opacity: 0.36;
}

.calendar-day.is-today {
  border-color: rgba(59, 130, 246, 0.46);
}

.calendar-day.is-selected {
  background: rgba(249, 115, 22, 0.95);
  border-color: rgba(249, 115, 22, 1);
  color: #fff;
}

.day-number {
  position: absolute;
  top: 6px;
  left: 6px;
}

.day-count {
  position: absolute;
  right: 4px;
  bottom: 4px;
  max-width: calc(100% - 8px);
  min-width: 14px;
  height: 14px;
  padding: 0 3px;
  border-radius: 999px;
  background: rgba(14, 165, 233, 0.16);
  color: rgb(2, 132, 199);
  font-size: 9px;
  line-height: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.calendar-day.is-selected .day-count {
  background: rgba(255, 255, 255, 0.24);
  color: #fff;
}

.day-tooltip {
  position: absolute;
  left: 50%;
  bottom: calc(100% + 6px);
  z-index: 3;
  transform: translateX(-50%);
  display: none;
  white-space: nowrap;
  padding: 4px 6px;
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.92);
  color: #fff;
  font-size: 11px;
  pointer-events: none;
}

.calendar-day:hover .day-tooltip {
  display: block;
}

.calendar-foot {
  min-height: 26px;
  margin-top: 9px;
  font-size: 12px;
}

.calendar-today,
.calendar-clear {
  height: 24px;
  padding: 0 8px;
  border-radius: 6px;
  border: 1px solid rgba(148, 163, 184, 0.28);
  background: rgba(148, 163, 184, 0.08);
}

.calendar-today:hover,
.calendar-clear:hover {
  border-color: rgba(249, 115, 22, 0.34);
  color: rgb(249, 115, 22);
}

.calendar-scope {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  opacity: 0.66;
}

:global(html.dark) .day-count {
  background: rgba(125, 211, 252, 0.18);
  color: rgb(125, 211, 252);
}
</style>
