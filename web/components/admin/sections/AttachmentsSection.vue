<template>
  <section id="attachments-section" :class="adminShellCardClass">
    <AttachmentManager :theme="theme" :is-cloud="attachmentStorageEnabled" />
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRuntimeConfig } from '#imports'
import AttachmentManager from '~/components/admin/AttachmentManager.vue'

const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const theme = props.theme
const adminShellCardClass = props.adminShellCardClass
const baseApi = useRuntimeConfig().public.baseApi || '/api'
const attachmentStorageEnabled = ref(false)

onMounted(async () => {
  try {
    const response = await fetch(`${baseApi}/frontend/config`, { credentials: 'include' })
    const body = await response.json()
    if (body?.code === 1) attachmentStorageEnabled.value = !!body.data?.attachmentStorageEnabled
  } catch {}
})
</script>
