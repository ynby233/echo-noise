import { readFileSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'

// Vditor 3.10.9 compares nodeName case-sensitively, but fixTask passes "li".
// Correct that call so the library's own Enter/Backspace/undo behavior runs.
export const patchVditorTask = source => source.replace(
  /(var taskItemElement = [^\n]*hasClosestByMatchTag[^\n]*\(startContainer, )"li"(\);)/,
  '$1"LI"$2',
)

export const applyVditorTaskPatch = () => {
  const path = createRequire(import.meta.url).resolve('vditor')
  const source = readFileSync(path, 'utf8')
  const patched = patchVditorTask(source)
  if (patched !== source) writeFileSync(path, patched)
}

if (process.argv.includes('--apply')) applyVditorTaskPatch()
