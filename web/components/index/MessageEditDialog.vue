<template>
<UModal v-model="showEditModal" :ui="{ width: 'sm:max-w-3xl' }">
    <div class="edit-modal-shell" :class="{ 'is-dark': props.isContentDark }">
      <input
        ref="editAttachmentInputRef"
        type="file"
        multiple
        class="hidden"
        @change="handleEditAttachmentChange"
      />

      <div class="edit-modal-header">
        <div class="edit-modal-title-block">
          <h3 class="edit-modal-title">编辑内容</h3>
        </div>
        <button type="button" class="edit-icon-button nw-action-btn nw-tooltip-anchor" data-tooltip="关闭" aria-label="关闭" @click="showEditModal = false">
          <UIcon name="i-mdi-close" class="w-5 h-5" />
        </button>
      </div>

      <div class="edit-modal-body">
        <textarea
          ref="editTextareaRef"
          v-model="editingContent"
          placeholder="编辑内容..."
          rows="10"
          class="edit-content-textarea"
        />

        <div class="edit-toolbar">
          <div class="edit-toolbar-left">
            <AudioRecorder
              @audio-uploaded="handleEditAudioUploaded"
              @upload-progress="handleEditAudioUploadProgress"
              @prepare-insert="prepareEditAudioInsert"
              @insert-cancelled="clearEditAudioInsertTarget"
            />
            <button
              type="button"
              class="tb-btn edit-media-button nw-action-btn nw-tooltip-anchor"
              data-tooltip="上传附件"
              aria-label="上传附件"
              :disabled="isEditUploading"
              @click="triggerEditAttachmentInput"
            >
              <UIcon :name="editUploadKind === 'attachment' ? 'i-mdi-loading' : 'i-heroicons-paper-clip'" class="w-5 h-5" :class="{ 'edit-spin': editUploadKind === 'attachment' }" />
            </button>
            <div v-if="canChangeVisibility" ref="editVisibilityControlRef" class="visibility-control nw-action-btn nw-action-btn--label nw-tooltip-anchor" :data-tooltip="`可见范围：${editVisibilityLabel}`">
              <UIcon :name="editVisibilityIcon" class="w-5 h-5" />
              <button
                type="button"
                class="visibility-trigger"
                aria-label="选择可见范围"
                aria-haspopup="listbox"
                :aria-expanded="showEditVisibilityMenu"
                @click.stop="toggleEditVisibilityMenu"
              >
                <span>{{ editVisibilityLabel }}</span>
                <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
              </button>
            </div>
            <div v-if="canChangePublishTime" ref="editPublishTimeControlRef" class="publish-time-control nw-action-btn nw-action-btn--label nw-tooltip-anchor" :data-tooltip="editPublishTimeLabel === '选择时间' ? '自定义发布时间' : `发布时间：${editPublishTimeLabel}`">
              <UIcon name="i-mdi-calendar-clock-outline" class="w-5 h-5" />
              <button
                type="button"
                class="publish-time-trigger"
                aria-label="选择发布时间"
                aria-haspopup="dialog"
                :aria-expanded="showEditPublishDateMenu"
                @click.stop="toggleEditPublishDateMenu"
              >
                <span>{{ editPublishTimeLabel }}</span>
                <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
              </button>
            </div>
          </div>
        </div>

        <span v-if="isEditUploading" class="edit-upload-status">{{ editUploadLabel }} {{ editUploadProgress }}%</span>

        <div class="edit-preview-block">
          <div class="edit-preview-title">预览</div>
          <div :class="['edit-preview-surface', { 'edit-preview-surface--shadow-open': editPreviewHasAttachmentCard }]">
            <MarkdownRenderer :content="editingContent" :enableGithubCard="props.siteConfig?.enableGithubCard === true" />
          </div>
        </div>
      </div>

      <div class="edit-modal-footer">
        <button type="button" class="edit-footer-button nw-action-btn nw-action-btn--label" :disabled="isSaving" @click="showEditModal = false">取消</button>
        <button type="button" class="edit-footer-button nw-action-btn nw-action-btn--label nw-action-btn--primary" :disabled="isSaving" @click="saveEditedMessage">
          <UIcon v-if="isSaving" name="i-mdi-loading" class="w-4 h-4 edit-spin" />
          <span>{{ isSaving ? '保存中' : '保存' }}</span>
        </button>
      </div>
    </div>
  </UModal>

  <Teleport to="body">
    <div
      v-if="showEditVisibilityMenu"
      ref="editVisibilityMenuRef"
      class="floating-control-menu visibility-floating-menu nw-floating-menu"
      :style="editVisibilityMenuStyle"
      role="listbox"
      @mousedown.stop
    >
      <button
        v-for="option in messageVisibilityOptions"
        :key="option.value"
        type="button"
        class="floating-control-option nw-floating-option"
        :class="{ 'is-selected': option.value === editingVisibility }"
        role="option"
        :aria-selected="option.value === editingVisibility"
        @click="selectEditVisibility(option.value)"
      >
        <UIcon :name="option.icon" class="w-4 h-4" />
        <span>{{ option.label }}</span>
      </button>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="showEditPublishDateMenu"
      ref="editPublishDateMenuRef"
      class="floating-control-menu publish-datetime-menu nw-floating-menu"
      :class="{ 'is-dark': isContentDark }"
      :style="editPublishDateMenuStyle"
      role="dialog"
      aria-label="发布时间选择"
      @mousedown.stop
    >
      <div class="publish-date-head">
        <button type="button" class="floating-icon-btn" aria-label="上个月" @click="moveEditPublishMonth(-1)">
          <UIcon name="i-heroicons-chevron-left" class="w-4 h-4" />
        </button>
        <div class="publish-date-picker-controls" aria-label="选择年月">
          <button
            ref="editPublishYearPickerButton"
            type="button"
            class="publish-date-title publish-picker-trigger"
            aria-label="选择年份"
            aria-haspopup="listbox"
            :aria-expanded="openEditPublishPicker === 'year'"
            @click.stop="toggleEditPublishPicker('year')"
          >
            <span>{{ editPublishPickerMonth.getFullYear() }}年</span>
            <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
          </button>
          <button
            ref="editPublishMonthPickerButton"
            type="button"
            class="publish-date-title publish-picker-trigger"
            aria-label="选择月份"
            aria-haspopup="listbox"
            :aria-expanded="openEditPublishPicker === 'month'"
            @click.stop="toggleEditPublishPicker('month')"
          >
            <span>{{ editPublishPickerMonth.getMonth() + 1 }}月</span>
            <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
          </button>
        </div>
        <button type="button" class="floating-icon-btn" aria-label="下个月" @click="moveEditPublishMonth(1)">
          <UIcon name="i-heroicons-chevron-right" class="w-4 h-4" />
        </button>
      </div>
      <div class="publish-date-weekdays">
        <span v-for="label in editPublishWeekLabels" :key="label">{{ label }}</span>
      </div>
      <div class="publish-date-grid">
        <button
          v-for="day in editPublishPickerDays"
          :key="day.key"
          type="button"
          class="publish-date-day"
          :class="{
            'is-muted': !day.inMonth,
            'is-today': day.isToday,
            'is-selected': day.selected
          }"
          @click="selectEditPublishDay(day)"
        >
          {{ day.day }}
        </button>
      </div>
      <div class="publish-time-panel">
        <div ref="editPublishHourColumnRef" class="publish-time-column" aria-label="小时">
          <button
            v-for="hour in editPublishHourOptions"
            :key="hour"
            type="button"
            class="publish-time-option"
            :class="{ 'is-current': hour === editPublishCurrentHour, 'is-selected': hour === editPublishDraftHour }"
            @click="setEditPublishHour(hour)"
          >
            {{ pad2(hour) }}
          </button>
        </div>
        <div ref="editPublishMinuteColumnRef" class="publish-time-column" aria-label="分钟">
          <button
            v-for="minute in editPublishMinuteOptions"
            :key="minute"
            type="button"
            class="publish-time-option"
            :class="{ 'is-current': minute === editPublishCurrentMinute, 'is-selected': minute === editPublishDraftMinute }"
            @click="setEditPublishMinute(minute)"
          >
            {{ pad2(minute) }}
          </button>
        </div>
      </div>
      <div class="publish-date-actions">
        <button type="button" class="floating-action-btn clear-action-btn nw-action-btn nw-action-btn--label nw-action-btn--danger" @click="clearEditPublishDate">清除</button>
        <button type="button" class="floating-action-btn cancel-action-btn nw-action-btn nw-action-btn--label" @click="useEditPublishNow">现在</button>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="openEditPublishPicker"
      ref="editPublishPickerMenuRef"
      class="publish-picker-floating-menu nw-floating-menu"
      :class="[`is-${openEditPublishPicker}`, { 'is-dark': isContentDark }]"
      :style="editPublishPickerMenuStyle"
      role="listbox"
      @mousedown.stop
    >
      <button
        v-for="option in editPublishPickerOptions"
        :key="option.value"
        type="button"
        class="publish-picker-floating-option nw-floating-option"
        :class="{ 'is-selected': option.selected }"
        role="option"
        :aria-selected="option.selected"
        @click="selectEditPublishPickerValue(option.value)"
      >
        {{ option.label }}
      </button>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRuntimeConfig } from '#imports'
import { useToast } from '#ui/composables/useToast'
import AudioRecorder from './AudioRecorderButton.vue'
import MarkdownRenderer from './MarkdownRenderer.vue'
import type { MessageVisibility } from '~/types/models'
import { createAudioMarkdown, resolveUploadedMediaUrl, uploadMediaFiles } from '~/utils/media-upload'
import { applyCurrentEditOperation, createMessageEditSession, type MessageEditSessionToken } from '~/utils/message-edit-session'
import { messageVisibility, messageVisibilityIcon, messageVisibilityLabel, messageVisibilityOptions, messageVisibilityRequiresPrivate, normalizeMessageVisibility } from '~/utils/message-visibility'
import { useUserStore } from '~/store/user'

// Owns the complete edit-dialog lifecycle. MessageList only decides whether an
// edit may start and applies the saved patch emitted through this interface.
const props = defineProps<{ siteConfig?: any; isContentDark?: boolean }>()
const emit = defineEmits<{ saved: [payload: { id: number; patch: any }] }>()
const config = useRuntimeConfig()
const BASE_API = config.public.baseApi || '/api'
const userStore = useUserStore()

const showEditModal = ref(false);
const editingContent = ref('');
const editingMessageId = ref<number | null>(null);
const editingMessage = ref<any | null>(null);
const editingVisibility = ref<MessageVisibility>('public');
const editingPublishedAtInput = ref('');
const isSaving = ref(false);
// 附件标记会渲染成带外阴影的卡片/占位块（含已删除附件的失败占位块）。
// 预览面板默认是 300px 的滚动裁剪盒，会把这些阴影贴边切掉，
// 与正文里 file-attachment-shadow-open 放开裁剪后的观感不一致，所以这里做同样的判定。
const EDIT_ATTACHMENT_MARKER_RE = /\[(?:图片|视频|音频|文件)附件：/;
const editPreviewHasAttachmentCard = computed(() => EDIT_ATTACHMENT_MARKER_RE.test(editingContent.value));
const editTextareaRef = ref<any>(null);
const editAttachmentInputRef = ref<HTMLInputElement | null>(null);
type EditInsertTarget = { start: number; end: number; session: MessageEditSessionToken };
const editAudioInsertTarget = ref<EditInsertTarget | null>(null);
const editSession = createMessageEditSession()
const editUploadProgress = ref(0);
const editUploadKind = ref<'audio' | 'attachment' | ''>('');
const editUploadLabel = ref('');
const isEditUploading = computed(() => editUploadProgress.value > 0);
const editVisibilityLabel = computed(() => messageVisibilityLabel(editingVisibility.value));
const editVisibilityIcon = computed(() => messageVisibilityIcon(editingVisibility.value));
const showEditVisibilityMenu = ref(false);
const showEditPublishDateMenu = ref(false);
type EditPublishPickerType = 'year' | 'month';
type EditPublishDateDay = {
  key: string;
  date: string;
  day: number;
  inMonth: boolean;
  isToday: boolean;
  selected: boolean;
};
const EDIT_PUBLISH_MIN_YEAR = 1971;
const EDIT_PUBLISH_MAX_YEAR = 2099;
const editPublishYearOptions = Array.from({ length: EDIT_PUBLISH_MAX_YEAR - EDIT_PUBLISH_MIN_YEAR + 1 }, (_, index) => EDIT_PUBLISH_MIN_YEAR + index);
const editPublishMonthOptions = Array.from({ length: 12 }, (_, index) => index + 1);
const editPublishWeekLabels = ['一', '二', '三', '四', '五', '六', '日'];
const editPublishHourOptions = Array.from({ length: 24 }, (_, index) => index);
const editPublishMinuteOptions = Array.from({ length: 60 }, (_, index) => index);
const editPublishCurrentHour = computed(() => new Date().getHours());
const editPublishCurrentMinute = computed(() => new Date().getMinutes());
const openEditPublishPicker = ref<EditPublishPickerType | ''>('');
const editVisibilityControlRef = ref<HTMLElement | null>(null);
const editVisibilityMenuRef = ref<HTMLElement | null>(null);
const editPublishTimeControlRef = ref<HTMLElement | null>(null);
const editPublishDateMenuRef = ref<HTMLElement | null>(null);
const editPublishYearPickerButton = ref<HTMLElement | null>(null);
const editPublishMonthPickerButton = ref<HTMLElement | null>(null);
const editPublishPickerMenuRef = ref<HTMLElement | null>(null);
const editPublishHourColumnRef = ref<HTMLElement | null>(null);
const editPublishMinuteColumnRef = ref<HTMLElement | null>(null);
const editVisibilityMenuStyle = ref<Record<string, string>>({});
const editPublishDateMenuStyle = ref<Record<string, string>>({});
const editPublishPickerMenuStyle = ref<Record<string, string>>({});
const editPublishPickerMonth = ref(new Date(new Date().getFullYear(), new Date().getMonth(), 1));
const editPublishDraftDate = ref('');
const editPublishDraftHour = ref(0);
const editPublishDraftMinute = ref(0);
const pad2 = (value: number) => String(value).padStart(2, '0');
const formatEditLocalDate = (date: Date) => `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`;
const formatEditDatetimeLocal = (date: string, hour: number, minute: number) => `${date}T${pad2(hour)}:${pad2(minute)}`;
const parseEditDatetimeLocal = (value: string) => {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(String(value || '').trim());
  if (!match) return null;
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  if (Number.isNaN(date.getTime()) || hour < 0 || hour > 23 || minute < 0 || minute > 59) return null;
  return { date, dateText: formatEditLocalDate(date), hour, minute };
};
const editPublishTimeLabel = computed(() => {
  const parsed = parseEditDatetimeLocal(editingPublishedAtInput.value);
  if (!parsed) return '选择时间';
  return `${parsed.dateText} ${pad2(parsed.hour)}:${pad2(parsed.minute)}`;
});
const editPublishPickerOptions = computed(() => {
  if (openEditPublishPicker.value === 'year') {
    return editPublishYearOptions.map((year) => ({ value: year, label: `${year}年`, selected: year === editPublishPickerMonth.value.getFullYear() }));
  }
  if (openEditPublishPicker.value === 'month') {
    const current = editPublishPickerMonth.value.getMonth() + 1;
    return editPublishMonthOptions.map((month) => ({ value: month, label: `${month}月`, selected: month === current }));
  }
  return [];
});
const editPublishPickerDays = computed<EditPublishDateDay[]>(() => {
  const first = new Date(editPublishPickerMonth.value.getFullYear(), editPublishPickerMonth.value.getMonth(), 1);
  const startOffset = (first.getDay() + 6) % 7;
  const todayText = formatEditLocalDate(new Date());
  const days: EditPublishDateDay[] = [];
  for (let index = 0; index < 42; index += 1) {
    const date = new Date(first.getFullYear(), first.getMonth(), 1 - startOffset + index);
    const value = formatEditLocalDate(date);
    days.push({
      key: value,
      date: value,
      day: date.getDate(),
      inMonth: date.getMonth() === first.getMonth(),
      isToday: value === todayText,
      selected: value === editPublishDraftDate.value
    });
  }
  return days;
});

type EditFloatingMenuPlacement = 'below' | 'above-right';
const clampEditFloatingValue = (value: number, min: number, max: number) => Math.min(Math.max(value, min), Math.max(min, max));
const getEditFixedCoordinateScale = () => {
  if (typeof window === 'undefined') return 1;
  const zoom = Number.parseFloat(window.getComputedStyle(document.body).zoom || '1');
  return Number.isFinite(zoom) && zoom > 0 ? zoom : 1;
};
const getEditFixedViewport = (scale: number) => {
  const viewport = window.visualViewport;
  const left = (viewport?.offsetLeft || 0) / scale;
  const top = (viewport?.offsetTop || 0) / scale;
  const width = (viewport?.width || window.innerWidth) / scale;
  const height = (viewport?.height || window.innerHeight) / scale;
  return { left, top, right: left + width, bottom: top + height };
};
const getEditFixedRect = (element: HTMLElement, scale: number) => {
  const rect = element.getBoundingClientRect();
  const viewport = window.visualViewport;
  const offsetLeft = viewport?.offsetLeft || 0;
  const offsetTop = viewport?.offsetTop || 0;
  return {
    left: (rect.left + offsetLeft) / scale,
    right: (rect.right + offsetLeft) / scale,
    top: (rect.top + offsetTop) / scale,
    bottom: (rect.bottom + offsetTop) / scale,
    width: rect.width / scale,
    height: rect.height / scale
  };
};
const positionEditFloatingMenu = (
  trigger: HTMLElement | null,
  menu: HTMLElement | null,
  styleRef: { value: Record<string, string> },
  minWidth = 120,
  placement: EditFloatingMenuPlacement = 'below'
) => {
  if (!trigger || typeof window === 'undefined') return;
  const scale = getEditFixedCoordinateScale();
  const rect = getEditFixedRect(trigger, scale);
  const viewport = getEditFixedViewport(scale);
  const menuWidth = Math.max(menu?.offsetWidth || minWidth, minWidth, rect.width);
  const menuHeight = menu?.offsetHeight || 180;
  const pad = 8;
  const gap = 4;
  const minLeft = viewport.left + pad;
  const maxLeft = Math.max(minLeft, viewport.right - menuWidth - pad);
  const idealLeft = placement === 'above-right' ? rect.right - menuWidth : rect.left + rect.width / 2 - menuWidth / 2;
  const aboveTop = rect.top - menuHeight - gap;
  const belowTop = rect.bottom + gap;
  const minTop = viewport.top + pad;
  const maxTop = Math.max(minTop, viewport.bottom - menuHeight - pad);
  const idealTop = placement === 'above-right' && aboveTop >= minTop ? aboveTop : belowTop;
  styleRef.value = {
    position: 'fixed',
    left: `${clampEditFloatingValue(idealLeft, minLeft, maxLeft)}px`,
    top: `${clampEditFloatingValue(idealTop, minTop, maxTop)}px`,
    right: 'auto',
    bottom: 'auto',
    transform: 'none',
    minWidth: `${Math.max(minWidth, rect.width)}px`
  };
};
const scheduleEditFloatingMenuPosition = (positioner: () => void) => {
  positioner();
  if (typeof window !== 'undefined') {
    window.requestAnimationFrame(() => {
      positioner();
      window.requestAnimationFrame(positioner);
    });
  }
};
const scrollEditSelectedOptionToRow = (container: HTMLElement | null, selector: string, rowIndex = 0) => {
  const selected = container?.querySelector<HTMLElement>(selector);
  if (!container || !selected || typeof window === 'undefined') return;
  const optionSelector = selector.replace('.is-selected', '');
  const options = Array.from(container.querySelectorAll<HTMLElement>(optionSelector));
  const selectedIndex = options.indexOf(selected);
  if (selectedIndex < 0) return;
  const style = window.getComputedStyle(container);
  const gap = Number.parseFloat(style.rowGap || style.gap || '0');
  const paddingTop = Number.parseFloat(style.paddingTop || '0');
  const step = selected.offsetHeight + (Number.isFinite(gap) ? gap : 0);
  const maxScrollTop = Math.max(0, container.scrollHeight - container.clientHeight);
  const target = paddingTop + selectedIndex * step - step * Math.max(0, rowIndex);
  container.scrollTop = clampEditFloatingValue(target, 0, maxScrollTop);
};
const scrollEditPublishPickerSelectionToTop = () => {
  scrollEditSelectedOptionToRow(editPublishPickerMenuRef.value, '.publish-picker-floating-option.is-selected');
};
const scrollEditPublishTimeSelectionToSecondRow = () => {
  scrollEditSelectedOptionToRow(editPublishHourColumnRef.value, '.publish-time-option.is-selected', 1);
  scrollEditSelectedOptionToRow(editPublishMinuteColumnRef.value, '.publish-time-option.is-selected', 1);
};
const closeEditFloatingMenus = () => {
  showEditVisibilityMenu.value = false;
  showEditPublishDateMenu.value = false;
  openEditPublishPicker.value = '';
};
const positionEditVisibilityMenu = () => positionEditFloatingMenu(editVisibilityControlRef.value, editVisibilityMenuRef.value, editVisibilityMenuStyle, 106, 'above-right');
const positionEditPublishDateMenu = () => positionEditFloatingMenu(editPublishTimeControlRef.value, editPublishDateMenuRef.value, editPublishDateMenuStyle, 292, 'above-right');
const positionEditPublishPickerMenu = () => {
  if (!openEditPublishPicker.value || typeof window === 'undefined') return;
  const trigger = openEditPublishPicker.value === 'year' ? editPublishYearPickerButton.value : editPublishMonthPickerButton.value;
  if (!trigger) return;
  const scale = getEditFixedCoordinateScale();
  const rect = getEditFixedRect(trigger, scale);
  const viewport = getEditFixedViewport(scale);
  const menu = editPublishPickerMenuRef.value;
  const menuWidth = Math.ceil(rect.width);
  const menuHeight = menu?.offsetHeight || (openEditPublishPicker.value === 'year' ? 204 : 167);
  const pad = 8;
  const gap = 4;
  const minLeft = viewport.left + pad;
  const maxLeft = Math.max(minLeft, viewport.right - menuWidth - pad);
  const idealLeft = rect.left + rect.width / 2 - menuWidth / 2;
  const minTop = viewport.top + pad;
  const maxTop = Math.max(minTop, viewport.bottom - menuHeight - pad);
  const belowTop = rect.bottom + gap;
  const aboveTop = rect.top - menuHeight - gap;
  const idealTop = belowTop + menuHeight <= viewport.bottom - pad ? belowTop : (aboveTop >= minTop ? aboveTop : belowTop);
  editPublishPickerMenuStyle.value = {
    position: 'fixed',
    left: `${clampEditFloatingValue(idealLeft, minLeft, maxLeft)}px`,
    top: `${clampEditFloatingValue(idealTop, minTop, maxTop)}px`,
    right: 'auto',
    bottom: 'auto',
    transform: 'none',
    width: `${menuWidth}px`,
    minWidth: `${menuWidth}px`,
    visibility: 'visible'
  };
};
const toggleEditVisibilityMenu = async () => {
  showEditPublishDateMenu.value = false;
  openEditPublishPicker.value = '';
  showEditVisibilityMenu.value = !showEditVisibilityMenu.value;
  if (showEditVisibilityMenu.value) {
    await nextTick();
    scheduleEditFloatingMenuPosition(positionEditVisibilityMenu);
  }
};
const selectEditVisibility = (value: MessageVisibility) => {
  editingVisibility.value = value;
  showEditVisibilityMenu.value = false;
};
const syncEditPublishDraftFromInput = () => {
  const parsed = parseEditDatetimeLocal(editingPublishedAtInput.value);
  const base = parsed || (() => {
    const now = new Date();
    return { date: now, dateText: formatEditLocalDate(now), hour: now.getHours(), minute: now.getMinutes() };
  })();
  editPublishPickerMonth.value = new Date(base.date.getFullYear(), base.date.getMonth(), 1);
  editPublishDraftDate.value = base.dateText;
  editPublishDraftHour.value = base.hour;
  editPublishDraftMinute.value = base.minute;
};
const applyEditPublishDraft = () => {
  if (!editPublishDraftDate.value) return;
  editingPublishedAtInput.value = formatEditDatetimeLocal(editPublishDraftDate.value, editPublishDraftHour.value, editPublishDraftMinute.value);
};
const toggleEditPublishDateMenu = async () => {
  showEditVisibilityMenu.value = false;
  openEditPublishPicker.value = '';
  showEditPublishDateMenu.value = !showEditPublishDateMenu.value;
  if (showEditPublishDateMenu.value) {
    syncEditPublishDraftFromInput();
    await nextTick();
    scrollEditPublishTimeSelectionToSecondRow();
    scheduleEditFloatingMenuPosition(positionEditPublishDateMenu);
  }
};
const toggleEditPublishPicker = async (type: EditPublishPickerType) => {
  openEditPublishPicker.value = openEditPublishPicker.value === type ? '' : type;
  if (openEditPublishPicker.value) {
    editPublishPickerMenuStyle.value = {
      position: 'fixed',
      left: '0px',
      top: '0px',
      right: 'auto',
      bottom: 'auto',
      visibility: 'hidden'
    };
    await nextTick();
    scrollEditPublishPickerSelectionToTop();
    scheduleEditFloatingMenuPosition(positionEditPublishPickerMenu);
  }
};
const selectEditPublishPickerValue = (value: number) => {
  if (openEditPublishPicker.value === 'year' && Number.isFinite(value)) {
    editPublishPickerMonth.value = new Date(value, editPublishPickerMonth.value.getMonth(), 1);
  } else if (openEditPublishPicker.value === 'month' && Number.isFinite(value)) {
    editPublishPickerMonth.value = new Date(editPublishPickerMonth.value.getFullYear(), value - 1, 1);
  }
  openEditPublishPicker.value = '';
  nextTick(() => scheduleEditFloatingMenuPosition(positionEditPublishDateMenu));
};
const moveEditPublishMonth = (delta: number) => {
  editPublishPickerMonth.value = new Date(editPublishPickerMonth.value.getFullYear(), editPublishPickerMonth.value.getMonth() + delta, 1);
  nextTick(() => scheduleEditFloatingMenuPosition(positionEditPublishDateMenu));
};
const selectEditPublishDay = (day: EditPublishDateDay) => {
  editPublishDraftDate.value = day.date;
  if (!day.inMonth) {
    const parsed = new Date(`${day.date}T00:00:00`);
    if (!Number.isNaN(parsed.getTime())) editPublishPickerMonth.value = new Date(parsed.getFullYear(), parsed.getMonth(), 1);
  }
  applyEditPublishDraft();
};
const setEditPublishHour = (hour: number) => {
  editPublishDraftHour.value = hour;
  applyEditPublishDraft();
};
const setEditPublishMinute = (minute: number) => {
  editPublishDraftMinute.value = minute;
  applyEditPublishDraft();
};
const useEditPublishNow = () => {
  const now = new Date();
  editPublishPickerMonth.value = new Date(now.getFullYear(), now.getMonth(), 1);
  editPublishDraftDate.value = formatEditLocalDate(now);
  editPublishDraftHour.value = now.getHours();
  editPublishDraftMinute.value = now.getMinutes();
  applyEditPublishDraft();
  showEditPublishDateMenu.value = false;
  openEditPublishPicker.value = '';
};
const clearEditPublishDate = () => {
  editingPublishedAtInput.value = '';
  showEditPublishDateMenu.value = false;
  openEditPublishPicker.value = '';
};
const handleEditFloatingMenuPointerDown = (event: MouseEvent | PointerEvent) => {
  if (!showEditVisibilityMenu.value && !showEditPublishDateMenu.value && !openEditPublishPicker.value) return;
  const target = event.target as Node | null;
  if (!target) return;
  if (editVisibilityControlRef.value?.contains(target) || editVisibilityMenuRef.value?.contains(target)) return;
  if (editPublishTimeControlRef.value?.contains(target) || editPublishDateMenuRef.value?.contains(target)) return;
  if (editPublishYearPickerButton.value?.contains(target) || editPublishMonthPickerButton.value?.contains(target) || editPublishPickerMenuRef.value?.contains(target)) return;
  closeEditFloatingMenus();
};
const handleEditFloatingMenuViewportChange = () => {
  if (showEditVisibilityMenu.value) positionEditVisibilityMenu();
  if (showEditPublishDateMenu.value) positionEditPublishDateMenu();
  if (openEditPublishPicker.value) positionEditPublishPickerMenu();
};

onMounted(() => {
  try {
    document.addEventListener('pointerdown', handleEditFloatingMenuPointerDown, true);
    window.addEventListener('resize', handleEditFloatingMenuViewportChange);
    window.addEventListener('scroll', handleEditFloatingMenuViewportChange, true);
    window.visualViewport?.addEventListener('resize', handleEditFloatingMenuViewportChange);
    window.visualViewport?.addEventListener('scroll', handleEditFloatingMenuViewportChange);
  } catch {}
});
onBeforeUnmount(() => {
  try {
    document.removeEventListener('pointerdown', handleEditFloatingMenuPointerDown, true);
    window.removeEventListener('resize', handleEditFloatingMenuViewportChange);
    window.removeEventListener('scroll', handleEditFloatingMenuViewportChange, true);
    window.visualViewport?.removeEventListener('resize', handleEditFloatingMenuViewportChange);
    window.visualViewport?.removeEventListener('scroll', handleEditFloatingMenuViewportChange);
  } catch {}
});
watch(showEditModal, (visible) => {
  if (!visible) {
    editSession.close()
    closeEditFloatingMenus();
    clearEditAudioInsertTarget();
  }
});

const getEditTextareaElement = (): HTMLTextAreaElement | null => {
  const target = editTextareaRef.value as any
  if (!target) return null
  if (target instanceof HTMLTextAreaElement) return target
  const direct = target.textarea || target.input || target.$el
  if (direct instanceof HTMLTextAreaElement) return direct
  return direct?.querySelector?.('textarea') || null
}

const prepareEditAudioInsert = () => {
  const textarea = getEditTextareaElement()
  const contentLength = editingContent.value.length
  const start = Math.max(0, Math.min(contentLength, Number(textarea?.selectionStart ?? contentLength)))
  const end = Math.max(start, Math.min(contentLength, Number(textarea?.selectionEnd ?? start)))
  const session = editSession.capture()
  if (!session) return
  editAudioInsertTarget.value = { start, end, session }
}

const clearEditAudioInsertTarget = () => {
  editAudioInsertTarget.value = null
}

const insertEditingMarkdown = async (markdown: string, preparedTarget: EditInsertTarget | null = null) => {
  const textarea = getEditTextareaElement()
  if (!textarea) {
    editingContent.value += markdown
    return
  }
  const contentLength = editingContent.value.length
  const start = preparedTarget
    ? Math.max(0, Math.min(contentLength, preparedTarget.start))
    : Number(textarea.selectionStart ?? contentLength)
  const end = preparedTarget
    ? Math.max(start, Math.min(contentLength, preparedTarget.end))
    : Number(textarea.selectionEnd ?? start)
  const before = editingContent.value.slice(0, start)
  const after = editingContent.value.slice(end)
  editingContent.value = `${before}${markdown}${after}`
  await nextTick()
  const nextTextarea = getEditTextareaElement()
  if (nextTextarea) {
    const nextCursor = start + markdown.length
    nextTextarea.focus()
    nextTextarea.setSelectionRange(nextCursor, nextCursor)
  }
}

const handleEditAudioUploaded = async (audioUrl: string) => {
  const preparedTarget = editAudioInsertTarget.value
  clearEditAudioInsertTarget()
  if (!preparedTarget || !editSession.isCurrent(preparedTarget.session) || !showEditModal.value) return
  const audioMarkdown = createAudioMarkdown(resolveUploadedMediaUrl(audioUrl, String(BASE_API || '/api')))
  await insertEditingMarkdown(audioMarkdown, preparedTarget)
}

const handleEditAudioUploadProgress = (percent: number) => {
  editUploadProgress.value = percent
  if (percent > 0) {
    editUploadKind.value = 'audio'
    editUploadLabel.value = '音频上传中'
  } else if (editUploadKind.value === 'audio') {
    editUploadKind.value = ''
    editUploadLabel.value = ''
  }
}

const resetEditUploadState = () => {
  setTimeout(() => {
    editUploadProgress.value = 0
    editUploadKind.value = ''
    editUploadLabel.value = ''
  }, 400)
}

const triggerEditAttachmentInput = () => {
  if (!userStore.isLogin) {
    useToast().add({ title: '提示', description: '请登录后操作', color: 'orange', timeout: 2000 })
    return
  }
  if (isEditUploading.value) return
  editAttachmentInputRef.value?.click()
}

const handleEditAttachmentChange = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  if (!files.length) return
  const session = editSession.capture()
  if (!session) return
  const isCurrentUpload = () => editSession.isCurrent(session) && showEditModal.value
  editUploadKind.value = 'attachment'
  editUploadLabel.value = '附件上传中'
  editUploadProgress.value = 1

  try {
    await applyCurrentEditOperation(
      editSession,
      session,
      () => uploadMediaFiles({
        files,
        kind: 'auto',
        baseApi: String(BASE_API || '/api'),
        token: userStore.token || '',
        onProgress: (percent) => { if (isCurrentUpload()) editUploadProgress.value = percent }
      }),
      async (uploaded) => {
        if (uploaded.length) await insertEditingMarkdown(uploaded.map((item) => item.markdown).join(''))
        editUploadProgress.value = 100
        useToast().add({
          title: '成功',
          description: uploaded.length > 1 ? `已上传 ${uploaded.length} 个附件` : '附件上传成功',
          color: 'green',
          timeout: 2000
        })
      },
    )
  } catch (error: any) {
    if (!isCurrentUpload()) return
    useToast().add({
      title: '错误',
      description: error?.message || '附件上传失败',
      color: 'red',
      timeout: 2000
    })
  } finally {
    if (input) input.value = ''
    if (isCurrentUpload()) resetEditUploadState()
  }
}

const toDatetimeLocalValue = (value: any) => {
  const date = new Date(value || '')
  if (Number.isNaN(date.getTime())) return ''
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
  return local.toISOString().slice(0, 16)
}

const datetimeLocalToISO = (value: string) => {
  const raw = String(value || '').trim()
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return ''
  return date.toISOString()
}

const applyEditedMessage = (id: number, patch: any) => emit('saved', { id, patch })

type MessageEditOpenOptions = {
  message: any
  canChangeVisibility: boolean
  canChangePublishTime: boolean
}
const canChangeVisibility = ref(false)
const canChangePublishTime = ref(false)
const open = ({ message, canChangeVisibility: allowVisibility, canChangePublishTime: allowPublishTime }: MessageEditOpenOptions) => {
  editSession.open(Number(message.id))
  editingMessageId.value = message.id
  editingMessage.value = message
  canChangeVisibility.value = allowVisibility
  canChangePublishTime.value = allowPublishTime
  editingVisibility.value = messageVisibility(message)
  editingPublishedAtInput.value = allowPublishTime ? toDatetimeLocalValue(message.created_at) : ''
  editingContent.value = message.content
  if (message.image_url) {
    editingContent.value += '\n\n<!-- 附件图片(编辑时可删除) -->\n![附件图片](' + BASE_API + message.image_url + ')\n<!-- 附件图片结束 -->'
  }
  showEditModal.value = true
}

const saveEditedMessage = async () => {
  if (!editingMessageId.value) return;

  isSaving.value = true;
  try {
    // 获取当前编辑的消息
    const currentMsg = editingMessage.value;
    if (!currentMsg) return;

    // 处理编辑内容，移除附件图片的 Markdown 标记
    let processedContent = editingContent.value;

    // 移除附件图片的 Markdown 标记
    processedContent = processedContent.replace(/\n*<!-- 附件图片\(编辑时可删除\) -->\n!\[附件图片\]\(.*?\)\n<!-- 附件图片结束 -->\n*/g, '');

    const originalPublishTime = toDatetimeLocalValue(currentMsg.created_at)
    const canUpdateVisibility = canChangeVisibility.value
    const canUpdatePublishTime = canChangePublishTime.value
    const nextCreatedAt = canUpdatePublishTime ? datetimeLocalToISO(editingPublishedAtInput.value) : ''
    if (canUpdatePublishTime && editingPublishedAtInput.value && !nextCreatedAt) {
      useToast().add({
        title: '发布时间格式无效',
        color: 'red',
        timeout: 2000
      });
      return;
    }
    const publishTimeChanged = canUpdatePublishTime && !!nextCreatedAt && editingPublishedAtInput.value !== originalPublishTime
    const contentChanged = processedContent !== currentMsg.content
    const nextVisibility = normalizeMessageVisibility(editingVisibility.value, !!currentMsg.private)
    const visibilityChanged = canUpdateVisibility && nextVisibility !== messageVisibility(currentMsg)

    // 检查内容、发布时间或可见范围是否有修改
    if (!contentChanged && !publishTimeChanged && !visibilityChanged) {
      useToast().add({
        title: '内容未修改',
        description: '请修改内容、发布时间或可见范围后再保存',
        color: 'orange',
        timeout: 2000
      });
      isSaving.value = false;
      return;
    }
    const payload: any = {
      content: processedContent,
      image_url: currentMsg.image_url
    }
    if (canUpdateVisibility) {
      payload.visibility = nextVisibility
      payload.private = messageVisibilityRequiresPrivate(nextVisibility)
    }
    if (publishTimeChanged) {
      payload.created_at = nextCreatedAt
    }
    const response = await fetch(`${BASE_API}/messages/${editingMessageId.value}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      },
      credentials: 'include',
      body: JSON.stringify(payload)
    });

    if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

    const data = await response.json();
    if (data.code === 1) {
      const updatedData = data.data || {}
      const savedVisibility = normalizeMessageVisibility(updatedData.visibility ?? nextVisibility, !!updatedData.private)
      const savedPrivate = typeof updatedData.private === 'boolean' ? updatedData.private : messageVisibilityRequiresPrivate(savedVisibility)
      applyEditedMessage(editingMessageId.value, {
        content: updatedData.content ?? processedContent,
        image_url: updatedData.image_url ?? currentMsg.image_url,
        created_at: updatedData.created_at ?? (publishTimeChanged ? nextCreatedAt : currentMsg.created_at),
        visibility: savedVisibility,
        private: savedPrivate
      })
      showEditModal.value = false;
      useToast().add({
        title: '更新成功',
        color: 'green',
        timeout: 2000
      });
    } else {
      throw new Error(data.msg || '保存失败');
    }
  } catch (error) {
    console.error('更新消息失败:', error);
    useToast().add({
      title: '更新失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    isSaving.value = false;
  }
};

defineExpose({ open })
</script>

<style scoped>
.edit-modal-shell {
  --edit-border: rgba(15, 23, 42, 0.10);
  --edit-surface: #ffffff;
  --edit-panel: #f8fafc;
  --edit-panel-strong: #f1f5f9;
  --edit-text: #111827;
  --edit-muted: #64748b;
  --edit-control: #ffffff;
  --edit-media-bg: rgba(249, 115, 22, 0.08);
  --edit-media-border: rgba(249, 115, 22, 0.24);
  --edit-media-text: #9a3412;
  --nw-tooltip-bg: rgba(255, 255, 255, 0.96);
  --nw-tooltip-text: #111827;
  --nw-tooltip-border: rgba(15, 23, 42, 0.14);
  --nw-tooltip-shadow: 0 10px 24px rgba(15, 23, 42, 0.16);
  --nw-floating-bg: rgba(255, 255, 255, 0.96);
  --nw-floating-text: #111827;
  --nw-floating-border: rgba(15, 23, 42, 0.12);
  --nw-floating-hover-bg: rgba(15, 23, 42, 0.06);
  --nw-floating-hover-border: rgba(249, 115, 22, 0.34);
  --nw-floating-selected-bg: rgba(249, 115, 22, 0.14);
  --nw-floating-selected-border: rgba(249, 115, 22, 0.42);
  --nw-floating-shadow: 0 18px 36px rgba(15, 23, 42, 0.16);
  width: 100%;
  max-height: min(86vh, 860px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--edit-border);
  border-radius: 12px;
  background: var(--edit-surface);
  color: var(--edit-text);
  box-shadow: 0 18px 44px rgba(15, 23, 42, 0.18);
}

:global(html.dark) .edit-modal-shell,
.edit-modal-shell.is-dark {
  --edit-border: rgba(255, 255, 255, 0.14);
  --edit-surface: #0f172a;
  --edit-panel: rgba(255, 255, 255, 0.055);
  --edit-panel-strong: rgba(255, 255, 255, 0.085);
  --edit-text: #f8fafc;
  --edit-muted: #94a3b8;
  --edit-control: rgba(15, 23, 42, 0.72);
  --edit-media-bg: rgba(255, 255, 255, 0.08);
  --edit-media-border: rgba(255, 255, 255, 0.16);
  --edit-media-text: #f8fafc;
  --nw-tooltip-bg: rgba(15, 23, 42, 0.96);
  --nw-tooltip-text: #f8fafc;
  --nw-tooltip-border: rgba(255, 255, 255, 0.18);
  --nw-tooltip-shadow: 0 12px 30px rgba(0, 0, 0, 0.38);
  --nw-floating-bg: rgba(15, 23, 42, 0.96);
  --nw-floating-text: #f8fafc;
  --nw-floating-border: rgba(255, 255, 255, 0.16);
  --nw-floating-hover-bg: rgba(249, 115, 22, 0.26);
  --nw-floating-hover-border: rgba(249, 115, 22, 0.58);
  --nw-floating-selected-bg: rgba(249, 115, 22, 0.24);
  --nw-floating-selected-border: rgba(251, 146, 60, 0.52);
  --nw-floating-shadow: 0 18px 38px rgba(0, 0, 0, 0.42);
  box-shadow: 0 22px 54px rgba(2, 6, 23, 0.58);
}

.edit-modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 56px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--edit-border);
  background: var(--edit-panel);
}

.edit-modal-title-block { min-width: 0; }
.edit-modal-title { margin: 0; font-size: 17px; line-height: 1.35; font-weight: 700; color: var(--edit-text); }

.edit-icon-button {
  border: 1px solid var(--edit-border);
  background: var(--edit-control);
  color: var(--edit-text);
  transition: background-color .18s ease, border-color .18s ease, color .18s ease, transform .18s ease, opacity .18s ease;
}

.visibility-indicator {
  width: 1rem;
  height: 1rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.edit-icon-button {
  width: 34px;
  height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  border-radius: 10px;
}

.edit-modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 0;
  overflow: auto;
  padding: 16px;
}

.edit-toolbar {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  min-height: 48px;
  padding: 8px;
  border: 1px solid var(--edit-border);
  border-radius: 10px;
  background: var(--edit-panel);
}

.edit-toolbar-left {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}

.edit-modal-shell .tb-btn {
  width: 36px;
  min-width: 36px;
  height: 36px;
}

.edit-modal-shell .visibility-control,
.edit-modal-shell .publish-time-control {
  width: max-content;
  max-width: min(210px, calc(100vw - 32px));
  padding: 0 8px;
}

.edit-modal-shell .visibility-trigger,
.edit-modal-shell .publish-time-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  gap: 3px;
  min-width: 0;
  max-width: 148px;
  height: 28px;
  border: 0;
  background: transparent;
  color: inherit;
  font-size: 12px;
  font-weight: 650;
  line-height: 1;
  outline: none;
}

.edit-modal-shell .visibility-trigger span,
.edit-modal-shell .publish-time-trigger span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.edit-modal-shell .visibility-trigger svg,
.edit-modal-shell .publish-time-trigger svg {
  flex: 0 0 auto;
  opacity: .72;
}

.edit-icon-button:hover:not(:disabled) {
  transform: translate3d(0,0,0) scale(1.06);
  border-color: var(--nw-floating-hover-border, var(--edit-border));
  background: var(--nw-floating-hover-bg, var(--edit-panel-strong));
}

.edit-icon-button:disabled {
  cursor: not-allowed;
  opacity: .58;
}

.edit-upload-status {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 650;
  color: rgb(234, 88, 12);
}

.edit-modal-shell.is-dark .edit-upload-status,
:global(html.dark) .edit-upload-status { color: rgb(251, 146, 60); }

.edit-content-textarea {
  width: 100%;
  min-height: 260px;
  resize: vertical;
  border: 1px solid var(--edit-border);
  border-radius: 10px;
  background: var(--edit-control);
  color: var(--edit-text);
  padding: 12px;
  font-family: inherit;
  font-size: 13px;
  font-weight: 400;
  line-height: 1.7;
  letter-spacing: normal;
  outline: none;
  transition: border-color .18s ease, box-shadow .18s ease, background-color .18s ease;
}

.edit-content-textarea::placeholder {
  color: var(--edit-muted);
  font-family: inherit;
  font-weight: 400;
  letter-spacing: normal;
}
.edit-content-textarea:focus {
  border-color: rgba(249, 115, 22, 0.62);
  box-shadow: 0 0 0 3px rgba(249, 115, 22, 0.16);
}

.floating-control-menu {
  position: fixed;
  z-index: 5004;
  border: 1px solid var(--nw-floating-border);
  border-radius: 12px;
  background: var(--nw-floating-bg);
  color: var(--nw-floating-text);
  box-shadow: var(--nw-floating-shadow);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}

.nw-floating-menu.is-dark {
  --nw-floating-bg: rgba(15, 23, 42, 0.96);
  --nw-floating-text: #f8fafc;
  --nw-floating-border: rgba(255, 255, 255, 0.16);
  --nw-floating-hover-bg: rgba(249, 115, 22, 0.26);
  --nw-floating-hover-border: rgba(249, 115, 22, 0.58);
  --nw-floating-selected-bg: rgba(249, 115, 22, 0.24);
  --nw-floating-selected-border: rgba(251, 146, 60, 0.52);
  --nw-floating-shadow: 0 18px 38px rgba(0, 0, 0, 0.42);
}

.publish-datetime-menu {
  --nw-date-cell-bg: rgba(15, 23, 42, 0.05);
  --nw-time-column-bg: rgba(15, 23, 42, 0.06);
  --nw-current-bg: rgba(59, 130, 246, 0.22);
  --nw-current-border: rgba(96, 165, 250, 0.68);
  --nw-current-text: var(--nw-floating-text);
  --nw-picker-button-bg: rgba(15, 23, 42, 0.04);
}

.publish-datetime-menu.is-dark {
  --nw-date-cell-bg: rgba(255, 255, 255, 0.09);
  --nw-time-column-bg: rgba(255, 255, 255, 0.07);
  --nw-picker-button-bg: rgba(255, 255, 255, 0.10);
  --nw-current-bg: rgba(59, 130, 246, 0.26);
  --nw-current-border: rgba(96, 165, 250, 0.74);
}

.visibility-floating-menu { display: grid; gap: 4px; padding: 8px; }

.floating-control-option {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 32px;
  padding: 0 10px;
  border: 1px solid transparent;
  border-radius: 9px;
  color: inherit;
  font-size: 12px;
  font-weight: 650;
  text-align: left;
  transition: background-color .15s ease, border-color .15s ease, color .15s ease;
}

.floating-control-option:hover,
.floating-control-option:focus-visible {
  outline: none;
  border-color: var(--nw-floating-hover-border);
  background: var(--nw-floating-hover-bg);
}

.floating-control-option.is-selected {
  border-color: var(--nw-floating-selected-border);
  background: var(--nw-floating-selected-bg);
  color: var(--nw-floating-text);
}

.edit-preview-block {
  border-top: 1px solid var(--edit-border);
  padding-top: 12px;
}

.edit-preview-title {
  margin-bottom: 8px;
  color: var(--edit-muted);
  font-size: 13px;
  font-weight: 650;
}

.edit-preview-surface {
  max-height: 300px;
  overflow: auto;
  padding: 14px;
  border: 1px solid var(--edit-border);
  border-radius: 10px;
  background: var(--edit-control);
  color: var(--edit-text);
}

/*
 * 附件卡片与失败占位块的阴影向下最多外扩 48px（offset 16 + blur 32），向上 16px（blur - offset）。
 * 预览面板一旦是滚动裁剪盒，这些阴影就会被贴边切掉，
 * 而笔记正文侧是靠 file-attachment-shadow-open 放开裁剪的。
 * 为了两边观感一致，这里同样交出裁剪权：滚动由 .edit-modal-body 承担，
 * 上下按实际外扩量留白，让首尾卡片的阴影有地方落。
 */
.edit-preview-surface--shadow-open {
  max-height: none;
  overflow: visible;
  padding: 18px 14px 48px;
}

.edit-modal-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--edit-border);
  background: var(--edit-panel);
}

.edit-footer-button {
  min-width: 64px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 0 12px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 650;
  line-height: 1;
}

.edit-spin { animation: edit-spin 1s linear infinite; }
@keyframes edit-spin { to { transform: rotate(360deg); } }

@media screen and (max-width: 640px) {
  .edit-modal-shell { max-height: 90vh; border-radius: 10px; }
  .edit-modal-header,
  .edit-modal-body,
  .edit-modal-footer { padding-left: 12px; padding-right: 12px; }
  .edit-toolbar { align-items: stretch; }
  .edit-toolbar-left { width: 100%; }
  .edit-modal-shell .visibility-control,
  .edit-modal-shell .publish-time-control { flex: 1 1 150px; max-width: none; }
  .edit-modal-footer { justify-content: stretch; }
  .edit-footer-button { flex: 1 1 0; }
}
</style>
