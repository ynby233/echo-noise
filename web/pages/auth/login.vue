<template>
  <div class="fixed inset-0 bg-gradient-to-br from-slate-900 via-indigo-950 to-slate-800">
    <div class="absolute inset-0 backdrop-blur-xl"></div>
    <div class="relative z-10 flex min-h-screen items-center justify-center p-4">
      <UCard class="nw-modal-card is-dark auth-page-card w-full max-w-md text-white border border-slate-700/40 shadow-2xl" :ui="{ rounded: 'rounded-none', ring: 'ring-0', shadow: 'shadow-none' }">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-2">
            <UIcon name="i-heroicons-lock-closed" class="w-6 h-6 text-orange-300" />
            <h1 class="text-lg font-semibold">登录</h1>
          </div>
          <UButton variant="link" color="orange" class="text-sm" @click="goRegister">去注册</UButton>
        </div>
        <UForm :state="form" @submit.prevent="onSubmit">
          <UFormGroup label="用户名/已绑定邮箱" class="mb-3">
            <UInput v-model="form.username" color="orange" placeholder="请输入用户名或已绑定邮箱" />
          </UFormGroup>
          <UFormGroup label="密码" class="mb-2">
            <UInput
              v-model="form.password"
              :type="showPassword ? 'text' : 'password'"
              placeholder="请输入密码"
              color="orange"
              autocomplete="current-password"
              autocorrect="off"
              autocapitalize="off"
              spellcheck="false"
              :ui="{ icon: { trailing: { pointer: 'pointer-events-auto' } } }"
            >
              <template #trailing>
                <UButton
                  :icon="showPassword ? 'i-heroicons-eye' : 'i-heroicons-eye-slash'"
                  variant="ghost"
                  color="gray"
                  type="button"
                  :aria-label="showPassword ? '隐藏密码' : '查看密码'"
                  @mousedown.prevent
                  @click.stop="showPassword = !showPassword"
                />
              </template>
            </UInput>
          </UFormGroup>
          <div class="flex justify-between items-center mb-3">
            <UButton variant="ghost" color="orange" size="sm" @click="showForgot = true">忘记密码</UButton>
            <UButton :loading="submitting" :disabled="submitting" type="submit" color="orange">登录</UButton>
          </div>
        </UForm>
      </UCard>
    </div>

    <UModal v-model="showForgot" :ui="{ background: 'bg-transparent dark:bg-transparent', shadow: 'shadow-none', rounded: 'rounded-none' }">
      <UCard class="nw-modal-card is-dark text-white border border-slate-700/40" :ui="{ rounded: 'rounded-none', ring: 'ring-0', shadow: 'shadow-none' }">
        <div class="font-semibold mb-2">找回密码</div>
        <p class="text-sm opacity-80 mb-4">请通过Vocechat联系管理员进行处理</p>
        <div class="flex justify-end">
          <UButton color="orange" @click="showForgot = false">知道了</UButton>
        </div>
      </UCard>
    </UModal>
  </div>
  <UNotifications />
</template>

<script setup lang="ts">
definePageMeta({ layout: false })
import { useUserStore } from '~/store/user'
import { useToast } from '#imports'
const user = useUserStore()
const route = useRoute()
const router = useRouter()
const baseApi = useRuntimeConfig().public.baseApi || '/api'

const form = reactive({ username: '', password: '' })
const submitting = ref(false)
const showForgot = ref(false)
const showPassword = ref(false)

const onSubmit = async () => {
  submitting.value = true
  const controller = new AbortController()
  const timeout = setTimeout(() => {
    controller.abort()
    useToast().add({ title: '登录失败', description: '请求超时或服务器不可用', color: 'red' })
    submitting.value = false
  }, 8000)
  try {
    const ok = await user.login({ username: form.username, password: form.password })
    if (ok) {
      useToast().add({ title: '登录成功', color: 'orange' })
      const redirect = (route.query.redirect as string) || '/status'
      router.push(redirect)
    }
  } catch (e) {
    useToast().add({ title: '登录失败', description: '请检查账号密码与后端服务', color: 'red' })
  } finally {
    clearTimeout(timeout)
    submitting.value = false
  }
}

const goRegister = async () => {
  try {
    const res = await fetch(`${baseApi}/frontend/config`, { credentials: 'include' })
    const data = await res.json()
    const allowed = !!data?.data?.allowRegistration
    if (!allowed) {
      useToast().add({ title: '提示', description: '站点已关闭用户注册', color: 'orange' })
      return
    }
    useRouter().push('/auth/register')
  } catch {
    useRouter().push('/auth/register')
  }
}

onMounted(async () => {
  const ok = await user.checkLoginStatus()
  if (ok) router.push('/status')
})
</script>
