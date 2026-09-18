import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const workflow = await readFile(new URL('../../.github/workflows/docker-publish.yml', import.meta.url), 'utf8')

assert.match(workflow, /push:\s*\n\s*branches:\s*\[main\][\s\S]*paths-ignore:/, 'main pushes must build unless explicitly documentation-only')
assert.match(workflow, /release:\s*\n\s*types:\s*\[published\]/, 'published releases must drive stable builds')
assert.match(workflow, /workflow_dispatch:/, 'manual rebuilds must remain available')
assert.match(workflow, /concurrency:[\s\S]*queue:\s*max[\s\S]*cancel-in-progress:\s*false/, 'queued channel builds must not be cancelled by later pushes')
assert.match(workflow, /edge target must belong to main/, 'manual edge builds must reject commits outside main')
assert.match(workflow, /edge-mcp/, 'edge channel tag must be published')
assert.match(workflow, /stable-mcp/, 'stable channel tag must be published')
assert.doesNotMatch(workflow, /latest-mcp/, 'the retired latest-mcp tag must not be published')
assert.match(workflow, /channel-policy\.sh/, 'channel movement must reject rollback and divergence')
assert.match(workflow, /resolve-release\.sh/, 'release tags must resolve to their exact commit')
assert.match(workflow, /Checkout exact target source[\s\S]*ref:\s*\$\{\{ steps\.meta\.outputs\.revision \}\}[\s\S]*path:\s*source/, 'the image context must be the resolved target commit')
assert.match(workflow, /context:\s*source/, 'the build must use the exact target checkout')
assert.match(workflow, /build-args:[\s\S]*REVISION=\$\{\{ steps\.meta\.outputs\.revision \}\}/, 'images must receive the full revision')
assert.match(workflow, /build-args:[\s\S]*BUILD_TIME=\$\{\{ steps\.meta\.outputs\.build_time \}\}/, 'images must receive the build time')
assert.match(workflow, /sha-\$\{REVISION\}-mcp/, 'edge builds must retain a full-revision image tag')
assert.match(workflow, /CHANNEL" == stable[\s\S]*publish_tag "\$target" "\$CANDIDATE"/, 'stable must publish its own formal-version binary')

const smoke = workflow.indexOf('Smoke test candidate')
const promote = workflow.indexOf('Move channel tag')
assert.ok(smoke >= 0 && promote > smoke, 'the candidate must pass smoke testing before its channel moves')

console.log('Docker channel workflow contract passed')
