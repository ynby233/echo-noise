import { resolveManagedAttachmentURL } from './media-url'

export type UploadKind = 'image' | 'video' | 'audio' | 'file'
type UploadRequestKind = UploadKind | 'auto'

export type UploadMediaProgress = (percent: number, index: number, total: number) => void

export type UploadedMedia = {
  rawUrl: string
  url: string
  markdown: string
  file: File
}

type UploadMediaFilesOptions = {
  files: File[]
  kind: UploadRequestKind
  baseApi: string
  token?: string
  onProgress?: UploadMediaProgress
}

const VIDEO_TYPES = ['video/mp4', 'video/webm', 'video/quicktime', 'video/x-msvideo']
const VIDEO_EXTENSIONS = ['.mp4', '.webm', '.mov', '.avi']
const AUDIO_TYPES = ['audio/webm', 'audio/ogg', 'audio/mpeg', 'audio/mp4', 'audio/wav', 'audio/x-wav', 'audio/flac', 'audio/x-flac']
const AUDIO_EXTENSIONS = ['.webm', '.ogg', '.mp3', '.m4a', '.wav', '.flac']
const RECORDING_NAME_RE = /录音|(^|[-_\s.])(recording|voice|memo|capture)([-_\s.]|$)/i
const MAX_ATTACHMENT_SIZE = 5 * 1024 * 1024 * 1024
const MAX_IMAGE_SIZE = MAX_ATTACHMENT_SIZE
const MAX_VIDEO_SIZE = MAX_ATTACHMENT_SIZE
const MAX_AUDIO_SIZE = MAX_ATTACHMENT_SIZE

export const normalizeCloudObjectURL = (url: string): string => {
  const raw = String(url || '')
  if (!/^https?:\/\//.test(raw)) return raw
  try {
    const parsed = new URL(raw)
    const parts = parsed.pathname.split('/').filter(Boolean)
    if (parts[0] === 'note') {
      parsed.pathname = '/' + parts.slice(1).join('/')
      return parsed.toString()
    }
    return raw
  } catch {
    return raw.replace('/note/', '/')
  }
}

export const resolveUploadedMediaUrl = (url: string, baseApi = '/api'): string => {
  const raw = String(url || '').trim()
  if (!raw) return ''
  return resolveManagedAttachmentURL(baseApi, normalizeCloudObjectURL(raw))
}

const ATTACHMENT_LABELS: Record<Exclude<UploadKind, 'file'>, string> = {
  image: '图片附件',
  video: '视频附件',
  audio: '音频附件',
}

const sanitizeAttachmentName = (name?: string, url?: string) => {
  const fallback = (() => {
    try {
      const parsed = new URL(String(url || ''), typeof window !== 'undefined' ? window.location.origin : 'http://local')
      const last = decodeURIComponent(parsed.pathname.split('/').filter(Boolean).pop() || '')
      return last
    } catch {
      return String(url || '').split('/').filter(Boolean).pop() || ''
    }
  })()
  const clean = String(name || fallback || '').replace(/[\r\n\[\]]+/g, ' ').trim()
  return clean || '未命名附件'
}

const createAttachmentMarkdown = (kind: UploadKind, url: string, name?: string): string => {
  const label = kind === 'file' ? '文件附件' : ATTACHMENT_LABELS[kind]
  return `\n[${label}：${sanitizeAttachmentName(name, url)}](${url})\n`
}

export const createImageMarkdown = (url: string, name?: string): string => createAttachmentMarkdown('image', url, name)

export const createVideoMarkdown = (url: string, name?: string): string => createAttachmentMarkdown('video', url, name)

export const createAudioMarkdown = (url: string, name?: string): string => createAttachmentMarkdown('audio', url, name)

export const createFileMarkdown = (url: string, name?: string): string => createAttachmentMarkdown('file', url, name)

const fileExtension = (file: File) => {
  const index = file.name.lastIndexOf('.')
  return index >= 0 ? file.name.slice(index).toLowerCase() : ''
}

const baseMimeType = (type: string) => String(type || '').split(';')[0].trim().toLowerCase()

export const detectUploadKind = (file: File): UploadKind => {
  const ext = fileExtension(file)
  const mime = baseMimeType(file.type)
  if (mime.startsWith('image/')) return 'image'
  if (mime.startsWith('audio/') || AUDIO_TYPES.includes(mime)) return 'audio'
  if (mime.startsWith('video/') || VIDEO_TYPES.includes(mime)) return 'video'
  if (AUDIO_EXTENSIONS.includes(ext) && (ext !== '.webm' || RECORDING_NAME_RE.test(file.name))) return 'audio'
  if (VIDEO_EXTENSIONS.includes(ext)) return 'video'
  return 'file'
}

export const validateMediaFile = (file: File, kind: UploadKind) => {
  if (file.size > MAX_ATTACHMENT_SIZE) throw new Error('附件大小不能超过 5GB')
  if (kind === 'file') return

  if (kind === 'image') {
    if (!file.type.startsWith('image/')) {
      throw new Error('仅支持图片文件')
    }
    if (file.size > MAX_IMAGE_SIZE) throw new Error('图片大小不能超过 5GB')
    return
  }

  const ext = fileExtension(file)
  if (kind === 'audio') {
    if (!AUDIO_TYPES.includes(baseMimeType(file.type)) && !AUDIO_EXTENSIONS.includes(ext)) {
      throw new Error('仅支持 WEBM、OGG、MP3、M4A、WAV、FLAC 格式的音频')
    }
    if (file.size > MAX_AUDIO_SIZE) throw new Error('音频不能超过 5GB')
    return
  }

  if (!VIDEO_TYPES.includes(baseMimeType(file.type)) && !VIDEO_EXTENSIONS.includes(ext)) {
    throw new Error('仅支持 MP4、WEBM、MOV、AVI 格式的视频')
  }
  if (file.size > MAX_VIDEO_SIZE) throw new Error('视频不能超过 5GB')
}

const endpointFor = (kind: UploadKind) => {
  if (kind === 'image') return '/images/upload'
  if (kind === 'audio') return '/audio/upload'
  if (kind === 'file') return '/attachments/upload'
  return '/video/upload'
}
const fieldFor = (kind: UploadKind) => {
  if (kind === 'image') return 'image'
  if (kind === 'audio') return 'audio'
  if (kind === 'file') return 'file'
  return 'video'
}
const labelFor = (kind: UploadKind) => {
  if (kind === 'file') return '附件'
  if (kind === 'image') return '图片'
  if (kind === 'audio') return '音频'
  return '视频'
}

const uploadOneMediaFile = async (
  file: File,
  kind: UploadKind,
  baseApi: string,
  token: string,
  onProgress?: (percent: number) => void
): Promise<string> => {
  validateMediaFile(file, kind)
  const formData = new FormData()
  formData.append(fieldFor(kind), file)

  return await new Promise<string>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    const cleanBaseApi = String(baseApi || '/api').replace(/\/$/, '')
    xhr.open('POST', `${cleanBaseApi}${endpointFor(kind)}`, true)
    xhr.withCredentials = true
    if (kind === 'video') xhr.timeout = 0
    if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    onProgress?.(1)

    xhr.upload.onprogress = (event) => {
      if (!event.lengthComputable) return
      const percent = Math.round((event.loaded / event.total) * 100)
      onProgress?.(Math.max(1, Math.min(99, percent)))
    }

    xhr.onload = () => {
      try {
        const data = JSON.parse(xhr.responseText || '{}')
        if (xhr.status >= 200 && xhr.status < 300 && data?.code === 1 && data?.data) {
          resolve(String(data.data))
        } else {
          reject(new Error(data?.msg || `${labelFor(kind)}上传失败`))
        }
      } catch (error: any) {
        reject(new Error(error?.message || `${labelFor(kind)}上传失败`))
      }
    }

    xhr.onerror = () => reject(new Error(`${labelFor(kind)}上传失败`))
    xhr.ontimeout = () => reject(new Error('上传耗时较长，请稍后确认是否已上传成功'))
    xhr.send(formData)
  })
}

export const uploadMediaFiles = async ({ files, kind, baseApi, token = '', onProgress }: UploadMediaFilesOptions): Promise<UploadedMedia[]> => {
  const selected = files.filter(Boolean)
  const total = selected.length
  const results: UploadedMedia[] = []
  if (!total) return results

  selected.forEach((file) => validateMediaFile(file, kind === 'auto' ? detectUploadKind(file) : kind))

  for (let index = 0; index < selected.length; index += 1) {
    const file = selected[index]
    const resolvedKind = kind === 'auto' ? detectUploadKind(file) : kind
    const rawUrl = await uploadOneMediaFile(file, resolvedKind, baseApi, token, (percent) => {
      const aggregate = Math.round(((index + percent / 100) / total) * 99)
      onProgress?.(Math.max(1, Math.min(99, aggregate)), index + 1, total)
    })
    const url = resolveUploadedMediaUrl(rawUrl, baseApi)
    results.push({
      rawUrl,
      url,
      markdown: resolvedKind === 'image' ? createImageMarkdown(url, file.name) : (resolvedKind === 'audio' ? createAudioMarkdown(url, file.name) : (resolvedKind === 'file' ? createFileMarkdown(url, file.name) : createVideoMarkdown(url, file.name))),
      file,
    })
  }

  onProgress?.(100, total, total)
  return results
}
