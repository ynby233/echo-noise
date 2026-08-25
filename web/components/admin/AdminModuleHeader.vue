<template>
  <header class="admin-module-header">
    <div class="admin-module-heading">
      <span class="admin-module-icon" :class="`is-${accent}`" aria-hidden="true">
        <UIcon :name="icon" class="h-[18px] w-[18px]" />
      </span>
      <div class="min-w-0">
        <div class="admin-module-title-row">
          <h2 class="admin-module-title text-base">{{ title }}</h2>
          <UBadge v-if="badge" color="gray" variant="soft" size="xs">{{ badge }}</UBadge>
        </div>
        <p class="admin-module-description" :class="theme?.mutedText || 'text-slate-500'">{{ description }}</p>
      </div>
    </div>
    <div v-if="$slots.actions" class="admin-module-actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  title: string
  description: string
  icon: string
  badge?: string
  accent?: 'primary' | 'warning' | 'slate'
  theme?: Record<string, string>
}>(), {
  badge: '',
  accent: 'primary',
  theme: () => ({})
})
</script>

<style scoped>
.admin-module-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 16px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.admin-module-heading {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 10px;
}

.admin-module-icon {
  display: inline-flex;
  width: 32px;
  height: 32px;
  flex: 0 0 32px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
}

.admin-module-icon.is-primary {
  color: rgb(79, 70, 229);
  background: rgba(99, 102, 241, 0.12);
}

.admin-module-icon.is-warning {
  color: rgb(217, 119, 6);
  background: rgba(245, 158, 11, 0.14);
}

.admin-module-icon.is-slate {
  color: rgb(71, 85, 105);
  background: rgba(100, 116, 139, 0.13);
}

:global(.dark) .admin-module-icon.is-slate {
  color: rgb(203, 213, 225);
}

.admin-module-title-row {
  display: flex;
  min-height: 24px;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.admin-module-title {
  font-weight: 700;
  line-height: 1.5;
  letter-spacing: -0.01em;
}

.admin-module-description {
  max-width: 48rem;
  margin-top: 2px;
  font-size: 12px;
  line-height: 1.6;
}

.admin-module-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 760px) {
  .admin-module-header {
    align-items: stretch;
    flex-direction: column;
  }

  .admin-module-actions {
    justify-content: stretch;
  }

  .admin-module-actions :deep(button) {
    width: 100%;
    justify-content: center;
  }
}

@media (max-width: 520px) {
  .admin-module-header {
    padding: 14px;
  }
}
</style>
