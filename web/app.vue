<template>
  <div>
    <NuxtPage />
    <Notification />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import { useUserStore } from './store/user'
import Notification from './components/widgets/Notification.vue'

const userStore = useUserStore()

const AUTH_SYNC_INTERVAL_MS = 60 * 1000
const AUTH_SYNC_MIN_INTERVAL_MS = 5 * 1000

let authSyncTimer: number | undefined
let authSyncInFlight: Promise<unknown> | null = null
let lastAuthSyncAt = 0

const TOOLTIP_SUPPRESSED_CLASS = 'nw-tooltip-suppressed'
const TOOLTIP_ANCHOR_SELECTOR = '.nw-tooltip-anchor[data-tooltip], .nw-tooltip-anchor[data-label]'

let tooltipEl: HTMLDivElement | null = null
let tooltipAnchor: HTMLElement | null = null

const tooltipAnchorFromEvent = (event: Event) => {
  const target = event.target
  if (!(target instanceof Element)) return null
  return target.closest<HTMLElement>(TOOLTIP_ANCHOR_SELECTOR)
}

const getTooltipText = (anchor: HTMLElement) => {
  return (anchor.dataset.tooltip || anchor.dataset.label || '').trim()
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

const positionTooltip = (anchor: HTMLElement) => {
  if (!tooltipEl) return
  const rect = anchor.getBoundingClientRect()
  const tooltipRect = tooltipEl.getBoundingClientRect()
  const gap = 10
  const below = anchor.classList.contains('nw-tooltip-below')
  const rawLeft = rect.left + rect.width / 2 - tooltipRect.width / 2
  const left = Math.max(8, Math.min(rawLeft, window.innerWidth - tooltipRect.width - 8))
  const top = below
    ? Math.min(rect.bottom + gap, window.innerHeight - tooltipRect.height - 8)
    : Math.max(8, rect.top - tooltipRect.height - gap)

  tooltipEl.style.left = `${left}px`
  tooltipEl.style.top = `${top}px`
}

const showTooltip = (anchor: HTMLElement) => {
  if (anchor.classList.contains(TOOLTIP_SUPPRESSED_CLASS)) return
  const text = getTooltipText(anchor)
  if (!text) return
  const el = ensureTooltipEl()
  tooltipAnchor = anchor
  el.textContent = text
  el.style.display = 'block'
  positionTooltip(anchor)
}

const hideTooltip = () => {
  tooltipAnchor = null
  if (tooltipEl) tooltipEl.style.display = 'none'
}

const handleTooltipPointerOver = (event: PointerEvent) => {
  const anchor = tooltipAnchorFromEvent(event)
  if (!anchor) return
  if (event.relatedTarget instanceof Node && anchor.contains(event.relatedTarget)) return
  showTooltip(anchor)
}

const handleTooltipFocusIn = (event: FocusEvent) => {
  const anchor = tooltipAnchorFromEvent(event)
  if (anchor) showTooltip(anchor)
}

const handleTooltipPointerMove = () => {
  if (tooltipAnchor) positionTooltip(tooltipAnchor)
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
  document.addEventListener('focusin', handleTooltipFocusIn, true)
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
  document.removeEventListener('focusin', handleTooltipFocusIn, true)
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
