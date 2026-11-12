<template>
  <BackupWorkbench kind="attachments" :theme="theme" :admin-shell-card-class="adminShellCardClass" />
</template>

<script setup lang="ts">
import BackupWorkbench from './BackupWorkbench.vue'
defineProps<{ theme: Record<string, string>, adminShellCardClass: string | string[] | Record<string, boolean> }>()
</script>
