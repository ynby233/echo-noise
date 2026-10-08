import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import strictAssert from 'node:assert/strict'
import vm from 'node:vm'
import ts from 'typescript'

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

// Execute the page helpers so assertions cover their effect, not just spelling.
const applyNmpThemeStart = homePage.indexOf('const applyNmpTheme =')
const applyNmpThemeEnd = homePage.indexOf('const shouldShowMusicPlayer =', applyNmpThemeStart)
strictAssert.ok(applyNmpThemeStart >= 0 && applyNmpThemeEnd > applyNmpThemeStart)
const contentTheme = { value: 'light' }
let htmlDark = false
const attributes = new Map()
const applied = []
const player = { config: { theme: 'auto' }, setTheme: theme => applied.push(theme) }
const element = {
  getAttribute: name => attributes.get(name),
  setAttribute: (name, value) => attributes.set(name, value),
  neteasePlayer: player,
}
const sandbox = vm.createContext({
  contentTheme, frontendConfig: { value: { musicTheme: 'dark' } },
  document: {
    documentElement: { classList: { contains: name => name === 'dark' && htmlDark } },
    querySelector: () => element,
  },
})
const helpers = ts.transpileModule(`${resolveNmpThemeBody}\n${homePage.slice(applyNmpThemeStart, applyNmpThemeEnd)}\nglobalThis.helpers = { resolveNmpTheme, applyNmpTheme }`, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText
vm.runInContext(helpers, sandbox)
const { resolveNmpTheme, applyNmpTheme } = sandbox.helpers
strictAssert.equal(resolveNmpTheme({ musicTheme: 'dark' }), 'light')
applyNmpTheme(element, { musicTheme: 'dark' }, player)
strictAssert.equal(attributes.get('data-theme'), 'light')
strictAssert.equal(player.config.theme, 'light')
contentTheme.value = 'dark'
applyNmpTheme()
strictAssert.equal(attributes.get('data-theme'), 'dark')
strictAssert.equal(player.config.theme, 'dark')
contentTheme.value = 'light'
htmlDark = true
strictAssert.equal(resolveNmpTheme({ musicTheme: 'light' }), 'dark')
htmlDark = false
applyNmpTheme(element, { musicTheme: 'auto' }, player)
strictAssert.equal(attributes.get('data-theme'), 'light')
strictAssert.equal(player.config.theme, 'light')
strictAssert.deepEqual(applied, ['light', 'dark', 'light'])

console.log('music player theme sync checks passed')
