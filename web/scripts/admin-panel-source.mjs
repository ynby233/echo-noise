import { readdir, readFile } from 'node:fs/promises'
import { readdirSync, readFileSync } from 'node:fs'

const webRoot = new URL('../', import.meta.url)

export const readAdminPanelSource = async () => {
  const sectionsDirectory = new URL('components/admin/sections/', webRoot)
  const sectionFiles = (await readdir(sectionsDirectory)).filter(name => name.endsWith('.vue') || name.endsWith('.ts')).sort()
  const sources = await Promise.all([
    readFile(new URL('components/index/StatusPanel.vue', webRoot), 'utf8'),
    readFile(new URL('assets/css/admin-sections.css', webRoot), 'utf8'),
    ...sectionFiles.map(name => readFile(new URL(name, sectionsDirectory), 'utf8')),
  ])
  return sources.join('\n')
}

export const readAdminPanelSourceSync = () => {
  const sectionsDirectory = new URL('components/admin/sections/', webRoot)
  const sectionFiles = readdirSync(sectionsDirectory).filter(name => name.endsWith('.vue') || name.endsWith('.ts')).sort()
  return [
    readFileSync(new URL('components/index/StatusPanel.vue', webRoot), 'utf8'),
    readFileSync(new URL('assets/css/admin-sections.css', webRoot), 'utf8'),
    ...sectionFiles.map(name => readFileSync(new URL(name, sectionsDirectory), 'utf8')),
  ].join('\n')
}
