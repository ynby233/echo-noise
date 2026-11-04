export interface CommonTagTrigger {
  start: number
  end: number
  query: string
}

// Account common tags use plain #tag spelling, without media metadata suffixes.
const tagPattern = /^[^\s#\p{P}\p{S}]+$/u

export const normalizeCommonTags = (tags: readonly string[]): string[] => {
  const result: string[] = []
  const seen = new Set<string>()
  for (const value of tags) {
    const tag = String(value || '').trim().replace(/^#+/, '')
    if (!tagPattern.test(tag) || seen.has(tag)) continue
    seen.add(tag)
    result.push(tag)
  }
  return result
}

export const findCommonTagTrigger = (text: string, caret = text.length): CommonTagTrigger | null => {
  const end = Math.max(0, Math.min(caret, text.length))
  const match = /#([^\s#]*)$/u.exec(text.slice(0, end))
  if (!match || (match[1] && !tagPattern.test(match[1]))) return null
  return { start: end - match[0].length, end, query: match[1] }
}

export const filterCommonTags = (tags: readonly string[], query: string): string[] => {
  const needle = query.toLocaleLowerCase()
  return tags.filter(tag => tag.toLocaleLowerCase().includes(needle))
}

export const commonTagInsertion = (tag: string, before: string, after: string): string => {
  const left = before && !/\s$/u.test(before) ? ' ' : ''
  const right = !after || !/^\s/u.test(after) ? ' ' : ''
  return `${left}#${tag}${right}`
}

export const replaceCommonTagRange = (value: string, start: number, end: number, tag: string) => {
  const from = Math.max(0, Math.min(start, value.length))
  const to = Math.max(from, Math.min(end, value.length))
  const before = value.slice(0, from)
  const after = value.slice(to)
  const text = commonTagInsertion(tag, before, after)
  return { value: before + text + after, caret: from + text.length, text, start: from, end: to }
}

export type CommonTagKeyAction = 'next' | 'previous' | 'select' | 'close' | null

export const commonTagKeyAction = (
  event: Pick<KeyboardEvent, 'key' | 'shiftKey' | 'ctrlKey' | 'metaKey' | 'altKey' | 'isComposing' | 'keyCode'>,
  open: boolean,
  inScope: boolean,
  composing = false
): CommonTagKeyAction => {
  if (!open || !inScope || composing || event.isComposing || event.keyCode === 229) return null
  if (event.key === 'Escape') return 'close'
  if (event.ctrlKey || event.metaKey || event.altKey) return null
  if (event.key === 'Tab') return event.shiftKey ? 'previous' : 'next'
  if (event.key === 'ArrowDown') return 'next'
  if (event.key === 'ArrowUp') return 'previous'
  if (event.key === 'Enter') return 'select'
  return null
}

export const moveCommonTagIndex = (index: number, count: number, direction: number): number => {
  if (!count) return -1
  return ((index + direction) % count + count) % count
}
