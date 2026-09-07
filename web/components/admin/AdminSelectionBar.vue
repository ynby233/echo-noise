<template>
  <div class="admin-selection-bar" role="group" aria-label="批量选择与操作">
    <div class="admin-selection-summary">
      <label class="admin-selection-toggle">
        <input
          type="checkbox"
          :checked="checked"
          :indeterminate="partialSelected ?? (selected > 0 && !checked)"
          :disabled="disabled || total === 0"
          :aria-label="scopeLabel"
          @change="emit('select-all', ($event.target as HTMLInputElement).checked)"
        />
        <span>{{ scopeLabel }}</span>
      </label>
      <span class="admin-selection-count" aria-live="polite">
        <slot name="summary">已选 {{ selected }} / {{ total }} 项</slot>
      </span>
    </div>
    <div class="admin-selection-actions">
      <UButton class="admin-action" size="sm" color="gray" variant="soft" icon="i-heroicons-x-mark" :disabled="disabled || selected === 0" @click="emit('clear')">取消选择</UButton>
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
const props = withDefaults(defineProps<{
  selected: number
  total: number
  scopeLabel?: string
  disabled?: boolean
  allSelected?: boolean
  partialSelected?: boolean
}>(), { scopeLabel: '全选当前页', disabled: false, allSelected: undefined, partialSelected: undefined })

const emit = defineEmits<{
  'select-all': [selected: boolean]
  clear: []
}>()
const checked = computed(() => props.allSelected ?? (props.total > 0 && props.selected >= props.total))
</script>

<style scoped>
.admin-selection-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px 20px;
  padding: 12px 16px;
  border: 1px solid var(--admin-line);
  border-radius: var(--admin-radius);
  background: var(--admin-subtle);
}
.admin-selection-summary,
.admin-selection-toggle,
.admin-selection-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.admin-selection-summary,
.admin-selection-actions {
  min-width: 0;
  flex-wrap: wrap;
}
.admin-selection-toggle {
  min-height: 36px;
  cursor: pointer;
  font-size: 13px;
  color: var(--admin-ink);
}
.admin-selection-toggle input {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  accent-color: var(--admin-accent);
}
.admin-selection-toggle:has(input:disabled) {
  cursor: default;
  opacity: .6;
}
.admin-selection-count {
  font-size: 12px;
  color: var(--admin-muted);
}
.admin-selection-actions {
  gap: 8px;
}
@media (max-width: 600px) {
  .admin-selection-bar { padding: 12px; }
  .admin-selection-summary { gap: 4px 12px; }
  .admin-selection-actions { width: 100%; }
}
</style>
