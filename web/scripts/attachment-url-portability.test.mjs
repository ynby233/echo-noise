import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const tmp = await mkdtemp(join(tmpdir(), 'echo-attachment-url-portability-'))
const mediaUrlBundle = join(tmp, 'media-url.mjs')
const mediaUploadBundle = join(tmp, 'media-upload.mjs')

await Promise.all([
  build({
    entryPoints: [join(webRoot, 'utils/media-url.ts')],
    outfile: mediaUrlBundle,
    bundle: true,
    format: 'esm',
    platform: 'node',
    target: 'node18',
  }),
  build({
    entryPoints: [join(webRoot, 'utils/media-upload.ts')],
    outfile: mediaUploadBundle,
    bundle: true,
    format: 'esm',
    platform: 'node',
    target: 'node18',
  }),
])

try {
  const mediaUrl = await import(pathToFileURL(mediaUrlBundle).href)
  const mediaUpload = await import(pathToFileURL(mediaUploadBundle).href)
  const renderer = await readFile(join(webRoot, 'components/index/MarkdownRenderer.vue'), 'utf8')

  globalThis.window = { location: { origin: 'http://192.168.220.218:27184' } }

  assert.equal(
    mediaUpload.resolveUploadedMediaUrl('/api/files/refs/logical-id/auth.json', '/api'),
    '/api/files/refs/logical-id/auth.json',
    'new uploads must persist a host-independent attachment URL instead of the current browser origin',
  )

  assert.equal(
    typeof mediaUrl.resolveManagedAttachmentURL,
    'function',
    'the shared media URL module must expose a managed-attachment portability resolver',
  )
  assert.equal(
    mediaUrl.resolveManagedAttachmentURL(
      '/api',
      'https://old.example.test:20714/api/files/refs/logical-id/auth.json?download=1#preview',
    ),
    '/api/files/refs/logical-id/auth.json?download=1#preview',
    'old absolute managed-attachment URLs must follow the current site origin after an IP, domain, or port change',
  )
  assert.equal(
    mediaUrl.resolveManagedAttachmentURL(
      'https://api.example.test/api',
      'https://old.example.test/api/audio/refs/logical-id/recording.webm',
    ),
    'https://api.example.test/api/audio/refs/logical-id/recording.webm',
    'an explicitly configured cross-origin API must remain the authority for managed attachments',
  )
  assert.equal(
    mediaUrl.resolveManagedAttachmentURL('/api', 'https://cdn.example.test/files/report.pdf'),
    'https://cdn.example.test/files/report.pdf',
    'ordinary external links must never be rewritten as local attachments',
  )

  assert.match(
    renderer,
    /import \{[^}]*resolveManagedAttachmentURL[^}]*\} from '~\/utils\/media-url'/,
    'the Markdown renderer must use the shared portability resolver for old note content',
  )
  assert.match(
    renderer,
    /const url = resolveManagedAttachmentURL\(String\(BASE_API[^\n]+rawUrl/,
    'attachment markers must be normalized before their href, media source, and failure probe are built',
  )
  assert.match(
    renderer,
    /const trustedAttachmentOrigins = new Set[\s\S]*?window\.location\.origin[\s\S]*?new URL\(String\(BASE_API[\s\S]*?trustedAttachmentOrigins\.has\(parsed\.origin\)/,
    'automatic deletion probes must be limited to the current site or its configured API origin',
  )
  assert.match(
    renderer,
    /method: 'HEAD'[\s\S]*?credentials: 'include'[\s\S]*?headers: \{ Range: 'bytes=0-0' \}[\s\S]*?credentials: 'include'/,
    'trusted attachment probes must carry the authenticated session for both HEAD and Range fallback requests',
  )

  // 受管附件的每个读取入口都必须走共享解析器。手写 startsWith('http') 会把历史
  // 绝对地址原样透传，换域名或换 IP 后 <img> 变成跨站请求，SameSite=Lax 会话
  // Cookie 被浏览器拦下，私有笔记的图片就整片变成失败占位块。
  const managedReaders = [
    ['pages/index.vue', 'the home gallery, avatars, and header backgrounds'],
    ['components/index/StatusPanel.vue', 'the admin media previews and background list'],
    ['components/admin/AttachmentManager.vue', 'the attachment manager previews and downloads'],
    ['components/index/MessageList.vue', 'the message images and avatars'],
    ['components/comments/BuiltinComments.vue', 'the comment avatars'],
    ['components/index/UserNotificationCenter.vue', 'the notification actor avatars'],
    ['components/index/InfoFeedList.vue', 'the information feed avatars'],
    ['components/index/MarkdownRenderer.vue', 'the note body media sources'],
  ]

  for (const [relativePath, description] of managedReaders) {
    const source = await readFile(join(webRoot, relativePath), 'utf8')
    assert.match(
      source,
      /import \{[^}]*resolveManagedAttachmentURL[^}]*\} from '~\/utils\/media-url'/,
      `${relativePath} must resolve managed attachments through the shared portability resolver for ${description}`,
    )
    assert.doesNotMatch(
      source,
      /startsWith\('http'\)\s*\?/,
      `${relativePath} must not hand-roll a startsWith('http') passthrough that pins ${description} to a stale host`,
    )
  }

  // 上传写入侧必须存站点无关的地址，否则脏数据会再次进入配置与笔记。
  const statusPanel = await readFile(join(webRoot, 'components/index/StatusPanel.vue'), 'utf8')
  assert.match(
    statusPanel,
    /import \{[^}]*resolveUploadedMediaUrl[^}]*\} from '~\/utils\/media-upload'/,
    'admin uploads must persist attachment URLs through the shared upload resolver',
  )
  assert.equal(
    (statusPanel.match(/resolveUploadedMediaUrl\(/g) || []).length >= 2,
    true,
    'both the site avatar upload and the header background upload must persist host-independent URLs',
  )

  // 背景图归一化在后台与前台各有一份实现，两份都要跟随当前 origin，否则预览与线上会各自失效。
  const homePage = await readFile(join(webRoot, 'pages/index.vue'), 'utf8')
  for (const [label, source] of [['pages/index.vue', homePage], ['components/index/StatusPanel.vue', statusPanel]]) {
    assert.match(
      source,
      /const normalizeHeaderBackground[\s\S]{0,400}?resolveManagedAttachmentURL\(/,
      `${label} must normalize header background URLs so they follow the current site origin`,
    )
  }

  // 外链必须保持原样：默认配置里就带着 picsum 这类第三方背景图。
  assert.equal(
    mediaUrl.resolveManagedAttachmentURL('/api', 'https://picsum.photos/1600/500'),
    'https://picsum.photos/1600/500',
    'external header backgrounds must never be rewritten as local attachments',
  )

  console.log('attachment URL portability checks passed')
} finally {
  delete globalThis.window
  await rm(tmp, { recursive: true, force: true })
}
