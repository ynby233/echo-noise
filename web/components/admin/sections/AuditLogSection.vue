<template>
  <section id="admin-audit-section" :class="adminPanelCardClass">
    <AdminAuditLogPanel :is-primary-admin="isPrimaryAdmin" :theme="theme" />
  </section>
</template>

<script setup lang="ts">
import { toRefs } from 'vue'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'

const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const { theme, adminPanelCardClass } = toRefs(props)
const { isPrimaryAdmin } = useAdminCapabilities()
</script>
