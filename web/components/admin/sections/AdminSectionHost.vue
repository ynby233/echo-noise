<!-- Owns one admin section's asynchronous load, stale-result rejection and
retry UI. The loaded section owns its form state and cleanup. -->
<template>
  <div class="admin-section-host">
    <div v-if="state === 'loading'" class="admin-section-loading" aria-live="polite">
      <UIcon name="i-heroicons-arrow-path" class="h-5 w-5 animate-spin" />
      <span>正在加载面板…</span>
    </div>
    <div v-else-if="state === 'error'" class="admin-section-load-error" role="alert">
      <UIcon name="i-heroicons-exclamation-triangle" class="h-5 w-5" />
      <span>{{ errorMessage }}</span>
      <UButton size="sm" color="primary" variant="soft" class="admin-action" @click="load">重试</UButton>
    </div>
    <component
      :is="resolvedComponent"
      v-else-if="resolvedComponent"
      v-bind="$attrs"
      @restore-success="$emit('restore-success')"
    />
  </div>
</template>

<script setup lang="ts">
import { markRaw, ref, watch, type Component } from 'vue'
import type { AdminSectionLoader } from './registry'

defineOptions({ inheritAttrs: false })
const props = defineProps<{ loader: AdminSectionLoader }>()
defineEmits<{ 'restore-success': [] }>()

const resolvedComponent = ref<Component | null>(null)
const state = ref<'loading' | 'ready' | 'error'>('loading')
const errorMessage = ref('面板加载失败，请重试。')
let loadSequence = 0

const load = async () => {
  const sequence = ++loadSequence
  state.value = 'loading'
  errorMessage.value = '面板加载失败，请重试。'
  try {
    const sectionModule = await props.loader()
    if (sequence !== loadSequence) return
    resolvedComponent.value = markRaw(sectionModule.default)
    state.value = 'ready'
  } catch (error: any) {
    if (sequence !== loadSequence) return
    resolvedComponent.value = null
    errorMessage.value = error?.message || '面板加载失败，请重试。'
    state.value = 'error'
  }
}

watch(() => props.loader, load, { immediate: true })
</script>

<style scoped>
.admin-section-loading,
.admin-section-load-error {
  min-height: 14rem;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  padding: 2rem;
  border: 1px solid rgb(148 163 184 / 0.28);
  border-radius: 1rem;
}

.admin-section-load-error {
  flex-wrap: wrap;
  color: rgb(239 68 68);
}
</style>
