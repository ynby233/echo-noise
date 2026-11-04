<template>
  <span class="common-tag-picker">
    <button
      ref="buttonRef"
      type="button"
      class="common-tag-trigger"
      :class="buttonClass"
      :disabled="disabled"
      aria-label="选择常用标签"
      data-tooltip="常用标签"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="open ? menuId : undefined"
      @mousedown.prevent="rememberSelection"
      @click.stop="toggleMenu"
      @keydown.down.prevent="openFromButton(true)"
      @keydown.up.prevent="openFromButton(true)"
    >
      <UIcon name="i-heroicons-hashtag" class="w-5 h-5" />
    </button>
    <!-- A manual popover escapes transformed/backdrop-filter containing blocks
         while staying in the dialog's DOM and focus scope. -->
    <Teleport to="body" :disabled="supportsPopover">
    <div
      v-if="open"
      ref="menuRef"
      :id="menuId"
      :popover="supportsPopover ? 'manual' : undefined"
      class="floating-control-menu nw-floating-menu common-tag-menu"
      :class="{ 'is-dark': dark }"
      :style="menuStyle"
      role="listbox"
      aria-label="常用标签"
      :aria-activedescendant="candidates.length ? `${menuId}-${activeIndex}` : undefined"
      tabindex="-1"
      @mousedown.prevent.stop
      @click.stop
    >
      <div class="common-tag-heading">常用标签</div>
      <button
        v-for="(tag, index) in candidates"
        :id="`${menuId}-${index}`"
        :key="tag"
        type="button"
        role="option"
        tabindex="-1"
        class="floating-control-option nw-floating-option common-tag-option"
        :class="{ 'is-selected': index === activeIndex }"
        :aria-selected="index === activeIndex"
        @mouseenter="activeIndex = index"
        @click="selectTag(tag)"
      >#{{ tag }}</button>
      <p v-if="!normalizedTags.length" class="common-tag-empty" role="status">暂未设置常用标签，请前往后台概览设置。</p>
      <p v-else-if="!candidates.length" class="common-tag-empty" role="status">没有匹配的常用标签</p>
    </div>
    </Teleport>
  </span>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { positionFloatingMenu } from '~/utils/floating-menu'
import {
  commonTagInsertion, commonTagKeyAction, filterCommonTags, findCommonTagTrigger,
  moveCommonTagIndex, normalizeCommonTags
} from '~/utils/common-tag-picker'
import {
  captureCommonTagSelection, commonTagCaretRect, commonTagEventInScope,
  insertCommonTagText, narrowCommonTagSelection
} from '~/utils/common-tag-picker-dom'
import type { CommonTagEditor, CommonTagSelection } from '~/utils/common-tag-picker-dom'

const props = withDefaults(defineProps<{
  /** Normalized account preferences; spellings do not include the leading #. */
  tags: readonly string[]
  /** Return the current editable DOM, including an active inline table textarea. */
  getEditor: () => CommonTagEditor | null
  /** For Vditor use its public insertValue; textarea always uses native input. */
  insertText?: (text: string) => void
  disabled?: boolean
  dark?: boolean
  buttonClass?: string
}>(), { disabled: false, dark: false, buttonClass: '' })

const buttonRef = ref<HTMLButtonElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const open = ref(false)
const query = ref('')
const activeIndex = ref(0)
const menuStyle = ref<Record<string, string>>({})
const menuId = `common-tag-menu-${getCurrentInstance()!.uid}`
const supportsPopover = ref(false)
const normalizedTags = computed(() => normalizeCommonTags(props.tags))
const candidates = computed(() => filterCommonTags(normalizedTags.value, query.value))
let savedSelection: CommonTagSelection | null = null
let lastSelection: CommonTagSelection | null = null
let mode: 'button' | 'caret' = 'button'
let composing = false
let inserting = false
let triggerArmed = false
let disposed = false
let refreshTimer: ReturnType<typeof setTimeout> | undefined
let refreshFrame = 0
let positionFrame = 0
const disposers: Array<() => void> = []

const editorFocused = (editor: CommonTagEditor | null) => !!editor && (
  editor === document.activeElement || editor.contains(document.activeElement)
)
const inControls = (target: EventTarget | null) => commonTagEventInScope(target, buttonRef.value, menuRef.value)

const rememberSelection = () => {
  const selection = captureCommonTagSelection(props.getEditor())
  if (selection) lastSelection = selection
}

const close = () => {
  open.value = false
  triggerArmed = false
  savedSelection = null
}

const positionMenu = () => {
  if (!open.value || disposed) return
  const editor = props.getEditor()
  if (props.disabled || !editor?.isConnected || (savedSelection && savedSelection.editor !== editor)) {
    close()
    return
  }
  // floating-menu only consumes getBoundingClientRect; a virtual caret anchor
  // shares its existing viewport/zoom clamping without adding a DOM marker.
  const anchor = mode === 'caret' && savedSelection
    ? { getBoundingClientRect: () => commonTagCaretRect(savedSelection!) } as HTMLElement
    : buttonRef.value
  positionFloatingMenu(anchor, menuRef.value, menuStyle, 200, 'above-align-left')
}

const schedulePosition = () => {
  if (!open.value || disposed) return
  cancelAnimationFrame(positionFrame)
  positionFrame = requestAnimationFrame(positionMenu)
}

const revealMenu = () => {
  open.value = true
  void nextTick(() => {
    if (!disposed && open.value) {
      if (supportsPopover.value && menuRef.value && !menuRef.value.matches(':popover-open')) {
        menuRef.value.showPopover()
      }
      positionMenu()
      schedulePosition()
    }
  })
}

const openFromButton = (keyboard = false) => {
  if (props.disabled || composing) return
  rememberSelection()
  const editor = props.getEditor()
  if (!editor) return
  let selection = lastSelection?.editor === editor ? lastSelection : null
  // With no previous editor caret, insertion starts at the editor's end.
  if (!selection) {
    editor.focus({ preventScroll: true })
    if (editor.tagName === 'TEXTAREA') {
      const textarea = editor as HTMLTextAreaElement
      textarea.setSelectionRange(textarea.value.length, textarea.value.length)
    } else {
      const range = document.createRange()
      range.selectNodeContents(editor)
      range.collapse(false)
      const domSelection = window.getSelection()
      domSelection?.removeAllRanges()
      domSelection?.addRange(range)
    }
    selection = captureCommonTagSelection(editor)
  }
  if (!selection) return
  const trigger = selection.start === selection.end ? findCommonTagTrigger(selection.prefix) : null
  savedSelection = trigger ? narrowCommonTagSelection(selection, trigger) : selection
  mode = 'button'
  query.value = ''
  activeIndex.value = 0
  revealMenu()
  if (keyboard) void nextTick(() => { if (open.value && !disposed) menuRef.value?.focus({ preventScroll: true }) })
}

const toggleMenu = (event: MouseEvent) => {
  if (open.value) close()
  else openFromButton(event.detail === 0)
}

const refreshFromEditor = () => {
  if (disposed || composing || props.disabled) return
  const editor = props.getEditor()
  if (!editorFocused(editor)) return
  const selection = captureCommonTagSelection(editor)
  if (!selection) return
  lastSelection = selection
  if (!triggerArmed && !open.value) return
  if (mode === 'button' && open.value) return
  const trigger = selection.start === selection.end ? findCommonTagTrigger(selection.prefix) : null
  if (!trigger || !normalizedTags.value.length) {
    close()
    return
  }
  savedSelection = narrowCommonTagSelection(selection, trigger)
  if (query.value !== trigger.query) activeIndex.value = 0
  query.value = trigger.query
  mode = 'caret'
  revealMenu()
}

const scheduleRefresh = () => {
  if (disposed) return
  clearTimeout(refreshTimer)
  cancelAnimationFrame(refreshFrame)
  // Vditor can replace # nodes and restore its selection after the input event.
  refreshTimer = setTimeout(() => {
    refreshFromEditor()
    refreshFrame = requestAnimationFrame(refreshFromEditor)
  }, 0)
}

const selectTag = (tag: string) => {
  if (!savedSelection || composing || props.disabled || !normalizedTags.value.includes(tag)) return
  const selection = savedSelection
  if (selection.editor !== props.getEditor()) { close(); return }
  const text = commonTagInsertion(tag, selection.before, selection.after)
  // Close first so Vditor's synchronous input/selection updates cannot reopen it.
  close()
  inserting = true
  try {
    insertCommonTagText(selection, text, props.insertText)
    rememberSelection()
  } finally {
    inserting = false
    triggerArmed = false
  }
}

const onKeydown = (event: KeyboardEvent) => {
  const scoped = commonTagEventInScope(event.target, props.getEditor(), menuRef.value)
    || (open.value && inControls(event.target))
  const action = commonTagKeyAction(event, open.value, scoped, composing)
  if (!action || props.disabled) return
  // Window capture runs before Vditor's already-registered document capture
  // handlers, including its Tab insertion and Enter/table processing.
  event.preventDefault()
  event.stopImmediatePropagation()
  if (action === 'close') {
    const menuFocused = !!menuRef.value?.contains(document.activeElement)
    close()
    if (menuFocused) buttonRef.value?.focus({ preventScroll: true })
  } else if (action === 'select') {
    if (mode === 'caret' && editorFocused(props.getEditor())) refreshFromEditor()
    const tag = candidates.value[activeIndex.value]
    if (tag) selectTag(tag)
  } else {
    activeIndex.value = moveCommonTagIndex(activeIndex.value, candidates.value.length, action === 'next' ? 1 : -1)
    void nextTick(() => menuRef.value?.querySelector<HTMLElement>('.is-selected')?.scrollIntoView({ block: 'nearest' }))
  }
}

const onInput = (event: Event) => {
  if (inserting || !commonTagEventInScope(event.target, props.getEditor(), null) || composing || (event as InputEvent).isComposing) return
  triggerArmed = true
  mode = 'caret'
  scheduleRefresh()
}
const onCompositionStart = (event: Event) => {
  if (!commonTagEventInScope(event.target, props.getEditor(), null)) return
  composing = true
  close()
}
const onCompositionEnd = (event: Event) => {
  if (!commonTagEventInScope(event.target, props.getEditor(), null)) return
  composing = false
  triggerArmed = true
  mode = 'caret'
  scheduleRefresh()
}
const onSelectionChange = () => {
  if (!editorFocused(props.getEditor()) || composing) return
  rememberSelection()
  if (open.value && mode === 'caret') scheduleRefresh()
}
const onPointerDown = (event: Event) => {
  if (inControls(event.target)) return
  if (commonTagEventInScope(event.target, props.getEditor(), null)) {
    if (mode === 'button') close()
    scheduleRefresh()
  } else close()
}
const onFocusIn = (event: Event) => {
  if (!inControls(event.target) && !commonTagEventInScope(event.target, props.getEditor(), null)) {
    composing = false
    close()
  }
}

watch(candidates, (tags) => {
  activeIndex.value = tags.length ? Math.max(0, Math.min(activeIndex.value, tags.length - 1)) : -1
  if (open.value) void nextTick(schedulePosition)
})
watch(() => props.disabled, (disabled) => { if (disabled) close() })
watch(() => props.getEditor(), (editor, previous) => {
  if (editor !== previous) { close(); lastSelection = null; composing = false }
})

onMounted(() => {
  supportsPopover.value = typeof HTMLElement.prototype.showPopover === 'function'
  const listen = (target: EventTarget, name: string, handler: EventListener, capture = true) => {
    target.addEventListener(name, handler, capture)
    disposers.push(() => target.removeEventListener(name, handler, capture))
  }
  listen(window, 'keydown', onKeydown as EventListener)
  listen(window, 'input', onInput)
  listen(window, 'compositionstart', onCompositionStart)
  listen(window, 'compositionend', onCompositionEnd)
  listen(window, 'pointerdown', onPointerDown)
  listen(window, 'focusin', onFocusIn)
  listen(document, 'selectionchange', onSelectionChange)
  listen(window, 'scroll', schedulePosition)
  listen(window, 'resize', schedulePosition)
  if (window.visualViewport) {
    listen(window.visualViewport, 'resize', schedulePosition)
    listen(window.visualViewport, 'scroll', schedulePosition)
  }
})
onBeforeUnmount(() => {
  disposed = true
  close()
  clearTimeout(refreshTimer)
  cancelAnimationFrame(refreshFrame)
  cancelAnimationFrame(positionFrame)
  disposers.splice(0).forEach(dispose => dispose())
  lastSelection = null
})

defineExpose({ close })
</script>

<style scoped>
.common-tag-picker { display: inline-flex; align-items: center; }
.common-tag-trigger { display: inline-flex; align-items: center; justify-content: center; }
.common-tag-trigger:disabled { opacity: .45; cursor: not-allowed; }
.common-tag-menu {
  margin: 0;
  inset: auto;
  z-index: 10050;
  width: 220px;
  max-width: calc(100vw - 16px);
  max-height: min(280px, calc(100dvh - 16px));
  overflow: auto;
  padding: 6px;
  border: 1px solid var(--nw-floating-border);
  border-radius: 12px;
  background: var(--nw-floating-bg);
  color: var(--nw-floating-text);
  box-shadow: var(--nw-floating-shadow);
}
.common-tag-heading { padding: 5px 8px; color: #64748b; font-size: 12px; }
.common-tag-option {
  display: block;
  width: 100%;
  padding: 8px 10px;
  border: 1px solid transparent;
  border-radius: 7px;
  text-align: left;
  overflow-wrap: anywhere;
  font-size: 14px;
  line-height: 1.4;
}
.common-tag-option.is-selected { background: var(--nw-floating-selected-bg); border-color: var(--nw-floating-selected-border); }
.common-tag-empty { margin: 0; padding: 8px; color: #64748b; font-size: 13px; line-height: 1.6; }
.common-tag-menu.is-dark {
  --nw-floating-bg: #0f172a;
  --nw-floating-text: #f8fafc;
  --nw-floating-border: rgb(255 255 255 / 18%);
  --nw-floating-shadow: 0 18px 42px rgb(0 0 0 / 42%);
  --nw-floating-selected-bg: rgb(249 115 22 / 30%);
  --nw-floating-selected-border: rgb(249 115 22 / 70%);
}
.is-dark .common-tag-heading, .is-dark .common-tag-empty { color: #94a3b8; }
</style>
