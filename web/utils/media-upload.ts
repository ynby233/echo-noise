type UploadKind = 'image' | 'video' | 'audio'

export type UploadMediaProgress = (percent: number, index: number, total: number) => void

export type UploadedMedia = {
  rawUrl: string
  url: string
  markdown: string
  file: File
}

type UploadMediaFilesOptions = {
  files: File[]
  kind: UploadKind
  baseApi: string
  token?: string
  onProgress?: UploadMediaProgress
}

const VIDEO_TYPES = ['video/mp4', 'video/webm', 'video/quicktime', 'video/x-msvideo']
const VIDEO_EXTENSIONS = ['.mp4', '.webm', '.mov', '.avi']
const AUDIO_TYPES = ['audio/webm', 'audio/ogg', 'audio/mpeg', 'audio/mp4', 'audio/wav', 'audio/x-wav']
const AUDIO_EXTENSIONS = ['.webm', '.ogg', '.mp3', '.m4a', '.wav']
const MAX_IMAGE_SIZE = 50 * 1024 * 1024
const MAX_VIDEO_SIZE = 1024 * 1024 * 1024
const MAX_AUDIO_SIZE = 200 * 1024 * 1024

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
  if (/^https?:\/\//i.test(raw)) return normalizeCloudObjectURL(raw)

  const path = raw.startsWith('/') ? raw : `/${raw}`
  const base = String(baseApi || '/api').replace(/\/$/, '') || '/api'

  if (/^https?:\/\//i.test(base)) {
    try {
      const parsed = new URL(base)
      const basePath = parsed.pathname.replace(/\/$/, '')
      const origin = `${parsed.protocol}//${parsed.host}`
      if (basePath && path.startsWith(`${basePath}/`)) return `${origin}${path}`
      if (path.startsWith('/api/')) return `${origin}${path}`
      return `${origin}${basePath}${path}`
    } catch {
      return `${base}${path}`
    }
  }

  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  if (base && path.startsWith(`${base}/`)) return `${origin}${path}`
  return `${origin}${base}${path}`
}

const ATTACHMENT_LABELS: Record<UploadKind, string> = {
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
  const label = ATTACHMENT_LABELS[kind]
  return `\n[${label}：${sanitizeAttachmentName(name, url)}](${url})\n`
}

export const createImageMarkdown = (url: string, name?: string): string => createAttachmentMarkdown('image', url, name)

export const createVideoMarkdown = (url: string, name?: string): string => createAttachmentMarkdown('video', url, name)

export const createAudioMarkdown = (url: string, name?: string): string => createAttachmentMarkdown('audio', url, name)

const fileExtension = (file: File) => {
  const index = file.name.lastIndexOf('.')
  return index >= 0 ? file.name.slice(index).toLowerCase() : ''
}

const baseMimeType = (type: string) => String(type || '').split(';')[0].trim().toLowerCase()

export const validateMediaFile = (file: File, kind: UploadKind) => {
  if (kind === 'image') {
    if (!file.type.startsWith('image/')) {
      throw new Error('仅支持图片文件')
    }
    if (file.size > MAX_IMAGE_SIZE) throw new Error('图片大小不能超过 50MB')
    return
  }

  const ext = fileExtension(file)
  if (kind === 'audio') {
    if (!AUDIO_TYPES.includes(baseMimeType(file.type)) || !AUDIO_EXTENSIONS.includes(ext)) {
      throw new Error('仅支持 WEBM、OGG、MP3、M4A、WAV 格式的音频')
    }
    if (file.size > MAX_AUDIO_SIZE) throw new Error('音频不能超过200MB')
    return
  }

  if (!VIDEO_TYPES.includes(baseMimeType(file.type)) || !VIDEO_EXTENSIONS.includes(ext)) {
    throw new Error('仅支持 MP4、WEBM、MOV、AVI 格式的视频')
  }
  if (file.size > MAX_VIDEO_SIZE) throw new Error('视频不能超过1024MB')
}

const endpointFor = (kind: UploadKind) => {
  if (kind === 'image') return '/images/upload'
  if (kind === 'audio') return '/audio/upload'
  return '/video/upload'
}
const fieldFor = (kind: UploadKind) => {
  if (kind === 'image') return 'image'
  if (kind === 'audio') return 'audio'
  return 'video'
}
const labelFor = (kind: UploadKind) => {
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

  selected.forEach((file) => validateMediaFile(file, kind))

  for (let index = 0; index < selected.length; index += 1) {
    const file = selected[index]
    const rawUrl = await uploadOneMediaFile(file, kind, baseApi, token, (percent) => {
      const aggregate = Math.round(((index + percent / 100) / total) * 99)
      onProgress?.(Math.max(1, Math.min(99, aggregate)), index + 1, total)
    })
    const url = resolveUploadedMediaUrl(rawUrl, baseApi)
    results.push({
      rawUrl,
      url,
      markdown: kind === 'image' ? createImageMarkdown(url, file.name) : (kind === 'audio' ? createAudioMarkdown(url, file.name) : createVideoMarkdown(url, file.name)),
      file,
    })
  }

  onProgress?.(100, total, total)
  return results
}
