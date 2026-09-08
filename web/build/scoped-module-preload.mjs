import { fileURLToPath } from 'node:url'

// Vite knows each import's dependencies. Scope feature errors before Nuxt can
// swallow/broadcast them; ordinary route imports retain Vite's implementation.
/** @returns {import('vite').Plugin} */
export const scopedModulePreload = () => ({
  name: 'site:scoped-module-preload',
  apply: 'build',
  enforce: 'post',
  transform(code, id) {
    if (id !== '\0vite/preload-helper.js') return
    const ast = this.parse(code)
    const declaration = ast.body.find(node => node.type === 'ExportNamedDeclaration'
      && node.declaration?.declarations?.some(item => item.id.name === '__vitePreload'))
      ?.declaration.declarations.find(item => item.id.name === '__vitePreload')
    const fn = declaration?.init
    if (fn?.type !== 'FunctionExpression' || fn.params.length !== 3) {
      this.error('Cannot attach feature loading to Vite preload helper; check the installed Vite implementation.')
    }
    const [load, deps, importer] = fn.params.map(param => param.name)
    const position = fn.body.start + 1
    return {
      code: `import { runScopedPreload as __sitePreload } from ${JSON.stringify(fileURLToPath(new URL('../utils/retryable-module.ts', import.meta.url)))};\n`
        + code.slice(0, position)
        + `\nconst owned = __sitePreload(${load}, (${deps} || []).map(dep => assetsURL(dep, ${importer})));\nif (owned) return owned;\n`
        + code.slice(position),
      map: null,
    }
  },
})
