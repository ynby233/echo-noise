<template>
  <div class="relative min-h-screen overflow-hidden bg-slate-950">
    <div class="absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,rgba(56,189,248,0.18),transparent_38%),radial-gradient(circle_at_80%_15%,rgba(168,85,247,0.2),transparent_40%),radial-gradient(circle_at_50%_80%,rgba(16,185,129,0.16),transparent_35%)]" />
    <div class="absolute inset-0 backdrop-blur-3xl" />
    <div class="relative z-10 flex min-h-screen items-center justify-center px-4">
      <UCard class="w-full max-w-xl border border-white/15 bg-slate-900/70 shadow-[0_20px_80px_-30px_rgba(56,189,248,0.45)]">
        <div class="space-y-6 text-center">
          <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-cyan-500/20 ring-1 ring-cyan-300/40">
            <UIcon :name="iconName" class="h-9 w-9 text-cyan-200" />
          </div>
          <div class="space-y-2">
            <h1 class="text-2xl font-bold tracking-wide text-white">{{ title }}</h1>
            <p class="mx-auto max-w-md text-sm leading-6 text-slate-200/90">{{ description }}</p>
          </div>
          <div class="flex flex-col items-center justify-center gap-3 sm:flex-row">
            <UButton color="primary" size="lg" class="w-full sm:w-auto sm:hidden" @click="goLogin">前往登录</UButton>
            <UButton color="primary" size="lg" class="hidden w-full sm:inline-flex sm:w-auto" @click="goHome">返回首页</UButton>
            <UButton variant="soft" color="gray" size="lg" class="hidden w-full sm:inline-flex sm:w-auto" @click="goLogin">返回登录</UButton>
          </div>
        </div>
      </UCard>
    </div>
  </div>
</template>

<script setup lang="ts">
// @ts-ignore Nuxt macro
definePageMeta({ layout: false })
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const router = useRouter()
const route = useRoute()

const reason = computed(() => String(route.query.reason || 'entry'))
const redirectPath = computed(() => String(route.query.redirect || '/status'))
const title = computed(() => reason.value === 'expired' ? '登录状态已过期' : '当前页面需要登录后访问')
const description = computed(() => reason.value === 'expired'
  ? '你的登录状态已失效，请重新登录后继续访问后台功能。'
  : '检测到你直接访问了后台管理页，请先完成登录认证。')
const iconName = computed(() => reason.value === 'expired' ? 'i-heroicons-shield-exclamation' : 'i-heroicons-lock-closed')

const goHome = () => {
  router.push('/')
}
const goLogin = () => {
  router.push({ path: '/', query: { login: '1', mode: 'login', redirect: redirectPath.value } })
}
</script>
