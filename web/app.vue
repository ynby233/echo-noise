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

onMounted(() => {
  userStore.getUser()
  window.addEventListener('pageshow', syncAuthState)
  window.addEventListener('focus', syncAuthState)
  document.addEventListener('visibilitychange', syncAuthStateWhenVisible)

  authSyncTimer = window.setInterval(syncAuthStateWhenVisible, AUTH_SYNC_INTERVAL_MS)
})

onBeforeUnmount(() => {
  window.removeEventListener('pageshow', syncAuthState)
  window.removeEventListener('focus', syncAuthState)
  document.removeEventListener('visibilitychange', syncAuthStateWhenVisible)
  if (authSyncTimer) window.clearInterval(authSyncTimer)
})
</script>
