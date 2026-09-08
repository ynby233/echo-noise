import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { readAdminPanelSource } from './admin-panel-source.mjs'

const component = await readAdminPanelSource()

assert.doesNotMatch(
  component,
  /\{ key: 'site-default-theme', label: '主题与布局'/,
  '主题与布局不应继续占用独立导航入口',
)
assert.match(
  component,
  /id="site-default-theme-section"/,
  '主题与布局应合并到网站配置页面',
)
assert.match(
  component,
  /\['site-default-theme', 'site-pwa', 'site-github-card'\]\.includes\(key\)[^\n]+key = 'site'/,
  '历史主题与布局 hash 应兼容跳转到网站配置',
)
for (const [key, label] of [['site-pwa', 'PWA 模式'], ['site-github-card', 'GitHub 卡片']]) {
  assert.doesNotMatch(component, new RegExp(`\\{ key: '${key}', label: '${label}'`), `${label}不应继续占用独立导航入口`)
  assert.match(component, new RegExp(`id="${key}-section"`), `${label}应合并到网站配置页面`)
  assert.match(component, /\['site-default-theme', 'site-pwa', 'site-github-card'\]\.includes\(key\)[^\n]+key = 'site'/, `${label}历史 hash 应兼容跳转到网站配置`)
}
assert.match(component, /<UInput[^>]+v-model="form.welcomeDescription"/, '一句话简介应使用单行输入框')
assert.match(component, /<UInput[^>]+v-model="form.pwaDescription"/, 'PWA 简短描述应使用单行输入框')
assert.match(component, /v-else-if="key === 'aboutMarkdown' \|\| key === 'pageFooterHTML'"/, '正文 Markdown 与页脚 HTML 配置应使用多行输入框')
assert.doesNotMatch(
  component,
  /v-else-if="[^"]*(?:subtitleText|commentPageDescription|announcementPageDescription|aboutPageDescription)[^"]*"[^>]*>\s*<UTextarea/,
  '欢迎语及留言、公告、关于页面说明应使用单行输入框',
)
assert.match(component, /subtitleText:\s*'[^']*'/, '欢迎语必须保留独立配置项')

assert.match(component, /class="admin-profile-grid"/, '用户信息应使用平衡的响应式工作台网格')
for (const modifier of ['username', 'avatar', 'description', 'password', 'token']) {
  assert.ok(component.includes(`admin-profile-card--${modifier}`), `用户信息网格缺少 ${modifier} 卡片位置`)
}
assert.equal(
  (component.match(/>API Token<\/div>/g) || []).length,
  1,
  '不同角色应共用同一份 Token 布局，不能复制两套模板',
)

assert.match(component, /class="site-config-grid\b/, '站点信息应使用响应式卡片网格')
assert.match(component, /:class="group\.wide \? 'site-config-card site-config-card--wide' : 'site-config-card'"/, '站点信息卡片应按内容长度决定跨度')
assert.match(component, /\.site-config-card--wide\s*\{[^}]*grid-column:\s*1 \/ -1/s, '长内容和头部图应占据完整行')
assert.match(component, /const canManage = computed\(\(\) => can\('site_settings\.manage'\)\)/, '站点配置编辑能力必须独立于查看能力')
assert.ok((component.match(/admin-readonly-settings/g) || []).length >= 4, '无管理权限时，网站配置与站点信息必须切换为只读表面')
assert.match(component, /\.admin-readonly-settings\s+button[^}]+display:\s*none/s, '只读表面不得保留保存、重置或上传按钮')

console.log('admin information layout contract passed')
