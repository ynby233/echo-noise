// 附件失败占位块的统一文案来源：笔记正文（MarkdownRenderer）与首页最新图集共用，
// 避免同一种提示在两处各写一份而逐渐漂移。视觉部分见 assets/css/attachment-failure.css。
export type AttachmentFailureKind = 'image' | 'video' | 'audio' | 'file'

export const deletedAttachmentText = (kind: AttachmentFailureKind) => {
  if (kind === 'image') return '该图片已被删除'
  if (kind === 'video') return '该视频已被删除'
  if (kind === 'audio') return '该音频已被删除'
  return '该文件已被删除'
}

export const attachmentFailureTitle = (kind: AttachmentFailureKind) => {
  if (kind === 'image') return '图片加载失败'
  if (kind === 'video') return '视频播放失败'
  if (kind === 'audio') return '音频无法播放'
  return '附件加载失败'
}

export const attachmentFailureDetail = (kind: AttachmentFailureKind, deleted: boolean) => {
  if (deleted) return deletedAttachmentText(kind)
  if (kind === 'image') return '图片可能已被删除或暂时无法访问'
  if (kind === 'video') return '视频可能已被删除或暂时无法访问'
  if (kind === 'audio') return '音频可能已被删除或暂时无法访问'
  return '文件可能已被删除或暂时无法访问'
}

export const attachmentFailureAriaLabel = (kind: AttachmentFailureKind, deleted = false) =>
  `${attachmentFailureTitle(kind)}：${attachmentFailureDetail(kind, deleted)}`