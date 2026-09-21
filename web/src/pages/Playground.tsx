import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ImagePlus, Send, Square, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/StatusBadge'
import { ChatMarkdown } from '@/components/ChatMarkdown'
import { useModels } from '@/hooks/useModels'
import { useChatSessionState } from '@/hooks/useChatSessionState'
import { getChatSession, getLastPlaygroundModel, rememberPlaygroundModel, type ChatDisplayMessage } from '@/lib/playgroundSessions'
import { api, streamChat } from '@/lib/api'
import type { ChatContentPart, ChatMessage, Model, PredictResponse } from '@/lib/types'
import { cn } from '@/lib/utils'
import { useToast } from '@/components/Toast'

export function PlaygroundPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const { models } = useModels(4000)
  const running = useMemo(() => (models ?? []).filter((m) => m.live.state === 'running'), [models])
  const selected = useMemo(() => (models ?? []).find((m) => m.name === id || m.id === id), [models, id])

  // Return to the last selected session when entering from the sidebar.
  useEffect(() => {
    if (selected) rememberPlaygroundModel(selected.id)
    if (!id && models) {
      const previous = models.find((model) => model.id === getLastPlaygroundModel())
      const target = previous ?? running[0]
      if (target) nav(`/playground/${target.name}`, { replace: true })
    }
  }, [id, models, running, selected, nav])

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
  const [system, setSystem] = useChatSessionState(model.id, 'system')
  const [messages, setMessages] = useChatSessionState(model.id, 'messages')
  const [input, setInput] = useChatSessionState(model.id, 'input')
  const supportsVision = model.live.details?.vision === true
  const [images, setImages] = useChatSessionState(model.id, 'images')
  const [readingImages, setReadingImages] = useChatSessionState(model.id, 'readingImages')
  const imageInput = useRef<HTMLInputElement>(null)
  const [temperature, setTemperature] = useChatSessionState(model.id, 'temperature')
  const [maxTokens, setMaxTokens] = useChatSessionState(model.id, 'maxTokens')
  const [thinking, setThinking] = useChatSessionState(model.id, 'thinking')
  const [streaming, setStreaming] = useChatSessionState(model.id, 'streaming')
  const [stats, setStats] = useChatSessionState(model.id, 'stats')
  const session = getChatSession(model.id)
  const scroller = useRef<HTMLDivElement>(null)

  function withoutReasoning(message: ChatDisplayMessage): ChatMessage {
    const { reasoning: _reasoning, ...requestMessage } = message
    return requestMessage
  }

  async function attachImages(files: File[]) {
    if (!supportsVision || streaming || readingImages) return
    setReadingImages(true)
    try {
      const attachments = await Promise.all(files.map(async (file) => {
        if (!['image/png', 'image/jpeg', 'image/webp', 'image/gif'].includes(file.type)) {
          throw new Error(`${file.name}: use a PNG, JPEG, WebP, or GIF image.`)
        }
        if (file.size > 5 * 1024 * 1024) throw new Error(`${file.name}: images must be 5 MB or smaller.`)
        const url = await new Promise<string>((resolve, reject) => {
          const reader = new FileReader()
          reader.onload = () => resolve(reader.result as string)
          reader.onerror = () => reject(new Error(`Could not read ${file.name}.`))
          reader.readAsDataURL(file)
        })
        return { name: file.name, url }
      }))
      setImages((current) => [...current, ...attachments])
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setReadingImages(false)
    }
  }

  useEffect(() => {
    scroller.current?.scrollTo({ top: scroller.current.scrollHeight })
  }, [messages])

  async function send() {
    const text = input.trim()
    if ((!text && !images.length) || getChatSession(model.id).getSnapshot().streaming || readingImages) return
    if (images.length && !supportsVision) return
    const content: ChatContentPart[] = [
      ...(text ? [{ type: 'text' as const, text }] : []),
      ...images.map((image): ChatContentPart => ({ type: 'image_url', image_url: { url: image.url } })),
    ]
    const history: ChatDisplayMessage[] = [...messages, { role: 'user', content: images.length ? content : text }]
    const requestHistory = history.map(withoutReasoning)
    const outgoing: ChatMessage[] = system.trim() ? [{ role: 'system', content: system.trim() }, ...requestHistory] : requestHistory
    // Include previous images and base64 expansion in the API's 32 MiB body limit.
    if (new Blob([JSON.stringify({ model: model.name, messages: outgoing, stream: true, temperature, max_tokens: maxTokens })]).size > 32 * 1024 * 1024) {
      toast.error('This conversation exceeds the 32 MB request limit. Remove attachments or clear the conversation.')
      return
    }
    setInput('')
    setImages([])
    setMessages([...history, { role: 'assistant', content: '' }])
    setStreaming(true)
    setStats('')
    const ctrl = new AbortController()
    session.setAbortController(ctrl)
    const started = performance.now()
    let chars = 0
    let first = 0
    let sawUsage = false
    try {
      await streamChat(
        model.name,
        outgoing,
        { temperature, max_tokens: maxTokens, chat_template_kwargs: { enable_thinking: thinking } },
        (c) => {
          if (c.reasoning) {
            setStats('Thinking…')
            setMessages((ms) => {
              const copy = ms.slice()
              const last = copy[copy.length - 1]
              copy[copy.length - 1] = { ...last, reasoning: (last.reasoning ?? '') + c.reasoning }
              return copy
            })
          }
          if (c.delta) {
            if (!first) first = performance.now()
            chars += c.delta.length
            setMessages((ms) => {
              const copy = ms.slice()
              const last = copy[copy.length - 1]
              copy[copy.length - 1] = { ...last, content: (typeof last.content === 'string' ? last.content : '') + c.delta }
              return copy
            })
          }
          if (c.usage) {
            sawUsage = true
            const secs = (performance.now() - started) / 1000
            setStats(`${c.usage.completion_tokens} tokens · ${(c.usage.completion_tokens / Math.max(secs, 0.001)).toFixed(1)} tok/s`)
          }
        },
        ctrl.signal,
      )
      if (!sawUsage) {
        const secs = (performance.now() - started) / 1000
        setStats(`${chars} chars in ${secs.toFixed(1)}s${first ? ` · TTFT ${((first - started) / 1000).toFixed(2)}s` : ''}`)
      }
    } catch (e) {
      if ((e as Error).name !== 'AbortError') {
        setStats(`Error: ${(e as Error).message}`)
        toast.error((e as Error).message)
      } else {
        setStats('Stopped')
      }
    } finally {
      setStreaming(false)
      session.setAbortController(null)
    }
  }

  return (
    <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_16rem]">
      <Card className="min-h-0 gap-0 py-0">
        <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto p-4">
          {messages.length === 0 && <div className="text-muted-foreground py-10 text-center text-sm">Send a message to start chatting with {model.name}.</div>}
          <div className="flex flex-col gap-3">
            {messages.map((m, i) => (
              <div key={i} className={cn('flex', m.role === 'user' ? 'justify-end' : 'justify-start')}>
                <div className={cn('min-w-0 max-w-[80%] rounded-2xl px-4 py-2 text-sm', m.role === 'user' ? 'bg-primary text-primary-foreground' : 'bg-muted')}>
                  {m.role === 'assistant' && (m.reasoning || (thinking && streaming && i === messages.length - 1)) && (
                    <details className="mb-2 rounded-lg border border-violet-500/20 bg-violet-500/5 px-3 py-2 text-xs" open={streaming && i === messages.length - 1}>
                      <summary className="text-muted-foreground cursor-pointer select-none">
                        <span className="font-medium">Thinking</span>{' '}
                        <span>{m.reasoning ? `(${m.reasoning.length.toLocaleString()} chars)` : '(in progress)'}</span>
                      </summary>
                      <div className="mt-2 max-h-64 overflow-y-auto border-t border-violet-500/10 pt-2 text-muted-foreground">
                        <ChatMarkdown>{m.reasoning || 'Waiting for reasoning…'}</ChatMarkdown>
                      </div>
                    </details>
                  )}
                  {Array.isArray(m.content) ? m.content.map((part, index) => (
                    part.type === 'text' ? <ChatMarkdown key={index}>{part.text}</ChatMarkdown> : (
                      <img key={index} src={part.image_url.url} alt={`Attached image ${index + 1}`} className="my-2 max-h-64 max-w-full rounded-lg object-contain" />
                    )
                  )) : m.content ? <ChatMarkdown>{m.content}</ChatMarkdown> : (streaming && i === messages.length - 1 ? <span className="text-muted-foreground animate-pulse">…</span> : '')}
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="border-t p-3">
          {images.length > 0 && (
            <div className="mb-3 flex flex-wrap gap-2">
              {images.map((image, index) => (
                <div key={index} className="relative rounded-lg border p-1">
                  <img src={image.url} alt={image.name} title={image.name} className="h-20 w-20 rounded object-cover" />
                  <Button size="icon" variant="secondary" className="absolute -top-2 -right-2 size-6" aria-label={`Remove ${image.name}`} onClick={() => setImages((current) => current.filter((_, i) => i !== index))}>
                    <X className="size-3" />
                  </Button>
                </div>
              ))}
            </div>
          )}
          <div className="flex items-end gap-2">
            {supportsVision && (
              <>
                <input ref={imageInput} type="file" accept="image/png,image/jpeg,image/webp,image/gif" multiple className="hidden" aria-label="Attach images" onChange={(e) => {
                  const files = Array.from(e.target.files ?? [])
                  e.target.value = ''
                  void attachImages(files)
                }} />
                <Button variant="outline" size="icon" aria-label="Attach images" title="Attach images (up to 5 MB each)" disabled={streaming || readingImages} onClick={() => imageInput.current?.click()}>
                  <ImagePlus />
                </Button>
              </>
            )}
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
              <Button variant="outline" onClick={() => session.abort.current?.abort()}>
                <Square /> Stop
              </Button>
            ) : (
              <Button onClick={send} disabled={(!input.trim() && !images.length) || readingImages}>
                <Send /> Send
              </Button>
            )}
          </div>
          <div className="text-muted-foreground mt-1 flex justify-between text-xs">
            <span>{readingImages ? 'Reading images…' : stats}</span>
            <button disabled={streaming || readingImages} className="hover:text-foreground inline-flex items-center gap-1 cursor-pointer disabled:opacity-50" onClick={() => { setMessages([]); setImages([]); setStats('') }}>
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
          <label className="flex items-center justify-between gap-3 text-sm" htmlFor="thinking">
            <span>
              <span className="block font-medium">Thinking</span>
              <span className="text-muted-foreground block text-xs">Allow reasoning models to spend tokens on analysis.</span>
            </span>
            <input id="thinking" type="checkbox" checked={thinking} onChange={(e) => setThinking(e.target.checked)} disabled={streaming} className="size-4 accent-violet-400 disabled:opacity-50" />
          </label>
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
