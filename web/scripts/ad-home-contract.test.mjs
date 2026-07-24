import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const [home, carousel] = await Promise.all([
  readFile(new URL('../pages/index.vue', import.meta.url), 'utf8'),
  readFile(new URL('../components/widgets/AdCarousel.vue', import.meta.url), 'utf8'),
])

assert.match(home, /<AdCarousel\s+:ads="leftAds"\s+:interval-ms="frontendConfig\.leftAdsIntervalMs"/, '桌面和移动广告应复用同一轮播模块')
assert.match(carousel, /normalizeAdConfigs/, '轮播应通过共享模块兼容旧广告配置')
assert.match(carousel, /resolveAdImageURL/, '广告图片应通过可移植媒体 URL 解析器加载')
assert.match(carousel, /aspect-ratio:\s*16\s*\/\s*9/, '广告应改为 16:9 以缩短首页左栏高度')
assert.match(carousel, /--ad-text-color/, '每条广告的文字颜色应传入渲染样式')
assert.match(carousel, /is-always/, '常驻模式应始终显示广告文字')
assert.match(carousel, /is-touch-revealed/, '无悬浮能力的移动端应支持先显示文字再访问链接')
assert.match(carousel, /aria-label="`切换到广告 \$\{index \+ 1\}`"/, '圆点应说明会切换到哪条广告')
assert.match(carousel, /:aria-pressed="index === adIndex"/, '圆点应暴露当前选中状态')
assert.match(carousel, /min-width:\s*32px;[\s\S]*min-height:\s*32px;/, '圆点应具有可触控的命中区域')

console.log('ad home contract checks passed')
