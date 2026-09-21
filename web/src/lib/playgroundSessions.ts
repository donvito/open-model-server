import type { ChatMessage } from './types.ts'

// Reasoning is display-only metadata. It is stripped before messages are sent
// back through the OpenAI-compatible API.
export type ChatDisplayMessage = ChatMessage & { reasoning?: string }

export interface ChatSessionState {
  system: string
  messages: ChatDisplayMessage[]
  input: string
  images: { name: string; url: string }[]
  readingImages: boolean
  temperature: number
  maxTokens: number
  thinking: boolean
  streaming: boolean
  stats: string
}

// Sessions belong to the browser tab, not the route displaying them. In-flight
// callbacks keep updating this store even when no component is subscribed.
class ChatSession {
  private state: ChatSessionState = {
    system: '', messages: [], input: '', images: [], readingImages: false,
    temperature: 0.7, maxTokens: 512, thinking: false, streaming: false, stats: '',
  }
  private listeners = new Set<() => void>()
  readonly abort: { current: AbortController | null } = { current: null }
  setAbortController(controller: AbortController | null) { this.abort.current = controller }
  getSnapshot = () => this.state
  subscribe = (listener: () => void) => {
    this.listeners.add(listener)
    return () => { this.listeners.delete(listener) }
  }
  set<K extends keyof ChatSessionState>(key: K, value: ChatSessionState[K] | ((previous: ChatSessionState[K]) => ChatSessionState[K])) {
    const next = typeof value === 'function' ? value(this.state[key]) : value
    this.state = { ...this.state, [key]: next }
    this.listeners.forEach((listener) => listener())
  }
}

const sessions = new Map<string, ChatSession>()
let lastModelId: string | undefined

export function getChatSession(modelId: string) {
  let session = sessions.get(modelId)
  if (!session) {
    session = new ChatSession()
    sessions.set(modelId, session)
  }
  return session
}

export function rememberPlaygroundModel(modelId: string) { lastModelId = modelId }
export function getLastPlaygroundModel() { return lastModelId }
