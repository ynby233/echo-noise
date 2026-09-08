<template>
  <section id="authorization-section" :class="adminPanelCardClass">
    <AdminAuthorizationCenter :theme="theme" />
  </section>
</template>

<script setup lang="ts">
const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const theme = props.theme
const adminPanelCardClass = props.adminPanelCardClass
</script>
