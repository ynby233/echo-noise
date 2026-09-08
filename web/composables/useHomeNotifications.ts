import { computed, ref } from 'vue'
import { useUserStore } from '~/store/user'
import { usePwaManager } from '~/composables/usePwaManager'
import { getRequest } from '~/utils/api'

// Homepage notification state survives panel unmounts. Navigation remains in the
// page; this module owns unread counts, return targets and the app badge update.
export const useHomeNotifications = () => {
  const userStore = useUserStore()
  const pwaManager = usePwaManager()
  const isLoggedIn = computed(() => !!(userStore.isLogin && userStore.user))
  const notificationTargetMessageId = ref<number | null>(null)
  const notificationTargetCommentId = ref<number | null>(null)
  const notificationTargetNotificationId = ref<number | null>(null)
  const notificationUnreadCount = ref(0)
  const announcementUnreadCount = ref(0)
  const notificationReturnPending = ref(false)
  const notificationReturnFocusId = ref<number | null>(null)
  const handleNotificationUnreadChange = (count: number) => {
    notificationUnreadCount.value = Math.max(0, Number(count || 0))
    void pwaManager.syncBadge(notificationUnreadCount.value)
  }

  const handleAnnouncementUnreadChange = (count: number) => {
    announcementUnreadCount.value = Math.max(0, Number(count || 0))
  }

  const loadNotificationUnreadCount = async () => {
    if (!isLoggedIn.value) {
      notificationUnreadCount.value = 0
      void pwaManager.syncBadge(0)
      return
    }
    const res = await getRequest<any>('notifications/unread-count', {}, { credentials: 'include', silent: true })
    const count = Number(res?.data?.unread_count ?? res?.data?.unreadCount ?? 0)
    notificationUnreadCount.value = Number.isFinite(count) ? Math.max(0, count) : 0
    void pwaManager.syncBadge(notificationUnreadCount.value)
  }

  return { notificationTargetMessageId, notificationTargetCommentId, notificationTargetNotificationId, notificationUnreadCount, announcementUnreadCount, notificationReturnPending, notificationReturnFocusId, handleNotificationUnreadChange, handleAnnouncementUnreadChange, loadNotificationUnreadCount }
}
