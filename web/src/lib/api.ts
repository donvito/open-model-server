import type {
  ChatMessage,
  CreateModelInput,
  LogLine,
  Model,
  PredictResponse,
  RuntimeInfo,
  SystemInfo,
  UpdateModelInput,
} from './types'

const KEY_STORAGE = 'modelserver.apiKey'

export function getApiKey(): string {
  return localStorage.getItem(KEY_STORAGE) ?? ''
}
export function setApiKey(k: string) {
  if (k) localStorage.setItem(KEY_STORAGE, k)
  else localStorage.removeItem(KEY_STORAGE)
}

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, message: string, code = '') {
    super(message)
    this.status = status
    this.code = code
  }
}

type Listener = () => void
const unauthorizedListeners = new Set<Listener>()
export function onUnauthorized(fn: Listener) {
  unauthorizedListeners.add(fn)
  return () => {
    unauthorizedListeners.delete(fn)
  }
}

function headers(json = true): HeadersInit {
  const h: Record<string, string> = {}
  if (json) h['Content-Type'] = 'application/json'
  const k = getApiKey()
  if (k) h['Authorization'] = `Bearer ${k}`
  return h
}

async function parseError(res: Response): Promise<ApiError> {
  let msg = `${res.status} ${res.statusText}`
  let code = ''
  try {
    const body = (await res.json()) as { error?: { message?: string; code?: string } }
    if (body?.error?.message) msg = body.error.message
    if (body?.error?.code) code = body.error.code
  } catch {
    /* non-JSON body */
  }
  if (res.status === 401) unauthorizedListeners.forEach((f) => f())
  return new ApiError(res.status, msg, code)
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(path, { ...init, headers: { ...headers(init.body != null), ...(init.headers ?? {}) } })
  if (!res.ok) throw await parseError(res)
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  health: () => request<{ status: string }>('/api/system/health'),
  system: () => request<SystemInfo>('/api/system'),
  runtimes: async () => (await request<{ runtimes: RuntimeInfo[] }>('/api/runtimes')).runtimes,
  runtimeLogs: async (name: string, lines = 300, signal?: AbortSignal) =>
    (await request<{ lines: LogLine[] }>(`/api/runtimes/${encodeURIComponent(name)}/logs?lines=${lines}`, { signal })).lines,

  listModels: async () => (await request<{ models: Model[] | null }>('/api/models')).models ?? [],
  getModel: (id: string) => request<Model>(`/api/models/${encodeURIComponent(id)}`),
  createModel: (input: CreateModelInput) =>
    request<Model>('/api/models', { method: 'POST', body: JSON.stringify(input) }),
  updateModel: (id: string, input: UpdateModelInput) =>
    request<Model>(`/api/models/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) }),
  deleteModel: (id: string) => request<void>(`/api/models/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  load: (id: string) => request<Model>(`/api/models/${encodeURIComponent(id)}/load`, { method: 'POST' }),
  unload: (id: string) => request<Model>(`/api/models/${encodeURIComponent(id)}/unload`, { method: 'POST' }),
  restart: (id: string) => request<Model>(`/api/models/${encodeURIComponent(id)}/restart`, { method: 'POST' }),
  logs: async (id: string, lines = 300) =>
    (await request<{ lines: LogLine[] | null }>(`/api/models/${encodeURIComponent(id)}/logs?lines=${lines}`)).lines ?? [],

  predict: (model: string, input: unknown, params?: Record<string, unknown>) =>
    request<PredictResponse>(`/v1/models/${encodeURIComponent(model)}/predict`, {
      method: 'POST',
      body: JSON.stringify({ input, params }),
    }),
}

/** Subscribe to the SSE log stream. Returns an unsubscribe function. */
export function streamLogs(id: string, onLine: (l: LogLine) => void, onError?: (e: Event) => void) {
  // EventSource cannot set headers; pass the key as a query param (server accepts both).
  const k = getApiKey()
  const url = `/api/models/${encodeURIComponent(id)}/logs/stream${k ? `?api_key=${encodeURIComponent(k)}` : ''}`
  const es = new EventSource(url)
  es.addEventListener('log', (ev) => {
    try {
      onLine(JSON.parse((ev as MessageEvent).data) as LogLine)
    } catch {
      /* ignore malformed */
    }
  })
  es.onerror = (e) => onError?.(e)
  return () => es.close()
}

export interface ChatChunk {
  delta: string
  reasoning?: string
  done: boolean
  usage?: { prompt_tokens: number; completion_tokens: number }
}

/** Streams an OpenAI-compatible chat completion. */
export async function streamChat(
  model: string,
  messages: ChatMessage[],
  params: {
    temperature?: number
    max_tokens?: number
    chat_template_kwargs?: Record<string, unknown>
  },
  onChunk: (c: ChatChunk) => void,
  signal?: AbortSignal,
) {
  const res = await fetch('/v1/chat/completions', {
    method: 'POST',
    headers: headers(),
    signal,
    body: JSON.stringify({ model, messages, stream: true, ...params }),
  })
  if (!res.ok) throw await parseError(res)
  if (!res.body) throw new ApiError(500, 'no response body')
  const reader = res.body.getReader()
  const dec = new TextDecoder()
  let buf = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buf += dec.decode(value, { stream: true })
    let idx: number
    while ((idx = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, idx).trim()
      buf = buf.slice(idx + 1)
      if (!line.startsWith('data:')) continue
      const data = line.slice(5).trim()
      if (data === '[DONE]') {
        onChunk({ delta: '', done: true })
        return
      }
      try {
        const j = JSON.parse(data) as {
          choices?: { delta?: { content?: string | null; reasoning_content?: string | null }; finish_reason?: string | null }[]
          usage?: { prompt_tokens: number; completion_tokens: number }
        }
        const delta = j.choices?.[0]?.delta?.content ?? ''
        const reasoning = j.choices?.[0]?.delta?.reasoning_content ?? ''
        const finished = Boolean(j.choices?.[0]?.finish_reason)
        onChunk({ delta, reasoning, done: finished, usage: j.usage })
      } catch {
        /* skip partial */
      }
    }
  }
  onChunk({ delta: '', done: true })
}
