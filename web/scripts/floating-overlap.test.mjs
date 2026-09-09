import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { stripTypeScriptTypes } from 'node:module'

const source = await readFile(fileURLToPath(new URL('../utils/floating-overlap.ts', import.meta.url)), 'utf8')
const strippedSource = stripTypeScriptTypes(source).replaceAll('export const ', 'const ')
const { rectanglesOverlap, getConcealedVisibleWidth } = Function(
  `${strippedSource}; return { rectanglesOverlap, getConcealedVisibleWidth }`,
)()

const rect = (left, top, right, bottom) => ({ left, top, right, bottom })

assert.equal(rectanglesOverlap(rect(1082, 110, 1138, 616), rect(827, 24, 1107, 837)), true)
assert.equal(rectanglesOverlap(rect(1118, 110, 1174, 616), rect(827, 24, 1107, 837)), false)
assert.equal(rectanglesOverlap(rect(100, 100, 120, 120), rect(120, 100, 140, 120)), false)
assert.equal(getConcealedVisibleWidth(1180, 1132, 20, 8), 20)
assert.equal(getConcealedVisibleWidth(1180, 1160, 20, 8), 12)
assert.equal(getConcealedVisibleWidth(1180, 1176, 20, 8), 0)

console.log('floating overlap checks passed')
