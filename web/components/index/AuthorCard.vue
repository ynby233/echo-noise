<template>
  <button
    ref="trigger" type="button" class="author-card-trigger" :aria-label="`${username || '匿名'}的作者资料`"
    :aria-expanded="opened" @pointerenter="onTriggerEnter" @pointerleave="scheduleClose"
    @click.stop="toggle" @keydown.down.prevent="openFromKeyboard" @focusout="onFocusOut"
  >
    <img :src="avatar" alt="" class="avatar-img" @error="emit('avatar-error', $event)" />
  </button>
  <Teleport to="body">
    <template v-if="opened">
      <div ref="probe" class="author-card-probe" aria-hidden="true" />
      <section
        ref="card" class="author-card" :class="{ 'is-dark': dark }" role="region"
        :aria-label="`${username || '匿名'}的作者资料`" tabindex="0"
        @pointerenter="cancelLeave" @pointerleave="scheduleClose"
        @focusin="cancelLeave" @focusout="onFocusOut"
      >
        <div class="author-card-header">
          <img :src="profileAvatar || avatar" alt="" class="avatar-img" @error="emit('avatar-error', $event)" />
          <strong>{{ username || '匿名' }}</strong>
        </div>
        <p class="author-card-description">{{ description || '—' }}</p>
        <p class="author-card-count">笔记 {{ count }}</p>
      </section>
    </template>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { positionAuthorCard } from '~/utils/author-card-position'

const props = defineProps<{
  avatar: string
  profileAvatar?: string
  username?: string
  description?: string
  count: number
  dark: boolean
  layoutKey: string
}>()
const emit = defineEmits<{
  open: []
  'avatar-error': [event: Event]
}>()
const trigger = ref<HTMLButtonElement | null>(null)
const card = ref<HTMLElement | null>(null)
const probe = ref<HTMLElement | null>(null)
const opened = ref(false)
let pinned = false
let hoverTimer: ReturnType<typeof setTimeout> | undefined
let leaveTimer: ReturnType<typeof setTimeout> | undefined
let frame = 0
let generation = 0
let stopTracking: (() => void) | undefined

function cancelLeave() {
  clearTimeout(leaveTimer)
  leaveTimer = undefined
}

function close() {
  generation++
  clearTimeout(hoverTimer)
  cancelLeave()
  stopTracking?.()
  stopTracking = undefined
  cancelAnimationFrame(frame)
  frame = 0
  opened.value = false
  pinned = false
}

function scheduleClose(event?: PointerEvent) {
  clearTimeout(hoverTimer)
  if (pinned || (event && event.pointerType !== 'mouse')) return
  cancelLeave()
  // Allow crossing the gap between the avatar and the teleported card.
  leaveTimer = setTimeout(() => {
    if (!card.value?.contains(document.activeElement)) close()
  }, 220)
}

function onFocusOut(event: FocusEvent) {
  if (!trigger.value?.contains(event.relatedTarget as Node | null) && !card.value?.contains(event.relatedTarget as Node | null)) close()
}

function updatePosition() {
  frame = 0
  if (!opened.value || !trigger.value || !card.value || !probe.value) return
  if (!positionAuthorCard(trigger.value, card.value, probe.value, window.matchMedia('(max-width: 640px)').matches)) close()
}

function schedulePosition() {
  if (opened.value && !frame) frame = requestAnimationFrame(updatePosition)
}

function onOutsidePointer(event: PointerEvent) {
  const path = event.composedPath()
  if (!path.includes(trigger.value!) && !path.includes(card.value!)) close()
}

function onEscape(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  if (card.value?.contains(document.activeElement)) trigger.value?.focus({ preventScroll: true })
  close()
}

function onOtherOpen(event: Event) {
  if ((event as CustomEvent).detail !== trigger.value) close()
}

function track() {
  const viewport = window.visualViewport
  const breakpoints = [window.matchMedia('(max-width: 640px)'), window.matchMedia('(max-width: 1024px)')]
  window.addEventListener('scroll', schedulePosition, true)
  window.addEventListener('resize', schedulePosition)
  viewport?.addEventListener('scroll', schedulePosition)
  viewport?.addEventListener('resize', schedulePosition)
  document.addEventListener('pointerdown', onOutsidePointer, true)
  document.addEventListener('keydown', onEscape)
  document.addEventListener('load', schedulePosition, true)
  document.addEventListener('transitionend', schedulePosition, true)
  window.addEventListener('noise:author-card-open', onOtherOpen)
  breakpoints.forEach(query => query.addEventListener('change', close))

  const resize = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(schedulePosition)
  if (card.value) resize?.observe(card.value)
  // Ancestor resizes cover async images/content above this anchor, including
  // nested scrolling lists. Mutations cover masonry transforms and reordering.
  const mutations = typeof MutationObserver === 'undefined' ? undefined : new MutationObserver(schedulePosition)
  const list = trigger.value?.closest('.message-list-item')?.parentElement
  if (list) mutations?.observe(list, { childList: true, subtree: true, attributes: true, attributeFilter: ['class', 'style'] })
  for (let element: HTMLElement | null = trigger.value; element; element = element.parentElement) {
    resize?.observe(element)
    if (element !== list) mutations?.observe(element, { attributes: true, attributeFilter: ['class', 'style', 'hidden'] })
  }
  stopTracking = () => {
    window.removeEventListener('scroll', schedulePosition, true)
    window.removeEventListener('resize', schedulePosition)
    viewport?.removeEventListener('scroll', schedulePosition)
    viewport?.removeEventListener('resize', schedulePosition)
    document.removeEventListener('pointerdown', onOutsidePointer, true)
    document.removeEventListener('keydown', onEscape)
    document.removeEventListener('load', schedulePosition, true)
    document.removeEventListener('transitionend', schedulePosition, true)
    window.removeEventListener('noise:author-card-open', onOtherOpen)
    breakpoints.forEach(query => query.removeEventListener('change', close))
    resize?.disconnect()
    mutations?.disconnect()
  }
}

async function open(pin = false) {
  clearTimeout(hoverTimer)
  cancelLeave()
  pinned = pinned || pin
  if (opened.value) return
  const session = ++generation
  window.dispatchEvent(new CustomEvent('noise:author-card-open', { detail: trigger.value }))
  opened.value = true
  emit('open')
  await nextTick()
  if (!opened.value || session !== generation) return
  updatePosition()
  if (opened.value) track()
}

async function openFromKeyboard() {
  await open(true)
  if (opened.value) card.value?.focus({ preventScroll: true })
}

function onTriggerEnter(event: PointerEvent) {
  if (event.pointerType !== 'mouse') return
  cancelLeave()
  clearTimeout(hoverTimer)
  hoverTimer = setTimeout(() => { void open() }, 120)
}

function toggle() {
  if (opened.value && pinned) close()
  else void open(true)
}

watch(() => props.layoutKey, close)
watch(() => [props.profileAvatar, props.description, props.count, props.username, props.dark], () => { void nextTick(schedulePosition) })
onBeforeUnmount(close)
</script>

<style scoped>
.author-card-trigger { display: inline-flex; flex: 0 0 auto; width: 36px; height: 36px; padding: 0; border: 0; border-radius: 50%; background: transparent; cursor: pointer; }
.author-card-trigger:focus-visible { outline: 2px solid #3b82f6; outline-offset: 3px; }
.avatar-img { width: 100%; height: 100%; border-radius: 50%; object-fit: cover; }
.author-card-probe { position: fixed; top: 0; left: 0; width: 100px; height: 100px; visibility: hidden; pointer-events: none; }
/* Below Nuxt UI's default modal (z-50), above the note's stacking contexts.
   Teleport avoids clipping without promoting informational UI to top-layer. */
.author-card { position: fixed; z-index: 40; top: 0; left: 0; visibility: hidden; box-sizing: border-box; width: 300px; min-width: 0; margin: 0; padding: 12px; overflow: auto; border: 1px solid rgba(0,0,0,.1); border-radius: 12px; background: #fff; color: #111827; box-shadow: 0 8px 24px rgba(0,0,0,.25); font-size: 14px; line-height: 1.4; overflow-wrap: anywhere; }
.author-card.is-dark { border-color: rgba(255,255,255,.14); background: rgb(15,24,39); color: #fff; }
.author-card-header { display: flex; gap: 10px; align-items: center; }
.author-card-header .avatar-img { flex: 0 0 40px; width: 40px; height: 40px; }
.author-card-header strong { min-width: 0; }
.author-card-description { margin: 10px 0 6px; white-space: pre-wrap; font-size: 12px; }
.author-card-count { margin: 0; color: #6b7280; font-size: 12px; }
.author-card.is-dark .author-card-count { color: #9ca3af; }
</style>
