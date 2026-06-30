import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const homePage = readFileSync(join(root, 'pages/index.vue'), 'utf8')
const resolveNmpThemeBody = homePage.slice(
  homePage.indexOf('const resolveNmpTheme ='),
  homePage.indexOf('const applyNmpTheme =')
)

const assert = (condition, message) => {
  if (!condition) {
    console.error(message)
    process.exit(1)
  }
}

assert(
  homePage.includes('const resolveNmpTheme =') &&
    homePage.includes("if (contentTheme.value === 'dark') return 'dark'") &&
    homePage.includes("document.documentElement.classList.contains('dark')") &&
    homePage.includes("return 'light'") &&
    !resolveNmpThemeBody.includes('normalizeMusicTheme') &&
    !resolveNmpThemeBody.includes('musicTheme'),
  'music player theme must resolve from the site contentTheme/html.dark state, not from musicTheme or browser prefers-color-scheme'
)

assert(
  homePage.includes('const applyNmpTheme =') &&
    homePage.includes("target.setAttribute('data-theme', theme)") &&
    homePage.includes('instance?.setTheme?.(theme)') &&
    homePage.includes('instance.config.theme = theme'),
  'music player theme application must update the DOM attribute, live player instance, and cached player config'
)

assert(
  /watch\(\(\) => contentTheme\.value,[\s\S]*?applyNmpTheme\(\)[\s\S]*?scheduleMusicPlayerReconcile\('theme-change'\)[\s\S]*?\{ flush: 'sync' \}/.test(homePage),
  'contentTheme changes must synchronously apply the music player theme before the async reconcile path'
)

assert(
  homePage.includes("el.setAttribute('data-theme', resolveNmpTheme(cfg))") &&
    !homePage.includes("el.setAttribute('data-theme', normalizeMusicTheme(cfg.musicTheme))"),
  'music player attributes must write the resolved light/dark theme instead of leaving data-theme as auto'
)

assert(
  homePage.includes('applyNmpTheme(el, cfg, player)') &&
    homePage.includes('applyNmpTheme(el, nextCfg, player)') &&
    !homePage.includes("player.setTheme?.(theme === 'auto' ?"),
  'music player reconcile and theme observers must use the same immediate theme application path'
)

console.log('music player theme sync checks passed')
