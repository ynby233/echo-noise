import { defineStore } from 'pinia'
import { computed, reactive, watch } from 'vue'
import { useUserStore } from '~/store/user'
import { getRequest, putRequest, deleteRequest } from '~/utils/api'
import { createAuthorSubscriptions, type AuthorSubscription, type AuthorSubscriptionEntry } from '~/utils/author-subscriptions'

export const useAuthorSubscriptionStore = defineStore('authorSubscriptions', () => {
  const user = useUserStore()
  const entries = reactive<Record<number, AuthorSubscriptionEntry>>({})
  const sessionKey = computed(() => JSON.stringify([user.isLogin, user.user?.userid, user.token]))
  const subscriptions = createAuthorSubscriptions(entries, (method, id) => {
    const path = `authors/${id}/subscription`
    const options = { credentials: 'include' as const, silent: true }
    if (method === 'GET') return getRequest<AuthorSubscription>(path, undefined, options)
    if (method === 'PUT') return putRequest<AuthorSubscription>(path, undefined, options)
    return deleteRequest<AuthorSubscription>(path, undefined, options)
  })
  // A store-owned watcher survives individual author cards unmounting.
  watch(() => [user.isLogin, user.user?.userid, user.token] as const, () => {
    subscriptions.setSession({ isLogin: user.isLogin, userId: user.user?.userid, token: user.token })
  }, { immediate: true, flush: 'sync' })
  return { entries, sessionKey, ...subscriptions }
})
