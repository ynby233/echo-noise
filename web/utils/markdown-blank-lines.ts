export const MARKDOWN_BLANK_LINE_SENTINEL = '\u00a0'

export const isMarkdownBlankLineSentinel = (value: string) => {
  return String(value || '').replace(/[\u200b\u200c\ufeff]/g, '').replace(/\u00a0/g, ' ').trim() === ''
}

const encodeBlankLineRuns = (value: string) => {
  return value.replace(/\n{3,}/g, (match) => {
    const extraBlankLines = Math.max(1, match.length - 2)
    const sentinels = Array.from({ length: extraBlankLines }, () => MARKDOWN_BLANK_LINE_SENTINEL).join('\n\n')
    return `\n\n${sentinels}\n\n`
  })
}

const getFenceMarker = (line: string) => {
  const match = line.match(/^ {0,3}(`{3,}|~{3,})/)
  if (!match) return null
  return { char: match[1][0], length: match[1].length }
}

const closesFence = (line: string, fence: { char: string; length: number }) => {
  const match = line.match(/^ {0,3}(`{3,}|~{3,})[ \t]*$/)
  return !!match && match[1][0] === fence.char && match[1].length >= fence.length
}

export const encodeMarkdownExtraBlankLines = (value: string) => {
  const source = String(value || '').replace(/\r\n?/g, '\n')
  let result = ''
  let plainChunk = ''
  let activeFence: { char: string; length: number } | null = null
  const flushPlainChunk = () => {
    result += encodeBlankLineRuns(plainChunk)
    plainChunk = ''
  }
  for (const rawLine of source.match(/[^\n]*(?:\n|$)/g) || []) {
    if (!rawLine) continue
    const line = rawLine.endsWith('\n') ? rawLine.slice(0, -1) : rawLine
    if (activeFence) {
      if (closesFence(line, activeFence)) {
        result += line
        activeFence = null
        if (rawLine.endsWith('\n')) plainChunk += '\n'
        continue
      }
      result += rawLine
      continue
    }
    const fence = getFenceMarker(line)
    if (fence) {
      flushPlainChunk()
      result += rawLine
      activeFence = fence
      continue
    }
    plainChunk += rawLine
  }
  flushPlainChunk()
  return result
}
