import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js'
import { z } from 'zod'
import { NoteClientError } from './client.js'

const id = z.union([z.string().regex(/^[1-9]\d*$/), z.number().int().positive().safe()])
const positive = z.number().int().positive().safe()
const pageSize = positive.max(100)
const visibility = z.enum(['public', 'users', 'contacts', 'private'])
const text = z.string().trim().min(1)
const format = z.enum(['text', 'json']).optional()
const media = text.refine(value => /^(https?:\/\/[^\s/]+|\/(?!\/))[^\s]*$/i.test(value), '图片或视频地址无效')
const pagination = {
  page: positive.optional(),
  page_number: positive.optional(),
  pageSize: pageSize.optional(),
  page_size: pageSize.optional(),
}
const author = { authorId: id.optional(), username: text.optional() }
const mutable = {
  content: z.string().min(1).optional(),
  private: z.boolean().optional().describe('兼容旧参数；显式 visibility 优先'),
  visibility: visibility.optional(),
  created_at: text.optional().describe('后端支持的发布时间；权限由后端检查'),
}

function invalid(message = '工具参数无效') {
  throw new NoteClientError('ARGUMENTS', message)
}

function alias(args, canonical, alternate, fallback) {
  if (args[canonical] !== undefined && args[alternate] !== undefined && args[canonical] !== args[alternate]) {
    invalid('同义参数的值冲突')
  }
  return args[canonical] ?? args[alternate] ?? fallback
}

function paging(args) {
  const { page_number, page_size, ...rest } = args
  return { ...rest, page: alias(args, 'page', 'page_number', 1), pageSize: alias(args, 'pageSize', 'page_size', 10) }
}

const markdownURL = url => url.replace(/[()<>]/g, char => `%${char.charCodeAt(0).toString(16).toUpperCase()}`)

function writing(args, publishing) {
  const { private: legacyPrivate, ...body } = args
  if (body.visibility === undefined) {
    if (legacyPrivate !== undefined) body.visibility = legacyPrivate ? 'private' : 'public'
    else if (publishing) body.visibility = 'public'
  }
  if (publishing) {
    const { type, image, images = [], imageURL, image_url, ...fields } = body
    const urls = [...new Set([image, ...images, imageURL, image_url].filter(Boolean))]
    const content = fields.content?.trim() || ''
    if (!content && !urls.length && !fields.video_url) invalid('必须提供正文、图片或视频')
    if (type === 'image' && !urls.length) invalid('图片类型必须提供图片')
    if (type === 'multipart' && !urls.length && !fields.video_url) invalid('混合类型必须提供图片或视频')
    if (urls.length > 1) {
      const markdown = urls.map(url => `![image](${markdownURL(url)})`).join('\n')
      fields.content = [content, markdown].filter(Boolean).join('\n\n')
    } else if (content) fields.content = content
    else delete fields.content
    if (urls.length) fields.image_url = image_url ?? urls[0]
    if (fields.video_url && !content.includes(fields.video_url)) {
      fields.content = [fields.content, `[video](${markdownURL(fields.video_url)})`].filter(Boolean).join('\n\n')
    }
    return fields
  }
  if (!publishing && Object.keys(body).every(key => key === 'id')) invalid('必须提供至少一个修改字段')
  return body
}

function readable(result, name, args) {
  const data = result?.data ?? result
  const items = Array.isArray(data) ? data : Array.isArray(data?.items) ? data.items : []
  if (!items.length) return '无匹配结果'
  const keyword = args.keyword ?? args.query ?? ''
  const header = name === 'page' ? '以下是分页内容：' : keyword ? `关于“${keyword}”的相关内容：` : '以下是最新内容：'
  const lines = items.map(item => {
    const content = String(item.content ?? '')
    const urls = [...new Set([item.image_url, item.image, item.imageURL, ...(Array.isArray(item.images) ? item.images : [])].filter(url => typeof url === 'string' && url && !content.includes(url)))]
    const body = [content, ...urls.map(url => `![image](${markdownURL(url)})`), item.video_url].filter(Boolean).join('\n')
    return `[${item.id ?? ''}] ${item.username ?? ''} ${item.created_at ?? item.createdAt ?? ''}\n\n${body}`
  })
  return `${header}\n\n${lines.join('\n\n')}\n\n共${items.length}条`
}

const definitions = [
  ['search', '搜索可见笔记；空关键词读取最新分页，#tag 读取标签；支持关键词和分页别名，默认 JSON，format=text 输出可读文本。',
    { keyword: z.string().trim().optional(), query: z.string().trim().optional(), ...pagination, ...author, format },
    args => { const { query, ...rest } = paging(args); return { ...rest, keyword: alias(args, 'keyword', 'query', '') } }],
  ['page', '分页读取笔记，默认第 1 页、每页 10 条，最多 100 条；携带当前身份鉴权。',
    { ...pagination, ...author, date: text.optional(), keyword: text.optional(), tag: text.optional(), pinScope: z.enum(['global', 'personal']).optional(), excludeId: id.optional(), format }, paging],
  ['message', '按明确 ID 读取一条可见笔记。', { id }],
  ['publish', '发布笔记；visibility 为 public/users/contacts/private，缺省 public，兼容 private 参数。',
    { ...mutable, content: z.string().optional(), type: z.enum(['text', 'markdown', 'md', 'image', 'multipart']).optional(), image: media.optional(), images: z.array(media).min(1).optional(), imageURL: media.optional(), image_url: media.optional(), video_url: media.optional(), notify: z.boolean().optional() }, args => writing(args, true)],
  ['update', '按 ID 修改笔记；不传 visibility/private 时保持原可见性，不传 content 时保持原内容。',
    { id, ...mutable }, args => writing(args, false)],
  ['delete', '仅按明确 ID 将笔记软删除到回收站；不接受关键词、不执行搜索、无其他删除回退。', { id }],
  ['pin', '设置或取消置顶。global 由后端检查管理员权限；personal 由后端检查本人所有权。',
    { id, pinned: z.boolean(), scope: z.enum(['global', 'personal']).optional() }],
  ['settings', '修改站点设置，使用 token 设置路由；由后端检查 AI 凭据及站点设置管理权限，拒绝时不回退到会话路由。',
    { allowRegistration: z.boolean().optional(), autoApproveRegistration: z.boolean().optional(), frontendSettings: z.record(z.unknown()).optional() },
    args => { if (!Object.keys(args).length) invalid('必须提供设置字段'); return args }],
  ['status', '读取服务状态。', {}],
  ['calendar', '读取当前身份可见笔记的日历统计，可按作者筛选。', { authorId: id.optional() }],
  ['config', '读取前端公开配置，递归移除敏感字段。', {}],
  ['login', '使用当前 AI 凭据及同账号用户名、密码续期；不切换账号、不轮换 token、不保存会话、不返回凭据。',
    { username: text, password: z.string().min(1) }],
  ['rss', '读取 RSS 原始文本；单独按文本处理，不按 JSON 解析。', {}],
  ['identity', '核验当前凭据确为 AI 身份，返回去敏感字段后的身份信息。', {}],
]

/** Return a fresh official SDK server bound to the supplied per-request client. */
export function createMcpServer(client) {
  const server = new McpServer({ name: 'ech0-noise-mcp', version: '0.2.0' })
  // SDK validation errors can contain attacker-controlled values or property names.
  server.createToolError = () => ({ isError: true, content: [{ type: 'text', text: '工具调用失败：参数无效或操作不可用' }] })
  for (const [name, description, shape, normalize = args => args] of definitions) {
    const strict = z.object(shape).strict()
    const tool = server.registerTool(name, { description, inputSchema: shape }, async args => {
      try {
        const parsed = strict.safeParse(args)
        if (!parsed.success) invalid()
        const { format: outputFormat, ...input } = parsed.data
        const normalized = normalize(input)
        const result = await client[name](normalized)
        const safe = client.sanitize(result, name === 'login' ? [parsed.data.password] : [])
        const output = outputFormat === 'text' ? readable(safe, name, normalized) : typeof safe === 'string' ? safe : JSON.stringify(safe ?? null)
        return { content: [{ type: 'text', text: output }] }
      } catch (error) {
        return {
          isError: true,
          content: [{ type: 'text', text: error instanceof NoteClientError ? error.message : '工具调用失败' }],
        }
      }
    })
    // SDK 1.23 converts a raw shape to a stripping object. Install strictness on
    // the returned RegisteredTool BEFORE SDK parsing, not only in the callback.
    tool.inputSchema = strict
  }
  return server
}
