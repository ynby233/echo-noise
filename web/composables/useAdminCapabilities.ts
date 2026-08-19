import { computed, ref, watch } from 'vue'
import { useUserStore } from '~/store/user'
import { getRequest } from '~/utils/api'

const capabilities = ref<string[]>([])
const snapshotUserID = ref(0)
let refreshInFlight: Promise<void> | null = null
let refreshUserID = 0
let refreshGeneration = -1
let capabilityGeneration = 0
let invalidationListenerInstalled = false
let identityWatcherInstalled = false
let invalidationScheduled = false
let invalidationRefreshInFlight = false

const currentUserID = (user: any) => Number(user?.userid || user?.id || user?.ID || user?.user_id || 0)

export const useAdminCapabilities = () => {
  const userStore = useUserStore()
  const isAdmin = computed(() => {
    const user: any = userStore.user
    return !!(userStore.isLogin && user && (user.is_admin || user.IsAdmin))
  })
  const userID = computed(() => currentUserID(userStore.user))
  const isPrimaryAdmin = computed(() => isAdmin.value && userID.value === 1)
  const hasCurrentSnapshot = computed(() => snapshotUserID.value === userID.value)
  const isReady = computed(() => !isAdmin.value || isPrimaryAdmin.value || hasCurrentSnapshot.value)
  const isLoading = computed(() => isAdmin.value && !isPrimaryAdmin.value && !hasCurrentSnapshot.value)
  const can = (capability: string) => isPrimaryAdmin.value || (hasCurrentSnapshot.value && capabilities.value.includes(capability))

  const refreshCapabilities = async () => {
    const requestedUserID = userID.value
    const requestedGeneration = capabilityGeneration
    if (!isAdmin.value || !requestedUserID) {
      capabilities.value = []
      snapshotUserID.value = 0
      return
    }
    if (refreshInFlight && refreshUserID === requestedUserID && refreshGeneration === requestedGeneration) return refreshInFlight

    refreshUserID = requestedUserID
    refreshGeneration = requestedGeneration
    refreshInFlight = (async () => {
      try {
        const body: any = await getRequest('admin/authorization/me', undefined, { credentials: 'include', silent: true })
        if (currentUserID(userStore.user) !== requestedUserID || capabilityGeneration !== requestedGeneration) return
        capabilities.value = body?.code === 1 ? (body.data?.capabilities || []) : []
        snapshotUserID.value = requestedUserID
      } catch {
        if (currentUserID(userStore.user) === requestedUserID && capabilityGeneration === requestedGeneration) {
          capabilities.value = []
          snapshotUserID.value = requestedUserID
        }
      }
    })().finally(() => {
      if (refreshUserID === requestedUserID && refreshGeneration === requestedGeneration) refreshInFlight = null
    })

    return refreshInFlight
  }

  if (typeof window !== 'undefined' && !invalidationListenerInstalled) {
    window.addEventListener('admin-capabilities-invalidated', () => {
      if (invalidationScheduled || invalidationRefreshInFlight) return
      invalidationScheduled = true
      queueMicrotask(() => {
        invalidationScheduled = false
        if (invalidationRefreshInFlight) return
        invalidationRefreshInFlight = true
        capabilityGeneration += 1
        capabilities.value = []
        snapshotUserID.value = 0
        void refreshCapabilities().finally(() => { invalidationRefreshInFlight = false })
      })
    })
    invalidationListenerInstalled = true
  }
  if (!identityWatcherInstalled) {
    watch([userID, isAdmin], ([nextUserID, nextIsAdmin], previous) => {
      const previousUserID = Number(previous?.[0] || 0)
      if (!nextIsAdmin || !nextUserID || (previousUserID > 0 && nextUserID !== previousUserID) || (snapshotUserID.value > 0 && snapshotUserID.value !== nextUserID)) {
        capabilityGeneration += 1
        capabilities.value = []
        snapshotUserID.value = 0
      }
      if (nextIsAdmin && nextUserID && snapshotUserID.value !== nextUserID) void refreshCapabilities()
    }, { immediate: true })
    identityWatcherInstalled = true
  }

  return { capabilities, isPrimaryAdmin, isReady, isLoading, can, refreshCapabilities }
}
