import { Server as HttpServer } from 'node:http'

export function closeServer(server) {
  if (server instanceof HttpServer) {
    return new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
  }
  return server.close()
}
