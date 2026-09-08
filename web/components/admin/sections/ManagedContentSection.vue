<template>
  <section :id="`${sectionKey}-section`" :class="adminPanelCardClass">
    <NoteManager v-if="sectionKey === 'notes'" :theme="theme" />
    <NoteManager v-else-if="sectionKey === 'recycle-bin'" :theme="theme" recycle-bin />
    <CommentManager v-else-if="sectionKey === 'comments'" :theme="theme" />
    <CommentManager v-else-if="sectionKey === 'comment-recycle-bin'" :theme="theme" recycle-bin />
  </section>
</template>

<script setup lang="ts">
import { toRefs } from 'vue'
import NoteManager from '~/components/admin/NoteManager.vue'
import CommentManager from '~/components/admin/CommentManager.vue'
import type { AdminSectionKey } from './registry'

const props = defineProps<{ sectionKey: AdminSectionKey, theme: any, adminPanelCardClass: any }>()
const { sectionKey, theme, adminPanelCardClass } = toRefs(props)
</script>
