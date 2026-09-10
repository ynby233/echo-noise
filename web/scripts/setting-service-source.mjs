import { readFile, readdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

// Structural settings contracts follow the responsibility-split service files
// instead of the retired setting_service.go path.

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))))

export const readSettingServiceSource = async () => {
  const names = (await readdir(join(repoRoot, 'internal/services')))
    .filter((name) => name.endsWith('_setting_service.go') || name.startsWith('frontend_config_') || name === 'login_expiry_service.go' || name === 'viewer_setting_service.go')
    .sort()
  return (await Promise.all(names.map((name) => readFile(join(repoRoot, 'internal/services', name), 'utf8')))).join('\n')
}
