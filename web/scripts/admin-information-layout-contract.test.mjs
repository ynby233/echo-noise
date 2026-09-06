import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const component = await readFile(new URL('../components/index/StatusPanel.vue', import.meta.url), 'utf8')

assert.doesNotMatch(
  component,
  /\{ key: 'site-default-theme', label: '主题与布局'/,
  '主题与布局不应继续占用独立导航入口',
)
assert.match(
  component,
  /id="site-default-theme-section"[^>]+v-if="isSectionVisible\('site'\)"/,
  '主题与布局应合并到网站配置页面',
)
assert.match(
  component,
  /sectionKey === 'site-default-theme'[^\n]+sectionKey = 'site'/,
  '历史主题与布局 hash 应兼容跳转到网站配置',
)
for (const [key, label] of [['site-pwa', 'PWA 模式'], ['site-github-card', 'GitHub 卡片']]) {
  assert.doesNotMatch(component, new RegExp(`\\{ key: '${key}', label: '${label}'`), `${label}不应继续占用独立导航入口`)
  assert.match(component, new RegExp(`id="${key}-section"[^>]+v-if="isSectionVisible\\('site'\\)"`), `${label}应合并到网站配置页面`)
  assert.match(component, new RegExp(`sectionKey === '${key}'[^\\n]+sectionKey = 'site'`), `${label}历史 hash 应兼容跳转到网站配置`)
}
assert.match(component, /<UInput[^>]+v-model="frontendConfig.welcomeDescription"/, '一句话简介应使用单行输入框')
assert.match(component, /<UInput[^>]+v-model="frontendConfig.pwaDescription"/, 'PWA 简短描述应使用单行输入框')
assert.match(component, /v-else-if="String\(key\) === 'aboutMarkdown'"/, '仅正文 Markdown 配置应使用多行输入框')
assert.doesNotMatch(
  component,
  /v-else-if="[^"]*(?:subtitleText|commentPageDescription|announcementPageDescription|aboutPageDescription)[^"]*"[^>]*>\s*<UTextarea/,
  '欢迎语及留言、公告、关于页面说明应使用单行输入框',
)
assert.match(component, /subtitleText:\s*'显示在首页主标题下方，仅支持单行文本。'/, '欢迎语提示必须与首页单行展示行为一致')

assert.match(component, /class="admin-profile-grid"/, '用户信息应使用平衡的响应式工作台网格')
for (const modifier of ['username', 'avatar', 'description', 'password', 'token']) {
  assert.ok(component.includes(`admin-profile-card--${modifier}`), `用户信息网格缺少 ${modifier} 卡片位置`)
}
assert.equal(
  (component.match(/>API Token<\/div>/g) || []).length,
  1,
  '不同角色应共用同一份 Token 布局，不能复制两套模板',
)
assert.match(component, /\.admin-profile-card--token\s*\{[^}]*grid-column:\s*1 \/ -1/s, 'Token 卡片应在桌面网格中占据完整行')

assert.match(component, /class="site-config-grid"/, '站点信息应使用响应式卡片网格')
assert.match(component, /siteConfigCardClass\(String\(key\)\)/, '站点信息卡片应按内容长度决定跨度')
assert.match(component, /\.site-config-card--wide\s*\{[^}]*grid-column:\s*1 \/ -1/s, '长内容和头部图应占据完整行')
assert.match(component, /const canManageSiteSettings = computed\(\(\) => can\('site_settings\.manage'\)\)/, '站点配置编辑能力必须独立于查看能力')
assert.ok((component.match(/admin-readonly-settings/g) || []).length >= 4, '无管理权限时，网站配置与站点信息必须切换为只读表面')
assert.match(component, /\.admin-readonly-settings\s+:deep\(button\)[^}]+display:\s*none/s, '只读表面不得保留保存、重置或上传按钮')

console.log('admin information layout contract passed')
