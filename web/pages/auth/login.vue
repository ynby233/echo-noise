<template>
  <div class="fixed inset-0 bg-gradient-to-br from-slate-900 via-indigo-950 to-slate-800">
    <div class="absolute inset-0 backdrop-blur-xl"></div>
    <div class="relative z-10 flex min-h-screen items-center justify-center p-4">
      <UCard class="w-full max-w-md bg-slate-900/70 text-white border border-slate-700/40 shadow-2xl">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-2">
            <UIcon name="i-heroicons-lock-closed" class="w-6 h-6 text-indigo-300" />
            <h1 class="text-lg font-semibold">登录</h1>
          </div>
          <UButton variant="link" color="indigo" class="text-sm" @click="goRegister">去注册</UButton>
        </div>
        <UForm @submit.prevent="onSubmit">
          <UFormGroup label="用户名/已绑定邮箱" class="mb-3">
            <UInput v-model="form.username" placeholder="请输入用户名或已绑定邮箱" />
          </UFormGroup>
          <UFormGroup label="密码" class="mb-2">
            <UInput
              v-model="form.password"
              :type="showPassword ? 'text' : 'password'"
              placeholder="请输入密码"
              autocomplete="current-password"
              autocorrect="off"
              autocapitalize="off"
              spellcheck="false"
              :ui="{ icon: { trailing: { pointer: 'pointer-events-auto' } } }"
            >
              <template #trailing>
                <UButton
                  :icon="showPassword ? 'i-heroicons-eye-slash' : 'i-heroicons-eye'"
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
            <UButton variant="ghost" size="sm" @click="showForgot = true">忘记密码</UButton>
            <UButton :loading="submitting" :disabled="submitting" type="submit" color="primary">登录</UButton>
          </div>
        </UForm>
        <div class="mt-2" v-if="githubEnabled">
          <UButton class="w-full h-10 px-3 gap-2 justify-center font-medium bg-[#24292f] hover:bg-[#1f2328] text-white ring-1 ring-black/20" @click="loginWithGithub">
            <UIcon name="i-mdi-github" class="w-5 h-5" />
            <span>GitHub 一键登录</span>
          </UButton>
        </div>
      </UCard>
    </div>

    <UModal v-model="showForgot">
      <UCard class="bg-slate-900/80 text-white border border-slate-700/40">
        <div class="font-semibold mb-2">找回密码</div>
        <p class="text-sm opacity-80 mb-4">请通过Vocechat联系管理员进行处理</p>
        <div class="flex justify-end">
          <UButton color="primary" @click="showForgot = false">知道了</UButton>
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
const githubEnabled = ref(true)
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
      useToast().add({ title: '登录成功', color: 'green' })
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

const loginWithGithub = () => {
  window.location.href = `${baseApi}/oauth/github/login`
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
  try {
    const res = await fetch(`${baseApi}/frontend/config`, { credentials: 'include' })
    const data = await res.json()
    githubEnabled.value = !!data?.data?.frontendSettings?.githubOAuthEnabled
  } catch {}
})
</script>
