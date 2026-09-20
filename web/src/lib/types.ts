export type Runtime = 'llamacpp' | 'onnx'
export type Task = 'chat' | 'completion' | 'classification' | 'embedding' | 'reranking' | 'custom'
export type State = 'stopped' | 'starting' | 'running' | 'failed' | 'stopping'

export interface LiveStatus {
  state: State
  error?: string
  pid?: number
  port?: number
  started_at?: string
  details?: Record<string, unknown>
}

export interface Model {
  id: string
  name: string
  description: string
  runtime: Runtime
  task: Task
  model_path: string
  config: Record<string, unknown> | null
  status: State
  created_at: string
  updated_at: string
  live: LiveStatus
}

export interface CreateModelInput {
  name: string
  description?: string
  runtime?: string
  task?: string
  model_path: string
  config?: Record<string, unknown>
}

export interface UpdateModelInput {
  name?: string
  description?: string
  runtime?: string
  task?: string
  model_path?: string
  config?: Record<string, unknown>
}

export interface RuntimeInfo {
  name: Runtime
  available: boolean
  error?: string
  version?: string
  details?: Record<string, unknown>
}

export interface SystemInfo {
  version: string
  os: string
  arch: string
  cpus: number
  hostname: string
  go_version: string
  memory: { total_bytes?: number; available_bytes?: number }
  uptime_seconds: number
  runtimes: RuntimeInfo[]
  models: { total: number; running: number; failed: number }
  server: Record<string, unknown>
  paths: Record<string, string>
}

export interface LogLine {
  time: string
  source: 'stdout' | 'stderr' | 'system'
  text: string
}

export interface PredictResponse {
  model: string
  runtime: string
  task: string
  output: unknown
  timing: { latency_ms: number; tokens_per_second?: number; tokens?: number }
}

export interface ChatMessage {
  role: 'system' | 'user' | 'assistant'
  content: string
}

export const TASKS: Task[] = ['chat', 'completion', 'classification', 'embedding', 'reranking', 'custom']
export const RUNTIMES: Runtime[] = ['llamacpp', 'onnx']
