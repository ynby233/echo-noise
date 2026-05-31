<template>
    <div class="notify-panel-shell mb-6">
        <div class="notify-panel-toolbar">
            <div>
                <h2 class="text-xl font-semibold" :class="text">推送渠道配置</h2>
                <p class="mt-1 text-sm" :class="mutedText">统一展示渠道卡片与配置表单，不再通过“设置”按钮切换输入区域。</p>
            </div>
            <div class="flex flex-wrap gap-2">
                <UButton color="gray" variant="soft" @click="resetAll">恢复默认</UButton>
                <UButton color="primary" @click="saveAll">保存配置</UButton>
            </div>
        </div>

        <div class="notify-channel-grid" :class="props.disabled ? 'opacity-60 pointer-events-none' : ''">
            <button
                v-for="channel in channels"
                :key="channel.key"
                type="button"
                class="notify-channel-card"
                :class="[{ 'notify-channel-card-active': activeChannel === channel.key }, subtleBg]"
                @click="activeChannel = channel.key"
            >
                <div class="flex items-start justify-between gap-3">
                    <div class="flex items-center gap-3">
                        <div class="notify-channel-icon" :class="activeChannel === channel.key ? 'notify-channel-icon-active' : ''">
                            <UIcon :name="channel.icon" class="w-5 h-5" />
                        </div>
                        <div class="min-w-0 text-left">
                            <div class="font-semibold" :class="text">{{ channel.label }}</div>
                            <div class="text-xs mt-1 truncate" :class="mutedText">{{ channel.description }}</div>
                        </div>
                    </div>
                    <span class="notify-channel-badge" :class="channel.enabled ? 'notify-channel-badge-enabled' : 'notify-channel-badge-disabled'">
                        {{ channel.enabled ? '已启用' : '未启用' }}
                    </span>
                </div>
                <div class="notify-channel-preview mt-4">
                    <span class="text-xs" :class="mutedText">当前配置</span>
                    <div class="text-sm mt-1 truncate" :class="text">{{ channel.preview }}</div>
                </div>
            </button>
        </div>

        <div class="notify-panel-detail" :class="[subtleBg, props.disabled ? 'opacity-60 pointer-events-none' : '']">
            <div class="notify-panel-detail-head">
                <div>
                    <div class="flex items-center gap-2">
                        <div class="notify-channel-icon notify-channel-icon-active">
                            <UIcon :name="currentChannel.icon" class="w-5 h-5" />
                        </div>
                        <div class="text-lg font-semibold" :class="text">{{ currentChannel.label }} 推送</div>
                    </div>
                    <p class="mt-2 text-sm" :class="mutedText">{{ currentChannel.tip }}</p>
                </div>
                <div class="flex flex-wrap items-center gap-3">
                    <span class="notify-channel-badge" :class="currentChannel.enabled ? 'notify-channel-badge-enabled' : 'notify-channel-badge-disabled'">
                        {{ currentChannel.enabled ? '已启用' : '未启用' }}
                    </span>
                    <UToggle :model-value="currentChannel.enabled" @update:model-value="setChannelEnabled(currentChannel.key, !!$event)" />
                    <UButton color="primary" variant="soft" :disabled="props.disabled" @click="testNotify(currentChannel.key)">
                        测试当前渠道
                    </UButton>
                </div>
            </div>

            <div class="notify-field-grid">
                <template v-if="currentChannel.key === 'webhook'">
                    <div class="notify-field md:col-span-2">
                        <label class="notify-field-label" :class="text">Webhook 地址</label>
                        <p class="notify-field-tip" :class="mutedText">用于接收推送消息的完整地址。</p>
                        <UInput v-model="localConfig.webhookURL" placeholder="https://example.com/webhook" />
                    </div>
                </template>

                <template v-else-if="currentChannel.key === 'telegram'">
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">Bot Token</label>
                        <p class="notify-field-tip" :class="mutedText">机器人令牌，建议粘贴完整值后立即保存。</p>
                        <UInput v-model="localConfig.telegramToken" placeholder="123456:ABCDEF..." />
                    </div>
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">Chat ID</label>
                        <p class="notify-field-tip" :class="mutedText">支持群组或频道 ID。</p>
                        <UInput v-model="localConfig.telegramChatID" placeholder="-1001234567890" />
                    </div>
                </template>

                <template v-else-if="currentChannel.key === 'wework'">
                    <div class="notify-field md:col-span-2">
                        <label class="notify-field-label" :class="text">Webhook Key</label>
                        <p class="notify-field-tip" :class="mutedText">填写企微机器人 Webhook 的 key 部分即可。</p>
                        <UInput v-model="localConfig.weworkKey" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
                    </div>
                </template>

                <template v-else-if="currentChannel.key === 'feishu'">
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">Webhook 地址</label>
                        <p class="notify-field-tip" :class="mutedText">填写飞书机器人 Webhook URL。</p>
                        <UInput v-model="localConfig.feishuWebhook" placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..." />
                    </div>
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">签名密钥</label>
                        <p class="notify-field-tip" :class="mutedText">开启签名校验时填写，不需要可留空。</p>
                        <UInput v-model="localConfig.feishuSecret" placeholder="secret" type="password" />
                    </div>
                </template>

                <template v-else-if="currentChannel.key === 'twitter'">
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">API Key</label>
                        <p class="notify-field-tip" :class="mutedText">应用 API Key。</p>
                        <UInput v-model="localConfig.twitterApiKey" placeholder="API Key" />
                    </div>
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">API Secret</label>
                        <p class="notify-field-tip" :class="mutedText">应用 API Secret。</p>
                        <UInput v-model="localConfig.twitterApiSecret" placeholder="API Secret" />
                    </div>
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">Access Token</label>
                        <p class="notify-field-tip" :class="mutedText">账户授权 Token。</p>
                        <UInput v-model="localConfig.twitterAccessToken" placeholder="Access Token" />
                    </div>
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">Access Token Secret</label>
                        <p class="notify-field-tip" :class="mutedText">账户授权 Token Secret。</p>
                        <UInput v-model="localConfig.twitterAccessTokenSecret" placeholder="Access Token Secret" />
                    </div>
                </template>

                <template v-else-if="currentChannel.key === 'customHttp'">
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">请求地址</label>
                        <p class="notify-field-tip" :class="mutedText">支持任何自定义通知接口地址。</p>
                        <UInput v-model="localConfig.customHttpUrl" placeholder="https://example.com/notify" />
                    </div>
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">请求方法</label>
                        <p class="notify-field-tip" :class="mutedText">建议优先使用 POST。</p>
                        <USelect v-model="localConfig.customHttpMethod" :options="['POST', 'PUT', 'PATCH']" />
                    </div>
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">请求头</label>
                        <p class="notify-field-tip" :class="mutedText">填写 JSON 字符串，例如 Authorization 头。</p>
                        <UTextarea v-model="localConfig.customHttpHeaders" :rows="6" placeholder='{"Authorization":"Bearer token"}' />
                    </div>
                    <div class="notify-field">
                        <label class="notify-field-label" :class="text">请求体模板</label>
                        <p v-pre class="notify-field-tip" :class="mutedText">支持内容模板变量，例如 {{content}}。</p>
                        <UTextarea v-model="localConfig.customHttpBody" :rows="6" placeholder='{"content":"{{content}}"}' />
                    </div>
                </template>
            </div>
        </div>

        <div class="notify-test-grid" :class="props.disabled ? 'opacity-60 pointer-events-none' : ''">
            <UButton
                v-for="type in notifyTypes"
                :key="type"
                :variant="activeChannel === type ? 'solid' : 'soft'"
                :color="activeChannel === type ? 'primary' : 'gray'"
                @click="testNotify(type)"
                :disabled="props.disabled"
            >
                测试{{ getNotifyTypeName(type) }}
            </UButton>
        </div>
    </div>
</template>

<script setup lang="ts">
import { useToast } from '#ui/composables/useToast'
import { useRuntimeConfig } from '#imports'
import { ref, reactive, watch, onMounted, computed } from 'vue'

const emit = defineEmits(['save'])

const props = defineProps<{
    config: NotifyConfig;
    immediate?: boolean;
    subtleBg?: string;
    text?: string;
    mutedText?: string;
    disabled?: boolean;
}>();

interface NotifyConfig {
    webhookEnabled: boolean;
    webhookURL: string;
    telegramEnabled: boolean;
    telegramToken: string;
    telegramChatID: string;
    weworkEnabled: boolean;
    weworkKey: string;
    feishuEnabled: boolean;
    feishuWebhook: string;
    feishuSecret: string;
    twitterEnabled: boolean;
    twitterApiKey: string;
    twitterApiSecret: string;
    twitterAccessToken: string;
    twitterAccessTokenSecret: string;
    customHttpEnabled: boolean;
    customHttpUrl: string;
    customHttpMethod: string;
    customHttpHeaders: string;
    customHttpBody: string;
}

const localConfig = reactive<NotifyConfig>({
    webhookEnabled: false,
    webhookURL: '',
    telegramEnabled: false,
    telegramToken: '',
    telegramChatID: '',
    weworkEnabled: false,
    weworkKey: '',
    feishuEnabled: false,
    feishuWebhook: '',
    feishuSecret: '',
    twitterEnabled: false,
    twitterApiKey: '',
    twitterApiSecret: '',
    twitterAccessToken: '',
    twitterAccessTokenSecret: '',
    customHttpEnabled: false,
    customHttpUrl: '',
    customHttpMethod: 'POST',
    customHttpHeaders: '',
    customHttpBody: '{"content":"{{content}}"}'
})

// 默认配置（用于重置）
const defaultConfig: NotifyConfig = {
    webhookEnabled: false,
    webhookURL: 'WebhookURL',
    telegramEnabled: false,
    telegramToken: 'bot_token',
    telegramChatID: 'chat_id',
    weworkEnabled: false,
    weworkKey: 'WebhookURL',
    feishuEnabled: false,
    feishuWebhook: 'FeishuWebhook',
    feishuSecret: 'secret',
    twitterEnabled: false,
    twitterApiKey: 'twitter_api_key',
    twitterApiSecret: 'twitter_api_secret',
    twitterAccessToken: 'twitter_access_token',
    twitterAccessTokenSecret: 'twitter_access_token_secret',
    customHttpEnabled: false,
    customHttpUrl: 'https://example.com/notify',
    customHttpMethod: 'POST',
    customHttpHeaders: '{"Authorization":"Bearer token"}',
    customHttpBody: '{"content":"{{content}}"}'
}

const notifyTypes: string[] = ['webhook', 'telegram', 'wework', 'feishu', 'twitter', 'customHttp']
const activeChannel = ref('telegram')

const getNotifyTypeName = (type: string) => {
    const names: Record<string, string> = {
        webhook: 'Webhook',
        telegram: 'Telegram',
        wework: '企业微信',
        feishu: '飞书',
        twitter: 'Twitter',
        customHttp: '自定义HTTP'
    }
    return names[type] || type
}

const channelEnabledFieldMap: Record<string, keyof NotifyConfig> = {
    webhook: 'webhookEnabled',
    telegram: 'telegramEnabled',
    wework: 'weworkEnabled',
    feishu: 'feishuEnabled',
    twitter: 'twitterEnabled',
    customHttp: 'customHttpEnabled'
}

const channelMeta = [
    { key: 'webhook', label: 'Webhook', icon: 'i-heroicons-globe-alt', description: '通用 HTTP 回调', tip: '适合对接自建服务、Webhook 平台或第三方自动化工具。' },
    { key: 'telegram', label: 'Telegram', icon: 'i-mdi-telegram', description: '机器人消息推送', tip: '填写机器人 Token 与目标 Chat ID 后即可推送消息。' },
    { key: 'wework', label: '企业微信', icon: 'i-mdi-wechat', description: '企业微信群机器人', tip: '推荐用于内部告警与运维通知，Webhook Key 即可完成接入。' },
    { key: 'feishu', label: '飞书', icon: 'i-mdi-alpha-f-circle', description: '飞书机器人消息', tip: '支持 Webhook 与签名密钥，适合团队通知场景。' },
    { key: 'twitter', label: 'Twitter', icon: 'i-mdi-twitter', description: '社交媒体同步', tip: '适合将消息同步到 Twitter/X 平台。' },
    { key: 'customHttp', label: '自定义 HTTP', icon: 'i-heroicons-code-bracket-square', description: '自定义请求模板', tip: '可自由定义方法、请求头与请求体，适合任意通知接口。' }
] as const

const maskValue = (value: string, start = 6, end = 4) => {
    const text = String(value || '').trim()
    if (!text) return '未配置'
    if (text.length <= start + end) return text
    return `${text.slice(0, start)}${'•'.repeat(Math.min(12, text.length - start - end))}${text.slice(-end)}`
}

const getChannelPreview = (type: string) => {
    switch (type) {
        case 'webhook':
            return localConfig.webhookURL || '未填写 Webhook 地址'
        case 'telegram':
            return localConfig.telegramChatID ? `Chat ID ${localConfig.telegramChatID}` : '等待填写 Bot Token 与 Chat ID'
        case 'wework':
            return localConfig.weworkKey ? `Key ${maskValue(localConfig.weworkKey, 4, 4)}` : '未填写企微 Webhook Key'
        case 'feishu':
            return localConfig.feishuWebhook ? maskValue(localConfig.feishuWebhook, 14, 10) : '未填写飞书 Webhook'
        case 'twitter':
            return localConfig.twitterApiKey ? `API Key ${maskValue(localConfig.twitterApiKey, 4, 4)}` : '未填写 Twitter 授权信息'
        case 'customHttp':
            return localConfig.customHttpUrl || '未填写自定义请求地址'
        default:
            return '未配置'
    }
}

const channels = computed(() => channelMeta.map((item) => ({
    ...item,
    enabled: !!localConfig[channelEnabledFieldMap[item.key]],
    preview: getChannelPreview(item.key)
})))

const currentChannel = computed(() => channels.value.find((item) => item.key === activeChannel.value) || channels.value[0])

const setChannelEnabled = (type: string, enabled: boolean) => {
    const field = channelEnabledFieldMap[type]
    if (!field) return
    localConfig[field] = enabled as never
}

const resetAll = () => {
    Object.assign(localConfig, defaultConfig)
    useToast().add({ title: '已恢复默认值', description: '请点击保存以应用更改', color: 'indigo' })
}

const saveAll = async () => {
    try {
        const baseApi = useRuntimeConfig().public.baseApi || '/api'
        const response = await fetch(`${baseApi}/notify/config`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            credentials: 'include',
            body: JSON.stringify(localConfig)
        });
        
        const data = await response.json();
        if (data.code === 1) {
             // 强制更新所有配置字段
            if (data.data) {
                Object.keys(localConfig).forEach((key: string) => {
                    if ((data.data as any)[key] === undefined) return
                    if (key.endsWith('Enabled')) {
                        (localConfig as any)[key] = !!(data.data as any)[key] && (data.data as any)[key] !== 'false'
                    } else {
                        (localConfig as any)[key] = (data.data as any)[key] || ''
                    }
                })
            }
            emit('save', {...localConfig});
            useToast().add({
                title: '成功',
                description: '推送配置已保存',
                color: 'green'
            });
        } else {
            throw new Error(data.msg || '保存失败');
        }
    } catch (error: any) {
        console.error('保存配置失败:', error);
        useToast().add({
            title: '错误',
            description: error.message || '保存失败',
            color: 'red'
        });
    }
}

const testNotify = async (type: string) => {
    try {
        // 简单验证
        if (type === 'webhook' && localConfig.webhookEnabled && !localConfig.webhookURL) throw new Error('Webhook URL不能为空');
        // 其他验证可按需添加

        const response = await fetch('/api/notify/test', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            credentials: 'include',
            body: JSON.stringify({ type })
        });
        
        const data = await response.json();
        if (data.code === 1) {
            useToast().add({
                title: '成功',
                description: `${getNotifyTypeName(type)}测试消息已发送`,
                color: 'green'
            });
        } else {
            throw new Error(data.msg || '测试失败');
        }
    } catch (error: any) {
        console.error('测试失败:', error);
        useToast().add({
            title: '错误',
            description: error.message || '测试失败',
            color: 'red'
        });
    }
};

watch(() => props.config, (newConfig) => {
    if (newConfig) {
        Object.keys(localConfig).forEach((key: string) => {
            if ((newConfig as any)[key] !== undefined) {
                if (key.endsWith('Enabled')) {
                    (localConfig as any)[key] = !!(newConfig as any)[key] && (newConfig as any)[key] !== 'false';
                } else {
                    (localConfig as any)[key] = (newConfig as any)[key];
                }
            }
        });
        const firstEnabled = channels.value.find((item) => item.enabled)?.key
        activeChannel.value = firstEnabled || activeChannel.value || 'telegram'
    }
}, { deep: true, immediate: true });

onMounted(async () => {
    try {
        const response = await fetch('/api/notify/config', {
            method: 'GET',
            credentials: 'include',
            headers: {
                'Content-Type': 'application/json'
            }
        });
        const data = await response.json();
        if (data.code === 1 && data.data) {
            const config = data.data;
            Object.keys(localConfig).forEach((key: string) => {
                if ((config as any)[key] === undefined) return
                if (key.endsWith('Enabled')) {
                    (localConfig as any)[key] = !!(config as any)[key] && (config as any)[key] !== 'false'
                } else {
                    (localConfig as any)[key] = (config as any)[key] || ''
                }
            })
        }
    } catch (error: any) {
        console.error('获取配置失败:', error);
    }
});

const subtleBg = computed(() => props.subtleBg || 'bg-gray-800')
const text = computed(() => props.text || 'text-white')
const mutedText = computed(() => props.mutedText || 'text-gray-300')
</script>

<style scoped>
.notify-panel-shell {
    display: flex;
    flex-direction: column;
    gap: 18px;
}

.notify-panel-toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    justify-content: space-between;
    gap: 14px;
}

.notify-channel-grid {
    display: grid;
    grid-template-columns: repeat(1, minmax(0, 1fr));
    gap: 14px;
}

.notify-channel-card {
    border: 1px solid rgba(148, 163, 184, 0.18);
    border-radius: 18px;
    padding: 18px;
    text-align: left;
    transition: transform .18s ease, box-shadow .18s ease, border-color .18s ease, background-color .18s ease;
    box-shadow: 0 10px 28px rgba(15, 23, 42, 0.06);
}

.notify-channel-card:hover {
    transform: translateY(-1px);
    border-color: rgba(79, 70, 229, 0.26);
    box-shadow: 0 14px 30px rgba(15, 23, 42, 0.1);
}

.notify-channel-card-active {
    background: linear-gradient(135deg, #0f172a 0%, #111827 100%) !important;
    border-color: rgba(15, 23, 42, 0.82);
    box-shadow: 0 18px 34px rgba(15, 23, 42, 0.2);
}

.notify-channel-card-active :deep(.u-icon),
.notify-channel-card-active :deep(.uicon),
.notify-channel-card-active :deep(svg) {
    color: #ffffff;
}

.notify-channel-card-active .notify-channel-badge-disabled {
    background: rgba(255, 255, 255, 0.12);
    color: rgba(255, 255, 255, 0.82);
}

.notify-channel-card-active .notify-channel-badge-enabled {
    background: rgba(74, 222, 128, 0.16);
    color: #dcfce7;
}

.notify-channel-card-active .notify-channel-icon {
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
}

.notify-channel-card-active .notify-channel-preview span,
.notify-channel-card-active .notify-channel-preview div,
.notify-channel-card-active .notify-channel-card-label,
.notify-channel-card-active .notify-channel-card-description {
    color: #f8fafc !important;
}

.notify-channel-icon {
    width: 42px;
    height: 42px;
    border-radius: 14px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: rgba(79, 70, 229, 0.08);
    color: #4f46e5;
}

.notify-channel-icon-active {
    background: rgba(79, 70, 229, 0.14);
    color: #4338ca;
}

.notify-channel-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 999px;
    padding: 4px 10px;
    font-size: 12px;
    font-weight: 600;
    white-space: nowrap;
}

.notify-channel-badge-enabled {
    background: #dcfce7;
    color: #166534;
}

.notify-channel-badge-disabled {
    background: #e2e8f0;
    color: #475569;
}

.notify-panel-detail {
    border: 1px solid rgba(148, 163, 184, 0.18);
    border-radius: 22px;
    padding: 22px;
    box-shadow: 0 14px 36px rgba(15, 23, 42, 0.08);
}

.notify-panel-detail-head {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    padding-bottom: 18px;
    margin-bottom: 18px;
    border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.notify-field-grid {
    display: grid;
    grid-template-columns: repeat(1, minmax(0, 1fr));
    gap: 16px;
}

.notify-field {
    display: flex;
    flex-direction: column;
    gap: 8px;
}

.notify-field-label {
    font-size: 14px;
    font-weight: 600;
}

.notify-field-tip {
    font-size: 12px;
    line-height: 1.5;
}

.notify-test-grid {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
}

@media (min-width: 768px) {
    .notify-channel-grid {
        grid-template-columns: repeat(3, minmax(0, 1fr));
    }

    .notify-field-grid {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
}
</style>
