export const MARKDOWN_BLANK_LINE_SENTINEL = '\u00a0'
export const MARKDOWN_PRESERVED_BLANK_LINE_CLASS = 'markdown-preserved-blank-line'

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

export const serializeMarkdownEditorBlocks = (blocks: string[]) => {
  let output = ''
  let plainBlocks: string[] = []
  const flushPlainBlocks = () => {
    if (!plainBlocks.length) return
    const value = plainBlocks.join('\n')
    output += `${output && !output.endsWith('\n') ? '\n' : ''}${value}`
    plainBlocks = []
  }
  const appendPreservedBlankLine = () => {
    flushPlainBlocks()
    output = output.replace(/\n+$/g, '')
    if (output) output += '\n\n'
    output += `${MARKDOWN_BLANK_LINE_SENTINEL}\n\n`
  }

  blocks.forEach((block) => {
    const value = String(block ?? '')
    if (value === MARKDOWN_BLANK_LINE_SENTINEL) appendPreservedBlankLine()
    else plainBlocks.push(value)
  })
  flushPlainBlocks()

  return output.replace(/^\n+|\n+$/g, '')
}

export const markMarkdownPreservedBlankLineElements = (root: ParentNode | null | undefined) => {
  if (!root || typeof root.querySelectorAll !== 'function') return
  root.querySelectorAll<HTMLElement>('p, div').forEach((element) => {
    if (element.closest('pre, code, table')) return
    // A container may contain an NBSP paragraph beside an empty audio player.
    // Only blank leaf blocks are sentinels; never replace their ancestor tree.
    if (Array.from(element.children).some((child) => child.tagName !== 'BR')) return
    const text = element.textContent || ''
    if (!text.includes(MARKDOWN_BLANK_LINE_SENTINEL) || !isMarkdownBlankLineSentinel(text)) return
    element.classList.add(MARKDOWN_PRESERVED_BLANK_LINE_CLASS)
    element.setAttribute('data-markdown-preserved-blank-line', 'true')
    element.textContent = MARKDOWN_BLANK_LINE_SENTINEL
  })
}

const MARKDOWN_ATTACHMENT_CARD_SELECTOR = '.site-attachment-file, .site-attachment-audio, .note-music-marker'

const attachmentParagraphLines = (container: Element): Node[][] => {
  const lines: Node[][] = [[]]
  for (const node of Array.from(container.childNodes)) {
    const element = node.nodeType === 1 ? node as Element : null
    if (element?.tagName === 'BR') {
      lines.push([])
    } else if (element && !element.matches(`${MARKDOWN_ATTACHMENT_CARD_SELECTOR}, code, pre, table`) && element.querySelector('br')) {
      const parts = attachmentParagraphLines(element)
      parts.forEach((part, index) => {
        if (index) lines.push([])
        const wrapper = element.cloneNode(false)
        part.forEach((child) => wrapper.appendChild(child))
        lines[lines.length - 1].push(wrapper)
      })
    } else {
      lines[lines.length - 1].push(node)
    }
  }
  return lines
}

// Run after link/media enhancement, before audio mounting. Keep each original
// paragraph as one block so a blank separator cannot migrate across a card.
export const normalizeMarkdownAttachmentParagraphs = (root: HTMLElement | null) => {
  if (!root) return
  root.querySelectorAll<HTMLParagraphElement>('p').forEach((paragraph) => {
    if (paragraph.closest('pre, code, table')) return
    if (!paragraph.querySelector(MARKDOWN_ATTACHMENT_CARD_SELECTOR)) return
    const block = document.createElement('div')
    for (const attribute of Array.from(paragraph.attributes)) {
      block.setAttribute(attribute.name, attribute.value)
    }
    block.classList.add('markdown-attachment-paragraph')
    for (const nodes of attachmentParagraphLines(paragraph)) {
      const line = document.createElement('div')
      line.className = 'markdown-attachment-line'
      nodes.forEach((node) => line.appendChild(node))
      // A card supplies its own line box; a BR between rows is not another row.
      if (line.querySelector(MARKDOWN_ATTACHMENT_CARD_SELECTOR)) {
        line.classList.add('markdown-attachment-line--card')
      }
      if (!line.children.length && /^[\t\r\n ]*$/.test(line.textContent || '')) {
        line.classList.add('markdown-attachment-line--blank')
      }
      block.appendChild(line)
    }
    paragraph.replaceWith(block)
  })
}
