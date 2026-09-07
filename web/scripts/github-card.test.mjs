import assert from 'node:assert/strict'
import { parseGitHubRepositoryLink } from '../utils/github-card.ts'

for (const href of [
  'https://github.com/AlkaidLab/foundation-sunshine',
  'https://github.com/AlkaidLab/foundation-sunshine/',
  'https://github.com/AlkaidLab/foundation-sunshine.git',
  'https://github.com/AlkaidLab/foundation-sunshine/issues/1?test=1#comment',
]) {
  assert.deepEqual(parseGitHubRepositoryLink(href), { owner: 'AlkaidLab', repo: 'foundation-sunshine', href })
}
for (const href of [
  'https://github.com/AlkaidLab', 'https://github.com/',
  'https://github.com.evil.example/owner/repo', 'javascript:alert(1)',
  'https://user:password@github.com/owner/repo', 'https://github.com:8443/owner/repo',
  'https://github.com/owner/%22%3E',
]) assert.equal(parseGitHubRepositoryLink(href), null, href)
console.log('GitHub repository link checks passed')
