import assert from 'node:assert/strict'
import http from 'node:http'
import test from 'node:test'
import { closeServer } from '../src/shutdown.js'

test('HTTP shutdown waits for an active response before resolving', { timeout: 5000 }, async () => {
  let finish
  let started
  const requestStarted = new Promise(resolve => { started = resolve })
  const server = http.createServer((req, res) => {
    finish = () => res.end('write-completed')
    started()
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  const response = new Promise((resolve, reject) => {
    const req = http.get(`http://127.0.0.1:${server.address().port}`, { agent: false }, res => {
      let body = ''
      res.on('data', chunk => { body += chunk })
      res.on('end', () => resolve(body))
    })
    req.on('error', reject)
  })
  await requestStarted
  let stopped = false
  const stopping = closeServer(server).then(() => { stopped = true })
  await new Promise(resolve => setImmediate(resolve))
  assert.equal(stopped, false, 'must not exit while an active write response is pending')
  finish()
  assert.equal(await response, 'write-completed')
  await stopping
  assert.equal(stopped, true)
})

test('stdio shutdown waits for its asynchronous transport close', async () => {
  let complete
  let closed = false
  const stopping = closeServer({ close: () => new Promise(resolve => { complete = () => { closed = true; resolve() } }) })
  assert.equal(closed, false)
  complete()
  await stopping
  assert.equal(closed, true)
})
