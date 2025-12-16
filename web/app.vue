<template>
  <div>
    <NuxtPage />
    <Notification />
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useUserStore } from './store/user'
import Notification from './components/widgets/Notification.vue'

const userStore = useUserStore()

onMounted(() => {
  userStore.getUser()
})
</script>
