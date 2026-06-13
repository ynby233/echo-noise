<template>
  <div :class="containerClass">

    <div class="editor-box">
      <VditorEditor ref="vditorEditor" v-model="MessageContent" :theme="contentTheme" @ready="onEditorReady" />
      <div class="editor-toolbar">
        <div class="toolbar-left">
          <input
            id="file-input"
            ref="fileInput"
            type="file"
            accept="image/*"
            multiple
            @change="addImage"
            class="hidden"
            placeholder="选择图片"
          />
          <!-- 视频上传按钮 -->
          <VideoUpload
            @video-uploaded="handleVideoUploaded"
            @upload-progress="handleVideoUploadProgress"
          />
          <button type="button" class="tb-btn nw-tooltip-anchor" data-tooltip="上传图片" aria-label="上传图片" @click="triggerFileInput"><UIcon name="i-mdi-image-plus-outline" class="w-5 h-5" /></button>
          <!-- 新增图床上传按钮 -->
          <button type="button" class="tb-btn nw-tooltip-anchor" data-tooltip="图床上传" aria-label="图床上传" @click="showImageUploader = true"><UIcon name="i-mdi-cloud-upload-outline" class="w-5 h-5" /></button>
          <button type="button" class="tb-btn has-label notify-btn nw-tooltip-anchor" :class="{ 'is-enabled': enableNotify }" :data-tooltip="enableNotify ? '关闭推送' : '开启推送'" :aria-label="enableNotify ? '关闭推送' : '开启推送'" @click="toggleNotify">
            <UIcon :name="enableNotify ? 'i-mdi-bell-off-outline' : 'i-mdi-bell-ring-outline'" class="w-5 h-5" />
            <span class="notify-label">{{ enableNotify ? '关闭' : '开启' }}</span>
          </button>
          <div ref="visibilityControlRef" class="visibility-control nw-tooltip-anchor" :data-tooltip="`可见范围：${visibilityLabel}`">
            <UIcon :name="visibilityIcon" class="w-5 h-5" />
            <button
              type="button"
              class="visibility-select visibility-trigger"
              aria-label="可见范围"
              aria-haspopup="listbox"
              :aria-expanded="showVisibilityMenu"
              @click="toggleVisibilityMenu"
            >
              <span>{{ visibilityLabel }}</span>
              <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
            </button>
          </div>
          <div v-if="canSetPublishTime" ref="publishTimeControlRef" class="publish-time-control nw-tooltip-anchor" :data-tooltip="publishTimeLabel === '选择时间' ? '自定义发布时间' : `发布时间：${publishTimeLabel}`">
            <UIcon name="i-mdi-calendar-clock-outline" class="w-5 h-5" />
            <button
              type="button"
              class="publish-time-input publish-time-trigger"
              aria-label="发布时间"
              aria-haspopup="dialog"
              :aria-expanded="showPublishDateMenu"
              @click="togglePublishDateMenu"
            >
              <span>{{ publishTimeLabel }}</span>
              <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
            </button>
          </div>
        </div>
        <div class="toolbar-right">
          <span v-if="isEditorLoading" class="text-xs text-orange-400 flex items-center" style="margin-right: auto">
            <UIcon name="i-heroicons-arrow-path" class="w-4 h-4 animate-spin mr-1" />
            加载中...
          </span>
          <button type="button" class="tb-btn danger nw-tooltip-anchor" data-tooltip="清除" aria-label="清除" @click="clearForm"><UIcon name="i-heroicons-trash" class="w-5 h-5" /></button>
          <button type="button" class="tb-btn primary nw-tooltip-anchor" data-tooltip="发布" aria-label="发布" @click="addMessage"><UIcon name="i-mdi-send" class="w-5 h-5" /></button>
        </div>
        <div v-if="activeUploadPercent > 0 && activeUploadPercent < 100" class="upload-progress">
          <div class="upload-progress-track">
            <div
              class="upload-progress-fill"
              :class="activeUploadKind"
              :style="{ width: activeUploadPercent + '%' }"
            />
          </div>
          <div class="upload-progress-text">{{ activeUploadLabel }} {{ activeUploadPercent }}%</div>
        </div>
      </div>
    </div>

  <!-- 内容预览区域 - 仅在有内容时显示 -->
  <div v-if="MessageContentHtml" class="mx-auto w-full sm:max-w-4xl mt-4 preview-card">
    <div :class="[previewProseClass, 'max-w-none editor-preview']">
      <div v-html="MessageContentHtml"></div>
    </div>
  </div>

  <SearchMode 
    v-model="showSearchModal" 
    @search-result="handleSearchResult" 
  />
  <ImageHostingUploader
  v-if="showImageUploader"
  :position="imageUploaderPosition"
  @close="showImageUploader = false"
  @upload-success="handleImageHostingSuccess"
  @update:position="handlePositionUpdate"
/>

  <Teleport to="body">
    <div
      v-if="showVisibilityMenu"
      ref="visibilityMenuRef"
      class="floating-control-menu visibility-floating-menu nw-floating-menu"
      :style="visibilityMenuStyle"
      role="listbox"
      @mousedown.stop
    >
      <button
        v-for="option in messageVisibilityOptions"
        :key="option.value"
        type="button"
        class="floating-control-option nw-floating-option"
        :class="{ 'is-selected': option.value === Visibility }"
        role="option"
        :aria-selected="option.value === Visibility"
        @click="selectVisibility(option.value)"
      >
        <UIcon :name="option.icon" class="w-4 h-4" />
        <span>{{ option.label }}</span>
      </button>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="showPublishDateMenu"
      ref="publishDateMenuRef"
      class="floating-control-menu publish-datetime-menu nw-floating-menu"
      :style="publishDateMenuStyle"
      role="dialog"
      aria-label="发布时间选择"
      @mousedown.stop
    >
      <div class="publish-date-head">
        <button type="button" class="floating-icon-btn" aria-label="上个月" @click="movePublishMonth(-1)">
          <UIcon name="i-heroicons-chevron-left" class="w-4 h-4" />
        </button>
        <div class="publish-date-picker-controls" aria-label="选择年月">
          <button
            ref="publishYearPickerButton"
            type="button"
            class="publish-date-title publish-picker-trigger"
            aria-label="选择年份"
            aria-haspopup="listbox"
            :aria-expanded="openPublishPicker === 'year'"
            @click.stop="togglePublishPicker('year')"
          >
            <span>{{ publishPickerMonth.getFullYear() }}年</span>
            <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
          </button>
          <button
            ref="publishMonthPickerButton"
            type="button"
            class="publish-date-title publish-picker-trigger"
            aria-label="选择月份"
            aria-haspopup="listbox"
            :aria-expanded="openPublishPicker === 'month'"
            @click.stop="togglePublishPicker('month')"
          >
            <span>{{ publishPickerMonth.getMonth() + 1 }}月</span>
            <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
          </button>
        </div>
        <button type="button" class="floating-icon-btn" aria-label="下个月" @click="movePublishMonth(1)">
          <UIcon name="i-heroicons-chevron-right" class="w-4 h-4" />
        </button>
      </div>
      <div class="publish-date-weekdays">
        <span v-for="label in publishWeekLabels" :key="label">{{ label }}</span>
      </div>
      <div class="publish-date-grid">
        <button
          v-for="day in publishPickerDays"
          :key="day.key"
          type="button"
          class="publish-date-day"
          :class="{
            'is-muted': !day.inMonth,
            'is-today': day.isToday,
            'is-selected': day.selected
          }"
          @click="selectPublishDay(day)"
        >
          {{ day.day }}
        </button>
      </div>
      <div class="publish-time-panel">
        <div ref="publishHourColumnRef" class="publish-time-column" aria-label="小时">
          <button
            v-for="hour in publishHourOptions"
            :key="hour"
            type="button"
            class="publish-time-option"
            :class="{ 'is-selected': hour === publishDraftHour }"
            @click="setPublishHour(hour)"
          >
            {{ pad2(hour) }}
          </button>
        </div>
        <div ref="publishMinuteColumnRef" class="publish-time-column" aria-label="分钟">
          <button
            v-for="minute in publishMinuteOptions"
            :key="minute"
            type="button"
            class="publish-time-option"
            :class="{ 'is-selected': minute === publishDraftMinute }"
            @click="setPublishMinute(minute)"
          >
            {{ pad2(minute) }}
          </button>
        </div>
      </div>
      <div class="publish-date-actions">
        <button type="button" class="floating-action-btn" @click="clearPublishDate">清除</button>
        <button type="button" class="floating-action-btn primary" @click="usePublishNow">现在</button>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="openPublishPicker"
      ref="publishPickerMenuRef"
      class="publish-picker-floating-menu nw-floating-menu"
      :class="`is-${openPublishPicker}`"
      :style="publishPickerMenuStyle"
      role="listbox"
      @mousedown.stop
    >
      <button
        v-for="option in publishPickerOptions"
        :key="option.value"
        type="button"
        class="publish-picker-floating-option nw-floating-option"
        :class="{ 'is-selected': option.selected }"
        role="option"
        :aria-selected="option.selected"
        @click="selectPublishPickerValue(option.value)"
      >
        {{ option.label }}
      </button>
    </div>
  </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, inject, onMounted, onBeforeUnmount, watch, defineAsyncComponent, nextTick } from 'vue'
import type { Ref } from 'vue'
import type { MessageToSave, MessageVisibility } from "~/types/models";
import { useMessage } from "~/composables/useMessage";
import { useUserStore } from '~/store/user'
import { Fancybox } from '@fancyapps/ui'
import '@fancyapps/ui/dist/fancybox/fancybox.css'
import Vditor from 'vditor'
import 'vditor/dist/index.css'
const VditorEditor = defineAsyncComponent(() => import('./VditorEditor.vue'))
import SearchMode from './Searchmode.vue'
import { useMessageStore } from '~/store/message'
import { useNotifyStore } from '~/store/notify'
import VideoUpload from './VideoUpload.vue'
import ImageHostingUploader from '~/components/widgets/ImageHostingUploader.vue'
import { createVideoMarkdown, resolveUploadedMediaUrl, uploadMediaFiles } from '~/utils/media-upload'
const props = defineProps<{ wide?: boolean }>()
const containerClass = computed(() => (props.wide ? 'w-full max-w-none' : 'mx-auto w-full sm:max-w-4xl'))
const isEditorLoading = ref(true)
const onEditorReady = async () => {
  isEditorLoading.value = false
  await nextTick()
  try {
    const root = vditorEditor.value?.getRootElement?.() as HTMLElement | null
    if (root) root.addEventListener('focusin', scrollEditorIntoViewForMobile)
  } catch {}
}
const showImageUploader = ref(false)
const imageUploaderPosition = ref({ x: 400, y: 320 }) // 可根据实际调整
// 处理图床上传成功，插入编辑器
const handleImageHostingSuccess = (markdown: string) => {
  if (vditorEditor.value?.insertValue) {
    vditorEditor.value.insertValue(markdown)
    focusEditor()
    syncContentFromEditor()
  }
  showImageUploader.value = false
}
const handlePositionUpdate = (newPosition: { x: number; y: number }) => {
  imageUploaderPosition.value = newPosition;
};
const videoUploadProgress = ref(0); // 新增进度变量
const handleVideoUploadProgress = (percent: number) => {
  videoUploadProgress.value = percent;
};
const imageUploadProgress = ref(0)
const activeUploadPercent = computed(() => {
  if (videoUploadProgress.value > 0 && videoUploadProgress.value < 100) return videoUploadProgress.value
  if (imageUploadProgress.value > 0 && imageUploadProgress.value < 100) return imageUploadProgress.value
  return 0
})
const activeUploadKind = computed(() => {
  if (videoUploadProgress.value > 0 && videoUploadProgress.value < 100) return 'video'
  if (imageUploadProgress.value > 0 && imageUploadProgress.value < 100) return 'image'
  return ''
})
const activeUploadLabel = computed(() => (activeUploadKind.value === 'video' ? '视频' : '图片'))
const showSearchModal = ref(false);
const emit = defineEmits(['search-result','video-uploaded', 'before-upload', 'upload-progress']);
const handleSearchResult = (result: any) => {
  emit('search-result', result);
};
const toast = useToast()
const BASE_API = useRuntimeConfig().public.baseApi || '/api';
const { save } = useMessage();

const showHeatmap = inject('showHeatmap') as Ref<boolean>;
provide('showHeatmap', showHeatmap);

const toggleHeatmap = () => {
  showHeatmap.value = !showHeatmap.value;
};

const Username = ref("");
const MessageContent = ref("");
const MessageContentHtml = ref("");
const PublishedAtInput = ref("");
const messageVisibilityOptions: { value: MessageVisibility; label: string; icon: string }[] = [
  { value: 'public', label: '公开', icon: 'i-mdi-earth' },
  { value: 'users', label: '成员', icon: 'i-mdi-account-group-outline' },
  { value: 'contacts', label: '联系人', icon: 'i-mdi-account-multiple-check-outline' },
  { value: 'private', label: '私密', icon: 'i-mdi-lock-outline' }
]
const normalizeMessageVisibility = (value: any, fallbackPrivate = false): MessageVisibility => {
  const raw = String(value || '').trim().toLowerCase()
  if (raw === 'users' || raw === 'members' || raw === 'member' || raw === 'logged_in' || raw === 'logged-in') return 'users'
  if (raw === 'contacts') return 'contacts'
  if (raw === 'private') return 'private'
  if (raw === 'public') return 'public'
  return fallbackPrivate ? 'private' : 'public'
}
const initialPostVisibility = (): MessageVisibility => {
  if (typeof window === 'undefined') return 'public'
  return normalizeMessageVisibility(localStorage.getItem('postVisibility'), localStorage.getItem('postPrivate') === 'true')
}
const Visibility = ref<MessageVisibility>(initialPostVisibility())
const Private = computed(() => Visibility.value !== 'public')
const visibilityLabel = computed(() => messageVisibilityOptions.find((option) => option.value === Visibility.value)?.label || '公开')
const visibilityIcon = computed(() => messageVisibilityOptions.find((option) => option.value === Visibility.value)?.icon || 'i-mdi-earth')
const showVisibilityMenu = ref(false)
const showPublishDateMenu = ref(false)
type PublishPickerType = 'year' | 'month'
const PUBLISH_MIN_YEAR = 1971
const PUBLISH_MAX_YEAR = 2099
const publishYearOptions = Array.from({ length: PUBLISH_MAX_YEAR - PUBLISH_MIN_YEAR + 1 }, (_, index) => PUBLISH_MIN_YEAR + index)
const publishMonthOptions = Array.from({ length: 12 }, (_, index) => index + 1)
const openPublishPicker = ref<PublishPickerType | ''>('')
const visibilityControlRef = ref<HTMLElement | null>(null)
const visibilityMenuRef = ref<HTMLElement | null>(null)
const publishTimeControlRef = ref<HTMLElement | null>(null)
const publishDateMenuRef = ref<HTMLElement | null>(null)
const publishYearPickerButton = ref<HTMLElement | null>(null)
const publishMonthPickerButton = ref<HTMLElement | null>(null)
const publishPickerMenuRef = ref<HTMLElement | null>(null)
const publishHourColumnRef = ref<HTMLElement | null>(null)
const publishMinuteColumnRef = ref<HTMLElement | null>(null)
const visibilityMenuStyle = ref<Record<string, string>>({})
const publishDateMenuStyle = ref<Record<string, string>>({})
const publishPickerMenuStyle = ref<Record<string, string>>({})
const pad2 = (value: number) => String(value).padStart(2, '0')
const publishWeekLabels = ['一', '二', '三', '四', '五', '六', '日']
const publishHourOptions = Array.from({ length: 24 }, (_, index) => index)
const publishMinuteOptions = Array.from({ length: 60 }, (_, index) => index)
const publishPickerMonth = ref(new Date(new Date().getFullYear(), new Date().getMonth(), 1))
const publishDraftDate = ref('')
const publishDraftHour = ref(0)
const publishDraftMinute = ref(0)
const contentTheme = inject('contentTheme') as Ref<string>
const toggleContentTheme = inject('toggleContentTheme') as (() => void) | undefined
const toggleTheme = () => {
  toggleContentTheme && toggleContentTheme()
  if (typeof window !== 'undefined') {
    document.documentElement.classList.toggle('dark', contentTheme.value === 'dark')
  }
}
const fileInput = ref<HTMLInputElement | null>(null);
const vditorEditor = ref<any>(null); // 需要支持 insertValue
// 预览跟随内容自动显示，无需手动开关

const DRAFT_KEY = 'addform_draft_v1'
let draftSaveTimer: any = null
let previewRenderTimer: any = null

const focusEditor = async () => {
  try {
    await nextTick()
    vditorEditor.value?.focus?.()
  } catch {}
}

const scrollEditorIntoViewForMobile = async () => {
  try {
    const isMobile = typeof window !== 'undefined' && window.matchMedia && window.matchMedia('(max-width: 520px)').matches
    if (!isMobile) return
    await nextTick()
    const root = vditorEditor.value?.getRootElement?.() as HTMLElement | null
    const target = root || (document.querySelector('.editor-box') as HTMLElement | null)
    if (!target) return
    setTimeout(() => {
      try { target.scrollIntoView({ block: 'start', behavior: 'smooth' }) } catch {}
    }, 220)
  } catch {}
}

const saveDraft = () => {
  try {
    const content = (MessageContent.value || '').trim()
    if (!content) {
      localStorage.removeItem(DRAFT_KEY)
      return
    }
    localStorage.setItem(
      DRAFT_KEY,
      JSON.stringify({ content: MessageContent.value || '', private: !!Private.value, visibility: Visibility.value, notify: !!enableNotify.value, savedAt: Date.now() })
    )
  } catch {}
}

const scheduleDraftSave = () => {
  if (draftSaveTimer) clearTimeout(draftSaveTimer)
  draftSaveTimer = setTimeout(() => saveDraft(), 800)
}

const clearDraft = () => {
  try { localStorage.removeItem(DRAFT_KEY) } catch {}
}

const syncContentFromEditor = () => {
  try {
    const val = vditorEditor.value?.getValue?.()
    if (typeof val === 'string') MessageContent.value = val
  } catch {}
}

const previewProseClass = computed(() => contentTheme.value === 'dark' ? 'prose prose-invert' : 'prose')

const notifyStore = useNotifyStore()
const enableNotify = ref(localStorage.getItem('enableNotify') === 'true')

const clearForm = () => {
  Username.value = "";
  MessageContent.value = "";
  MessageContentHtml.value = "";
  PublishedAtInput.value = "";
  clearDraft()
  
  if (vditorEditor.value) {
    vditorEditor.value.clear();
  }
};

const userStore = useUserStore();
const canSetPublishTime = computed(() => {
  const user = userStore.user as any
  return !!(user?.is_admin || user?.IsAdmin)
})

const datetimeLocalToISO = (value: string) => {
  const raw = String(value || '').trim()
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return ''
  return date.toISOString()
}

type PublishDateDay = {
  key: string
  date: string
  day: number
  inMonth: boolean
  isToday: boolean
  selected: boolean
}

const formatLocalDate = (date: Date) => `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
const formatDatetimeLocal = (date: string, hour: number, minute: number) => `${date}T${pad2(hour)}:${pad2(minute)}`
const parseDatetimeLocal = (value: string) => {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(String(value || '').trim())
  if (!match) return null
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]))
  const hour = Number(match[4])
  const minute = Number(match[5])
  if (Number.isNaN(date.getTime()) || hour < 0 || hour > 23 || minute < 0 || minute > 59) return null
  return { date, dateText: formatLocalDate(date), hour, minute }
}

const publishTimeLabel = computed(() => {
  const parsed = parseDatetimeLocal(PublishedAtInput.value)
  if (!parsed) return '选择时间'
  return `${parsed.dateText} ${pad2(parsed.hour)}:${pad2(parsed.minute)}`
})
const publishPickerOptions = computed(() => {
  if (openPublishPicker.value === 'year') {
    return publishYearOptions.map((year) => ({ value: year, label: `${year}年`, selected: year === publishPickerMonth.value.getFullYear() }))
  }
  if (openPublishPicker.value === 'month') {
    const current = publishPickerMonth.value.getMonth() + 1
    return publishMonthOptions.map((month) => ({ value: month, label: `${month}月`, selected: month === current }))
  }
  return []
})
const publishPickerDays = computed<PublishDateDay[]>(() => {
  const first = new Date(publishPickerMonth.value.getFullYear(), publishPickerMonth.value.getMonth(), 1)
  const startOffset = (first.getDay() + 6) % 7
  const todayText = formatLocalDate(new Date())
  const days: PublishDateDay[] = []
  for (let index = 0; index < 42; index += 1) {
    const date = new Date(first.getFullYear(), first.getMonth(), 1 - startOffset + index)
    const value = formatLocalDate(date)
    days.push({
      key: value,
      date: value,
      day: date.getDate(),
      inMonth: date.getMonth() === first.getMonth(),
      isToday: value === todayText,
      selected: value === publishDraftDate.value
    })
  }
  return days
})

type FloatingMenuPlacement = 'below' | 'above-right'

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

const positionFloatingMenu = (
  trigger: HTMLElement | null,
  menu: HTMLElement | null,
  styleRef: Ref<Record<string, string>>,
  minWidth = 120,
  placement: FloatingMenuPlacement = 'below'
) => {
  if (!trigger || typeof window === 'undefined') return
  const scale = getFixedCoordinateScale()
  const rect = getFixedRect(trigger, scale)
  const viewport = getFixedViewport(scale)
  const menuWidth = Math.max(menu?.offsetWidth || minWidth, minWidth, rect.width)
  const menuHeight = menu?.offsetHeight || 180
  const pad = 8
  const gap = 4
  const minLeft = viewport.left + pad
  const maxLeft = Math.max(minLeft, viewport.right - menuWidth - pad)
  const idealLeft = placement === 'above-right'
    ? rect.right - menuWidth
    : rect.left + rect.width / 2 - menuWidth / 2
  const aboveTop = rect.top - menuHeight - gap
  const belowTop = rect.bottom + gap
  const minTop = viewport.top + pad
  const maxTop = Math.max(minTop, viewport.bottom - menuHeight - pad)
  const idealTop = placement === 'above-right' && aboveTop >= minTop ? aboveTop : belowTop
  styleRef.value = {
    position: 'fixed',
    left: `${clamp(idealLeft, minLeft, maxLeft)}px`,
    top: `${clamp(idealTop, minTop, maxTop)}px`,
    right: 'auto',
    bottom: 'auto',
    transform: 'none',
    minWidth: `${Math.max(minWidth, rect.width)}px`
  }
}

const scheduleFloatingMenuPosition = (positioner: () => void) => {
  positioner()
  if (typeof window !== 'undefined') {
    window.requestAnimationFrame(() => {
      positioner()
      window.requestAnimationFrame(positioner)
    })
  }
}

const scrollSelectedOptionToRow = (container: HTMLElement | null, selector: string, rowIndex = 0) => {
  const selected = container?.querySelector<HTMLElement>(selector)
  if (!container || !selected || typeof window === 'undefined') return
  const optionSelector = selector.replace('.is-selected', '')
  const options = Array.from(container.querySelectorAll<HTMLElement>(optionSelector))
  const selectedIndex = options.indexOf(selected)
  if (selectedIndex < 0) return
  const style = window.getComputedStyle(container)
  const gap = Number.parseFloat(style.rowGap || style.gap || '0')
  const paddingTop = Number.parseFloat(style.paddingTop || '0')
  const step = selected.offsetHeight + (Number.isFinite(gap) ? gap : 0)
  const maxScrollTop = Math.max(0, container.scrollHeight - container.clientHeight)
  const target = paddingTop + selectedIndex * step - step * Math.max(0, rowIndex)
  container.scrollTop = clamp(target, 0, maxScrollTop)
}

const scrollPublishPickerSelectionToTop = () => {
  scrollSelectedOptionToRow(publishPickerMenuRef.value, '.publish-picker-floating-option.is-selected')
}

const scrollPublishTimeSelectionToSecondRow = () => {
  scrollSelectedOptionToRow(publishHourColumnRef.value, '.publish-time-option.is-selected', 1)
  scrollSelectedOptionToRow(publishMinuteColumnRef.value, '.publish-time-option.is-selected', 1)
}

const closeFloatingMenus = () => {
  showVisibilityMenu.value = false
  showPublishDateMenu.value = false
  openPublishPicker.value = ''
}

const positionVisibilityMenu = () => positionFloatingMenu(visibilityControlRef.value, visibilityMenuRef.value, visibilityMenuStyle, 106, 'above-right')
const positionPublishDateMenu = () => positionFloatingMenu(publishTimeControlRef.value, publishDateMenuRef.value, publishDateMenuStyle, 292, 'above-right')
const positionPublishPickerMenu = () => {
  if (!openPublishPicker.value || typeof window === 'undefined') return
  const trigger = openPublishPicker.value === 'year' ? publishYearPickerButton.value : publishMonthPickerButton.value
  if (!trigger) return
  const scale = getFixedCoordinateScale()
  const rect = getFixedRect(trigger, scale)
  const viewport = getFixedViewport(scale)
  const menu = publishPickerMenuRef.value
  const menuWidth = Math.ceil(rect.width)
  const menuHeight = menu?.offsetHeight || (openPublishPicker.value === 'year' ? 204 : 167)
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
  publishPickerMenuStyle.value = {
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

const toggleVisibilityMenu = async () => {
  showPublishDateMenu.value = false
  openPublishPicker.value = ''
  showVisibilityMenu.value = !showVisibilityMenu.value
  if (showVisibilityMenu.value) {
    await nextTick()
    scheduleFloatingMenuPosition(positionVisibilityMenu)
  }
}

const selectVisibility = (value: MessageVisibility) => {
  Visibility.value = value
  showVisibilityMenu.value = false
}

const syncPublishDraftFromInput = () => {
  const parsed = parseDatetimeLocal(PublishedAtInput.value)
  const base = parsed || (() => {
    const now = new Date()
    return { date: now, dateText: formatLocalDate(now), hour: now.getHours(), minute: now.getMinutes() }
  })()
  publishPickerMonth.value = new Date(base.date.getFullYear(), base.date.getMonth(), 1)
  publishDraftDate.value = base.dateText
  publishDraftHour.value = base.hour
  publishDraftMinute.value = base.minute
}

const applyPublishDraft = () => {
  if (!publishDraftDate.value) return
  PublishedAtInput.value = formatDatetimeLocal(publishDraftDate.value, publishDraftHour.value, publishDraftMinute.value)
}

const togglePublishDateMenu = async () => {
  showVisibilityMenu.value = false
  openPublishPicker.value = ''
  showPublishDateMenu.value = !showPublishDateMenu.value
  if (showPublishDateMenu.value) {
    syncPublishDraftFromInput()
    await nextTick()
    scrollPublishTimeSelectionToSecondRow()
    scheduleFloatingMenuPosition(positionPublishDateMenu)
  }
}

const togglePublishPicker = async (type: PublishPickerType) => {
  openPublishPicker.value = openPublishPicker.value === type ? '' : type
  if (openPublishPicker.value) {
    publishPickerMenuStyle.value = {
      position: 'fixed',
      left: '0px',
      top: '0px',
      right: 'auto',
      bottom: 'auto',
      visibility: 'hidden'
    }
    await nextTick()
    scrollPublishPickerSelectionToTop()
    scheduleFloatingMenuPosition(positionPublishPickerMenu)
  }
}

const selectPublishPickerValue = (value: number) => {
  if (openPublishPicker.value === 'year' && Number.isFinite(value)) {
    publishPickerMonth.value = new Date(value, publishPickerMonth.value.getMonth(), 1)
  } else if (openPublishPicker.value === 'month' && Number.isFinite(value)) {
    publishPickerMonth.value = new Date(publishPickerMonth.value.getFullYear(), value - 1, 1)
  }
  openPublishPicker.value = ''
  nextTick(() => scheduleFloatingMenuPosition(positionPublishDateMenu))
}

const movePublishMonth = (delta: number) => {
  publishPickerMonth.value = new Date(publishPickerMonth.value.getFullYear(), publishPickerMonth.value.getMonth() + delta, 1)
  nextTick(() => scheduleFloatingMenuPosition(positionPublishDateMenu))
}

const selectPublishDay = (day: PublishDateDay) => {
  publishDraftDate.value = day.date
  if (!day.inMonth) {
    const parsed = new Date(`${day.date}T00:00:00`)
    if (!Number.isNaN(parsed.getTime())) publishPickerMonth.value = new Date(parsed.getFullYear(), parsed.getMonth(), 1)
  }
  applyPublishDraft()
}

const setPublishHour = (hour: number) => {
  publishDraftHour.value = hour
  applyPublishDraft()
}

const setPublishMinute = (minute: number) => {
  publishDraftMinute.value = minute
  applyPublishDraft()
}

const usePublishNow = () => {
  const now = new Date()
  publishPickerMonth.value = new Date(now.getFullYear(), now.getMonth(), 1)
  publishDraftDate.value = formatLocalDate(now)
  publishDraftHour.value = now.getHours()
  publishDraftMinute.value = now.getMinutes()
  applyPublishDraft()
  showPublishDateMenu.value = false
  openPublishPicker.value = ''
}

const clearPublishDate = () => {
  PublishedAtInput.value = ''
  showPublishDateMenu.value = false
  openPublishPicker.value = ''
}

const handleFloatingMenuPointerDown = (event: MouseEvent) => {
  const target = event.target as Node | null
  if (!target) return
  if (visibilityControlRef.value?.contains(target) || visibilityMenuRef.value?.contains(target)) return
  if (publishTimeControlRef.value?.contains(target) || publishDateMenuRef.value?.contains(target)) return
  if (publishYearPickerButton.value?.contains(target) || publishMonthPickerButton.value?.contains(target) || publishPickerMenuRef.value?.contains(target)) return
  closeFloatingMenus()
}

const handleFloatingMenuViewportChange = () => {
  if (showVisibilityMenu.value) positionVisibilityMenu()
  if (showPublishDateMenu.value) positionPublishDateMenu()
  if (openPublishPicker.value) positionPublishPickerMenu()
}

const checkLogin = () => {
  if (!userStore.isLogin) {
    toast.add({
      title: '提示',
      description: '请先登录',
      color: 'orange',
      timeout: 2000
    });
    return false;
  }
  return true;
};

const triggerFileInput = () => {
  fileInput.value?.click();
};

const addImage = async (event: Event) => {
  if (!checkLogin()) return;
  const input = event.target as HTMLInputElement;
  const files = input.files ? Array.from(input.files) : [];

  if (!files.length) {
    toast.add({
      title: '错误',
      description: '没有选择文件',
      color: 'red',
      timeout: 2000
    });
    return;
  }

  try {
    const uploaded = await uploadMediaFiles({
      files,
      kind: 'image',
      baseApi: String(BASE_API || '/api'),
      token: userStore.token || '',
      onProgress: (percent) => { imageUploadProgress.value = percent }
    })
    if (uploaded.length && vditorEditor.value?.insertValue) {
      vditorEditor.value.insertValue(uploaded.map((item) => item.markdown).join(''))
      syncContentFromEditor()
      focusEditor()
    }
    imageUploadProgress.value = 100
    setTimeout(() => { imageUploadProgress.value = 0 }, 400)
    toast.add({
      title: '成功',
      description: uploaded.length > 1 ? `已上传 ${uploaded.length} 张图片` : '图片上传成功',
      color: 'green',
      timeout: 2000
    });
  } catch (error: any) {
    console.error('上传错误:', error);
    toast.add({
      title: '错误',
      description: error.message || '图片上传失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    if (fileInput.value) {
      fileInput.value.value = '';
    }
    if (imageUploadProgress.value !== 0) {
      setTimeout(() => { imageUploadProgress.value = 0 }, 800)
    }
  }
};

const handleVideoUploaded = (videoUrl: string) => {
  const videoTag = createVideoMarkdown(resolveUploadedMediaUrl(videoUrl, String(BASE_API || '/api')))
  if (vditorEditor.value?.insertValue) {
    vditorEditor.value.insertValue(videoTag)
    syncContentFromEditor()
    focusEditor()
  }
};

const INLINE_IMAGE_REG = /!\s*(https?:\/\/[^\s!]+\.(?:png|jpe?g|gif|webp))(?:\?[^\s!]*)?/gi;
const normalizeInlineImageLinks = (md: string): string => md.replace(INLINE_IMAGE_REG, (m, url) => `![](${url})`);

const applyImageGridHTML = (html: string) => {
  const parser = new DOMParser();
  const doc = parser.parseFromString(html, 'text/html');
  const fullSizeRenderSelector = [
    '[data-render-source="xiaohongshu"]',
    '[data-render-source="xhs"]',
    '[data-render-source="rednote"]',
    '.xiaohongshu-render',
    '.xhs-render',
    '.rednote-render',
    '.xiaohongshu-render-image',
    '.xhs-render-image',
    '.rednote-render-image',
  ].join(',');
  const shouldKeepFullSizeImage = (el: Element | null) => !!el?.closest(fullSizeRenderSelector);
  const isPureImageParagraph = (p: Element) => {
    let ok = true;
    const children = Array.from(p.childNodes);
    if (children.length === 0) return false;
    for (const node of children) {
      if (node.nodeType === Node.ELEMENT_NODE) {
        const el = node as Element;
        const tag = el.tagName.toLowerCase();
        if (tag === 'img') {
          if (shouldKeepFullSizeImage(el)) { ok = false; break; }
          continue;
        }
        const linkedImg = tag === 'a' && el.childElementCount === 1 ? el.querySelector('img') : null;
        if (linkedImg) {
          if (shouldKeepFullSizeImage(el) || shouldKeepFullSizeImage(linkedImg)) { ok = false; break; }
          continue;
        }
        if (tag === 'br') { ok = false; break; }
        ok = false; break;
      } else if (node.nodeType === Node.TEXT_NODE) {
        if ((node.textContent || '').trim() !== '') { ok = false; break; }
      }
    }
    return ok;
  };

  const paras = Array.from(doc.body.querySelectorAll('p'));
  const runs: Element[][] = [];
  let current: Element[] = [];
  for (const p of paras) {
    if (isPureImageParagraph(p)) {
      const last = current[current.length - 1];
      if (!last || last.nextElementSibling === p) {
        current.push(p);
      } else {
        if (current.length >= 2) runs.push(current);
        current = [p];
      }
    } else {
      if (current.length >= 2) runs.push(current);
      current = [];
    }
  }
  if (current.length >= 2) runs.push(current);

  const getSingleImagePayload = (p: Element) => {
    const anchors = Array.from(p.querySelectorAll('a')).filter((a) => a.querySelector('img')) as HTMLAnchorElement[];
    const anchoredImages = anchors.map((a) => a.querySelector('img')).filter(Boolean) as HTMLImageElement[];
    const looseImages = Array.from(p.querySelectorAll('img')).filter((img) => !img.closest('a')) as HTMLImageElement[];
    const count = anchoredImages.length + looseImages.length;
    if (count !== 1) return null;
    return anchors[0]
      ? { anchor: anchors[0], img: anchoredImages[0] }
      : { anchor: null, img: looseImages[0] };
  };

  const ensurePreviewImageAnchor = (payload: { anchor: HTMLAnchorElement | null; img: HTMLImageElement }, group: string) => {
    const src = payload.img.getAttribute('src') || payload.img.src || '';
    if (payload.anchor) {
      const href = payload.anchor.getAttribute('href') || '';
      if (!href || href === '#' || href.startsWith('javascript:')) payload.anchor.setAttribute('href', src);
      payload.anchor.setAttribute('data-fancybox', group);
      payload.anchor.classList.add('inline-image-link');
      return payload.anchor;
    }
    const anchor = doc.createElement('a');
    anchor.setAttribute('href', src);
    anchor.setAttribute('data-fancybox', group);
    anchor.className = 'inline-image-link';
    anchor.appendChild(payload.img);
    return anchor;
  };

  const wrapSingleImageParagraph = (p: Element) => {
    const payload = getSingleImagePayload(p);
    if (!payload) return;
    const wrapper = doc.createElement('div');
    wrapper.className = 'single-media inline-image-thumb';
    wrapper.appendChild(ensurePreviewImageAnchor(payload, 'editor-preview-image'));
    p.replaceWith(wrapper);
  };

  for (const run of runs) {
    const grid = doc.createElement('div');
    const count = run.length;
    const cols = count === 2 || count === 4 ? 2 : Math.min(3, count);
    grid.className = `image-grid cols-${cols}`;
    const group = `grid-${Math.random().toString(36).slice(2)}`;
    for (const p of run) {
      const img = p.querySelector('img') as HTMLImageElement | null;
      const a = p.querySelector('a') as HTMLAnchorElement | null;
      if (!img && !a) continue;
      const item = doc.createElement('div');
      item.className = 'image-grid-item';
      let anchor: HTMLAnchorElement;
      if (a && a.querySelector('img')) {
        anchor = a;
        anchor.setAttribute('data-fancybox', group);
        if (!anchor.getAttribute('href')) {
          const innerImg = a.querySelector('img') as HTMLImageElement;
          anchor.setAttribute('href', innerImg.src);
        }
      } else if (img) {
        anchor = doc.createElement('a');
        anchor.setAttribute('href', img.src);
        anchor.setAttribute('data-fancybox', group);
        anchor.appendChild(img);
      } else {
        continue;
      }
      item.appendChild(anchor);
      grid.appendChild(item);
    }
    const first = run[0];
    first.replaceWith(grid);
    for (let i = 1; i < run.length; i++) run[i].remove();
  }

  Array.from(doc.body.querySelectorAll('p')).forEach((p) => {
    if (isPureImageParagraph(p)) wrapSingleImageParagraph(p);
  });
  return doc.body.innerHTML;
};

watch(Visibility, (value) => {
  if (typeof window === 'undefined') return
  localStorage.setItem('postVisibility', value)
  localStorage.setItem('postPrivate', value !== 'public' ? 'true' : 'false')
  scheduleDraftSave()
})

watch(MessageContent, (val) => {
  scheduleDraftSave()
  if (previewRenderTimer) clearTimeout(previewRenderTimer)
  previewRenderTimer = setTimeout(async () => {
    const raw = await Vditor.md2html(normalizeInlineImageLinks(val || ""));
    MessageContentHtml.value = applyImageGridHTML(raw);
    nextTick(() => {
      const roots = document.querySelectorAll('.editor-preview');
      roots.forEach((root) => {
        root.querySelectorAll('.image-grid-item img').forEach((imgEl) => {
          const img = imgEl as HTMLImageElement;
          const parent = img.parentElement as HTMLElement;
          const setAR = () => {
            const w = img.naturalWidth;
            const h = img.naturalHeight;
            parent.classList.remove('ar-169', 'ar-34', 'ar-11');
            if (w > h) parent.classList.add('ar-169');
            else if (h > w) parent.classList.add('ar-34');
            else parent.classList.add('ar-11');
          };
          if (img.complete && img.naturalWidth && img.naturalHeight) setAR();
          else img.addEventListener('load', setAR, { once: true });
        });
      });
    });
  }, 220)
});

watch(() => userStore.isLogin, (newLoginState) => {
  if (newLoginState) {
    enableNotify.value = localStorage.getItem('enableNotify') === 'true';
  }
}, { immediate: true });

onMounted(async () => {
  Fancybox.bind("[data-fancybox]", {});
  document.addEventListener('mousedown', handleFloatingMenuPointerDown)
  window.addEventListener('resize', handleFloatingMenuViewportChange)
  window.addEventListener('scroll', handleFloatingMenuViewportChange, true)
  window.visualViewport?.addEventListener('resize', handleFloatingMenuViewportChange)
  window.visualViewport?.addEventListener('scroll', handleFloatingMenuViewportChange)
  if (!userStore.isLogin) {
    const token = localStorage.getItem('token');
    if (token) {
      await userStore.fetchUserInfo();
    }
  }
  Visibility.value = initialPostVisibility()
  contentTheme.value = localStorage.getItem('contentTheme') || contentTheme.value

  try {
    const raw = localStorage.getItem(DRAFT_KEY)
    if (raw) {
      const draft = JSON.parse(raw)
      const draftContent = String(draft?.content || '')
      if (draftContent.trim().length > 0 && MessageContent.value.trim().length === 0) {
        MessageContent.value = draftContent
        Visibility.value = normalizeMessageVisibility(draft?.visibility, typeof draft?.private === 'boolean' ? draft.private : Visibility.value === 'private')
        if (typeof draft?.notify === 'boolean') enableNotify.value = draft.notify
        vditorEditor.value?.setValue?.(draftContent)
        toast.add({ title: '草稿已恢复', description: '已自动恢复上次未发布内容', color: 'green', timeout: 2000 })
      }
    }
  } catch {}

  const onBeforeUnload = (e: BeforeUnloadEvent) => {
    if ((MessageContent.value || '').trim().length === 0) return
    e.preventDefault()
    e.returnValue = ''
  }
  window.addEventListener('beforeunload', onBeforeUnload)
  onBeforeUnmount(() => window.removeEventListener('beforeunload', onBeforeUnload))
});

onBeforeUnmount(() => {
  Fancybox.destroy();
  document.removeEventListener('mousedown', handleFloatingMenuPointerDown)
  window.removeEventListener('resize', handleFloatingMenuViewportChange)
  window.removeEventListener('scroll', handleFloatingMenuViewportChange, true)
  window.visualViewport?.removeEventListener('resize', handleFloatingMenuViewportChange)
  window.visualViewport?.removeEventListener('scroll', handleFloatingMenuViewportChange)
});
const toggleNotify = () => {
  enableNotify.value = !enableNotify.value;
  localStorage.setItem('enableNotify', enableNotify.value.toString());
};

const addMessage = async () => {
  if (!checkLogin()) return;
  syncContentFromEditor()

  if (!MessageContent.value.trim()) {
    toast.add({
      title: '错误',
      description: '请输入内容或上传图片/视频',
      color: 'red',
      timeout: 2000
    });
    return;
  }

  const message: MessageToSave = {
    username: Username.value,
    content: MessageContent.value,
    private: Private.value,
    visibility: Visibility.value,
    notify: enableNotify.value,
  };
  const publishTime = canSetPublishTime.value ? datetimeLocalToISO(PublishedAtInput.value) : ''
  if (publishTime) {
    message.created_at = publishTime
  }

  try {
    const response = await save(message);
    if (response) {
      clearForm();
      clearDraft()
    }
  } catch (error: any) {
    console.error('发布错误:', error);
    toast.add({
      title: '错误',
      description: error.message || '发布失败',
      color: 'red',
      timeout: 2000
    });
  }
};
</script>

<style scoped>
.editor-box { background: #ffffff; border: 1px solid #e5e7eb; border-radius: 12px; box-shadow: 0 10px 24px rgba(0,0,0,.08); padding: 8px; color:#111827; }
.editor-toolbar { display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:6px; padding:6px; border-radius:12px; background: rgba(255,255,255,0.85); flex-wrap: wrap; overflow: visible; position: sticky; bottom: 0; z-index: 95; backdrop-filter: saturate(1.1) blur(6px); }
.toolbar-left, .toolbar-right { display:flex; align-items:center; gap:8px; flex-wrap: wrap; }
.tb-btn { display:flex; align-items:center; justify-content:center; flex: 0 0 auto; width:36px; min-width:36px; height:36px; border-radius:12px; background: rgba(15,23,42,0.06); color:#374151; transition: background-color .18s ease, transform .18s ease, border-color .18s ease, box-shadow .18s ease; border:1px solid rgba(15,23,42,0.08); box-shadow:none; }
.tb-btn:hover { transform: translate3d(0,0,0) scale(1.06); border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.tb-btn.primary { border-color: rgba(37,99,235,.72); background: #3b82f6; color: #fff; box-shadow: inset 0 0 0 1px rgba(255,255,255,0.16); }
.tb-btn.primary:hover { border-color: rgba(29,78,216,.8); background: #2563eb; }
.tb-btn.danger,
.tb-btn.danger:hover { border-color: rgba(234,88,12,.95); background: linear-gradient(135deg, rgba(251,146,60,.95), rgba(234,88,12,.95)); color: #fff; box-shadow: inset 0 0 0 1px rgba(255,255,255,0.18); }
.tb-btn.has-label { width: auto; min-width: 66px; gap: 5px; padding: 0 10px; }
.notify-btn.is-enabled { background: rgba(249,115,22,0.16); color: #c2410c; box-shadow: inset 0 0 0 1px rgba(249,115,22,0.32); }
.notify-label { font-size: 12px; line-height: 1; white-space: nowrap; }
.publish-time-control { display:flex; align-items:center; gap:5px; min-height:36px; height:36px; width: max-content; max-width: min(210px, calc(100vw - 32px)); border-radius:12px; background: rgba(0,0,0,0.06); color:#374151; padding:0 8px; border:1px solid rgba(15,23,42,0.08); box-shadow: none; transition: background-color .18s ease, border-color .18s ease, transform .18s ease; }
.visibility-control { display:flex; align-items:center; gap:5px; min-height:36px; height:36px; width: max-content; border-radius:12px; background: rgba(0,0,0,0.06); color:#374151; padding:0 8px; border:1px solid rgba(15,23,42,0.08); box-shadow: none; transition: background-color .18s ease, border-color .18s ease, transform .18s ease; }
.publish-time-control:hover,
.publish-time-control:focus-within,
.visibility-control:hover,
.visibility-control:focus-within { transform: translate3d(0,0,0) scale(1.06); border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.visibility-select { width: auto; min-width: 46px; max-width: 76px; height: 28px; padding: 0; border: 0; border-radius: 9px; outline: none; background: transparent; color: inherit; font-size: 12px; cursor: pointer; }
.visibility-trigger,
.publish-time-trigger { display: inline-flex; align-items: center; justify-content: space-between; gap: 3px; }
.visibility-trigger span,
.publish-time-trigger span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.visibility-trigger svg,
.publish-time-trigger svg { flex: 0 0 auto; opacity: .72; }
.publish-time-input { width: max-content; max-width: 148px; min-height: 28px; padding: 0; border: none; outline: none; background: transparent; color: inherit; font-size: 12px; text-align: left; }
.floating-control-menu { position: fixed; z-index: 5004; border: 1px solid var(--nw-floating-border); border-radius: 12px; background: var(--nw-floating-bg); color: var(--nw-floating-text); box-shadow: var(--nw-floating-shadow); backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px); }
.visibility-floating-menu { display: grid; gap: 4px; padding: 8px; }
.floating-control-option { display: flex; align-items: center; gap: 8px; min-height: 32px; padding: 0 10px; border-radius: 9px; border: 1px solid transparent; color: inherit; font-size: 12px; font-weight: 650; text-align: left; transition: background-color .15s ease, border-color .15s ease, color .15s ease; }
.floating-control-option:hover,
.floating-control-option:focus-visible { outline: none; border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.floating-control-option.is-selected { border-color: var(--nw-floating-selected-border); background: var(--nw-floating-selected-bg); color: var(--nw-floating-text); }
.publish-datetime-menu { width: 292px; padding: 10px; }
.publish-date-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 8px; }
.publish-date-picker-controls { display: inline-flex; align-items: center; justify-content: center; gap: 4px; min-width: 0; }
.publish-date-title { font-size: 13px; font-weight: 700; color: inherit; }
.publish-picker-trigger { min-height: 28px; padding: 0 7px; border-radius: 8px; border: 1px solid transparent; background: rgba(15,23,42,0.04); display: inline-flex; align-items: center; justify-content: center; gap: 3px; white-space: nowrap; }
.publish-picker-trigger:hover,
.publish-picker-trigger:focus-visible { border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); outline: none; }
.publish-picker-trigger:first-child { width: 75px; }
.publish-picker-trigger:last-child { width: 50px; }
.floating-icon-btn { width: 28px; height: 28px; display: inline-flex; align-items: center; justify-content: center; border-radius: 8px; border: 1px solid var(--nw-floating-border); background: rgba(15,23,42,0.04); color: inherit; }
.floating-icon-btn:hover { border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.publish-picker-floating-menu { position: fixed; z-index: 5005; box-sizing: border-box; display: grid; gap: 4px; max-height: 204px; overflow-y: auto; padding: 8px; border: 1px solid var(--nw-floating-border); border-radius: 10px; background: var(--nw-floating-bg); color: var(--nw-floating-text); box-shadow: var(--nw-floating-shadow); scrollbar-width: none; }
.publish-picker-floating-menu::-webkit-scrollbar { width: 0; height: 0; }
.publish-picker-floating-menu.is-month { gap: 3px; max-height: 167px; padding: 4px; }
.publish-picker-floating-option { box-sizing: border-box; display: inline-flex; align-items: center; justify-content: center; min-height: 28px; min-width: 0; width: 100%; padding: 0 6px; border-radius: 8px; border: 1px solid transparent; color: inherit; font-size: 12px; font-weight: 650; line-height: 1; text-align: center; white-space: nowrap; transition: background-color .15s ease, border-color .15s ease, color .15s ease; }
.publish-picker-floating-menu.is-month .publish-picker-floating-option { min-height: 24px; padding: 0 4px; }
.publish-picker-floating-option:hover,
.publish-picker-floating-option:focus-visible { outline: none; border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.publish-picker-floating-option.is-selected { border-color: var(--nw-floating-selected-border); background: var(--nw-floating-selected-bg); color: var(--nw-floating-text); }
.publish-date-weekdays,
.publish-date-grid { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 3px; }
.publish-date-weekdays { margin-bottom: 4px; color: rgba(71,85,105,0.72); font-size: 10px; font-weight: 700; text-align: center; }
.publish-date-day { height: 28px; border-radius: 8px; border: 1px solid transparent; background: rgba(15,23,42,0.05); color: var(--nw-floating-text); font-size: 12px; line-height: 1; }
.publish-date-day:hover { border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.publish-date-day.is-muted { opacity: .38; }
.publish-date-day.is-today { border-color: rgba(96,165,250,0.68); background: rgba(59,130,246,0.22); }
.publish-date-day.is-selected { border-color: var(--nw-floating-selected-border); background: var(--nw-floating-selected-bg); color: var(--nw-floating-text); }
.publish-time-panel { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin-top: 10px; }
.publish-time-column { box-sizing: border-box; display: grid; grid-auto-rows: 28px; gap: 4px; height: 124px; max-height: 124px; overflow-y: auto; padding: 0; border-radius: 10px; background: rgba(15,23,42,0.06); scrollbar-width: none; }
.publish-time-column::-webkit-scrollbar { width: 0; height: 0; }
.publish-time-option { box-sizing: border-box; border-radius: 7px; border: 1px solid transparent; color: inherit; font-size: 12px; font-weight: 650; }
.publish-time-option:hover { border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.publish-time-option.is-selected { border-color: var(--nw-floating-selected-border); background: var(--nw-floating-selected-bg); color: var(--nw-floating-text); }
.publish-date-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px; }
.floating-action-btn { height: 30px; padding: 0 12px; border-radius: 9px; border: 1px solid var(--nw-floating-border); background: rgba(15,23,42,0.04); color: inherit; font-size: 12px; font-weight: 650; }
.floating-action-btn:hover { border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.floating-action-btn.primary { border-color: var(--nw-floating-selected-border); background: var(--nw-floating-selected-bg); color: var(--nw-floating-text); }
.tb-sep { width:1px; height:24px; background: rgba(0,0,0,0.12); margin: 0 2px; }
.preview-card { backdrop-filter: blur(8px); background: #ffffff; border: 1px solid #e5e7eb; border-radius: 12px; padding: 8px; color:#111827; }
html.dark .editor-box { background: var(--home-surface-dark, #202a36); border: 1px solid rgba(255,255,255,0.16); color:#fff; }
html.dark .editor-toolbar { background: rgba(39, 50, 66, 0.68); backdrop-filter: saturate(1.1) blur(6px); }
html.dark .tb-btn { background: rgba(255,255,255,0.06); color:#cbd5e1; border-color: rgba(255,255,255,0.12); }
html.dark .tb-btn:hover { background: var(--nw-floating-hover-bg); border-color: var(--nw-floating-hover-border); }
html.dark .tb-btn.primary { border-color: rgba(37,99,235,.72); background: #3b82f6; color: #fff; }
html.dark .tb-btn.primary:hover { border-color: rgba(29,78,216,.8); background: #2563eb; }
html.dark .tb-btn.danger,
html.dark .tb-btn.danger:hover { border-color: rgba(234,88,12,.95); background: linear-gradient(135deg, rgba(251,146,60,.95), rgba(234,88,12,.95)); color: #fff; }
html.dark .notify-btn.is-enabled { background: rgba(249,115,22,0.22); color: #fed7aa; box-shadow: inset 0 0 0 1px rgba(251,146,60,0.38); }
html.dark .publish-time-control,
html.dark .visibility-control { background: rgba(255,255,255,0.06); color:#cbd5e1; border-color: rgba(255,255,255,0.12); }
html.dark .publish-time-control:hover,
html.dark .publish-time-control:focus-within,
html.dark .visibility-control:hover,
html.dark .visibility-control:focus-within { background: var(--nw-floating-hover-bg); border-color: var(--nw-floating-hover-border); }
html.dark .visibility-select { background: transparent; border: 0; color: inherit; }
html.dark .floating-icon-btn,
html.dark .floating-action-btn,
html.dark .publish-picker-trigger { background: rgba(255,255,255,0.06); }
html.dark .publish-date-weekdays { color: rgba(226,232,240,0.66); }
html.dark .publish-date-day { background: rgba(255,255,255,0.06); }
html.dark .publish-time-column { background: rgba(15,23,42,0.46); }
html.dark .tb-sep { background: rgba(255,255,255,0.12); }
html.dark .preview-card { background: rgba(39, 50, 66, 0.68); border: 1px solid rgba(255,255,255,0.18); color:#fff; }
.editor-toolbar :deep(.u-button) { border:none !important; box-shadow:none !important; background: transparent !important; color:#374151 !important; }
html.dark .editor-toolbar :deep(.u-button) { border:none !important; box-shadow:none !important; background: rgba(255,255,255,0.06) !important; color:#cbd5e1 !important; }
.upload-progress { flex-basis: 100%; order: 10; display: flex; align-items: center; gap: 10px; pointer-events: none; padding: 0 4px; margin-top: 6px; }
.upload-progress-track { flex: 1; height: 4px; border-radius: 999px; background: rgba(0,0,0,0.12); overflow: hidden; }
.upload-progress-fill { height: 100%; border-radius: 999px; }
.upload-progress-fill.image { background: linear-gradient(90deg, rgba(167,139,250,1), rgba(244,114,182,1)); }
.upload-progress-fill.video { background: linear-gradient(90deg, rgba(96,165,250,1), rgba(52,211,153,1)); }
.upload-progress-text { font-size: 12px; line-height: 1; color: rgba(17,24,39,0.6); min-width: 76px; text-align: right; }
html.dark .upload-progress-track { background: rgba(255,255,255,0.14); }
html.dark .upload-progress-text { color: rgba(226,232,240,0.72); }
.editor-preview p { margin: 0.5rem 0; }
.editor-preview img { margin: 0.4rem 0; }
.editor-preview .inline-image-thumb {
  width: var(--inline-image-thumb-size);
  height: var(--inline-image-thumb-size);
  max-width: 100%;
  margin: 6px 0;
  overflow: hidden;
  border-radius: 10px;
  display: block;
}
.editor-preview .inline-image-thumb > a,
.editor-preview .inline-image-thumb > img {
  display: block;
  width: 100% !important;
  height: 100% !important;
}
.editor-preview .inline-image-thumb img {
  width: 100% !important;
  height: 100% !important;
  min-height: 0 !important;
  margin: 0 !important;
  object-fit: cover;
  object-position: center;
  border-radius: inherit;
}
.image-grid {
  display: grid;
  gap: 6px;
  margin: 0;
  width: 100%;
  grid-auto-flow: dense;
  align-items: stretch;
  justify-items: stretch;
}
.image-grid.cols-2 { grid-template-columns: repeat(2, 1fr); }
.image-grid.cols-3 { grid-template-columns: repeat(3, 1fr); }
.image-grid-item {
  position: relative;
  aspect-ratio: 1 / 1;
  overflow: hidden;
  border-radius: 10px;
}
.image-grid-item > a { display: block; width: 100%; height: 100%; }
.image-grid-item > a > img { width: 100%; height: 100%; object-fit: cover; object-position: center; display: block; }
.image-grid-item.ar-169 { aspect-ratio: 16 / 9; }
.image-grid-item.ar-34 { aspect-ratio: 3 / 4; }
.image-grid-item.ar-11 { aspect-ratio: 1 / 1; }
.image-grid-item img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  margin: 0;
}
</style>
