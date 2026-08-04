import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const messageList = await readFile(new URL('../components/index/MessageList.vue', import.meta.url), 'utf8')
const messageStore = await readFile(new URL('../store/message.ts', import.meta.url), 'utf8')
const messageTypes = await readFile(new URL('../types/models.ts', import.meta.url), 'utf8')

assert.match(messageTypes, /personal_pinned\?: boolean/, 'messages must expose personal_pinned')
assert.match(messageTypes, /pinScope\?: ['"]latest['"]\s*\|\s*['"]personal['"]/, 'page queries must carry the pin scope')
assert.match(messageStore, /pinScope: query\.pinScope \?\? ['"]latest['"]/, 'list cache keys must distinguish latest and personal ordering')
assert.match(messageStore, /pinScope === ['"]personal['"] \? ['"]personal['"] : ['"]global['"]/, 'pin writes must choose an explicit scope endpoint')
assert.match(messageStore, /refresh|loadMessagePage/, 'pin writes must refresh authoritative server ordering')
assert.match(messageList, /pinScope.*isPersonalTab|isPersonalTab.*pinScope/, 'message list must send personal scope for the personal tab')
assert.match(messageList, /personal_pinned/, 'personal tab must read personal_pinned')
assert.match(messageList, /全站置顶|全站缃《|global/, 'latest controls must name the global pin scope')
assert.match(messageList, /个人置顶|涓汉缃《|personal/, 'personal controls must name the personal pin scope')
assert.doesNotMatch(messageList, /const pinnedTopItems|buildDisplayMessages =/, 'frontend must not splice a separate pinned list into paginated results')
