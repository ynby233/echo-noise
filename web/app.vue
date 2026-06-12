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

const tooltipAnchorFromEvent = (event: Event) => {
  const target = event.target
  if (!(target instanceof Element)) return null
  return target.closest<HTMLElement>(TOOLTIP_ANCHOR_SELECTOR)
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
}

const clearSuppressedTooltipOnPointerOut = (event: PointerEvent) => {
  const anchor = tooltipAnchorFromEvent(event)
  if (!anchor) return
  const nextTarget = event.relatedTarget
  if (nextTarget instanceof Node && anchor.contains(nextTarget)) return
  anchor.classList.remove(TOOLTIP_SUPPRESSED_CLASS)
}

const clearSuppressedTooltipOnFocusOut = (event: FocusEvent) => {
  tooltipAnchorFromEvent(event)?.classList.remove(TOOLTIP_SUPPRESSED_CLASS)
}

onMounted(() => {
  userStore.getUser()
  window.addEventListener('pageshow', syncAuthState)
  window.addEventListener('focus', syncAuthState)
  document.addEventListener('visibilitychange', syncAuthStateWhenVisible)
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
  document.removeEventListener('pointerdown', suppressTooltipOnActivation, true)
  document.removeEventListener('click', suppressTooltipOnActivation, true)
  document.removeEventListener('pointerout', clearSuppressedTooltipOnPointerOut, true)
  document.removeEventListener('focusout', clearSuppressedTooltipOnFocusOut, true)
  if (authSyncTimer) window.clearInterval(authSyncTimer)
})
</script>
