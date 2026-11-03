<!-- Owns one admin section's asynchronous load, stale-result rejection and
retry UI. The loaded section owns its form state and cleanup. -->
<template>
  <div class="admin-section-host">
    <section v-if="state === 'loading'" class="admin-section-pending" :class="$attrs['admin-panel-card-class']" aria-busy="true">
      <h2 class="px-4 pt-4 text-lg font-semibold">{{ sectionTitle }}</h2>
      <p class="px-4 py-3 text-sm" role="status">正在准备配置界面…</p>
    </section>
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
import { markRaw, onUnmounted, ref, watch, type Component } from 'vue'
import type { AdminSectionLoader } from './registry'
import type { AdminSectionModuleCache } from './module-cache'

defineOptions({ inheritAttrs: false })
const props = defineProps<{ loader: AdminSectionLoader, moduleCache: AdminSectionModuleCache, sectionTitle: string }>()
defineEmits<{ 'restore-success': [] }>()

const resolvedComponent = ref<Component | null>(null)
const state = ref<'loading' | 'ready' | 'error'>('loading')
const errorMessage = ref('面板加载失败，请重试。')
let loadSequence = 0

const load = async () => {
  const sequence = ++loadSequence
  const cached = props.moduleCache.get(props.loader)
  if (cached) {
    resolvedComponent.value = markRaw(cached.default)
    state.value = 'ready'
    return
  }
  state.value = 'loading'
  errorMessage.value = '面板加载失败，请重试。'
  try {
    const sectionModule = await props.moduleCache.load(props.loader)
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
onUnmounted(() => { ++loadSequence })
</script>

<style scoped>
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
