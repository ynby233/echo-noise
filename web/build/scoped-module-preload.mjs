import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { parse } from 'es-module-lexer/js'

const retryDirectory = '__retry__'
const relativeSpecifier = (from, to) => {
  const relative = path.posix.relative(path.posix.dirname(from), to)
  return relative.startsWith('.') ? relative : `./${relative}`
}
const resolveSpecifier = (from, specifier) => path.posix.normalize(path.posix.join(path.posix.dirname(from), specifier))
const retryName = file => path.posix.join(path.posix.dirname(file), retryDirectory, path.posix.basename(file))

const recoveryAssets = recoverableModules => ({
  name: 'site:module-recovery-assets',
  apply: 'build',
  generateBundle(_options, bundle) {
    const chunks = new Map(Object.values(bundle)
      .filter(item => item.type === 'chunk' && item.fileName.endsWith('.js'))
      .map(item => [item.fileName, item]))

    // Chunks in the initial entry graph contain Vue, Pinia and app singletons.
    // Recovery copies continue importing those original URLs. Only lazy graph
    // chunks get alternate URLs so a poisoned browser module record is avoided.
    const initial = new Set()
    const visitInitial = (file) => {
      if (initial.has(file)) return
      initial.add(file)
      const chunk = chunks.get(file)
      for (const dependency of chunk?.imports || []) {
        if (chunks.has(dependency)) visitInitial(dependency)
      }
    }
    for (const chunk of chunks.values()) {
      if (chunk.isEntry) visitInitial(chunk.fileName)
    }

    const recoverable = new Set()
    const visitRecoverable = (file) => {
      if (recoverable.has(file) || initial.has(file)) return
      recoverable.add(file)
      const chunk = chunks.get(file)
      for (const dependency of chunk?.imports || []) {
        if (chunks.has(dependency)) visitRecoverable(dependency)
      }
    }
    for (const chunk of chunks.values()) {
      if ([...Object.keys(chunk.modules)].some(id => recoverableModules.has(id.split('?')[0]))) {
        visitRecoverable(chunk.fileName)
      }
    }
    const copied = new Map([...recoverable].map(file => [file, retryName(file)]))

    for (const [file, destination] of copied) {
      const source = chunks.get(file).code
      const [imports] = parse(source)
      const edits = []
      for (const item of imports) {
        if (item.d === -2) {
          if (source.slice(item.e, item.e + 4) === '.url') {
            edits.push({
              start: item.s,
              end: item.e + 4,
              value: `new URL(${JSON.stringify(relativeSpecifier(destination, file))}, import.meta.url).href`,
            })
          }
          continue
        }
        if (!item.n || !item.n.startsWith('.')) continue
        const dependency = resolveSpecifier(file, item.n)
        if (!chunks.has(dependency)) continue
        const target = item.d === -1 && copied.has(dependency) ? copied.get(dependency) : dependency
        const specifier = relativeSpecifier(destination, target)
        if (item.d === -1) edits.push({ start: item.s, end: item.e, value: specifier })
        else edits.push({ start: item.s, end: item.e, value: JSON.stringify(specifier) })
      }
      let code = source
      for (const edit of edits.sort((a, b) => b.start - a.start)) {
        code = code.slice(0, edit.start) + edit.value + code.slice(edit.end)
      }
      this.emitFile({ type: 'asset', fileName: destination, source: code })
    }
  },
})

// Vite knows each import's dependencies. Scope feature errors before Nuxt can
// swallow/broadcast them; ordinary route imports retain Vite's implementation.
/** @returns {import('vite').Plugin[]} */
export const scopedModulePreload = () => {
  const recoverableModules = new Set()
  return [{
  name: 'site:scoped-module-preload',
  apply: 'build',
  enforce: 'post',
  async transform(code, id) {
    if (id !== '\0vite/preload-helper.js') {
      if (!/\b(?:asyncFeature|createRetryableModule)\s*\(/.test(code)) return
      const [imports] = parse(code)
      for (const item of imports) {
        if (item.d < 0 || !item.n) continue
        const resolved = await this.resolve(item.n, id, { skipSelf: true })
        if (resolved) recoverableModules.add(resolved.id.split('?')[0])
      }
      return
    }
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
  }, recoveryAssets(recoverableModules)]
}
