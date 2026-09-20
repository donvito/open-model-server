import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Send, Square, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/StatusBadge'
import { useModels } from '@/hooks/useModels'
import { api, streamChat } from '@/lib/api'
import type { ChatMessage, Model, PredictResponse } from '@/lib/types'
import { cn } from '@/lib/utils'
import { useToast } from '@/components/Toast'

export function PlaygroundPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const { models } = useModels(4000)
  const running = useMemo(() => (models ?? []).filter((m) => m.live.state === 'running'), [models])
  const selected = useMemo(() => (models ?? []).find((m) => m.name === id || m.id === id), [models, id])

  // Default to the first running model when none is selected.
  useEffect(() => {
    if (!id && running.length > 0) nav(`/playground/${running[0].name}`, { replace: true })
  }, [id, running, nav])

  return (
    <div className="flex h-[calc(100vh-3rem)] flex-col gap-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Playground</h1>
          <p className="text-muted-foreground text-sm">Try a running model. Requests go through the same public API.</p>
        </div>
        <div className="flex items-center gap-2">
          <Select value={selected?.name ?? ''} onValueChange={(v) => nav(`/playground/${v}`)}>
            <SelectTrigger className="w-72">
              <SelectValue placeholder={models === null ? 'Loading…' : running.length ? 'Select a model' : 'No running models'} />
            </SelectTrigger>
            <SelectContent>
              {(models ?? []).map((m) => (
                <SelectItem key={m.id} value={m.name} disabled={m.live.state !== 'running'}>
                  <span className="flex items-center gap-2">
                    {m.name}
                    <Badge variant="secondary" className="text-[10px]">
                      {m.task}
                    </Badge>
                    {m.live.state !== 'running' && <span className="text-muted-foreground text-xs">({m.live.state})</span>}
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {selected && <StatusBadge state={selected.live.state} />}
        </div>
      </div>

      {!selected ? (
        <Card>
          <CardContent className="text-muted-foreground py-10 text-center text-sm">
            {models && running.length === 0 ? 'Load a model from the Models page first.' : 'Pick a model to start.'}
          </CardContent>
        </Card>
      ) : selected.live.state !== 'running' ? (
        <Card>
          <CardContent className="text-muted-foreground py-10 text-center text-sm">
            {selected.name} is {selected.live.state}. Load it to use the playground.
          </CardContent>
        </Card>
      ) : selected.task === 'chat' || selected.task === 'completion' ? (
        <ChatPanel key={selected.id} model={selected} />
      ) : (
        <PredictPanel key={selected.id} model={selected} />
      )}
    </div>
  )
}

function ChatPanel({ model }: { model: Model }) {
  const toast = useToast()
  const [system, setSystem] = useState('')
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [temperature, setTemperature] = useState(0.7)
  const [maxTokens, setMaxTokens] = useState(512)
  const [streaming, setStreaming] = useState(false)
  const [stats, setStats] = useState<string>('')
  const abort = useRef<AbortController | null>(null)
  const scroller = useRef<HTMLDivElement>(null)

  useEffect(() => {
    scroller.current?.scrollTo({ top: scroller.current.scrollHeight })
  }, [messages])

  async function send() {
    const text = input.trim()
    if (!text || streaming) return
    setInput('')
    const history: ChatMessage[] = [...messages, { role: 'user', content: text }]
    setMessages([...history, { role: 'assistant', content: '' }])
    setStreaming(true)
    setStats('')
    const ctrl = new AbortController()
    abort.current = ctrl
    const started = performance.now()
    let chars = 0
    let first = 0
    try {
      const outgoing: ChatMessage[] = system.trim() ? [{ role: 'system', content: system.trim() }, ...history] : history
      await streamChat(
        model.name,
        outgoing,
        { temperature, max_tokens: maxTokens },
        (c) => {
          if (c.delta) {
            if (!first) first = performance.now()
            chars += c.delta.length
            setMessages((ms) => {
              const copy = ms.slice()
              const last = copy[copy.length - 1]
              copy[copy.length - 1] = { ...last, content: last.content + c.delta }
              return copy
            })
          }
          if (c.usage) {
            const secs = (performance.now() - started) / 1000
            setStats(`${c.usage.completion_tokens} tokens · ${(c.usage.completion_tokens / Math.max(secs, 0.001)).toFixed(1)} tok/s`)
          }
        },
        ctrl.signal,
      )
      if (!stats) {
        const secs = (performance.now() - started) / 1000
        setStats(`${chars} chars in ${secs.toFixed(1)}s${first ? ` · TTFT ${((first - started) / 1000).toFixed(2)}s` : ''}`)
      }
    } catch (e) {
      if ((e as Error).name !== 'AbortError') toast.error((e as Error).message)
    } finally {
      setStreaming(false)
      abort.current = null
    }
  }

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[1fr_16rem] gap-4">
      <Card className="min-h-0 gap-0 py-0">
        <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto p-4">
          {messages.length === 0 && <div className="text-muted-foreground py-10 text-center text-sm">Send a message to start chatting with {model.name}.</div>}
          <div className="flex flex-col gap-3">
            {messages.map((m, i) => (
              <div key={i} className={cn('flex', m.role === 'user' ? 'justify-end' : 'justify-start')}>
                <div className={cn('max-w-[80%] whitespace-pre-wrap rounded-2xl px-4 py-2 text-sm', m.role === 'user' ? 'bg-primary text-primary-foreground' : 'bg-muted')}>
                  {m.content || (streaming && i === messages.length - 1 ? <span className="text-muted-foreground animate-pulse">…</span> : '')}
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="border-t p-3">
          <div className="flex items-end gap-2">
            <Textarea
              rows={2}
              value={input}
              placeholder="Type a message… (Enter to send, Shift+Enter for newline)"
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  send()
                }
              }}
            />
            {streaming ? (
              <Button variant="outline" onClick={() => abort.current?.abort()}>
                <Square /> Stop
              </Button>
            ) : (
              <Button onClick={send} disabled={!input.trim()}>
                <Send /> Send
              </Button>
            )}
          </div>
          <div className="text-muted-foreground mt-1 flex justify-between text-xs">
            <span>{stats}</span>
            <button className="hover:text-foreground inline-flex items-center gap-1 cursor-pointer" onClick={() => setMessages([])}>
              <Trash2 className="size-3" /> clear
            </button>
          </div>
        </div>
      </Card>
      <Card className="gap-4 py-4">
        <CardContent className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="sys">System prompt</Label>
            <Textarea id="sys" rows={4} value={system} onChange={(e) => setSystem(e.target.value)} placeholder="You are a helpful assistant." className="text-xs" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="temp">Temperature: {temperature.toFixed(2)}</Label>
            <input id="temp" type="range" min={0} max={2} step={0.05} value={temperature} onChange={(e) => setTemperature(Number(e.target.value))} className="accent-violet-400" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="max">Max tokens</Label>
            <Input id="max" type="number" min={1} max={32768} value={maxTokens} onChange={(e) => setMaxTokens(Number(e.target.value) || 1)} />
          </div>
          <p className="text-muted-foreground text-xs">
            Streams from <code>POST /v1/chat/completions</code>.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

function PredictPanel({ model }: { model: Model }) {
  const toast = useToast()
  const isText = model.task === 'classification' || model.task === 'embedding' || model.task === 'reranking'
  const [text, setText] = useState(isText ? 'This is a wonderful product, I love it!' : '')
  const [raw, setRaw] = useState(
    model.task === 'custom'
      ? JSON.stringify({ input: { input_name: { shape: [1, 4], data: [1, 2, 3, 4] } } }, null, 2)
      : model.task === 'reranking'
        ? JSON.stringify({ input: { query: 'what is a cat', documents: ['A cat is a small domesticated animal.', 'Paris is in France.'] } }, null, 2)
        : '',
  )
  const [mode, setMode] = useState<'text' | 'json'>(model.task === 'custom' || model.task === 'reranking' ? 'json' : 'text')
  const [result, setResult] = useState<PredictResponse | null>(null)
  const [busy, setBusy] = useState(false)

  async function run() {
    setBusy(true)
    try {
      let input: unknown
      let params: Record<string, unknown> | undefined
      if (mode === 'json') {
        const parsed = JSON.parse(raw) as { input?: unknown; params?: Record<string, unknown> }
        input = parsed.input ?? parsed
        params = parsed.params
      } else {
        const lines = text
          .split('\n')
          .map((s) => s.trim())
          .filter(Boolean)
        input = lines.length > 1 ? lines : lines[0] ?? ''
      }
      setResult(await api.predict(model.name, input, params))
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid min-h-0 flex-1 grid-cols-2 gap-4">
      <Card className="gap-3 py-4">
        <CardContent className="flex h-full flex-col gap-3">
          <div className="flex items-center justify-between">
            <Label>Input</Label>
            {isText && (
              <div className="bg-muted flex rounded-md p-0.5 text-xs">
                {(['text', 'json'] as const).map((m) => (
                  <button key={m} className={cn('rounded px-2 py-0.5 cursor-pointer', mode === m ? 'bg-background shadow' : 'text-muted-foreground')} onClick={() => setMode(m)}>
                    {m}
                  </button>
                ))}
              </div>
            )}
          </div>
          {mode === 'text' ? (
            <Textarea className="flex-1 font-mono text-xs" value={text} onChange={(e) => setText(e.target.value)} placeholder={'One input per line for batch inference'} />
          ) : (
            <Textarea className="flex-1 font-mono text-xs" value={raw} onChange={(e) => setRaw(e.target.value)} placeholder='{"input": ..., "params": {...}}' />
          )}
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground text-xs">
              <code>POST /v1/models/{model.name}/predict</code>
            </span>
            <Button onClick={run} disabled={busy}>
              <Send /> {busy ? 'Running…' : 'Run'}
            </Button>
          </div>
        </CardContent>
      </Card>
      <Card className="gap-3 py-4">
        <CardContent className="flex h-full flex-col gap-3">
          <div className="flex items-center justify-between">
            <Label>Output</Label>
            {result && (
              <span className="text-muted-foreground text-xs">
                {result.timing.latency_ms.toFixed(1)} ms{result.timing.tokens ? ` · ${result.timing.tokens} tokens` : ''}
              </span>
            )}
          </div>
          <ResultView result={result} />
        </CardContent>
      </Card>
    </div>
  )
}

function ResultView({ result }: { result: PredictResponse | null }) {
  if (!result) return <div className="text-muted-foreground flex-1 text-sm">Run an inference to see the structured output.</div>
  const out = result.output as Record<string, unknown> | null
  const scores = out && Array.isArray(out.scores) ? (out.scores as { label: string; score: number }[]) : null
  const batch = out && Array.isArray(out.results) ? (out.results as { label: string; score: number; scores: { label: string; score: number }[] }[]) : null
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-auto">
      {scores && <ScoreBars scores={scores} />}
      {batch && (
        <div className="grid gap-2">
          {batch.map((r, i) => (
            <div key={i} className="rounded-md border p-2">
              <div className="mb-1 text-xs font-medium">
                #{i + 1}: {r.label} <span className="text-muted-foreground">({(r.score * 100).toFixed(1)}%)</span>
              </div>
              <ScoreBars scores={r.scores} />
            </div>
          ))}
        </div>
      )}
      <pre className="bg-black/40 min-h-0 flex-1 overflow-auto rounded-md p-3 font-mono text-xs">{JSON.stringify(result.output, null, 2)}</pre>
    </div>
  )
}

function ScoreBars({ scores }: { scores: { label: string; score: number }[] }) {
  return (
    <div className="grid gap-1">
      {scores.slice(0, 8).map((s) => (
        <div key={s.label} className="grid grid-cols-[7rem_1fr_3rem] items-center gap-2 text-xs">
          <span className="truncate" title={s.label}>
            {s.label}
          </span>
          <div className="bg-muted h-2 overflow-hidden rounded-full">
            <div className="h-full bg-violet-400" style={{ width: `${Math.max(1, s.score * 100)}%` }} />
          </div>
          <span className="text-muted-foreground text-right">{(s.score * 100).toFixed(1)}%</span>
        </div>
      ))}
    </div>
  )
}
