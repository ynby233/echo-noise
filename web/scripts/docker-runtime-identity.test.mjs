import assert from 'node:assert/strict'
import { chmod, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { spawnSync } from 'node:child_process'
import { join, relative, resolve } from 'node:path'
import { parse } from 'yaml'

const repo = resolve(import.meta.dirname, '../..')
const workflow = parse(await readFile(join(repo, '.github/workflows/docker-publish.yml'), 'utf8'))
const smoke = workflow.jobs['build-mcp-image'].steps.find(step => step.name === 'Smoke test candidate')?.run
assert.ok(smoke, 'candidate smoke step must exist')
const finalSmoke = workflow.jobs['build-mcp-image'].steps.find(step => step.name === 'Smoke test final immutable target')?.run
assert.ok(finalSmoke, 'the final selected artifact must be independently smoke tested')

const temp = await mkdtemp(join(repo, '.tmp-runtime-identity-'))
const bin = join(temp, 'bin')
await mkdir(bin)
const docker = join(bin, 'docker')
await writeFile(docker, `#!/bin/sh
case "$1" in
  pull|rm|logs) exit 0 ;;
  image)
    case "$*" in
      *json*) printf '%s\\n' '{"org.opencontainers.image.revision":"2222222222222222222222222222222222222222","org.opencontainers.image.version":"v2.0.0","org.opencontainers.image.created":"2026-09-18T08:00:00Z"}' ;;
      *) printf '%s\\n' '2222222222222222222222222222222222222222' ;;
    esac ;;
  run)
    case "$*" in
      *--entrypoint*) printf '%s\\n' "$FAKE_RUNTIME" ;;
      *) printf '%s\\n' fake-container ;;
    esac ;;
  inspect) printf '%s\\n' healthy ;;
  *) echo "unexpected docker call: $*" >&2; exit 2 ;;
esac
`)
await chmod(docker, 0o755)

try {
  const bash = process.env.BASH_PATH || (process.platform === 'win32' ? 'D:/Git/bin/bash.exe' : 'bash')
  const relativeBin = relative(repo, bin).replaceAll('\\', '/')
  for (const command of [smoke, finalSmoke]) {
    const script = `export PATH="$PWD/${relativeBin}:$PATH"\n${command}`
    for (const [name, runtime, accepted] of [
      ['mismatch', { identity: 'v1.0.0', version: 'v1.0.0', revision: '1111111111111111111111111111111111111111', built_at: '2026-09-01T08:00:00Z' }, false],
      ['missing', {}, false],
      ['valid', { identity: 'v2.0.0', version: 'v2.0.0', revision: '2222222222222222222222222222222222222222', built_at: '2026-09-18T08:00:00Z' }, true],
    ]) {
      const result = spawnSync(bash, ['-c', script], {
        cwd: repo,
        encoding: 'utf8',
        env: {
          ...process.env,
          CANDIDATE: 'example.invalid/echo-noise:candidate',
          EXPECTED_REVISION: '2222222222222222222222222222222222222222',
          EXPECTED_VERSION: 'v2.0.0',
          EXPECTED_BUILD_TIME: '2026-09-18T08:00:00Z',
          GITHUB_RUN_ID: '1',
          GITHUB_RUN_ATTEMPT: '1',
          FAKE_RUNTIME: JSON.stringify(runtime),
          TARGET_REF: 'example.invalid/echo-noise:existing-fixed-target',
          REVISION: '2222222222222222222222222222222222222222',
          VERSION: 'v2.0.0',
        },
      })
      if (accepted) assert.equal(result.status, 0, `${name}: ${result.stderr}`)
      else {
        assert.notEqual(result.status, 0, `smoke accepted ${name} runtime identity\nstdout: ${result.stdout}\nstderr: ${result.stderr}`)
        assert.match(result.stderr, /executable build identity do not match expected target/, 'failure must be identity validation, not a broken test harness')
      }
    }
  }
} finally {
  await rm(temp, { recursive: true, force: true })
}

console.log('Docker runtime identity smoke contract passed')
