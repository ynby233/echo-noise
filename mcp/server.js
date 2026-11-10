import { pathToFileURL } from 'node:url'
import { closeServer } from './src/shutdown.js'
import { startFromEnv } from './src/transports.js'

export { startFromEnv } from './src/transports.js'

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const server = await startFromEnv()
    if (server) {
      let stopping = false
      const shutdown = async () => {
        if (stopping) return
        stopping = true
        const deadline = setTimeout(() => process.exit(1), 5000)
        deadline.unref()
        try {
          await closeServer(server)
          clearTimeout(deadline)
          process.exit(0)
        } catch {
          process.exit(1)
        }
      }
      process.once('SIGINT', shutdown)
      process.once('SIGTERM', shutdown)
    }
  } catch {
    // Configuration and upstream errors can contain credentials. Never print them.
    process.stderr.write('MCP startup failed; check transport configuration and AI credential setup.\n')
    process.exitCode = 1
  }
}
