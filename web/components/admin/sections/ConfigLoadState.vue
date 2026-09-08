<template>
  <div v-if="loading" class="px-4 py-3" role="status">正在读取配置…</div>
  <div v-if="error" class="px-4 py-3 flex items-center gap-3" role="alert">
    <span>{{ error }}</span><UButton size="sm" @click="$emit('retry')">重试</UButton>
  </div>
</template>
<script setup lang="ts">
defineProps<{ loading: boolean, error: string }>()
defineEmits<{ retry: [] }>()
</script>
