import assert from 'node:assert/strict'
import { mkdtemp, mkdir, writeFile, readFile, chmod, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { spawnSync } from 'node:child_process'

const repo = resolve(import.meta.dirname, '../..')
const bash = process.env.BASH_PATH || (process.platform === 'win32' ? 'D:/Git/bin/bash.exe' : 'bash')
const temp = await mkdtemp(join(tmpdir(), 'mcp-module-isolation-'))
try {
  await mkdir(join(temp, 'bin'))
  await writeFile(join(temp, 'bin/node'), '#!/bin/sh\nprintf attempted > "$MCP_TEST_MARKER"\nexit 71\n')
  await chmod(join(temp, 'bin/node'), 0o755)
  const wrapper = join(repo, 'mcp/start-with-app.sh').replaceAll('\\', '/')
  const marker = join(temp, 'attempted').replaceAll('\\', '/')
  const quote = value => "'" + value.replaceAll("'", "'\\''") + "'"
  const shellPath = value => value.replaceAll('\\', '/').replace(/^([A-Za-z]):\//, (_, drive) => `/${drive.toLowerCase()}/`)
  for (const [name, enabled, transport, shouldStart] of [
    ['default stdio is on demand', 'true', 'stdio', false],
    ['disabled HTTP', 'false', 'http', false],
    ['numeric disabled HTTP', '0', 'http', false],
    ['failed HTTP addon', 'true', 'http', true],
  ]) {
    await rm(marker, { force: true })
    const command = `export PATH=${quote(shellPath(join(temp, 'bin')))}:"$PATH"\nsh ${quote(wrapper)} sh -c 'sleep 0.1; printf app-completed; exit 23'`
    const result = spawnSync(bash, ['-c', command], {
      encoding: 'utf8', timeout: 10000,
      env: { ...process.env, MCP_ENABLED: enabled, MCP_TRANSPORT: transport, MCP_TEST_MARKER: marker },
    })
    assert.equal(result.error, undefined, `${name}: ${result.error}`)
    assert.equal(result.status, 23, `${name}: app exit code must be preserved: ${result.stderr}`)
    assert.match(result.stdout, /app-completed/, name)
    const started = await readFile(marker, 'utf8').then(() => true, () => false)
    assert.equal(started, shouldStart, name)
  }
  const dockerfile = await readFile(join(repo, 'Dockerfile'), 'utf8')
  const final = dockerfile.split(/FROM .* AS final\r?\n/)[1]?.split(/^FROM /m)[0]
  assert.ok(final)
  assert.doesNotMatch(final, /mcp-build|nodejs|COPY .*mcp\//, 'plain final must not depend on MCP build/runtime')
  assert.match(dockerfile, /FROM final AS final-mcp[\s\S]*COPY --from=mcp-build[\s\S]*start-with-app\.sh/)
  const workflow = await readFile(join(repo, '.github/workflows/docker-publish.yml'), 'utf8')
  assert.match(workflow, /target: final-mcp/, 'published MCP channels must include the module')
  const compose = await readFile(join(repo, 'docker-compose.yml'), 'utf8')
  assert.match(compose, /target: \$\{APP_BUILD_TARGET:-final-mcp\}/)
  assert.doesNotMatch(compose, /depends_on:|^  mcp:/m, 'application must not require a separate MCP service')
} finally {
  await rm(temp, { recursive: true, force: true })
}
console.log('MCP optional module startup and packaging isolation passed')
