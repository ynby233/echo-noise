<template>
  <div>
    <NuxtPage />
    <Notification />
    <section v-if="moduleRecoveryFailed" class="module-recovery-notice" role="alert" aria-label="页面恢复">
      <p>资源多次加载失败。请在网络恢复后刷新页面。</p>
      <p>刷新前会保存写笔记草稿；其他未保存修改请先处理。</p>
      <p v-if="recoverySaveFailed">草稿保存失败，未刷新页面。请先复制草稿后再尝试。</p>
      <button type="button" class="nw-action-btn" @click="reloadAfterSavingDraft">保存草稿并刷新</button>
      <button type="button" class="nw-action-btn" @click="moduleRecoveryFailed = false">暂不刷新</button>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useUserStore } from './store/user'
import Notification from './components/widgets/Notification.vue'
import { installMediaFancybox } from '~/utils/media-viewer-delegation'
const mediaToast = useToast()
const moduleRecoveryFailed = ref(false)
const recoverySaveFailed = ref(false)
const showModuleRecovery = () => { moduleRecoveryFailed.value = true }
const reloadAfterSavingDraft = () => {
  // Mounted composers synchronously flush the live editor, including text
  // entered less than one autosave debounce ago. Storage failure cancels reload.
  const save = new Event('save-draft-before-recovery', { cancelable: true })
  recoverySaveFailed.value = !window.dispatchEvent(save)
  if (!recoverySaveFailed.value) window.location.reload()
}
let disposeMediaViewer: (() => void) | undefined
onMounted(() => {
  window.addEventListener('module-recovery-failed', showModuleRecovery)
  disposeMediaViewer = installMediaFancybox(() => mediaToast.add({ title: '预览加载失败', description: '请再次点击图片或视频重试。', color: 'red' }))
})
onBeforeUnmount(() => disposeMediaViewer?.())
onBeforeUnmount(() => window.removeEventListener('module-recovery-failed', showModuleRecovery))

const userStore = useUserStore()

const AUTH_SYNC_INTERVAL_MS = 60 * 1000
const AUTH_SYNC_MIN_INTERVAL_MS = 5 * 1000

let authSyncTimer: number | undefined
let authSyncInFlight: Promise<unknown> | null = null
let lastAuthSyncAt = 0

const TOOLTIP_SUPPRESSED_CLASS = 'nw-tooltip-suppressed'
const FANCYBOX_TOOLTIP_CLASS = 'nw-tooltip--fancybox'
const TABLE_OVERLAY_TOOLTIP_CLASS = 'nw-tooltip--table-overlay'
const TOOLTIP_ANCHOR_SELECTOR = '[data-tooltip]:not(.nw-tooltip-anchor-local), [data-label]:not(.nw-tooltip-anchor-local), .vditor-tooltipped[aria-label]'

let tooltipEl: HTMLDivElement | null = null
let tooltipAnchor: HTMLElement | null = null

const tooltipAnchorFromEvent = (event: Event) => {
  const target = event.target
  if (!(target instanceof Element)) return null
  return target.closest<HTMLElement>(TOOLTIP_ANCHOR_SELECTOR)
}

const getTooltipText = (anchor: HTMLElement) => {
  return (anchor.dataset.tooltip || anchor.dataset.label || anchor.getAttribute('aria-label') || '').trim()
}

const ensureTooltipEl = () => {
  if (tooltipEl) return tooltipEl
  tooltipEl = document.createElement('div')
  tooltipEl.className = 'nw-tooltip'
  tooltipEl.setAttribute('role', 'tooltip')
  tooltipEl.style.display = 'none'
  document.body.appendChild(tooltipEl)
  return tooltipEl
}

const bodyZoomRatio = () => {
  const body = document.body
  if (!body) return 1
  const layoutWidth = body.offsetWidth || document.documentElement.clientWidth || window.innerWidth || 1
  const visualWidth = body.getBoundingClientRect().width || layoutWidth
  const ratio = visualWidth / layoutWidth
  return Number.isFinite(ratio) && ratio > 0 ? ratio : 1
}

const positionTooltip = (anchor: HTMLElement) => {
  if (!tooltipEl) return
  const rect = anchor.getBoundingClientRect()
  const tooltipRect = tooltipEl.getBoundingClientRect()
  const gap = 10
  const below = anchor.classList.contains('nw-tooltip-below')
  const viewport = window.visualViewport
  const viewportLeft = viewport?.offsetLeft || 0
  const viewportTop = viewport?.offsetTop || 0
  const viewportWidth = viewport?.width || window.innerWidth
  const viewportHeight = viewport?.height || window.innerHeight
  const zoom = bodyZoomRatio()

  const rawLeft = viewportLeft + rect.left + rect.width / 2 - tooltipRect.width / 2
  const left = Math.max(viewportLeft + 8, Math.min(rawLeft, viewportLeft + viewportWidth - tooltipRect.width - 8))
  const top = below
    ? Math.min(viewportTop + rect.bottom + gap, viewportTop + viewportHeight - tooltipRect.height - 8)
    : Math.max(viewportTop + 8, viewportTop + rect.top - tooltipRect.height - gap)

  tooltipEl.style.left = `${left / zoom}px`
  tooltipEl.style.top = `${top / zoom}px`
}

const showTooltip = (anchor: HTMLElement) => {
  if (anchor.classList.contains(TOOLTIP_SUPPRESSED_CLASS)) return
  if (!document.body.contains(anchor)) return
  const text = getTooltipText(anchor)
  if (!text) return
  const el = ensureTooltipEl()
  tooltipAnchor = anchor
  el.textContent = text
  el.classList.toggle(FANCYBOX_TOOLTIP_CLASS, !!anchor.closest('.fancybox__container'))
  el.classList.toggle(TABLE_OVERLAY_TOOLTIP_CLASS, !!anchor.closest('.editor-table-expand-overlay, .rendered-table-expand-overlay'))
  el.style.display = 'block'
  positionTooltip(anchor)
}

const hideTooltip = () => {
  tooltipAnchor = null
  if (tooltipEl) {
    tooltipEl.style.display = 'none'
    tooltipEl.classList.remove(FANCYBOX_TOOLTIP_CLASS)
    tooltipEl.classList.remove(TABLE_OVERLAY_TOOLTIP_CLASS)
  }
}

const handleTooltipPointerOver = (event: PointerEvent) => {
  const anchor = tooltipAnchorFromEvent(event)
  if (!anchor) return
  if (event.relatedTarget instanceof Node && anchor.contains(event.relatedTarget)) return
  showTooltip(anchor)
}

const handleTooltipPointerMove = () => {
  if (!tooltipAnchor) return
  if (!document.body.contains(tooltipAnchor) || tooltipAnchor.classList.contains(TOOLTIP_SUPPRESSED_CLASS)) {
    hideTooltip()
    return
  }
  positionTooltip(tooltipAnchor)
}

const hasLocalLoginState = () => !!userStore.isLogin || !!userStore.token

const syncAuthState = () => {
  if (!hasLocalLoginState()) return
  if (authSyncInFlight) return

  const now = Date.now()
  if (now - lastAuthSyncAt < AUTH_SYNC_MIN_INTERVAL_MS) return
  lastAuthSyncAt = now

  authSyncInFlight = userStore.getUser()
    .catch(() => false)
    .finally(() => {
      authSyncInFlight = null
    })
}

const syncAuthStateWhenVisible = () => {
  if (document.visibilityState === 'visible') syncAuthState()
}

const suppressTooltipOnActivation = (event: Event) => {
  tooltipAnchorFromEvent(event)?.classList.add(TOOLTIP_SUPPRESSED_CLASS)
  hideTooltip()
}

const clearSuppressedTooltipOnPointerOut = (event: PointerEvent) => {
  const anchor = tooltipAnchorFromEvent(event)
  if (!anchor) return
  const nextTarget = event.relatedTarget
  if (nextTarget instanceof Node && anchor.contains(nextTarget)) return
  anchor.classList.remove(TOOLTIP_SUPPRESSED_CLASS)
  if (tooltipAnchor === anchor) hideTooltip()
}

const clearSuppressedTooltipOnFocusOut = (event: FocusEvent) => {
  const anchor = tooltipAnchorFromEvent(event)
  anchor?.classList.remove(TOOLTIP_SUPPRESSED_CLASS)
  if (anchor && tooltipAnchor === anchor) hideTooltip()
}

onMounted(() => {
  userStore.getUser()
  window.addEventListener('pageshow', syncAuthState)
  window.addEventListener('focus', syncAuthState)
  document.addEventListener('visibilitychange', syncAuthStateWhenVisible)
  document.addEventListener('pointerover', handleTooltipPointerOver, true)
  document.addEventListener('pointermove', handleTooltipPointerMove, true)
  document.addEventListener('pointerdown', suppressTooltipOnActivation, true)
  document.addEventListener('click', suppressTooltipOnActivation, true)
  document.addEventListener('pointerout', clearSuppressedTooltipOnPointerOut, true)
  document.addEventListener('focusout', clearSuppressedTooltipOnFocusOut, true)

  authSyncTimer = window.setInterval(syncAuthStateWhenVisible, AUTH_SYNC_INTERVAL_MS)
})

onBeforeUnmount(() => {
  window.removeEventListener('pageshow', syncAuthState)
  window.removeEventListener('focus', syncAuthState)
  document.removeEventListener('visibilitychange', syncAuthStateWhenVisible)
  document.removeEventListener('pointerover', handleTooltipPointerOver, true)
  document.removeEventListener('pointermove', handleTooltipPointerMove, true)
  document.removeEventListener('pointerdown', suppressTooltipOnActivation, true)
  document.removeEventListener('click', suppressTooltipOnActivation, true)
  document.removeEventListener('pointerout', clearSuppressedTooltipOnPointerOut, true)
  document.removeEventListener('focusout', clearSuppressedTooltipOnFocusOut, true)
  tooltipEl?.remove()
  tooltipEl = null
  tooltipAnchor = null
  if (authSyncTimer) window.clearInterval(authSyncTimer)
})
</script>

<style scoped>
.module-recovery-notice {
  position: fixed;
  z-index: 10000;
  inset: auto 1rem 1rem;
  margin-inline: auto;
  max-width: 36rem;
  padding: 1rem;
  border: 1px solid #d97706;
  border-radius: 0.75rem;
  background: #fff7ed;
  color: #431407;
  box-shadow: 0 4px 24px #0003;
}
.module-recovery-notice p { margin-bottom: 0.5rem; }
.module-recovery-notice button { margin: 0.25rem 0.75rem 0 0; padding: 0.4rem 0.6rem; }
:global(.dark) .module-recovery-notice { background: #29201a; color: #ffedd5; }
</style>
