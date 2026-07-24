import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const [source, cropper] = await Promise.all([
  readFile(new URL('../components/index/StatusPanel.vue', import.meta.url), 'utf8'),
  readFile(new URL('../components/admin/ImageCropperModal.vue', import.meta.url), 'utf8'),
])

assert.match(cropper, /role="alert"/, '裁切失败时应在弹窗内给出可访问的错误提示')
assert.match(cropper, /catch \(error: any\)/, '裁切异常不应只抛给控制台')

assert.match(source, /\{ key: 'site-ads', label: '广告'/, '后台导航应统一命名为“广告”')
assert.doesNotMatch(source, />左侧广告模块</, '广告面板不应继续使用旧标题')
assert.match(source, /v-model="ad\.textColor"\s+type="color"\s+class="admin-bg-color-input"/, '每条广告应复用头部图的原生颜色选择器')
assert.match(source, /v-model="ad\.textDisplayMode"[\s\S]*?悬浮时显示[\s\S]*?常驻显示/, '每条广告应提供文字显示方式')
assert.match(source, /@click="chooseAdImage\(i\)"/, '每条广告应能选择本地图片')
assert.match(source, /@click="previewImage\(resolveAdImageURL\(baseApi, ad\.imageURL\)\)"/, '每条广告应通过可移植 URL 预览当前图片')
assert.match(source, /<ImageCropperModal[\s\S]*?@confirm="uploadCroppedAdImage"/, '广告上传应经过共享裁切预览流程')
assert.match(source, /uploadCroppedAdImage[\s\S]*?saveConfigFields\(\{ leftAds: frontendConfig\.leftAds \}\)/, '裁切上传成功后应使用会抛错的保存链路持久化广告引用')
assert.match(source, /fetch\(`\$\{baseApi\}\/images\/upload`/, '广告图片上传应尊重反向代理 API 前缀')
assert.match(source, /normalizeAdConfigs/, '后台读取和保存广告时应复用共享规范化模块')

console.log('ad admin contract checks passed')
