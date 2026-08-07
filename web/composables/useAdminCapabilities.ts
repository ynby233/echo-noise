import { computed, ref, watch } from 'vue'
import { useUserStore } from '~/store/user'
import { getRequest } from '~/utils/api'

const capabilities = ref<string[]>([])
const snapshotUserID = ref(0)
let refreshInFlight: Promise<void> | null = null
let refreshUserID = 0
let invalidationListenerInstalled = false

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
  const can = (capability: string) => isPrimaryAdmin.value || (hasCurrentSnapshot.value && capabilities.value.includes(capability))

  const refreshCapabilities = async () => {
    const requestedUserID = userID.value
    if (!isAdmin.value || !requestedUserID) {
      capabilities.value = []
      snapshotUserID.value = 0
      return
    }
    if (refreshInFlight && refreshUserID === requestedUserID) return refreshInFlight

    refreshUserID = requestedUserID
    refreshInFlight = (async () => {
      try {
        const body: any = await getRequest('admin/authorization/me', undefined, { credentials: 'include', silent: true })
        if (currentUserID(userStore.user) !== requestedUserID) return
        capabilities.value = body?.code === 1 ? (body.data?.capabilities || []) : []
        snapshotUserID.value = requestedUserID
      } catch {
        if (currentUserID(userStore.user) === requestedUserID) {
          capabilities.value = []
          snapshotUserID.value = requestedUserID
        }
      }
    })().finally(() => {
      if (refreshUserID === requestedUserID) refreshInFlight = null
    })

    return refreshInFlight
  }

  if (typeof window !== 'undefined' && !invalidationListenerInstalled) {
    window.addEventListener('admin-capabilities-invalidated', () => {
      capabilities.value = []
      snapshotUserID.value = 0
    })
    invalidationListenerInstalled = true
  }
  watch([userID, isAdmin], ([nextUserID, nextIsAdmin], previous) => {
    const previousUserID = Number(previous?.[0] || 0)
    if (!nextIsAdmin || !nextUserID || nextUserID !== previousUserID) {
      capabilities.value = []
      snapshotUserID.value = 0
    }
    if (nextIsAdmin && nextUserID) void refreshCapabilities()
  }, { immediate: true })

  return { capabilities, isPrimaryAdmin, can, refreshCapabilities }
}
