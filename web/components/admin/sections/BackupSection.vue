<template>
  <section id="backups-section" :class="adminShellCardClass">
    <AdminModuleHeader title="备份与恢复" icon="i-heroicons-archive-box" description="管理数据、附件、服务端配置备份和云连接。" :theme="theme" />
    <div class="backup-tabs" role="tablist" aria-label="备份工作区">
      <button v-for="tab in tabs" :id="`backup-tab-${tab}`" :key="tab" type="button" role="tab" :aria-selected="activeTab === tab" :aria-controls="`backup-panel-${tab}`" @click="selectTab(tab)">{{ labels[tab] }}</button>
    </div>
    <div class="backup-workspace-body">
      <div v-if="activeTab" :id="`backup-panel-${activeTab}`" role="tabpanel" :aria-labelledby="`backup-tab-${activeTab}`">
        <KeepAlive :max="4">
          <CloudBackupSection v-if="activeTab === 'cloud'" :theme="theme" :admin-shell-card-class="[]" :embedded="true" />
          <BackupWorkbench v-else :key="activeTab" :kind="activeTab" :theme="theme" :admin-shell-card-class="[]" :embedded="true" @restore-success="$emit('restore-success')" />
        </KeepAlive>
      </div>
      <p v-else role="status" class="py-4 text-sm" :class="theme.mutedText">当前账号没有可用的备份标签。服务端配置与云连接仅限站长。</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { accessibleBackupTabs, resolveBackupTab, type BackupTab } from '~/utils/admin-section-access'
import AdminModuleHeader from '~/components/admin/AdminModuleHeader.vue'

const BackupWorkbench = defineAsyncComponent(() => import('./BackupWorkbench.vue'))
const CloudBackupSection = defineAsyncComponent(() => import('./CloudBackupSection.vue'))
const props = defineProps<{
  theme: Record<string, string>
  adminShellCardClass: string | string[] | Record<string, boolean>
  backupTab?: string
}>()
const emit = defineEmits<{ 'restore-success': [], 'update:backup-tab': [tab: BackupTab] }>()
const { isPrimaryAdmin, can } = useAdminCapabilities()
const tabs = computed(() => accessibleBackupTabs(isPrimaryAdmin.value, can))
const labels: Record<BackupTab, string> = { data: '数据备份', attachments: '附件备份', 'server-config': '服务端配置', cloud: '云连接' }
const selectedTab = ref('')
const activeTab = computed(() => resolveBackupTab(selectedTab.value, tabs.value))
watch(() => props.backupTab, value => { selectedTab.value = value || '' }, { immediate: true })
const selectTab = (tab: BackupTab) => {
  if (!tabs.value.includes(tab)) return
  selectedTab.value = tab
  const aliases: Record<BackupTab, string> = { data: 'backup-data', attachments: 'backup-attachments', 'server-config': 'backup-server-config', cloud: 'cloud-backup' }
  const url = new URL(location.href)
  url.hash = `${aliases[tab]}-section`
  history.replaceState({}, document.title, url.toString())
  emit('update:backup-tab', tab)
}
</script>
