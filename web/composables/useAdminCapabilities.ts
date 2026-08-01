import { computed, ref } from 'vue'
import { useUserStore } from '~/store/user'

const capabilities = ref<string[]>([])
const snapshotUserID = ref(0)
let refreshInFlight: Promise<void> | null = null
let refreshUserID = 0

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
        const response = await fetch('/api/admin/authorization/me', { credentials: 'include' })
        const body = await response.json().catch(() => ({}))
        if (currentUserID(userStore.user) !== requestedUserID) return
        capabilities.value = response.ok && body?.code === 1 ? (body.data?.capabilities || []) : []
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

  return { capabilities, isPrimaryAdmin, can, refreshCapabilities }
}
