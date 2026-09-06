import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire, stripTypeScriptTypes } from 'node:module'
import vm from 'node:vm'

const source = readFileSync(new URL('../components/index/VditorEditor.vue', import.meta.url), 'utf8')
const dependency = readFileSync(createRequire(import.meta.url).resolve('vditor'), 'utf8')
const methodStart = dependency.indexOf('Vditor.prototype.setTheme =')
const methodEnd = dependency.indexOf('\n    };', methodStart) + '\n    };'.length
const watcherStart = source.indexOf('watch(() => props.theme,')
const watcherEnd = source.indexOf('\n});', watcherStart) + '\n});'.length
const afterStart = source.indexOf('    after: () => {', source.indexOf('onMounted(async () =>'))
const afterEnd = source.indexOf('      preReadyEditorInsertBuffer.drain', afterStart)
const themes = []
const values = []
let themeChanged
const state = {
  props: { theme: 'light', modelValue: '- [ ] draft' },
  isReady: { value: false },
  Vditor: function Vditor() {},
  setTheme: instance => themes.push(instance.options.theme),
  watch: (_getter, callback) => { themeChanged = callback },
  ensureSafeEditorTableMarkdown: value => value,
  encodeMarkdownExtraBlankLines: value => value,
}
const context = vm.createContext(state)
vm.runInContext(dependency.slice(methodStart, methodEnd), context)
// Vditor returns the outer object before its asynchronous i18n load calls init().
state.vditorInstance = new state.Vditor()
state.vditorInstance.setValue = value => values.push(value)
vm.runInContext(stripTypeScriptTypes(source.slice(watcherStart, watcherEnd)), context)
state.props.theme = 'dark'
assert.doesNotThrow(() => themeChanged('dark'), 'theme hydration before i18n init must not access the missing internal instance')
assert.deepEqual(themes, [], 'theme updates must wait for the ready callback')
state.vditorInstance.vditor = { options: {} }
vm.runInContext(stripTypeScriptTypes(source.slice(afterStart, afterEnd).replace('    after: () => {', '(() => {') + '})()'), context)
assert.deepEqual(themes, ['dark'], 'ready must apply the latest theme received during initialization')
assert.deepEqual(values, ['- [ ] draft'], 'theme synchronization must retain initial draft restoration')
assert.equal(state.isReady.value, true)
themeChanged('light')
assert.deepEqual(themes, ['dark', 'classic'], 'theme changes after ready must continue working')
state.vditorInstance = null
assert.doesNotThrow(() => themeChanged('dark'), 'a removed editor must not receive theme calls')
console.log('editor theme readiness tests passed')
