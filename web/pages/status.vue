<template>
  <ClientOnly>
    <StatusPanelLazy />
  </ClientOnly>
</template>
<script setup lang="ts">
import { defineAsyncComponent } from 'vue'
const StatusPanelLazy = defineAsyncComponent(() => import('../components/index/StatusPanel.vue'))
definePageMeta({ middleware: 'auth' })
</script>
