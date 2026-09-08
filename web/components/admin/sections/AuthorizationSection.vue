<template>
  <section id="authorization-section" :class="adminPanelCardClass">
    <AdminAuthorizationCenter :theme="theme" />
  </section>
</template>

<script setup lang="ts">
import { toRefs } from 'vue'
const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const { theme, adminPanelCardClass } = toRefs(props)
</script>
