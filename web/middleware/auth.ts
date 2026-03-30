import { useUserStore } from '~/store/user'

export default defineNuxtRouteMiddleware(async (to) => {
  const publicPaths = ['/', '/auth/login', '/auth/register', '/auth/guide']
  if (publicPaths.includes(to.path)) return

  const baseApi = useRuntimeConfig().public.baseApi || '/api'
  try {
    const res = await $fetch(`${baseApi}/user`, { credentials: 'include' })
    if ((res as any)?.code === 1) return
  } catch {}

  try {
    const userStore = useUserStore()
    userStore.clearUserStatus()
  } catch {}

  return navigateTo({ path: '/auth/guide', query: { reason: 'entry', redirect: to.fullPath } })
})
