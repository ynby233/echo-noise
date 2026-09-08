import assert from 'node:assert/strict'
import { readdir, readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const repoRoot = dirname(webRoot)

const read = (relativePath) => readFile(join(repoRoot, relativePath), 'utf8')
const sourceFiles = {
  nuxt: await read('web/nuxt.config.ts'),
  media: await read('web/utils/media-fancybox.ts'),
  mediaRuntime: await read('web/utils/media-fancybox-runtime.ts'),
  music: await read('web/utils/meting-player.ts'),
  messageList: await read('web/components/index/MessageList.vue'),
  globals: await read('web/types/globals.d.ts'),
  builtinComments: await read('web/components/comments/BuiltinComments.vue'),
  notificationCenter: await read('web/components/index/UserNotificationCenter.vue'),
  statusPanel: await read('web/components/index/StatusPanel.vue'),
  indexPage: await read('web/pages/index.vue'),
  settingsService: await read('internal/services/setting_service.go'),
  seedService: await read('internal/services/seed_service.go'),
  models: await read('internal/models/models.go'),
  widgetTemplate: await read('htmlwidgets/note.html'),
  widgetScript: await read('htmlwidgets/js/note.js'),
  readme: await read('README.md'),
}

const forbiddenRuntimePattern = /waline|@waline\/client|window\.Waline|loadWalineAssets|useWaline|commentServer|commentSystem|serverURL|waline-wrapper/i
for (const [name, source] of Object.entries(sourceFiles)) {
  assert.doesNotMatch(source, forbiddenRuntimePattern, `${name} must not retain third-party comment runtime or configuration residue`)
}

assert.match(sourceFiles.media, /import\('\.\/media-fancybox-runtime'\)/, 'media preview remains available through the shared lazy loader')
assert.match(sourceFiles.mediaRuntime, /from '@fancyapps\/ui'/, 'media runtime uses the installed viewer')
assert.match(sourceFiles.music, /aplayer.*[\s\S]*meting/i, 'music embeds retain their ordered runtime loader')
assert.match(sourceFiles.messageList, /<BuiltinComments\b/, 'message comments must keep using BuiltinComments')
assert.match(sourceFiles.builtinComments, /\.comment-wrapper\b/, 'builtin comments should use a neutral wrapper class')
assert.match(sourceFiles.notificationCenter, /\.comment-wrapper\b/, 'notification inline replies should use the neutral wrapper class')
assert.match(sourceFiles.readme, /仅支持内置评论/, 'README should state that comments are internal only')

const generatedHtml = []
const walk = async (directory) => {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) await walk(path)
    else if (entry.name.endsWith('.html')) generatedHtml.push(path)
  }
}
for (const outputRoot of [join(repoRoot, 'public'), join(webRoot, '.output', 'public')]) {
  await walk(outputRoot)
}
assert.ok(generatedHtml.length > 0, 'generated static output should be present for the residue check')
for (const path of generatedHtml) {
  const html = await readFile(path, 'utf8')
  assert.doesNotMatch(html, /waline|@waline\/client|window\.Waline/i, `${path} must not load third-party comment assets`)
}

console.log('third-party comment removal contract tests passed')
