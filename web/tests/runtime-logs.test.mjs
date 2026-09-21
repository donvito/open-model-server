import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

function harness() {
  const instances = [], timers = new Map(), statuses = []
  let timerId = 0, lines = []
  class EventSource {
    static CONNECTING = 0
    constructor(url) { this.url = url; this.readyState = 0; this.listeners = {}; instances.push(this) }
    addEventListener(event, listener) { this.listeners[event] = listener }
    close() { this.readyState = 2 }
    fail() { this.readyState = 2; this.onerror({}) }
    emit(event, payload) { this.listeners[event]({ data: JSON.stringify(payload) }) }
  }
  const exports = {}
  const source = ts.transpileModule(readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  runInNewContext(source, {
    exports, EventSource, localStorage: { getItem: () => 'test key' },
    setTimeout(fn, delay) { const id = ++timerId; timers.set(id, { fn, delay }); return id },
    clearTimeout(id) { timers.delete(id) },
  })
  const stop = exports.streamRuntimeLogs('llamacpp', {
    onStatus: status => statuses.push(status),
    onSnapshot: snapshot => { lines = snapshot },
    onLine: line => lines.push(line),
  })
  return { instances, timers, statuses, stop, get lines() { return lines },
    retry() { const [id, timer] = timers.entries().next().value; timers.delete(id); timer.fn(); return timer.delay },
  }
}

test('terminal HTTP failure reconnects, replaces snapshot, and rejects stale events', () => {
  const h = harness(), first = h.instances[0]
  assert.match(first.url, /api_key=test%20key/)
  first.emit('snapshot', { lines: [{ text: 'old' }] })
  first.fail()
  assert.equal(h.statuses.at(-1), 'reconnecting')
  assert.equal(h.retry(), 1000)
  const second = h.instances[1]
  second.onopen()
  second.emit('snapshot', { lines: [{ text: 'new' }] })
  first.emit('line', { text: 'stale' })
  second.emit('line', { text: 'live' })
  assert.equal(JSON.stringify(h.lines), JSON.stringify([{ text: 'new' }, { text: 'live' }]))
  h.stop()
  second.emit('line', { text: 'after cleanup' })
  assert.equal(h.lines.length, 2)
  assert.equal(second.readyState, 2)
})

test('retries back off to 30 seconds and cleanup cancels pending work', () => {
  const h = harness()
  for (const expected of [1000, 2000, 4000, 8000, 16000, 30000, 30000]) {
    h.instances.at(-1).fail()
    assert.equal(h.retry(), expected)
  }
  h.instances.at(-1).fail()
  h.stop()
  assert.equal(h.timers.size, 0)
  assert.equal(h.statuses.at(-1), 'closed')
})

test('native reconnect does not create a competing connection', () => {
  const h = harness()
  h.instances[0].onerror({})
  assert.equal(h.timers.size, 0)
  assert.equal(h.instances.length, 1)
  assert.equal(h.statuses.at(-1), 'reconnecting')
  h.stop()
})
