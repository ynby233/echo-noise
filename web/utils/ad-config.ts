import { resolveManagedAttachmentURL } from './media-url'

export type AdTextDisplayMode = 'hover' | 'always'

export type AdConfig = {
  imageURL: string
  linkURL: string
  description: string
  textColor: string
  textDisplayMode: AdTextDisplayMode
}

export type AdConfigInput = Partial<AdConfig> | null | undefined

const HEX_COLOR_PATTERN = /^#[0-9a-f]{6}$/i

export const normalizeAdConfig = (input: AdConfigInput): AdConfig => {
  const item = input && typeof input === 'object' ? input : {}
  const textColor = String(item.textColor || '').trim().toLowerCase()
  return {
    imageURL: String(item.imageURL || '').trim(),
    linkURL: String(item.linkURL || '').trim(),
    description: String(item.description || '').trim(),
    textColor: HEX_COLOR_PATTERN.test(textColor) ? textColor : '#ffffff',
    textDisplayMode: item.textDisplayMode === 'always' ? 'always' : 'hover',
  }
}

export const normalizeAdConfigs = (items: unknown): AdConfig[] => {
  if (!Array.isArray(items)) return []
  return items.map((item) => normalizeAdConfig(item as AdConfigInput)).filter((item) => item.imageURL !== '')
}

export const makeEmptyAdConfig = (): AdConfig => normalizeAdConfig({})

export const resolveAdImageURL = (baseApi: string, raw: string): string => resolveManagedAttachmentURL(baseApi, raw)
