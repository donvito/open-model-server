import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Bot, ImagePlus, MessageCircle, Minus, PanelRightClose, PanelRightOpen, Plus, Send, Settings2, Square, Trash2, UserRound, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
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
    <div className="flex h-full min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b bg-card px-5 py-2.5">
        <div className="hidden min-w-0 items-center gap-3 sm:flex">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-violet-500/10 text-violet-700 dark:text-violet-400">
            <MessageCircle className="size-4" aria-hidden="true" />
          </div>
          <div className="min-w-0">
            <h1 className="truncate text-sm font-semibold tracking-tight">Playground</h1>
            <p className="text-muted-foreground truncate text-[11px]">Inference workspace</p>
          </div>
        </div>
        <div className="flex min-w-0 flex-1 items-center justify-end gap-2 sm:flex-none">
          <span className="text-muted-foreground hidden text-[11px] font-medium uppercase tracking-[0.12em] sm:inline">Model</span>
          <Select value={selected?.name ?? ''} onValueChange={(v) => nav(`/playground/${v}`)}>
            <SelectTrigger className="h-9 min-w-0 flex-1 sm:w-64 sm:flex-none">
              <SelectValue placeholder={models === null ? 'Loading…' : running.length ? 'Select a model' : 'No running models'} />
            </SelectTrigger>
            <SelectContent>
              {(models ?? []).map((m) => (
                <SelectItem key={m.id} value={m.name} disabled={m.live.state !== 'running'}>
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="truncate">{m.name}</span>
                    <span className="text-muted-foreground text-xs">{m.task}</span>
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

function TemperatureSlider({ id, value, onChange, disabled = false }: { id: string; value: number; onChange: (value: number) => void; disabled?: boolean }) {
  const track = useRef<HTMLDivElement>(null)
  const min = 0
  const max = 2
  const step = 0.05
  const percentage = ((value - min) / (max - min)) * 100

  function snap(next: number) {
    return Number(Math.min(max, Math.max(min, Math.round(next / step) * step)).toFixed(2))
  }

  function setFromPointer(clientX: number) {
    const bounds = track.current?.getBoundingClientRect()
    if (!bounds || disabled || bounds.width === 0) return
    const ratio = Math.min(1, Math.max(0, (clientX - bounds.left) / bounds.width))
    onChange(snap(min + ratio * (max - min)))
  }

  function nudge(amount: number) {
    if (!disabled) onChange(snap(value + amount))
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (disabled) return
    let next: number | undefined
    if (event.key === 'ArrowLeft' || event.key === 'ArrowDown') next = value - step
    if (event.key === 'ArrowRight' || event.key === 'ArrowUp') next = value + step
    if (event.key === 'Home') next = min
    if (event.key === 'End') next = max
    if (next === undefined) return
    event.preventDefault()
    onChange(snap(next))
  }

  return (
    <div className="grid gap-1.5">
      <div className="flex items-center gap-1.5">
        <Button type="button" variant="ghost" size="icon-sm" className="text-muted-foreground" aria-label="Decrease temperature" disabled={disabled || value <= min} onClick={() => nudge(-step)}>
          <Minus />
        </Button>
        <div
          id={id}
          role="slider"
          tabIndex={disabled ? -1 : 0}
          aria-label="Temperature"
          aria-valuemin={min}
          aria-valuemax={max}
          aria-valuenow={value}
          aria-valuetext={value.toFixed(2)}
          aria-disabled={disabled || undefined}
          className={cn('relative flex h-8 min-w-0 flex-1 touch-none items-center rounded-lg outline-none transition-colors', disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer focus-visible:ring-2 focus-visible:ring-violet-500/40')}
          onKeyDown={onKeyDown}
          onPointerDown={(event) => {
            if (disabled) return
            event.currentTarget.setPointerCapture(event.pointerId)
            setFromPointer(event.clientX)
          }}
          onPointerMove={(event) => {
            if (event.currentTarget.hasPointerCapture(event.pointerId)) setFromPointer(event.clientX)
          }}
        >
          <div ref={track} className="relative mx-2 h-1.5 flex-1 rounded-full bg-zinc-200 dark:bg-zinc-800">
            <div className="absolute inset-y-0 left-0 rounded-full bg-primary" style={{ width: `${percentage}%` }} />
            <div className="absolute top-1/2 size-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-background bg-primary shadow-sm ring-1 ring-primary/30" style={{ left: `${percentage}%` }} />
          </div>
        </div>
        <Button type="button" variant="ghost" size="icon-sm" className="text-muted-foreground" aria-label="Increase temperature" disabled={disabled || value >= max} onClick={() => nudge(step)}>
          <Plus />
        </Button>
      </div>
      <div className="text-muted-foreground flex justify-between px-2 text-[10px]">
        <span>Focused</span>
        <span>0.00</span>
        <span>2.00</span>
      </div>
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
  const shouldAutoScroll = useRef(true)
  const hasMountedMessages = useRef(false)
  const [settingsOpen, setSettingsOpen] = useState(() => typeof window === 'undefined' || window.matchMedia('(min-width: 1024px)').matches)
  const settingsTrigger = useRef<HTMLButtonElement>(null)
  const settingsClose = useRef<HTMLButtonElement>(null)
  const previousSettingsOpen = useRef(settingsOpen)
  useEffect(() => {
    if (previousSettingsOpen.current !== settingsOpen) {
      if (settingsOpen && !window.matchMedia('(min-width: 1024px)').matches) settingsClose.current?.focus()
      if (!settingsOpen) settingsTrigger.current?.focus()
      previousSettingsOpen.current = settingsOpen
    }
  }, [settingsOpen])
  const suggestions = [
    'Explain a complex idea in simple terms',
    'Write a small Python utility',
    'Review this text for clarity',
  ]

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

  function handleConversationScroll() {
    const element = scroller.current
    if (!element) return
    const distanceFromBottom = element.scrollHeight - element.scrollTop - element.clientHeight
    shouldAutoScroll.current = distanceFromBottom < 64
  }

  useEffect(() => {
    const element = scroller.current
    if (!element || !messages.length) return
    if (!hasMountedMessages.current || shouldAutoScroll.current) {
      element.scrollTo({ top: element.scrollHeight })
    }
    hasMountedMessages.current = true
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
    shouldAutoScroll.current = true
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
    <div className="relative flex min-h-0 flex-1">
      <Card className={cn('min-h-0 min-w-0 flex-1 gap-0 overflow-hidden rounded-none border-0 bg-background py-0 shadow-none', settingsOpen && 'hidden lg:flex')}>
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-zinc-200/80 px-4 py-3 dark:border-zinc-800/80">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-violet-500/10 text-violet-700 dark:text-violet-400">
              <MessageCircle className="size-4" aria-hidden="true" />
            </div>
            <div className="min-w-0">
              <p className="text-muted-foreground text-[10px] font-semibold uppercase tracking-[0.14em]">Conversation</p>
              <div className="mt-0.5 flex items-center gap-2">
                <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
                  <span className={cn('size-1.5 rounded-full', streaming ? 'animate-pulse bg-violet-500' : 'bg-zinc-400 dark:bg-zinc-600')} aria-hidden="true" />
                  {streaming ? 'Generating' : 'Ready'}
                </span>
                {stats && <span className="text-muted-foreground hidden truncate text-xs sm:inline">· {stats}</span>}
              </div>
            </div>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="shrink-0 border-zinc-200/80 dark:border-zinc-800/80"
            aria-controls="playground-settings"
            ref={settingsTrigger}
            aria-expanded={settingsOpen}
            aria-label={settingsOpen ? 'Hide inspector' : 'Show inspector'}
            title={settingsOpen ? 'Hide inspector' : 'Show inspector'}
            onClick={() => setSettingsOpen((open) => !open)}
          >
            {settingsOpen ? <PanelRightClose /> : <PanelRightOpen />}
            <span className="hidden sm:inline">{settingsOpen ? 'Hide inspector' : 'Inspector'}</span>
          </Button>
        </div>

        <div ref={scroller} onScroll={handleConversationScroll} className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-5 sm:px-8 sm:py-7">
          {messages.length === 0 ? (
            <div className="flex min-h-full items-center justify-center py-8">
              <div className="w-full max-w-2xl">
                <div className="mb-6 flex flex-col items-center text-center">
                  <div className="mb-4 flex size-11 items-center justify-center rounded-xl border border-violet-500/20 bg-violet-500/10 text-violet-700 dark:text-violet-400">
                    <Bot className="size-5" aria-hidden="true" />
                  </div>
                  <h3 className="text-base font-semibold tracking-tight">Ready when you are</h3>
                  <p className="text-muted-foreground mt-1 max-w-md text-sm">Send a prompt to explore the model, or start with one of these examples.</p>
                </div>
                <div className="grid gap-2 sm:grid-cols-3">
                  {suggestions.map((suggestion) => (
                    <Button
                      key={suggestion}
                      type="button"
                      variant="outline"
                      className="h-auto min-h-16 justify-between gap-3 whitespace-normal rounded-xl border-zinc-200/80 bg-background/70 px-3 py-3 text-left text-xs leading-5 hover:border-violet-500/50 hover:bg-violet-500/5 dark:border-zinc-800/80"
                      onClick={() => {
                        setInput(suggestion)
                        requestAnimationFrame(() => document.getElementById('playground-message')?.focus())
                      }}
                    >
                      <span>{suggestion}</span>
                      <span className="shrink-0 text-[10px] font-semibold uppercase tracking-[0.12em] text-violet-700 dark:text-violet-400">Use</span>
                    </Button>
                  ))}
                </div>
              </div>
            </div>
          ) : (
            <div className="mx-auto flex w-full max-w-3xl flex-col gap-8 pb-4" role="log" aria-label="Chat messages" aria-live="polite">
              {messages.map((m, i) => {
                const isUser = m.role === 'user'
                return isUser ? (
                  <div key={i} className="flex items-end justify-end gap-2.5">
                    <div className="max-w-[80%] rounded-2xl rounded-br-md border border-zinc-200/90 bg-zinc-100 px-4 py-3 text-sm leading-6 text-zinc-800 shadow-sm dark:border-zinc-800 dark:bg-zinc-900/80 dark:text-zinc-100 sm:max-w-[75%]">
                      <div className="mb-1 text-[11px] font-semibold text-zinc-500 dark:text-zinc-400">You</div>
                      {Array.isArray(m.content) ? m.content.map((part, index) => (
                        part.type === 'text' ? <ChatMarkdown key={index}>{part.text}</ChatMarkdown> : (
                          <img key={index} src={part.image_url.url} alt={`Attached image ${index + 1}`} className="my-2 max-h-64 max-w-full rounded-lg object-contain" />
                        )
                      )) : m.content ? <ChatMarkdown>{m.content}</ChatMarkdown> : null}
                    </div>
                    <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-zinc-100 text-zinc-500 dark:bg-zinc-900 dark:text-zinc-400" aria-hidden="true">
                      <UserRound className="size-3.5" />
                    </div>
                  </div>
                ) : (
                  <div key={i} className="flex gap-3 sm:gap-4">
                    <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg bg-violet-500/10 text-violet-700 dark:text-violet-400" aria-hidden="true">
                      <Bot className="size-3.5" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="mb-1.5 flex items-center gap-2">
                        <span className="text-xs font-semibold">Assistant</span>
                        {streaming && i === messages.length - 1 && <span className="text-violet-700 inline-flex items-center gap-1 text-[11px] dark:text-violet-400"><span className="size-1.5 animate-pulse rounded-full bg-current" /> responding</span>}
                      </div>
                      <div className="max-w-none text-[15px] leading-7 text-foreground">
                        {m.reasoning || (thinking && streaming && i === messages.length - 1) ? (
                          <details className="mb-3 rounded-xl border border-zinc-200/80 bg-zinc-50/70 px-3 py-2 text-xs dark:border-zinc-800 dark:bg-zinc-900/50" open={streaming && i === messages.length - 1}>
                            <summary className="text-muted-foreground cursor-pointer select-none">
                              <span className="font-medium">Thinking</span>{' '}
                              <span>{m.reasoning ? `(${m.reasoning.length.toLocaleString()} chars)` : '(in progress)'}</span>
                            </summary>
                            <div className="mt-2 max-h-64 overflow-y-auto border-t border-zinc-200/80 pt-2 text-muted-foreground dark:border-zinc-800">
                              <ChatMarkdown>{m.reasoning || 'Waiting for reasoning…'}</ChatMarkdown>
                            </div>
                          </details>
                        ) : null}
                        {Array.isArray(m.content) ? m.content.map((part, index) => (
                          part.type === 'text' ? <ChatMarkdown key={index}>{part.text}</ChatMarkdown> : (
                            <img key={index} src={part.image_url.url} alt={`Attached image ${index + 1}`} className="my-2 max-h-64 max-w-full rounded-lg object-contain" />
                          )
                        )) : m.content ? <ChatMarkdown>{m.content}</ChatMarkdown> : (streaming && i === messages.length - 1 ? <span className="text-muted-foreground animate-pulse">…</span> : null)}
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>

        <div className="shrink-0 border-t border-zinc-200/80 bg-background/90 px-3 py-3 dark:border-zinc-800/80 sm:px-5 sm:py-4">
          <div className="mx-auto w-full max-w-3xl">
          {images.length > 0 && (
            <div className="mb-3 rounded-xl border border-zinc-200/80 bg-zinc-50/70 p-2 dark:border-zinc-800 dark:bg-zinc-900/50">
              <div className="text-muted-foreground mb-2 text-[10px] font-semibold uppercase tracking-[0.14em]">Attachments</div>
              <div className="flex max-h-28 flex-wrap gap-2 overflow-y-auto p-2">
                {images.map((image, index) => (
                  <div key={index} className="relative rounded-lg border border-zinc-200/80 bg-background p-1 shadow-sm dark:border-zinc-800">
                    <img src={image.url} alt={image.name} title={image.name} className="h-16 w-16 rounded object-cover sm:h-20 sm:w-20" />
                    <Button type="button" size="icon" variant="secondary" className="absolute -top-2 -right-2 size-6" aria-label={`Remove ${image.name}`} onClick={() => setImages((current) => current.filter((_, i) => i !== index))}>
                      <X className="size-3" />
                    </Button>
                  </div>
                ))}
              </div>
            </div>
          )}
          <div className="rounded-2xl border border-zinc-200/90 bg-card p-2 shadow-sm transition-colors focus-within:border-violet-500/70 focus-within:ring-4 focus-within:ring-violet-500/10 dark:border-zinc-800">
            <Textarea
              id="playground-message"
              rows={2}
              value={input}
              aria-label="Message"
              placeholder="Message the model…"
              className="min-h-20 max-h-44 resize-none border-0 bg-transparent px-2 py-1 text-[15px] leading-6 shadow-none focus-visible:ring-0"
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing || e.keyCode === 229) return
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  void send()
                }
              }}
            />
            <div className="mt-2 flex items-center justify-between gap-2 border-t border-zinc-200/80 px-1 pt-2 dark:border-zinc-800">
              <div className="flex min-w-0 items-center gap-2">
                {supportsVision && (
                  <>
                    <input ref={imageInput} type="file" accept="image/png,image/jpeg,image/webp,image/gif" multiple className="hidden" aria-label="Attach images" onChange={(e) => {
                      const files = Array.from(e.target.files ?? [])
                      e.target.value = ''
                      void attachImages(files)
                    }} />
                    <Button type="button" variant="ghost" size="icon-sm" aria-label="Attach images" title="Attach images (up to 5 MB each)" disabled={streaming || readingImages} onClick={() => imageInput.current?.click()}>
                      <ImagePlus />
                    </Button>
                  </>
                )}
                <span className="text-muted-foreground hidden truncate text-[11px] sm:inline">Enter sends · Shift+Enter for a new line</span>
              </div>
              {streaming ? (
                <Button type="button" variant="outline" size="sm" className="shrink-0 border-zinc-200/80 dark:border-zinc-800" aria-label="Stop generating" onClick={() => session.abort.current?.abort()}>
                  <Square /> <span className="hidden sm:inline">Stop</span>
                </Button>
              ) : (
                <Button type="button" size="sm" className="shrink-0 bg-primary text-primary-foreground hover:bg-primary/90" aria-label="Send message" onClick={() => void send()} disabled={(!input.trim() && !images.length) || readingImages}>
                  <Send /> <span className="hidden sm:inline">Send</span>
                </Button>
              )}
            </div>
          </div>
          <div className="text-muted-foreground mt-2 flex min-h-5 items-center justify-between gap-2 px-1 text-xs">
            <span aria-live="polite" className="truncate">{readingImages ? 'Reading images…' : stats}</span>
            <Button type="button" variant="ghost" size="sm" className="h-7 shrink-0 px-2 text-xs text-muted-foreground" disabled={streaming || readingImages} onClick={() => { setMessages([]); setImages([]); setStats('') }}>
              <Trash2 className="size-3" /> Clear
            </Button>
          </div>
          </div>
        </div>
      </Card>

      {settingsOpen && (
        <Card id="playground-settings" onKeyDown={event => { if (event.key === 'Escape') { event.preventDefault(); setSettingsOpen(false) } }} className="flex min-h-0 w-full flex-col gap-0 overflow-hidden rounded-none border-0 border-l py-0 shadow-none lg:h-full lg:w-72 lg:shrink-0">
          <div className="flex shrink-0 items-center justify-between gap-3 border-b border-zinc-200/80 bg-card/95 px-4 py-3 dark:border-zinc-800/80">
            <div className="flex min-w-0 items-center gap-2.5">
              <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-violet-500/10 text-violet-700 dark:text-violet-400">
                <Settings2 className="size-4" aria-hidden="true" />
              </div>
              <div className="min-w-0">
                <h2 className="text-sm font-semibold">Inspector</h2>
                <p className="text-muted-foreground mt-0.5 text-[11px]">Response controls</p>
              </div>
            </div>
            <Button ref={settingsClose} type="button" variant="ghost" size="icon-sm" aria-label="Hide inspector" title="Hide inspector" onClick={() => setSettingsOpen(false)}>
              <PanelRightClose />
            </Button>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
            <div className="grid gap-5">
              <section className="grid gap-2">
                <Label htmlFor="playground-system" className="text-[10px] font-semibold uppercase tracking-[0.14em]">System prompt</Label>
                <Textarea id="playground-system" rows={4} value={system} onChange={(e) => setSystem(e.target.value)} placeholder="Optional instructions" className="resize-y text-xs" aria-describedby="playground-system-help" />
                <p id="playground-system-help" className="text-muted-foreground text-[11px]">Applied before the conversation.</p>
              </section>
              <section className="grid gap-2">
                <Label className="flex items-center justify-between text-[10px] font-semibold uppercase tracking-[0.14em]">
                  <span>Temperature</span>
                  <span className="font-mono text-sm normal-case tracking-normal text-foreground">{temperature.toFixed(2)}</span>
                </Label>
                <TemperatureSlider id="playground-temperature" value={temperature} onChange={setTemperature} disabled={streaming} />
                <p className="text-muted-foreground text-[11px]">Focused to exploratory.</p>
              </section>
              <section className="grid gap-2">
                <Label htmlFor="playground-max-tokens" className="text-[10px] font-semibold uppercase tracking-[0.14em]">Max tokens</Label>
                <Input id="playground-max-tokens" type="number" min={1} max={32768} value={maxTokens} onChange={(e) => setMaxTokens(Number(e.target.value) || 1)} aria-describedby="playground-max-tokens-help" className="font-mono text-xs" />
                <p id="playground-max-tokens-help" className="text-muted-foreground text-[11px]">Maximum response length.</p>
              </section>
              <section className="flex items-center justify-between gap-3 rounded-xl border border-zinc-200/80 bg-zinc-50/70 p-3 dark:border-zinc-800 dark:bg-zinc-900/50">
                <Label htmlFor="playground-thinking" className="min-w-0 cursor-pointer">
                  <span className="block text-sm font-medium">Thinking</span>
                  <span className="text-muted-foreground mt-0.5 block text-[11px] font-normal">Allow extra reasoning.</span>
                </Label>
                <Switch id="playground-thinking" checked={thinking} onCheckedChange={setThinking} disabled={streaming} />
              </section>
              <p className="text-muted-foreground border-t border-zinc-200/80 pt-3 text-[11px] dark:border-zinc-800">Controls apply to the next response.</p>
            </div>
          </div>
        </Card>
      )}
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
    <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 overflow-auto p-4 lg:grid-cols-2">
      <Card className="min-h-80 min-w-0 gap-3 overflow-auto py-4 lg:min-h-0">
        <CardContent className="flex min-h-0 min-w-0 flex-1 flex-col gap-3">
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
      <Card className="min-h-80 min-w-0 gap-3 overflow-auto py-4 lg:min-h-0">
        <CardContent className="flex min-h-0 min-w-0 flex-1 flex-col gap-3">
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
      <pre className="bg-background min-h-24 flex-1 overflow-auto rounded-md border p-3 font-mono text-xs">{JSON.stringify(result.output, null, 2)}</pre>
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
            <div className="h-full bg-primary" style={{ width: `${Math.max(1, s.score * 100)}%` }} />
          </div>
          <span className="text-muted-foreground text-right">{(s.score * 100).toFixed(1)}%</span>
        </div>
      ))}
    </div>
  )
}
