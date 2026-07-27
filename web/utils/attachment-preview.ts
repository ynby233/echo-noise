// 这是“允许浏览器尝试打开”的格式表，不是浏览器能力检测。
// 服务端会对同一组安全类型返回 Content-Disposition:inline；是否使用内置阅读器、
// 调用系统应用或回退下载，最终由浏览器决定。主动内容（html/svg/xml）不得加入这里。
const BROWSER_PREVIEWABLE_ATTACHMENT_URL_RE = /\.(?:pdf|txt|text|csv|json)(?:[?#].*)?$/i

export const isBrowserPreviewableAttachmentUrl = (url: string) =>
  BROWSER_PREVIEWABLE_ATTACHMENT_URL_RE.test(String(url || ''))
