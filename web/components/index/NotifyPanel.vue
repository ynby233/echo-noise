<template>
    <div class="rounded-lg p-4 mb-6">
        <div class="flex justify-between items-center mb-4">
            <h2 class="text-xl font-semibold" :class="text">推送渠道配置</h2>
            <div class="flex gap-2">
                <UButton color="gray" variant="soft" @click="resetAll">恢复默认</UButton>
                <UButton color="primary" @click="saveAll">保存配置</UButton>
            </div>
        </div>

        <!-- 配置列表 -->
        <div class="space-y-4" :class="props.disabled ? 'opacity-60 pointer-events-none' : ''">
            <!-- Webhook配置 -->
            <div class="rounded p-3" :class="subtleBg">
                <div class="flex justify-between items-center mb-2">
                    <span :class="mutedText">Webhook推送</span>
                    <div class="flex items-center gap-2">
                        <span class="text-sm" :class="localConfig.webhookEnabled ? 'text-green-500' : 'text-gray-500'">
                            {{ localConfig.webhookEnabled ? '已启用' : '已禁用' }}
                        </span>
                        <UToggle
                            :model-value="localConfig.webhookEnabled"
                            @update:model-value="localConfig.webhookEnabled = !!$event"
                        />
                    </div>
                </div>
                <div class="space-y-2 mt-2">
                    <UInput
                        v-model="localConfig.webhookURL"
                        placeholder="Webhook URL"
                    />
                </div>
            </div>

            <!-- Telegram配置 -->
            <div class="rounded p-3" :class="subtleBg">
                <div class="flex justify-between items-center mb-2">
                    <span :class="mutedText">Telegram推送</span>
                    <div class="flex items-center gap-2">
                        <span class="text-sm" :class="localConfig.telegramEnabled ? 'text-green-500' : 'text-gray-500'">
                            {{ localConfig.telegramEnabled ? '已启用' : '已禁用' }}
                        </span>
                        <UToggle
                            :model-value="localConfig.telegramEnabled"
                            @update:model-value="localConfig.telegramEnabled = !!$event"
                        />
                    </div>
                </div>
                <div class="space-y-2 mt-2">
                    <UInput
                        v-model="localConfig.telegramToken"
                        placeholder="Bot Token"
                    />
                    <UInput
                        v-model="localConfig.telegramChatID"
                        placeholder="Chat ID"
                    />
                </div>
            </div>

            <!-- 企业微信配置 -->
            <div class="rounded p-3" :class="subtleBg">
                <div class="flex justify-between items-center mb-2">
                    <span :class="mutedText">企业微信推送</span>
                    <div class="flex items-center gap-2">
                        <span class="text-sm" :class="localConfig.weworkEnabled ? 'text-green-500' : 'text-gray-500'">
                            {{ localConfig.weworkEnabled ? '已启用' : '已禁用' }}
                        </span>
                        <UToggle
                            :model-value="localConfig.weworkEnabled"
                            @update:model-value="localConfig.weworkEnabled = !!$event"
                        />
                    </div>
                </div>
                <div class="space-y-2 mt-2">
                    <UInput
                        v-model="localConfig.weworkKey"
                        placeholder="WebHook Key"
                    />
                </div>
            </div>

            <!-- 飞书配置 -->
            <div class="rounded p-3" :class="subtleBg">
                <div class="flex justify-between items-center mb-2">
                    <span :class="mutedText">飞书推送</span>
                    <div class="flex items-center gap-2">
                        <span class="text-sm" :class="localConfig.feishuEnabled ? 'text-green-500' : 'text-gray-500'">
                            {{ localConfig.feishuEnabled ? '已启用' : '已禁用' }}
                        </span>
                        <UToggle
                            :model-value="localConfig.feishuEnabled"
                            @update:model-value="localConfig.feishuEnabled = !!$event"
                        />
                    </div>
                </div>
                <div class="space-y-2 mt-2">
                    <UInput
                        v-model="localConfig.feishuWebhook"
                        placeholder="WebHook URL"
                    />
                    <UInput
                        v-model="localConfig.feishuSecret"
                        placeholder="签名密钥"
                        type="password"
                    />
                </div>
            </div>

            <!-- Twitter配置 -->
            <div class="rounded p-3" :class="subtleBg">
                <div class="flex justify-between items-center mb-2">
                    <span :class="mutedText">Twitter推送</span>
                    <div class="flex items-center gap-2">
                        <span class="text-sm" :class="localConfig.twitterEnabled ? 'text-green-500' : 'text-gray-500'">
                            {{ localConfig.twitterEnabled ? '已启用' : '已禁用' }}
                        </span>
                        <UToggle
                            :model-value="localConfig.twitterEnabled"
                            @update:model-value="localConfig.twitterEnabled = !!$event"
                        />
                    </div>
                </div>
                <div class="space-y-2 mt-2">
                    <UInput v-model="localConfig.twitterApiKey" placeholder="API Key" />
                    <UInput v-model="localConfig.twitterApiSecret" placeholder="API Secret" />
                    <UInput v-model="localConfig.twitterAccessToken" placeholder="Access Token" />
                    <UInput v-model="localConfig.twitterAccessTokenSecret" placeholder="Access Token Secret" />
                </div>
            </div>

            <!-- 自定义HTTP配置 -->
            <div class="rounded p-3" :class="subtleBg">
                <div class="flex justify-between items-center mb-2">
                    <span :class="mutedText">自定义HTTP推送</span>
                    <div class="flex items-center gap-2">
                        <span class="text-sm" :class="localConfig.customHttpEnabled ? 'text-green-500' : 'text-gray-500'">
                            {{ localConfig.customHttpEnabled ? '已启用' : '已禁用' }}
                        </span>
                        <UToggle
                            :model-value="localConfig.customHttpEnabled"
                            @update:model-value="localConfig.customHttpEnabled = !!$event"
                        />
                    </div>
                </div>
                <div class="space-y-2 mt-2">
                    <UInput v-model="localConfig.customHttpUrl" placeholder="请求URL" />
                    <USelect v-model="localConfig.customHttpMethod" :options="['POST', 'PUT', 'PATCH']" />
                    <UTextarea v-model="localConfig.customHttpHeaders" placeholder="请求Headers（JSON格式）" />
                    <UTextarea v-model="localConfig.customHttpBody" placeholder='请求Body模板（如：{"content":"{{content}}"})' />
                </div>
            </div>
        </div>

        <!-- 测试按钮区域 -->
        <div class="mt-4 flex flex-wrap gap-2" :class="props.disabled ? 'opacity-60 pointer-events-none' : ''">
            <UButton
                v-for="type in notifyTypes"
                :key="type"
                @click="testNotify(type)"
                class="flex-grow sm:flex-grow-0"
                variant="solid"
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
