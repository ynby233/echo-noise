import { readFileSync } from 'node:fs'

// Structural regressions follow the editor shell and the implementation it
// delegates to. Behavior tests import the DOM session itself.
export const readEditorSource = () => [
  '../components/index/VditorEditor.vue',
  '../utils/editor-dom-session.ts',
].map(path => readFileSync(new URL(path, import.meta.url), 'utf8')).join('\n')
