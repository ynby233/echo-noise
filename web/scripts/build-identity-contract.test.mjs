import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const root = new URL('../..', import.meta.url)
const read = (path) => readFile(new URL(path, root), 'utf8')
const [dockerfile, dockerWorkflow, releaseWorkflow, sidecar, androidSetup, routes, panel] = await Promise.all([
  read('Dockerfile'),
  read('.github/workflows/docker-publish.yml'),
  read('.github/workflows/tauri-release-sign.yml'),
  read('desktop/tauri/build-sidecar.sh'),
  read('packaging/android/setup_backend.sh'),
  read('internal/routers/routers.go'),
  read('web/components/index/StatusPanel.vue'),
])

const linkerTarget = 'github.com/rcy1314/echo-noise/internal/buildinfo.Identity'
assert.ok(dockerfile.includes(linkerTarget), 'Docker server binary must embed the build identity')
assert.match(dockerWorkflow, /formal_version[\s\S]*version="\$short_sha"/, 'Docker defaults to short SHA and accepts a formal version')
assert.match(dockerWorkflow, /echo-noise:\$\{version\}-mcp/, 'formal Docker builds must also publish a vX.Y.Z image tag')
assert.ok(sidecar.includes(linkerTarget), 'desktop sidecars must embed the build identity')
assert.ok(androidSetup.includes(linkerTarget), 'Android embedded backend must embed the build identity')
assert.match(releaseWorkflow, /formal_release[\s\S]*BUILD_ID="\$\{GITHUB_SHA::12\}"/, 'native workflow must resolve short SHA unless explicitly formal')
assert.match(releaseWorkflow, /BUILD_ID="v\$\{VER\}"/, 'formal native builds must use normalized vX.Y.Z identity')
assert.match(releaseWorkflow, /saynote-\$\{BUILD_ID\}\.apk/, 'Android artifact name must carry the build identity')
assert.match(releaseWorkflow, /saynote-\$\{BUILD_ID\}\.dmg/, 'macOS artifact name must carry the build identity')
assert.match(releaseWorkflow, /saynote-.*BUILD_ID.*\.exe/, 'Windows artifact name must carry the build identity')
assert.match(routes, /authRoutes\.GET\("\/version\/build"/, 'build identity endpoint must be authenticated')
assert.match(panel, /id="version-section"[\s\S]{0,1800}v-if="isPrimaryAdmin"[\s\S]{0,300}versionInfo\.buildIdentity/, 'the version section must show the identity only to the primary administrator')
assert.match(panel, /version\/build/, 'the admin UI must read the server build identity')

console.log('build identity contract passed')
