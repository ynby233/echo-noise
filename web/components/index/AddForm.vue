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
            @change="addImage"
            class="hidden"
            placeholder="选择图片"
          />
          <!-- 视频上传按钮 -->
  <VideoUpload
    @video-uploaded="handleVideoUploaded"
    @before-upload="checkVideoLogin"
    @upload-progress="handleVideoUploadProgress"
  />
          <button class="tb-btn" @click="triggerFileInput" title="插入图片"><UIcon name="i-fluent-image-20-regular" class="w-5 h-5" /></button>
           <!-- 新增图床上传按钮 -->
           <button class="tb-btn" @click="showImageUploader = true" title="图床上传"><UIcon name="i-mdi-cloud-upload-outline" class="w-5 h-5" /></button>
          
          <div ref="visibilityControlRef" class="visibility-control" :title="`可见范围：${visibilityLabel}`">
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
          <button class="tb-btn" @click="toggleNotify" :title="enableNotify ? '关闭推送' : '开启推送'">
            <UIcon :name="enableNotify ? 'i-mdi-bell' : 'i-mdi-bell-off'" class="w-5 h-5" />
          </button>          
          <div v-if="canSetPublishTime" ref="publishTimeControlRef" class="publish-time-control" title="自定义发布时间">
            <UIcon name="i-mdi-calendar-clock-outline" class="w-4 h-4" />
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
          <button class="tb-btn" @click="clearForm" title="清空"><UIcon name="i-fluent-broom-16-regular" class="w-5 h-5" /></button>
          <button class="tb-btn primary" @click="addMessage" title="发布"><UIcon name="i-fluent-add-12-filled" class="w-5 h-5" /></button>
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
      class="floating-control-menu visibility-floating-menu"
      :style="visibilityMenuStyle"
      role="listbox"
      @mousedown.stop
    >
      <button
        v-for="option in messageVisibilityOptions"
        :key="option.value"
        type="button"
        class="floating-control-option"
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
      class="floating-control-menu publish-datetime-menu"
      :style="publishDateMenuStyle"
      role="dialog"
      aria-label="发布时间选择"
      @mousedown.stop
    >
      <div class="publish-date-head">
        <button type="button" class="floating-icon-btn" aria-label="上个月" @click="movePublishMonth(-1)">
          <UIcon name="i-heroicons-chevron-left" class="w-4 h-4" />
        </button>
        <span class="publish-date-title">{{ publishPickerTitle }}</span>
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
        <div class="publish-time-column" aria-label="小时">
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
        <div class="publish-time-column" aria-label="分钟">
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
const visibilityControlRef = ref<HTMLElement | null>(null)
const visibilityMenuRef = ref<HTMLElement | null>(null)
const publishTimeControlRef = ref<HTMLElement | null>(null)
const publishDateMenuRef = ref<HTMLElement | null>(null)
const visibilityMenuStyle = ref<Record<string, string>>({})
const publishDateMenuStyle = ref<Record<string, string>>({})
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

const normalizeCloudObjectURL = (u: string): string => {
  const raw = String(u || '')
  if (!/^https?:\/\//.test(raw)) return raw
  try {
    const parsed = new URL(raw)
    const parts = parsed.pathname.split('/').filter(Boolean)
    if (parts[0] === 'note') {
      parsed.pathname = '/' + parts.slice(1).join('/')
      return parsed.toString()
    }
    return raw
  } catch {
    return raw.replace('/note/', '/')
  }
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

const publishPickerTitle = computed(() => `${publishPickerMonth.value.getFullYear()}年${publishPickerMonth.value.getMonth() + 1}月`)
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

const positionFloatingMenu = (
  trigger: HTMLElement | null,
  menu: HTMLElement | null,
  styleRef: Ref<Record<string, string>>,
  minWidth = 120
) => {
  if (!trigger || typeof window === 'undefined') return
  const rect = trigger.getBoundingClientRect()
  const menuWidth = Math.max(menu?.offsetWidth || minWidth, minWidth, rect.width)
  const menuHeight = menu?.offsetHeight || 180
  const pad = 8
  const gap = 6
  const left = Math.min(Math.max(rect.left, pad), Math.max(pad, window.innerWidth - menuWidth - pad))
  const belowTop = rect.bottom + gap
  const aboveTop = rect.top - menuHeight - gap
  const top = belowTop + menuHeight <= window.innerHeight - pad || aboveTop < pad ? belowTop : aboveTop
  styleRef.value = {
    left: `${left}px`,
    top: `${Math.min(Math.max(top, pad), Math.max(pad, window.innerHeight - menuHeight - pad))}px`,
    minWidth: `${Math.max(minWidth, rect.width)}px`
  }
}

const closeFloatingMenus = () => {
  showVisibilityMenu.value = false
  showPublishDateMenu.value = false
}

const positionVisibilityMenu = () => positionFloatingMenu(visibilityControlRef.value, visibilityMenuRef.value, visibilityMenuStyle, 126)
const positionPublishDateMenu = () => positionFloatingMenu(publishTimeControlRef.value, publishDateMenuRef.value, publishDateMenuStyle, 292)

const toggleVisibilityMenu = async () => {
  showPublishDateMenu.value = false
  showVisibilityMenu.value = !showVisibilityMenu.value
  if (showVisibilityMenu.value) {
    await nextTick()
    positionVisibilityMenu()
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
  showPublishDateMenu.value = !showPublishDateMenu.value
  if (showPublishDateMenu.value) {
    syncPublishDraftFromInput()
    await nextTick()
    positionPublishDateMenu()
  }
}

const movePublishMonth = (delta: number) => {
  publishPickerMonth.value = new Date(publishPickerMonth.value.getFullYear(), publishPickerMonth.value.getMonth() + delta, 1)
  nextTick(positionPublishDateMenu)
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
}

const clearPublishDate = () => {
  PublishedAtInput.value = ''
  showPublishDateMenu.value = false
}

const handleFloatingMenuPointerDown = (event: MouseEvent) => {
  const target = event.target as Node | null
  if (!target) return
  if (visibilityControlRef.value?.contains(target) || visibilityMenuRef.value?.contains(target)) return
  if (publishTimeControlRef.value?.contains(target) || publishDateMenuRef.value?.contains(target)) return
  closeFloatingMenus()
}

const handleFloatingMenuViewportChange = () => {
  if (showVisibilityMenu.value) positionVisibilityMenu()
  if (showPublishDateMenu.value) positionPublishDateMenu()
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
  const input = document.getElementById("file-input");
  if (input) {
    input.click();
  }
};

const addImage = async (event: Event) => {
  if (!checkLogin()) return;
  const input = event.target as HTMLInputElement;
  const file = input.files ? input.files[0] : null;

  if (!file) {
    toast.add({
      title: '错误',
      description: '没有选择文件',
      color: 'red',
      timeout: 2000
    });
    return;
  }

  const allowedTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/webp'];
  const allowedExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.webp'];
  const fileExtension = file.name.toLowerCase().substring(file.name.lastIndexOf('.'));
  if (!allowedTypes.includes(file.type) || !allowedExtensions.includes(fileExtension)) {
    toast.add({
      title: '错误',
      description: '仅支持 JPG、PNG、GIF、WEBP 格式的图片',
      color: 'red',
      timeout: 2000
    });
    return;
  }
  const maxSize = 50 * 1024 * 1024; // 50MB
  if (file.size > maxSize) {
    toast.add({
      title: '错误',
      description: '图片大小不能超过 50MB',
      color: 'red',
      timeout: 2000
    });
    return;
  }

  try {
    const formData = new FormData();
    formData.append('image', file);
    imageUploadProgress.value = 1
    const data = await new Promise<any>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('POST', `${BASE_API}/images/upload`, true)
      xhr.withCredentials = true
      const token = userStore.token || ''
      if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
      xhr.upload.onprogress = (e) => {
        if (!e.lengthComputable) return
        const percent = Math.round((e.loaded / e.total) * 100)
        imageUploadProgress.value = Math.max(1, Math.min(99, percent))
      }
      xhr.onload = () => {
        try {
          const js = JSON.parse(xhr.responseText || '{}')
          if (xhr.status >= 200 && xhr.status < 300) {
            resolve(js)
          } else {
            reject(new Error(js?.msg || '图片上传失败'))
          }
        } catch (e: any) {
          reject(new Error(e?.message || '图片上传失败'))
        }
      }
      xhr.onerror = () => reject(new Error('图片上传失败'))
      xhr.send(formData)
    })

    if (data?.code === 1 && data?.data) {
      if (vditorEditor.value?.insertValue) {
        const origin = typeof window !== 'undefined' ? window.location.origin : ''
        const base = String(BASE_API || '/api')
        const ret = String(data.data || '')
        let full = ''
        if (ret.startsWith('http')) {
          full = normalizeCloudObjectURL(ret)
        } else {
          const path = ret.startsWith('/') ? ret : `/${ret}`
          if (/^https?:\/\//.test(base)) {
            full = `${base}${path}`
          } else {
            const cleanBase = base.replace(/\/$/, '')
            if (path.startsWith(cleanBase)) {
              full = `${origin}${path}`
            } else {
              full = `${origin}${cleanBase}${path}`
            }
          }
        }
        const imageMarkdown = `\n![](${full})\n`
        vditorEditor.value.insertValue(imageMarkdown)
        syncContentFromEditor()
        focusEditor()
      }
      imageUploadProgress.value = 100
      setTimeout(() => { imageUploadProgress.value = 0 }, 400)
      toast.add({
        title: '成功',
        description: '图片上传成功',
        color: 'green',
        timeout: 2000
      });
    } else {
      throw new Error(data?.msg || '图片上传失败');
    }
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
  const raw = String(videoUrl || '')
  const baseApi = useRuntimeConfig().public.baseApi || '/api'
  let full = raw
  if (/^https?:\/\//.test(raw)) {
    full = normalizeCloudObjectURL(raw)
  } else {
    const path = raw.startsWith('/') ? raw : `/${raw}`
    if (/^https?:\/\//.test(String(baseApi))) {
      const base = String(baseApi).replace(/\/api$/, '')
      full = `${base}${path}`
    } else {
      const origin = typeof window !== 'undefined' ? window.location.origin : ''
      full = `${origin}${path}`
    }
  }
  const videoTag = `<video width="100%" height="100%" src="${full}" controls loop></video>\n`
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
  const isPureImageParagraph = (p: Element) => {
    let ok = true;
    const children = Array.from(p.childNodes);
    if (children.length === 0) return false;
    for (const node of children) {
      if (node.nodeType === Node.ELEMENT_NODE) {
        const el = node as Element;
        const tag = el.tagName.toLowerCase();
        if (tag === 'img') continue;
        if (tag === 'a' && el.childElementCount === 1 && el.querySelector('img')) continue;
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
});
const toggleNotify = () => {
  enableNotify.value = !enableNotify.value;
  localStorage.setItem('enableNotify', enableNotify.value.toString());
};

const checkVideoLogin = (e: Event) => {
  if (!userStore.isLogin) {
    toast.add({
      title: '提示',
      description: '请登录后操作',
      color: 'orange',
      timeout: 2000
    });
    e.preventDefault && e.preventDefault();
    return false;
  }
  return true;
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
.editor-toolbar { display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:6px; padding:6px; border-radius:12px; background: rgba(255,255,255,0.85); flex-wrap: wrap; overflow:hidden; position: sticky; bottom: 0; z-index: 95; backdrop-filter: saturate(1.1) blur(6px); --publish-control-bg: rgba(17, 24, 39, 0.86); --publish-control-border: rgba(148, 163, 184, 0.34); --publish-control-border-active: rgba(96, 165, 250, 0.68); --publish-control-text: #f8fafc; }
.toolbar-left, .toolbar-right { display:flex; align-items:center; gap:8px; flex-wrap: wrap; }
.tb-btn { display:flex; align-items:center; justify-content:center; width:36px; height:36px; border-radius:12px; background: rgba(0,0,0,0.06); color:#374151; transition: all .18s ease; border:none; }
.tb-btn:hover { transform: translate3d(0,0,0) scale(1.06); background: rgba(0,0,0,0.12); }
.tb-btn.primary { background: linear-gradient(135deg, rgba(251,146,60,.95), rgba(234,88,12,.95)); color: #fff; }
.publish-time-control { display:flex; align-items:center; gap:6px; min-height:36px; border-radius:12px; background: var(--publish-control-bg); color:var(--publish-control-text); padding:0 10px; border: 1px solid var(--publish-control-border); box-shadow: none; backdrop-filter: blur(6px); -webkit-backdrop-filter: blur(6px); transition: border-color .15s ease, background-color .15s ease; }
.visibility-control { display:flex; align-items:center; gap:6px; min-height:36px; border-radius:12px; background: var(--publish-control-bg); color:var(--publish-control-text); padding:0 8px; border: 1px solid var(--publish-control-border); box-shadow: none; backdrop-filter: blur(6px); -webkit-backdrop-filter: blur(6px); transition: border-color .15s ease, background-color .15s ease; }
.publish-time-control:hover,
.publish-time-control:focus-within,
.visibility-control:hover,
.visibility-control:focus-within { border-color: var(--publish-control-border-active); }
.visibility-select { width: 82px; height: 28px; padding: 0 6px; border: 1px solid rgba(148, 163, 184, 0.28); border-radius: 9px; outline: none; background: rgba(15, 23, 42, 0.46); color: inherit; font-size: 12px; cursor: pointer; }
.visibility-trigger,
.publish-time-trigger { display: inline-flex; align-items: center; justify-content: space-between; gap: 4px; }
.visibility-trigger svg,
.publish-time-trigger svg { flex: 0 0 auto; opacity: .72; }
.publish-time-input { width: 166px; max-width: 48vw; min-height: 28px; border: none; outline: none; background: transparent; color: inherit; font-size: 12px; text-align: left; }
.publish-time-trigger span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.floating-control-menu { position: fixed; z-index: 5004; border: 1px solid rgba(255,255,255,0.16); border-radius: 12px; background: rgba(0,0,0,0.80); color: #f8fafc; box-shadow: 0 18px 42px rgba(0,0,0,0.38); backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px); }
.visibility-floating-menu { display: grid; gap: 4px; padding: 8px; }
.floating-control-option { display: flex; align-items: center; gap: 8px; min-height: 32px; padding: 0 10px; border-radius: 9px; border: 1px solid transparent; color: inherit; font-size: 12px; font-weight: 650; text-align: left; transition: background-color .15s ease, border-color .15s ease, color .15s ease; }
.floating-control-option:hover,
.floating-control-option:focus-visible { outline: none; border-color: rgba(249,115,22,0.38); background: rgba(249,115,22,0.18); }
.floating-control-option.is-selected { border-color: rgba(249,115,22,0.7); background: rgba(249,115,22,0.30); color: #fff; }
.publish-datetime-menu { width: 292px; padding: 10px; }
.publish-date-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 8px; }
.publish-date-title { font-size: 13px; font-weight: 700; color: #fff; }
.floating-icon-btn { width: 28px; height: 28px; display: inline-flex; align-items: center; justify-content: center; border-radius: 8px; border: 1px solid rgba(255,255,255,0.14); background: rgba(255,255,255,0.06); color: inherit; }
.floating-icon-btn:hover { border-color: rgba(249,115,22,0.34); background: rgba(249,115,22,0.16); }
.publish-date-weekdays,
.publish-date-grid { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 3px; }
.publish-date-weekdays { margin-bottom: 4px; color: rgba(226,232,240,0.66); font-size: 10px; font-weight: 700; text-align: center; }
.publish-date-day { height: 28px; border-radius: 8px; border: 1px solid transparent; background: rgba(255,255,255,0.06); color: #f8fafc; font-size: 12px; line-height: 1; }
.publish-date-day:hover { border-color: rgba(249,115,22,0.34); background: rgba(249,115,22,0.16); }
.publish-date-day.is-muted { opacity: .38; }
.publish-date-day.is-today { border-color: rgba(96,165,250,0.68); background: rgba(59,130,246,0.22); }
.publish-date-day.is-selected { border-color: rgba(249,115,22,0.82); background: rgba(249,115,22,0.34); color: #fff; }
.publish-time-panel { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin-top: 10px; }
.publish-time-column { display: grid; grid-auto-rows: 28px; gap: 4px; max-height: 116px; overflow-y: auto; padding: 4px; border-radius: 10px; background: rgba(15,23,42,0.46); scrollbar-width: thin; }
.publish-time-option { border-radius: 7px; border: 1px solid transparent; color: inherit; font-size: 12px; font-weight: 650; }
.publish-time-option:hover { border-color: rgba(249,115,22,0.34); background: rgba(249,115,22,0.16); }
.publish-time-option.is-selected { border-color: rgba(249,115,22,0.7); background: rgba(249,115,22,0.30); color: #fff; }
.publish-date-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px; }
.floating-action-btn { height: 30px; padding: 0 12px; border-radius: 9px; border: 1px solid rgba(255,255,255,0.14); background: rgba(255,255,255,0.06); color: inherit; font-size: 12px; font-weight: 650; }
.floating-action-btn:hover { border-color: rgba(249,115,22,0.34); background: rgba(249,115,22,0.16); }
.floating-action-btn.primary { border-color: rgba(249,115,22,0.72); background: rgba(249,115,22,0.32); color: #fff; }
.tb-sep { width:1px; height:24px; background: rgba(0,0,0,0.12); margin: 0 2px; }
.preview-card { backdrop-filter: blur(8px); background: #ffffff; border: 1px solid #e5e7eb; border-radius: 12px; padding: 8px; color:#111827; }
html.dark .editor-box { background: var(--home-surface-dark, #202a36); border: 1px solid rgba(255,255,255,0.16); color:#fff; }
html.dark .editor-toolbar { background: rgba(39, 50, 66, 0.68); backdrop-filter: saturate(1.1) blur(6px); }
html.dark .tb-btn { background: rgba(255,255,255,0.06); color:#cbd5e1; border:none; }
html.dark .tb-btn:hover { background: rgba(255,255,255,0.12); }
html.dark .publish-time-control,
html.dark .visibility-control { background: rgba(17,24,39,0.86); color:#f8fafc; border-color: rgba(148,163,184,0.34); }
html.dark .visibility-select { background: rgba(30,41,59,0.82); border-color: rgba(148,163,184,0.36); color:#f8fafc; }
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
