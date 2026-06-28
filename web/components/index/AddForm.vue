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
          <AudioRecorder
            @audio-uploaded="handleAudioUploaded"
            @upload-progress="handleAudioUploadProgress"
          />
          <button type="button" class="tb-btn nw-action-btn nw-tooltip-anchor" data-tooltip="上传图片" aria-label="上传图片" @click="triggerFileInput"><UIcon name="i-mdi-image-plus-outline" class="w-5 h-5" /></button>
          <!-- 视频上传按钮 -->
          <VideoUpload
            @video-uploaded="handleVideoUploaded"
            @upload-progress="handleVideoUploadProgress"
          />
          <!-- 新增图床上传按钮 -->
          <button type="button" class="tb-btn nw-action-btn nw-tooltip-anchor" data-tooltip="图床上传" aria-label="图床上传" @click="showImageUploader = true"><UIcon name="i-mdi-cloud-upload-outline" class="w-5 h-5" /></button>
          <button type="button" class="tb-btn nw-action-btn state-toggle-btn full-image-btn nw-tooltip-anchor" :class="{ 'is-enabled': fullImageAttachments }" :data-tooltip="`全图显示：${fullImageAttachments ? '已开启' : '已关闭'}`" :aria-label="`全图显示：${fullImageAttachments ? '已开启' : '已关闭'}`" :aria-pressed="fullImageAttachments" @click="toggleFullImageAttachments">
            <UIcon :name="fullImageAttachments ? 'i-mdi-image-size-select-actual' : 'i-mdi-image-size-select-large'" class="w-5 h-5" />
          </button>
          <button type="button" class="tb-btn nw-action-btn state-toggle-btn notify-btn nw-tooltip-anchor" :class="{ 'is-enabled': enableNotify }" :data-tooltip="`推送：${enableNotify ? '已开启' : '已关闭'}`" :aria-label="`推送：${enableNotify ? '已开启' : '已关闭'}`" :aria-pressed="enableNotify" @click="toggleNotify">
            <UIcon :name="enableNotify ? 'i-mdi-bell-ring-outline' : 'i-mdi-bell-outline'" class="w-5 h-5" />
          </button>
          <div ref="visibilityControlRef" class="visibility-control nw-action-btn nw-action-btn--label nw-tooltip-anchor" :data-tooltip="`可见范围：${visibilityLabel}`">
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
          <div v-if="canSetPublishTime" ref="publishTimeControlRef" class="publish-time-control nw-action-btn nw-action-btn--label nw-tooltip-anchor" :data-tooltip="publishTimeLabel === '选择时间' ? '自定义发布时间' : `发布时间：${publishTimeLabel}`">
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
          <button type="button" class="tb-btn nw-action-btn nw-action-btn--danger danger nw-tooltip-anchor" data-tooltip="清除" aria-label="清除" @click="clearForm"><UIcon name="i-heroicons-trash" class="w-5 h-5" /></button>
          <button type="button" class="tb-btn nw-action-btn nw-action-btn--primary primary nw-tooltip-anchor" data-tooltip="发布" aria-label="发布" @click="addMessage"><UIcon name="i-mdi-send" class="w-5 h-5" /></button>
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
      :class="['floating-control-menu visibility-floating-menu nw-floating-menu', { 'is-dark': contentTheme === 'dark' }]"
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
      :class="{ 'is-dark': contentTheme === 'dark' }"
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
            :class="{ 'is-current': hour === publishCurrentHour, 'is-selected': hour === publishDraftHour }"
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
            :class="{ 'is-current': minute === publishCurrentMinute, 'is-selected': minute === publishDraftMinute }"
            @click="setPublishMinute(minute)"
          >
            {{ pad2(minute) }}
          </button>
        </div>
      </div>
      <div class="publish-date-actions">
        <button type="button" class="floating-action-btn clear-action-btn nw-action-btn nw-action-btn--label nw-action-btn--danger" @click="clearPublishDate">清除</button>
        <button type="button" class="floating-action-btn cancel-action-btn nw-action-btn nw-action-btn--label" @click="usePublishNow">现在</button>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="openPublishPicker"
      ref="publishPickerMenuRef"
      class="publish-picker-floating-menu nw-floating-menu"
      :class="[`is-${openPublishPicker}`, { 'is-dark': contentTheme === 'dark' }]"
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
import { clamp, getFixedCoordinateScale, getFixedRect, getFixedViewport, positionFloatingMenu, scheduleFloatingMenuPosition } from '~/utils/floating-menu'
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
import AudioRecorder from './AudioRecorder.vue'
import VideoUpload from './VideoUpload.vue'
import ImageHostingUploader from '~/components/widgets/ImageHostingUploader.vue'
import { createAudioMarkdown, createVideoMarkdown, resolveUploadedMediaUrl, uploadMediaFiles } from '~/utils/media-upload'
import { createMediaFancyboxOptions } from '~/utils/media-fancybox'
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
const FULL_IMAGE_ATTACHMENTS_MARKER = '<!-- noise-full-image-attachments -->'
const FULL_IMAGE_ATTACHMENTS_MARKER_RE = /<!--\s*noise-full-image-attachments\s*-->\s*/gi
const hasFullImageAttachmentsMarker = (content: string) => {
  FULL_IMAGE_ATTACHMENTS_MARKER_RE.lastIndex = 0
  return FULL_IMAGE_ATTACHMENTS_MARKER_RE.test(String(content || ''))
}
const stripFullImageAttachmentsMarker = (content: string) => String(content || '').replace(FULL_IMAGE_ATTACHMENTS_MARKER_RE, '').trimStart()
const fullImageAttachments = ref(false)
const toggleFullImageAttachments = () => {
  fullImageAttachments.value = !fullImageAttachments.value
}
const buildPublishContent = (content: string) => {
  const clean = stripFullImageAttachmentsMarker(content).trim()
  return fullImageAttachments.value ? `${FULL_IMAGE_ATTACHMENTS_MARKER}\n${clean}` : clean
}
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
const audioUploadProgress = ref(0)
const handleAudioUploadProgress = (percent: number) => {
  audioUploadProgress.value = percent
}
const imageUploadProgress = ref(0)
const activeUploadPercent = computed(() => {
  if (audioUploadProgress.value > 0 && audioUploadProgress.value < 100) return audioUploadProgress.value
  if (videoUploadProgress.value > 0 && videoUploadProgress.value < 100) return videoUploadProgress.value
  if (imageUploadProgress.value > 0 && imageUploadProgress.value < 100) return imageUploadProgress.value
  return 0
})
const activeUploadKind = computed(() => {
  if (audioUploadProgress.value > 0 && audioUploadProgress.value < 100) return 'audio'
  if (videoUploadProgress.value > 0 && videoUploadProgress.value < 100) return 'video'
  if (imageUploadProgress.value > 0 && imageUploadProgress.value < 100) return 'image'
  return ''
})
const activeUploadLabel = computed(() => {
  if (activeUploadKind.value === 'audio') return '音频'
  if (activeUploadKind.value === 'video') return '视频'
  return '图片'
})
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
const DEFAULT_POST_VISIBILITY: MessageVisibility = 'users'
const initialPostVisibility = (): MessageVisibility => {
  if (typeof window === 'undefined') return DEFAULT_POST_VISIBILITY
  const storedVisibility = localStorage.getItem('postVisibility')
  const storedPrivate = localStorage.getItem('postPrivate')
  if (storedVisibility || storedPrivate !== null) {
    return normalizeMessageVisibility(storedVisibility, storedPrivate === 'true')
  }
  return DEFAULT_POST_VISIBILITY
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
const publishCurrentHour = computed(() => new Date().getHours())
const publishCurrentMinute = computed(() => new Date().getMinutes())
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

const ADD_FORM_MARKDOWN_EMPTY_TABLE_CELL = ' '

const escapeAddFormMarkdownTableCellText = (value: string) => {
  const normalized = String(value || '')
    .replace(/\r\n?/g, '\n')
    .replace(/[\u200b\u200c\ufeff]/g, '')
    .replace(/\n/g, '<br />')
    .replace(/\|/g, '&#124;')
    .trim()
  return normalized || ADD_FORM_MARKDOWN_EMPTY_TABLE_CELL
}

const isAddFormTableBreakMarker = (node: HTMLElement) => {
  const text = String(node.textContent || '').replace(/[\u200b\u200c\ufeff]/g, '').trim()
  if (/^<br\s*\/?\s*>$/i.test(text)) return true
  const html = String(node.innerHTML || '').replace(/[\u200b\u200c\ufeff]/g, '').trim()
  return /^<br\s*\/?\s*>$/i.test(html) || /^<code\b[^>]*>\s*<br\s*\/?\s*>\s*<\/code>$/i.test(html)
}

const readAddFormTableCellText = (cell: HTMLTableCellElement) => {
  const clone = cell.cloneNode(true) as HTMLElement
  clone.querySelectorAll('.editor-attachment-preview').forEach((node) => node.remove())
  clone.querySelectorAll<HTMLElement>('[data-type="html-inline"], .vditor-ir__node').forEach((node) => {
    if (isAddFormTableBreakMarker(node)) node.replaceWith(document.createTextNode('\n'))
  })
  clone.querySelectorAll('br').forEach((br) => br.replaceWith(document.createTextNode('\n')))
  return String(clone.textContent || '').replace(/\u00a0/g, ' ')
}

const formatAddFormMarkdownTableRow = (cells: string[]) => `| ${cells.map(escapeAddFormMarkdownTableCellText).join(' | ')} |`

const formatAddFormMarkdownDividerLine = (colCount: number) => `| ${Array.from({ length: Math.max(1, colCount) }, () => '---').join(' | ')} |`

const serializeAddFormTableDomAsMarkdown = (table: HTMLTableElement) => {
  const rows = Array.from(table.rows).map((row) => Array.from(row.cells).map((cell) => readAddFormTableCellText(cell as HTMLTableCellElement)))
  if (!rows.length) return ''
  const colCount = Math.max(1, ...rows.map((row) => row.length))
  const normalizedRows = rows.map((row) => Array.from({ length: colCount }, (_, index) => row[index] ?? ''))
  const header = normalizedRows[0] || []
  return [
    formatAddFormMarkdownTableRow(header),
    formatAddFormMarkdownDividerLine(colCount),
    ...normalizedRows.slice(1).map(formatAddFormMarkdownTableRow)
  ].join('\n')
}

const getAddFormEditorEditableElement = (root: HTMLElement) => {
  const candidates = Array.from(root.querySelectorAll<HTMLElement>('.vditor-ir pre.vditor-reset, .vditor-wysiwyg pre.vditor-reset, .vditor-sv .vditor-reset'))
  return candidates.find((node) => !!node.querySelector('table'))
    || candidates.find((node) => node.offsetParent !== null || node.getClientRects().length > 0)
    || candidates[0]
    || null
}

const readEditorDomTableSafeContent = () => {
  if (typeof document === 'undefined') return ''
  const root = document.querySelector<HTMLElement>('.editor-box .vditor, .vditor-container.vditor, .vditor')
  if (!root?.querySelector('.vditor-reset table')) return ''
  const editable = getAddFormEditorEditableElement(root)
  if (!editable) return ''
  const clone = editable.cloneNode(true) as HTMLElement
  clone.querySelectorAll('.editor-table-delete-button, .editor-table-expand-button, .editor-attachment-preview').forEach((node) => node.remove())
  clone.querySelectorAll('table').forEach((node) => {
    const markdown = serializeAddFormTableDomAsMarkdown(node as HTMLTableElement)
    node.replaceWith(document.createTextNode(markdown ? `\n${markdown}\n` : ''))
  })
  clone.querySelectorAll('br').forEach((br) => br.replaceWith(document.createTextNode('\n')))
  return String(clone.textContent || '').replace(/[\u200b\u200c\ufeff]/g, '').replace(/\u00a0/g, ' ').trim()
}

const readSafeEditorContent = () => {
  try {
    const val = vditorEditor.value?.getValue?.()
    if (typeof val === 'string') return val
  } catch {}
  const domTableContent = readEditorDomTableSafeContent()
  if (domTableContent) return domTableContent
  return MessageContent.value || ''
}

const syncContentFromEditor = () => {
  const val = readSafeEditorContent()
  if (val !== MessageContent.value) MessageContent.value = val
  return val
}

const saveDraft = () => {
  try {
    const editorContent = syncContentFromEditor()
    const content = (editorContent || '').trim()
    if (!content) {
      localStorage.removeItem(DRAFT_KEY)
      return
    }
    localStorage.setItem(
      DRAFT_KEY,
      JSON.stringify({ content: editorContent || '', private: !!Private.value, visibility: Visibility.value, notify: !!enableNotify.value, fullImageAttachments: !!fullImageAttachments.value, savedAt: Date.now() })
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

const previewProseClass = computed(() => contentTheme.value === 'dark' ? 'prose prose-invert' : 'prose')

const notifyStore = useNotifyStore()
const enableNotify = ref(false)

const clearForm = () => {
  Username.value = "";
  MessageContent.value = "";
  MessageContentHtml.value = "";
  PublishedAtInput.value = "";
  fullImageAttachments.value = false;
  enableNotify.value = false;
  Visibility.value = DEFAULT_POST_VISIBILITY;
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

const handleAudioUploaded = (audioUrl: string) => {
  const audioTag = createAudioMarkdown(resolveUploadedMediaUrl(audioUrl, String(BASE_API || '/api')))
  if (vditorEditor.value?.insertValue) {
    vditorEditor.value.insertValue(audioTag)
    syncContentFromEditor()
    focusEditor()
  }
};

const INLINE_IMAGE_REG = /!\s*(https?:\/\/[^\s!]+\.(?:png|jpe?g|gif|webp))(?:\?[^\s!]*)?/gi;
const normalizeInlineImageLinks = (md: string): string => md.replace(INLINE_IMAGE_REG, (m, url) => `![](${url})`);
const ATTACHMENT_LINK_REG = /\[(图片附件|视频附件|音频附件)：([^\]]+)\]\(([^)\s]+)\)/g
const escapePreviewAttr = (value: string) => String(value || '')
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;')
const replaceAttachmentMarkersForPreview = (md: string): string => String(md || '').replace(ATTACHMENT_LINK_REG, (_m, kind, name, url) => {
  const safeName = escapePreviewAttr(String(name || '').trim() || '未命名附件')
  const safeUrl = escapePreviewAttr(String(url || '').trim())
  if (!safeUrl) return _m
  if (kind === '图片附件') return `![${safeName}](${safeUrl})`
  if (kind === '视频附件') return `<video src="${safeUrl}" controls preload="metadata" style="width:100%;height:auto"></video>`
  return `<audio src="${safeUrl}" controls preload="metadata"></audio>`
})

const applyImageGridHTML = (html: string, keepImagesFullSize = false) => {
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

  const wrapFullImageParagraph = (p: Element, group: string) => {
    const payload = getSingleImagePayload(p);
    if (!payload) return;
    const wrapper = doc.createElement('div');
    wrapper.className = 'full-image-attachment';
    wrapper.appendChild(ensurePreviewImageAnchor(payload, group));
    p.replaceWith(wrapper);
  };

  const wrapSingleImageParagraph = (p: Element) => {
    const payload = getSingleImagePayload(p);
    if (!payload) return;
    const wrapper = doc.createElement('div');
    wrapper.className = 'single-media inline-image-thumb';
    wrapper.appendChild(ensurePreviewImageAnchor(payload, 'editor-preview-image'));
    p.replaceWith(wrapper);
  };

  if (keepImagesFullSize) {
    Array.from(doc.body.querySelectorAll('p')).forEach((p, index) => {
      if (isPureImageParagraph(p)) wrapFullImageParagraph(p, `editor-preview-full-image-${index}`);
    });
    return doc.body.innerHTML;
  }

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

watch([MessageContent, fullImageAttachments], ([val]) => {
  scheduleDraftSave()
  if (previewRenderTimer) clearTimeout(previewRenderTimer)
  previewRenderTimer = setTimeout(async () => {
    const rawValue = String(readSafeEditorContent() || val || "")
    if (rawValue !== MessageContent.value) MessageContent.value = rawValue
    const keepImagesFullSize = fullImageAttachments.value || hasFullImageAttachmentsMarker(rawValue)
    const previewValue = replaceAttachmentMarkersForPreview(stripFullImageAttachmentsMarker(rawValue))
    const raw = await Vditor.md2html(normalizeInlineImageLinks(previewValue));
    MessageContentHtml.value = applyImageGridHTML(raw, keepImagesFullSize);
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
  if (newLoginState) enableNotify.value = false;
}, { immediate: true });

onMounted(async () => {
  Fancybox.bind("[data-fancybox]", createMediaFancyboxOptions({ video: true }) as any);
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
        fullImageAttachments.value = typeof draft?.fullImageAttachments === 'boolean'
          ? draft.fullImageAttachments
          : hasFullImageAttachmentsMarker(draftContent)
        const editorContent = stripFullImageAttachmentsMarker(draftContent)
        MessageContent.value = editorContent
        Visibility.value = normalizeMessageVisibility(draft?.visibility, typeof draft?.private === 'boolean' ? draft.private : Visibility.value === 'private')
        if (typeof draft?.notify === 'boolean') enableNotify.value = draft.notify
        vditorEditor.value?.setValue?.(editorContent)
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
  Fancybox.unbind?.('[data-fancybox]');
  document.removeEventListener('mousedown', handleFloatingMenuPointerDown)
  window.removeEventListener('resize', handleFloatingMenuViewportChange)
  window.removeEventListener('scroll', handleFloatingMenuViewportChange, true)
  window.visualViewport?.removeEventListener('resize', handleFloatingMenuViewportChange)
  window.visualViewport?.removeEventListener('scroll', handleFloatingMenuViewportChange)
});
const toggleNotify = () => {
  enableNotify.value = !enableNotify.value;
};

const addMessage = async () => {
  if (!checkLogin()) return;
  syncContentFromEditor()

  if (!MessageContent.value.trim()) {
    toast.add({
      title: '错误',
      description: '请输入内容或上传图片/视频/音频',
      color: 'red',
      timeout: 2000
    });
    return;
  }

  const message: MessageToSave = {
    username: Username.value,
    content: buildPublishContent(MessageContent.value),
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
.editor-toolbar { display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:6px; padding:6px; border-radius:12px; background: rgba(255,255,255,0.85); flex-wrap: wrap; overflow: visible; position: relative; z-index: 95; backdrop-filter: saturate(1.1) blur(6px); }
.toolbar-left, .toolbar-right { display:flex; align-items:center; gap:8px; flex-wrap: wrap; }
.tb-btn { padding: 0; }
.state-toggle-btn { --nw-action-hover-border: rgba(15,23,42,0.16); --nw-action-hover-bg: rgba(15,23,42,0.1); --nw-action-hover-text: #111827; }
.state-toggle-btn.is-enabled { --nw-action-border: rgba(249,115,22,0.42); --nw-action-bg: rgba(249,115,22,0.18); --nw-action-text: #c2410c; --nw-action-hover-border: rgba(249,115,22,0.58); --nw-action-hover-bg: rgba(249,115,22,0.24); --nw-action-hover-text: #9a3412; }
.publish-time-control { max-width: min(210px, calc(100vw - 32px)); }
.visibility-control { width: max-content; }
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
.tb-sep { width:1px; height:24px; background: rgba(0,0,0,0.12); margin: 0 2px; }
.preview-card { backdrop-filter: blur(8px); background: #ffffff; border: 1px solid #e5e7eb; border-radius: 12px; padding: 8px; color:#111827; }
html.dark .editor-box { background: var(--home-surface-dark, #202a36); border: 1px solid rgba(255,255,255,0.16); color:#fff; }
html.dark .editor-toolbar { background: rgba(39, 50, 66, 0.68); backdrop-filter: saturate(1.1) blur(6px); }
html.dark .state-toggle-btn { --nw-action-hover-border: rgba(255,255,255,0.22); --nw-action-hover-bg: rgba(255,255,255,0.12); --nw-action-hover-text: #f8fafc; }
html.dark .state-toggle-btn.is-enabled { --nw-action-border: rgba(251,146,60,0.46); --nw-action-bg: rgba(249,115,22,0.26); --nw-action-text: #fed7aa; --nw-action-hover-border: rgba(251,146,60,0.66); --nw-action-hover-bg: rgba(249,115,22,0.34); --nw-action-hover-text: #fff7ed; }
html.dark .visibility-select { background: transparent; border: 0; color: inherit; }
:global(html.dark) .tb-sep { background: rgba(255,255,255,0.12); }

html.dark .preview-card { background: rgba(39, 50, 66, 0.68); border: 1px solid rgba(255,255,255,0.18); color:#fff; }
.editor-toolbar :deep(.u-button) { border:none !important; box-shadow:none !important; background: transparent !important; color:#374151 !important; }
html.dark .editor-toolbar :deep(.u-button) { border:none !important; box-shadow:none !important; background: rgba(255,255,255,0.06) !important; color:#cbd5e1 !important; }
.upload-progress { flex-basis: 100%; order: 10; display: flex; align-items: center; gap: 10px; pointer-events: none; padding: 0 4px; margin-top: 6px; }
.upload-progress-track { flex: 1; height: 4px; border-radius: 999px; background: rgba(0,0,0,0.12); overflow: hidden; }
.upload-progress-fill { height: 100%; border-radius: 999px; }
.upload-progress-fill.image { background: linear-gradient(90deg, rgba(167,139,250,1), rgba(244,114,182,1)); }
.upload-progress-fill.video { background: linear-gradient(90deg, rgba(96,165,250,1), rgba(52,211,153,1)); }
.upload-progress-fill.audio { background: linear-gradient(90deg, rgba(249,115,22,1), rgba(245,158,11,1)); }
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
.editor-preview :deep(.full-image-attachment) {
  width: 100%;
  max-width: 100%;
  margin: 8px 0;
  overflow: visible;
}
.editor-preview :deep(.full-image-attachment > a) {
  display: block;
  width: 100%;
  max-width: 100%;
}
.editor-preview :deep(.full-image-attachment img) {
  display: block;
  width: auto !important;
  max-width: 100% !important;
  height: auto !important;
  min-height: 0 !important;
  margin: 0 !important;
  object-fit: contain !important;
  border-radius: 12px;
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
