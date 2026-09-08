<template>
  <div>
  <section id="site-ads-section" :class="adminPanelCardClass">
    <AdminModuleHeader title="广告" icon="i-heroicons-photo" description="管理广告内容、展示位置与轮播设置。" :theme="theme"><template #actions><div class="flex items-center gap-3"><span :class="[enabled ? 'text-green-600' : 'text-red-600', 'text-sm']">{{ enabled ? '已启用' : '未启用' }}</span><UToggle v-model="enabled" /><UButton size="sm" color="primary" class="admin-action" @click="save">保存</UButton></div></template></AdminModuleHeader>
      <div class="px-4 pb-4"><div class="admin-ad-workspace"><div class="text-xs mb-2" :class="theme.mutedText">若同时配置单条与多条，优先显示多条</div><div class="admin-inline-toolbar"><div class="flex gap-2"><UButton size="sm" color="primary" class="admin-action" @click="ads.push(makeEmptyAdConfig())">新增广告</UButton><UButton size="sm" color="gray" variant="soft" class="admin-action" @click="reset">重置为默认</UButton></div><label class="flex items-center gap-2"><span class="text-sm" :class="theme.mutedText">轮播间隔(ms)</span><UInput v-model.number="interval" type="number" class="admin-input w-28" /></label></div><div class="admin-ad-grid"><div v-for="(ad, index) in ads" :key="index" class="admin-ad-editor" :class="theme.border"><div class="flex items-center justify-between mb-2"><div :class="theme.text">广告 #{{ index + 1 }}</div><UButton size="sm" color="red" variant="soft" class="admin-action" @click="ads.splice(index, 1)">删除</UButton></div><div class="admin-ad-content"><button v-if="ad.imageURL" type="button" class="admin-ad-preview" @click="previewImage(resolveAdImageURL(baseApi, ad.imageURL))"><img :src="resolveAdImageURL(baseApi, ad.imageURL)" alt="广告图片预览" /></button><div class="space-y-3"><UInput v-model="ad.imageURL" placeholder="海报图片 URL" class="admin-input w-full" /><div class="flex gap-2"><UButton size="sm" color="primary" variant="soft" class="admin-action" :loading="uploading && cropIndex === index" @click="chooseAdImage(index)">上传并裁切</UButton><UButton size="sm" color="gray" variant="soft" class="admin-action" :disabled="!ad.imageURL" @click="previewImage(resolveAdImageURL(baseApi, ad.imageURL))">预览</UButton></div><p class="text-xs" :class="theme.mutedText">推荐使用 16:9 图片；上传后可拖动和缩放裁切。</p></div><label class="admin-labeled-field admin-ad-link"><span>跳转链接</span><UInput class="admin-input w-full" v-model="ad.linkURL" /></label><div class="admin-ad-options"><label class="admin-bg-style-control"><span :class="theme.mutedText">文字颜色</span><input v-model="ad.textColor" type="color" class="admin-bg-color-input" /></label><label class="admin-bg-style-control"><span :class="theme.mutedText">文字显示</span><USelect class="admin-select" v-model="ad.textDisplayMode" :options="[{ label: '悬浮时显示', value: 'hover' }, { label: '常驻显示', value: 'always' }]" /></label></div><UTextarea v-model="ad.description" :rows="2" placeholder="描述文本（可选）" class="admin-textarea admin-ad-description" /></div></div></div></div><div class="admin-form-actions"><UButton size="sm" color="primary" class="admin-action" @click="save">保存广告配置</UButton></div></div>
  </section>
  <input ref="imageInput" type="file" accept="image/*" class="hidden" @change="onImageSelected" />
  <ImageCropperModal v-model="cropOpen" :src="cropSource" title="裁切广告图片" :aspect-ratio="16 / 9" :output-width="1280" @confirm="uploadCroppedAdImage" />
  <UModal v-model="previewOpen"><div class="p-2"><img :src="previewURL" class="max-h-[70vh] w-auto mx-auto rounded" alt="广告预览" /></div></UModal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRuntimeConfig } from '#imports'
import { useToast } from '#ui/composables/useToast'
import { makeEmptyAdConfig, normalizeAdConfigs, resolveAdImageURL, type AdConfig } from '~/utils/ad-config'
import { resolveUploadedMediaUrl } from '~/utils/media-upload'
import ImageCropperModal from '~/components/admin/ImageCropperModal.vue'
import { booleanSetting, loadFrontendSettings, saveFrontendSettings } from './frontend-settings'
const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const theme = props.theme
const adminPanelCardClass = props.adminPanelCardClass
const baseApi = useRuntimeConfig().public.baseApi || '/api'
const enabled = ref(true)
const interval = ref(4000)
const ads = ref<AdConfig[]>([])
const imageInput = ref<HTMLInputElement | null>(null)
const cropOpen = ref(false)
const cropSource = ref('')
const cropIndex = ref(-1)
const uploading = ref(false)
const previewOpen = ref(false)
const previewURL = ref('')
const load = async () => { const settings = (await loadFrontendSettings()).frontendSettings; enabled.value = booleanSetting(settings.leftAdEnabled, true); interval.value = Number(settings.leftAdsIntervalMs || 4000); ads.value = normalizeAdConfigs(settings.leftAds) }
const save = async () => { try { await saveFrontendSettings({ leftAdEnabled: enabled.value, leftAdsIntervalMs: Number(interval.value || 4000), leftAds: normalizeAdConfigs(ads.value) }); useToast().add({ title: '成功', description: '广告配置已更新', color: 'green' }); await load() } catch (error: any) { useToast().add({ title: '失败', description: error?.message, color: 'red' }) } }
const reset = () => { enabled.value = true; interval.value = 4000; ads.value = normalizeAdConfigs([makeEmptyAdConfig()]) }
const chooseAdImage = (index: number) => { cropIndex.value = index; imageInput.value?.click() }
const onImageSelected = (event: Event) => { const file = (event.target as HTMLInputElement).files?.[0]; if (!file) return; if (cropSource.value.startsWith('blob:')) URL.revokeObjectURL(cropSource.value); cropSource.value = URL.createObjectURL(file); cropOpen.value = true; (event.target as HTMLInputElement).value = '' }
const uploadCroppedAdImage = async (blob: Blob) => { if (cropIndex.value < 0 || !ads.value[cropIndex.value]) return; uploading.value = true; try { const data = new FormData(); data.append('image', new File([blob], 'advertisement.webp', { type: blob.type || 'image/webp' })); const response = await fetch(`${baseApi}/images/upload`, { method: 'POST', credentials: 'include', body: data }); const body = await response.json().catch(() => ({})); if (!response.ok || body?.code !== 1 || !body.data) throw new Error(body?.msg || '上传失败'); ads.value[cropIndex.value].imageURL = resolveUploadedMediaUrl(String(body.data), baseApi); await save(); cropOpen.value = false } catch (error: any) { useToast().add({ title: '上传失败', description: error?.message, color: 'red' }) } finally { uploading.value = false } }
const previewImage = (url: string) => { previewURL.value = url; previewOpen.value = !!url }
onMounted(() => { void load() })
onUnmounted(() => { if (cropSource.value.startsWith('blob:')) URL.revokeObjectURL(cropSource.value) })
</script>
