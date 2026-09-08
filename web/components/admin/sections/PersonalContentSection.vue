<template>
  <section :id="`${sectionKey}-section`" :class="adminPanelCardClass">
    <PersonalContentManager :section="personalSection" :theme="theme" />
  </section>
</template>

<script setup lang="ts">
import { toRefs } from 'vue'
import { computed } from 'vue'
import PersonalContentManager from '~/components/index/PersonalContentManager.vue'
import type { AdminSectionKey } from './registry'

const props = defineProps<{ sectionKey: AdminSectionKey, theme: any, adminPanelCardClass: any }>()
const { theme, adminPanelCardClass } = toRefs(props)
const personalSection = computed(() => ({
  'personal-notes': 'notes',
  'personal-note-recycle-bin': 'note-recycle-bin',
  'personal-interactions': 'interactions',
  'personal-interaction-recycle-bin': 'interaction-recycle-bin',
} as const)[props.sectionKey as 'personal-notes' | 'personal-note-recycle-bin' | 'personal-interactions' | 'personal-interaction-recycle-bin'])
</script>
