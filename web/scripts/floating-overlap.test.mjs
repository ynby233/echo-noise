import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { stripTypeScriptTypes } from 'node:module'

const source = await readFile(fileURLToPath(new URL('../utils/floating-overlap.ts', import.meta.url)), 'utf8')
const rectanglesOverlap = Function(`${stripTypeScriptTypes(source)
  .replace('export const rectanglesOverlap', 'const rectanglesOverlap')}; return rectanglesOverlap`)()

const rect = (left, top, right, bottom) => ({ left, top, right, bottom })

assert.equal(rectanglesOverlap(rect(1082, 110, 1138, 616), rect(827, 24, 1107, 837)), true)
assert.equal(rectanglesOverlap(rect(1118, 110, 1174, 616), rect(827, 24, 1107, 837)), false)
assert.equal(rectanglesOverlap(rect(100, 100, 120, 120), rect(120, 100, 140, 120)), false)

console.log('floating overlap checks passed')
