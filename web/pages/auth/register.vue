<template>
  <div class="fixed inset-0 bg-gradient-to-br from-slate-900 via-indigo-950 to-slate-800">
    <div class="absolute inset-0 backdrop-blur-xl"></div>
    <div class="relative z-10 flex min-h-screen items-center justify-center p-4">
      <UCard class="nw-modal-card is-dark auth-page-card w-full max-w-md text-white border border-slate-700/40 shadow-2xl" :ui="{ rounded: 'rounded-none', ring: 'ring-0', shadow: 'shadow-none' }">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-2">
            <UIcon name="i-heroicons-user-plus" class="w-6 h-6 text-orange-300" />
            <h1 class="text-lg font-semibold">注册</h1>
          </div>
          <NuxtLink to="/auth/login" class="text-sm text-orange-300 hover:text-orange-200">去登录</NuxtLink>
        </div>
        <UForm :state="form" @submit.prevent="onSubmit">
          <UFormGroup label="用户名" class="mb-3">
            <UInput v-model="form.username" color="orange" placeholder="请输入用户名" />
            <p class="mt-1 text-xs text-slate-400">用户名 2-20 字符，支持大小写英文字母、中文、日文、数字和下划线。</p>
          </UFormGroup>
          <UFormGroup label="密码" class="mb-3">
            <UInput v-model="form.password" color="orange" type="password" placeholder="请输入密码" />
          </UFormGroup>
          <UFormGroup label="验证码" class="mb-2">
            <div class="flex items-center gap-2">
              <UInput v-model="form.captcha" color="orange" placeholder="请输入验证码" />
              <img :src="captchaSrc" @click="refreshCaptcha" class="h-10 w-24 rounded border border-slate-700/40 cursor-pointer" alt="captcha" />
              <UBadge :color="remaining>0 ? 'orange' : 'red'" variant="soft">{{ remaining>0 ? `有效 ${remaining}s` : '已过期' }}</UBadge>
            </div>
          </UFormGroup>
          <div class="flex justify-end">
            <UButton :loading="submitting" :disabled="remaining<=0 || submitting" type="submit" color="orange">{{ remaining>0 ? '注册' : '验证码已过期' }}</UButton>
          </div>
        </UForm>
      </UCard>
    </div>
  </div>
  <UNotifications />
</template>

<script setup lang="ts">
definePageMeta({ layout: false })
import { useUserStore } from '~/store/user'
import { useToast } from '#imports'
import { useRouter, useRuntimeConfig } from '#imports'
const user = useUserStore()
const toast = useToast()
const router = useRouter()
const baseApi = useRuntimeConfig().public.baseApi || '/api'

const form = reactive({ username: '', password: '', captcha: '', captcha_id: '' })
const submitting = ref(false)
const captchaSrc = ref('')
const captchaId = ref('')
const captchaExpiresAt = ref<number | null>(null)
const remaining = ref(0)
let timer: any = null

const refreshCaptcha = async () => {
  try {
    const res = await fetch(`${baseApi}/captcha?json=1&ts=${Date.now()}`, { credentials: 'include' })
    const data = await res.json().catch(() => ({}))
    const svg = String(data?.data?.svg || '')
    const id = String(data?.data?.captcha_id || '')
    if (!svg || !id || data?.code !== 1) throw new Error(data?.msg || '获取验证码失败')
    captchaId.value = id
    form.captcha_id = id
    captchaSrc.value = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
    captchaExpiresAt.value = Date.now() + (Number(data?.data?.expires_in || 120) * 1000)
    remaining.value = Math.max(1, Number(data?.data?.expires_in || 120))
    if (timer) clearInterval(timer)
    timer = setInterval(() => {
      const r = Math.max(0, Math.ceil(((captchaExpiresAt.value || Date.now()) - Date.now()) / 1000))
      remaining.value = r
      if (r <= 0) clearInterval(timer)
    }, 1000)
  } catch (e) {
    remaining.value = 0
  }
}

const onSubmit = async () => {
  try {
    submitting.value = true
    if (!form.username || !form.password || !form.captcha) {
      throw new Error('请完整填写用户名、密码与验证码')
    }
    if ((captchaExpiresAt.value || 0) < Date.now()) {
      throw new Error('验证码已过期，请刷新后再提交')
    }
    const res = await fetch(`${baseApi}/register`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ username: form.username, password: form.password, captcha: form.captcha, captcha_id: form.captcha_id || captchaId.value })
    })
    const data = await res.json().catch(() => ({}))
    if (!res.ok || data.code !== 1) throw new Error(data?.msg || '注册失败')
    toast.add({ title: '申请已提交', description: '请等待管理员审核后再登录', color: 'orange' })
    router.push('/auth/login')
  } catch (e: any) {
    toast.add({ title: '注册失败', description: e.message || '请稍后重试', color: 'red' })
    refreshCaptcha()
  }
  finally {
    submitting.value = false
  }
}

onMounted(() => {
  refreshCaptcha()
  // 进入注册页时校验是否允许注册
  ;(async () => {
    try {
      const res = await fetch(`${baseApi}/frontend/config`, { credentials: 'include' })
      const data = await res.json()
      const allowed = !!data?.data?.allowRegistration
      if (!allowed) {
        useToast().add({ title: '提示', description: '站点已关闭用户注册', color: 'orange' })
        useRouter().push('/auth/login')
      }
    } catch {}
  })()
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
