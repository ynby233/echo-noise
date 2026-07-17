export const audioExtension = (type: string) => {
  if (type.includes('ogg')) return 'ogg'
  if (type.includes('mp4')) return 'm4a'
  return 'webm'
}

export const normalizeRecordingFileName = (value: unknown, type: string) => {
  const ext = audioExtension(type || 'audio/webm')
  let name = String(value || '')
    .trim()
    .replace(/[\\/:*?"<>|\u0000-\u001f]+/g, '-')
    .replace(/\s+/g, ' ')
    .replace(/[.\s]+$/g, '')
  if (!name) return ''

  const suffix = `.${ext}`
  if (name.toLowerCase().endsWith(suffix)) {
    name = name.slice(0, -suffix.length).replace(/[.\s]+$/g, '')
  }
  if (!name) return ''
  return `${name.slice(0, 119)}${suffix}`
}
