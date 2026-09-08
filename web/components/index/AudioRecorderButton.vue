<template>
  <AudioRecorder v-if="opened" v-bind="$attrs" :open-on-mount="true" @prepare-insert="$emit('prepare-insert')" />
  <button v-else type="button" class="tb-btn nw-action-btn nw-tooltip-anchor" data-tooltip="录音" aria-label="录音" aria-haspopup="dialog" @pointerdown="$emit('prepare-insert')" @click="opened = true">
    <UIcon name="i-mdi-microphone-outline" class="w-5 h-5" />
  </button>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { asyncFeature } from '~/utils/async-feature'
defineOptions({ inheritAttrs: false })
// Capture the editor insertion point before the lazy recorder replaces this button.
defineEmits(['prepare-insert'])
const opened = ref(false)
const AudioRecorder = asyncFeature(() => import('./AudioRecorder.vue'), '录音')
</script>
