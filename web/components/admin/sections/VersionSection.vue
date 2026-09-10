<template>
  <section id="version-section" :class="adminShellCardClass">
    <AdminModuleHeader title="版本与更新" icon="i-heroicons-arrow-path" description="查看当前构建信息并检查更新。" :theme="theme" />
    <div class="px-4 pb-4 space-y-4"><div class="admin-settings-grid">
      <section class="admin-form-section"><div class="admin-section-heading"><h3>安装信息</h3></div><div class="admin-fields-grid"><div><div class="text-sm" :class="theme.mutedText">当前版本</div><UBadge color="primary" variant="soft" class="admin-badge mt-1">{{ info.currentVersion || '最新' }}</UBadge></div><div v-if="isPrimaryAdmin"><div class="text-sm" :class="theme.mutedText">构建标记</div><UBadge color="gray" variant="soft" class="admin-badge mt-1 font-mono">{{ info.buildIdentity || 'unknown' }}</UBadge></div><div><div class="text-sm" :class="theme.mutedText">最新发布时间</div><div class="mt-1" :class="theme.text">{{ info.latestVersion || '—' }}</div></div></div></section>
      <section class="admin-form-section"><div class="admin-section-heading"><h3>维护操作</h3><p>检查发布时间，或执行当前账号有权限的更新操作。</p></div><div class="admin-setting-stack"><div class="flex items-center gap-2"><UButton size="sm" :loading="info.checking" color="primary" variant="soft" class="admin-action" @click="checkVersion">{{ info.checking ? '检测中...' : '检查更新' }}</UButton><UButton v-if="can('version.update')" size="sm" :loading="updating" color="orange" class="admin-action" @click="updateVersion">更新升级</UButton><UButton v-if="can('version.update') && runtime.staticSyncAvailable" size="sm" :loading="syncing" color="primary" variant="soft" class="admin-action" @click="syncStatic">同步静态资源</UButton></div><div v-if="updating" class="space-y-2"><UProgress :value="progress" color="orange" /><div class="text-xs" :class="theme.mutedText">{{ status }}</div></div><div v-if="success" class="text-sm text-green-600 dark:text-green-400">升级成功，将进入重启，请稍后</div></div></section>
    </div></div>
  </section>
</template>

<script setup lang="ts">
import { toRefs } from 'vue'
import { onMounted, reactive, ref } from 'vue'
import { useToast } from '#ui/composables/useToast'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const { theme, adminShellCardClass } = toRefs(props)
const { isPrimaryAdmin, can } = useAdminCapabilities()
const info = reactive({ checking: false, currentVersion: '', latestVersion: '', buildIdentity: '' })
const runtime = reactive({ staticSyncAvailable: true })
const updating = ref(false)
const syncing = ref(false)
const progress = ref(0)
const status = ref('')
const success = ref(false)
const formatTime = (value: any) => value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium', hour12: false, timeZone: 'Asia/Shanghai' }).format(new Date(value)) : ''
const load = async () => { const [runtimeResponse, build] = await Promise.allSettled([fetch('/api/version/runtime', { credentials: 'include' }).then(r => r.json()), isPrimaryAdmin.value ? fetch('/api/version/build', { credentials: 'include' }).then(r => r.json()) : Promise.resolve(null)]); if (runtimeResponse.status === 'fulfilled' && runtimeResponse.value?.code === 1) runtime.staticSyncAvailable = !runtimeResponse.value.data?.isContainer; if (build.status === 'fulfilled' && build.value?.code === 1) info.buildIdentity = String(build.value.data?.build_identity || 'unknown') }
const loadReleaseInfo = async (notify = false) => { info.checking = true; try { const response = await fetch('/api/version/check', { credentials: 'include', headers: { 'Cache-Control': 'no-cache', Pragma: 'no-cache' } }); const body = await response.json(); if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '检查更新失败'); info.currentVersion = String(body.data?.currentTag || ''); info.latestVersion = formatTime(body.data?.lastUpdateTime); if (notify) useToast().add({ title: '版本信息已更新', description: `${info.currentVersion} · ${info.latestVersion}`, color: 'green' }) } catch { if (notify) useToast().add({ title: '检查更新失败', description: '请检查网络后重试', color: 'red' }) } finally { info.checking = false } }
const checkVersion = () => loadReleaseInfo(true)
const updateVersion = async () => { updating.value = true; success.value = false; progress.value = 5; status.value = '连接升级通道...'; let settled = false; const finish = () => { if (!settled) { settled = true; setTimeout(() => { updating.value = false }, 6000) } }; try { const stream = new EventSource('/api/version/update/stream'); stream.onmessage = event => { let payload: any = {}; try { payload = JSON.parse(event.data || '{}') } catch {}; if (typeof payload.progress === 'number') progress.value = Math.max(progress.value, payload.progress); if (payload.message) status.value = payload.message; if (payload.type === 'error') { stream.close(); useToast().add({ title: '更新失败', description: payload.message || '升级失败', color: 'red' }); finish() } if (payload.type === 'success') { success.value = true; progress.value = 100 } if (payload.type === 'done') { stream.close(); finish(); setTimeout(() => location.reload(), 1500) } }; stream.onerror = async () => { stream.close(); try { const response = await fetch('/api/version/update', { method: 'POST', credentials: 'include' }); const body = await response.json(); if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '升级失败'); success.value = true; progress.value = 100; setTimeout(() => location.reload(), 1500) } catch (error: any) { useToast().add({ title: '更新失败', description: error?.message, color: 'red' }) } finally { finish() } } } catch (error: any) { useToast().add({ title: '更新失败', description: error?.message, color: 'red' }); finish() } }
const syncStatic = async () => { syncing.value = true; try { const response = await fetch('/api/version/static-sync', { method: 'POST', credentials: 'include' }); const body = await response.json(); if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '静态资源同步失败'); useToast().add({ title: body.msg || '静态资源已同步', color: 'green' }); setTimeout(() => location.reload(), 800) } catch (error: any) { useToast().add({ title: '同步失败', description: error?.message, color: 'red' }) } finally { syncing.value = false } }
onMounted(() => { void load(); void loadReleaseInfo() })
</script>
