<template>
  <ClientOnly>
    <StatusPanelLazy />
    <Notification />
  </ClientOnly>
</template>
<script setup lang="ts">
import { defineAsyncComponent } from 'vue'
import Notification from '../components/widgets/Notification.vue'
const StatusPanelLazy = defineAsyncComponent(() => import('../components/index/StatusPanel.vue'))
definePageMeta({ middleware: 'auth' })
</script>
