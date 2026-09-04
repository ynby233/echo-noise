import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { chmod, mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = new URL('../..', import.meta.url)
const rootPath = fileURLToPath(root)
const read = (path) => readFile(new URL(path, root), 'utf8')
const [dockerfile, healthcheck, routes] = await Promise.all([
  read('Dockerfile'),
  read('docker-healthcheck.sh'),
  read('internal/routers/routers.go'),
])

assert.match(routes, /GET\("\/api\/health\/live", controllers\.HealthLive\)/, 'router must expose a middleware-independent liveness probe')
assert.match(routes, /GET\("\/api\/health\/ready", controllers\.HealthReady\)/, 'router must expose a fail-fast database readiness probe')
assert.match(healthcheck, /runtime\.env/, 'healthcheck must read the persisted runtime environment')
assert.match(healthcheck, /config\.yaml/, 'healthcheck must read the persisted server port when HTTP_PORT is absent')
assert.match(healthcheck, /port="\$\{port:-1314\}"/, 'healthcheck must preserve the default HTTP port')
assert.match(healthcheck, /\/api\/health\/ready/, 'container health must reflect database readiness')
assert.match(dockerfile, /COPY \.\/docker-healthcheck\.sh \/app\/docker-healthcheck\.sh/, 'runtime image must include the healthcheck script')
assert.match(dockerfile, /HEALTHCHECK[\s\S]*\/app\/docker-healthcheck\.sh/, 'runtime image must declare its healthcheck')

const temp = await mkdtemp(join(tmpdir(), 'echo-noise-healthcheck-'))
try {
  const binDir = join(temp, 'bin')
  const capture = join(temp, 'wget-args.txt')
  await mkdir(binDir)
  const fakeWget = join(binDir, 'wget')
  await writeFile(fakeWget, '#!/bin/sh\nprintf "%s\\n" "$*" > "$WGET_CAPTURE"\n')
  await chmod(fakeWget, 0o755)

  const shell = process.platform === 'win32'
    ? resolve(execFileSync('git', ['--exec-path'], { encoding: 'utf8' }).trim(), '../../../usr/bin/sh.exe')
    : '/bin/sh'
  const shellPath = (value) => value
    .replaceAll('\\', '/')
    .replace(/^([A-Za-z]):\//, (_, drive) => `/${drive.toLowerCase()}/`)
  const commandPath = process.platform === 'win32'
    ? `${shellPath(binDir)}:${shellPath(dirname(shell))}`
    : `${binDir}:/usr/bin:/bin`

  const runProbe = async ({ configPort, environmentPort }) => {
    const configDir = join(temp, `config-${configPort ?? 'default'}-${environmentPort ?? 'none'}`)
    await mkdir(configDir)
    if (configPort) {
      await writeFile(join(configDir, 'config.yaml'), `server:\n  port: "${configPort}"\n  host: "0.0.0.0"\n`)
    }
    if (environmentPort) {
      await writeFile(join(configDir, 'runtime.env'), `HTTP_PORT=${environmentPort}\n`)
    }
    await rm(capture, { force: true })
    execFileSync(shell, [shellPath(join(rootPath, 'docker-healthcheck.sh'))], {
      env: {
        ...process.env,
        ECHO_NOISE_CONFIG_DIR: shellPath(configDir),
        PATH: commandPath,
        WGET_CAPTURE: shellPath(capture),
      },
      stdio: 'pipe',
    })
    return readFile(capture, 'utf8')
  }

  assert.match(
    await runProbe({ configPort: '27184' }),
    /http:\/\/127\.0\.0\.1:27184\/api\/health\/ready/,
    'healthcheck must use config.yaml server.port when runtime.env does not set HTTP_PORT',
  )
  assert.match(
    await runProbe({ configPort: '27184', environmentPort: '29000' }),
    /http:\/\/127\.0\.0\.1:29000\/api\/health\/ready/,
    'HTTP_PORT must override the persisted config.yaml port',
  )
  assert.match(
    await runProbe({}),
    /http:\/\/127\.0\.0\.1:1314\/api\/health\/ready/,
    'healthcheck must keep port 1314 as its final fallback',
  )
} finally {
  await rm(temp, { recursive: true, force: true })
}

console.log('docker healthcheck contract passed')
