<template>
  <DataBackupSection :theme="theme" :admin-shell-card-class="adminShellCardClass" />
</template>

<script setup lang="ts">
import DataBackupSection from './DataBackupSection.vue'
defineProps<{ theme: Record<string, string>, adminShellCardClass: string | string[] | Record<string, boolean> }>()
</script>
