import assert from 'node:assert/strict'
import { test } from 'node:test'
import { getChatSession, getLastPlaygroundModel, rememberPlaygroundModel } from './playgroundSessions.ts'

test('generation continues without a mounted subscriber and is visible on return', async () => {
  const session = getChatSession('navigation-test')
  const controller = new AbortController()
  session.abort.current = controller
  session.set('streaming', true)
  session.set('messages', [{ role: 'assistant', content: '' }])
  const unsubscribe = session.subscribe(() => {})
  unsubscribe() // navigate away
  await Promise.resolve()
  session.set('messages', (messages) => [{ ...messages[0], content: 'Generated while away' }])
  assert.equal(controller.signal.aborted, false)
  const returned = getChatSession('navigation-test')
  assert.equal(returned.getSnapshot().messages[0].content, 'Generated while away')
  assert.equal(returned.getSnapshot().streaming, true)
  let updates = 0
  const cleanup = returned.subscribe(() => { updates++ })
  session.set('streaming', false)
  assert.equal(updates, 1)
  assert.equal(returned.getSnapshot().streaming, false)
  cleanup()
})

test('drafts, attachments and settings survive navigation and models remain isolated', () => {
  const first = getChatSession('first-model')
  const second = getChatSession('second-model')
  first.set('input', 'Describe this')
  first.set('images', [{ name: 'photo.png', url: 'data:image/png;base64,abc' }])
  first.set('maxTokens', 2048)
  first.set('thinking', true)
  const returned = getChatSession('first-model').getSnapshot()
  assert.equal(returned.input, 'Describe this')
  assert.equal(returned.images.length, 1)
  assert.equal(returned.maxTokens, 2048)
  assert.equal(returned.thinking, true)
  assert.equal(second.getSnapshot().input, '')
  assert.equal(second.getSnapshot().images.length, 0)
})

test('Stop after returning aborts only the selected model', () => {
  const first = getChatSession('stop-first')
  const second = getChatSession('stop-second')
  first.abort.current = new AbortController()
  second.abort.current = new AbortController()
  getChatSession('stop-first').abort.current.abort()
  assert.equal(first.abort.current.signal.aborted, true)
  assert.equal(second.abort.current.signal.aborted, false)
})

test('last selected model is remembered', () => {
  rememberPlaygroundModel('second-model')
  assert.equal(getLastPlaygroundModel(), 'second-model')
})
