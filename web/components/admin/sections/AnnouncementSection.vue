<template>
  <div id="site-announcement-section">
    <section :class="adminPanelCardClass">
      <AdminModuleHeader title="公告栏" description="设置首页公告栏的显示状态与文案。" icon="i-heroicons-megaphone" :theme="theme">
        <template #actions><UToggle v-model="form.announcementEnabled" aria-label="启用公告栏" /><UButton size="sm" color="primary" class="admin-action" @click="save('announcementEnabled')">保存开关</UButton></template>
      </AdminModuleHeader>
      <div class="px-4 pb-4 admin-announcement-banner"><label class="block text-sm mb-2" :class="theme.mutedText" for="admin-announcement-text">公告栏文本</label><UTextarea id="admin-announcement-text" v-model="form.announcementText" :rows="2" placeholder="请输入公告内容" class="admin-textarea w-full mb-2" /><div class="flex justify-end"><UButton size="sm" color="primary" class="admin-action" @click="save('announcementText')">保存公告文本</UButton></div></div>
    </section>
    <div class="mt-4"><AdminAnnouncementManager :theme="theme" /></div>
  </div>
</template>

<script setup lang="ts">
import { toRefs } from 'vue'
import { onMounted, reactive } from 'vue'
import AdminAnnouncementManager from '~/components/admin/AdminAnnouncementManager.vue'
import { booleanSetting, loadFrontendSettings, saveFrontendSettings } from './frontend-settings'

const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const { theme, adminPanelCardClass } = toRefs(props)
const form = reactive({ announcementEnabled: false, announcementText: '' })
const toast = useToast()
const load = async () => { const { frontendSettings } = await loadFrontendSettings(); form.announcementEnabled = booleanSetting(frontendSettings.announcementEnabled); form.announcementText = String(frontendSettings.announcementText || '') }
const save = async (key: keyof typeof form) => {
  try { await saveFrontendSettings({ [key]: form[key] }); toast.add({ title: '成功', description: key === 'announcementEnabled' ? (form.announcementEnabled ? '已开启公告' : '已关闭公告') : '公告文本已更新', color: form.announcementEnabled ? 'green' : 'gray' }) }
  catch (error: any) { toast.add({ title: '失败', description: error.message || '保存失败', color: 'red' }); await load() }
}
onMounted(() => { void load() })
</script>
