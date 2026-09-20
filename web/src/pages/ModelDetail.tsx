import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { ArrowLeft, Pause, Play, Trash } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { StatusBadge } from '@/components/StatusBadge'
import { ModelActions } from '@/components/ModelActions'
import { ModelForm } from '@/components/ModelForm'
import { api, streamLogs } from '@/lib/api'
import type { LogLine, Model } from '@/lib/types'
import { cn, uptimeSince } from '@/lib/utils'

export function ModelDetailPage() {
  const { id = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const tab = params.get('tab') ?? 'overview'
  const [model, setModel] = useState<Model | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [editOpen, setEditOpen] = useState(false)

  const load = useCallback(async () => {
    try {
      setModel(await api.getModel(id))
      setError(null)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [id])

  useEffect(() => {
    load()
    const t = setInterval(load, 3000)
    return () => clearInterval(t)
  }, [load])

  if (error)
    return (
      <div className="flex flex-col gap-4">
        <BackLink />
        <Card className="border-destructive/40">
          <CardContent className="text-destructive text-sm">{error}</CardContent>
        </Card>
      </div>
    )
  if (!model) return <div className="text-muted-foreground text-sm">Loading…</div>

  const live = model.live
  return (
    <div className="flex flex-col gap-6">
      <BackLink />
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-3">
            <h1 className="truncate text-2xl font-semibold tracking-tight">{model.name}</h1>
            <StatusBadge state={live.state} />
          </div>
          <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-2 text-sm">
            <Badge variant="outline">{model.runtime}</Badge>
            <Badge variant="secondary">{model.task}</Badge>
            <span className="font-mono text-xs">{model.id}</span>
            {model.description && <span>· {model.description}</span>}
          </div>
        </div>
        <ModelActions model={model} onChanged={(m) => (m ? setModel(m) : load())} onEdit={() => setEditOpen(true)} />
      </div>

      {live.state === 'failed' && live.error && (
        <Card className="border-destructive/40">
          <CardContent className="text-sm">
            <div className="text-destructive font-medium">Last error</div>
            <pre className="mt-1 whitespace-pre-wrap font-mono text-xs">{live.error}</pre>
          </CardContent>
        </Card>
      )}

      <Tabs value={tab} onValueChange={(v) => setParams(v === 'overview' ? {} : { tab: v })}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="api">API</TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="mt-2">
          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Registry</CardTitle>
              </CardHeader>
              <CardContent>
                <dl className="grid grid-cols-[8rem_1fr] gap-y-2 text-sm">
                  <dt className="text-muted-foreground">Path</dt>
                  <dd className="break-all font-mono text-xs">{model.model_path}</dd>
                  <dt className="text-muted-foreground">Created</dt>
                  <dd>{new Date(model.created_at).toLocaleString()}</dd>
                  <dt className="text-muted-foreground">Updated</dt>
                  <dd>{new Date(model.updated_at).toLocaleString()}</dd>
                  <dt className="text-muted-foreground">Config</dt>
                  <dd>
                    <pre className="bg-muted/50 max-h-48 overflow-auto rounded-md p-2 font-mono text-xs">{JSON.stringify(model.config ?? {}, null, 2)}</pre>
                  </dd>
                </dl>
                <Button variant="outline" size="sm" className="mt-3" onClick={() => setEditOpen(true)}>
                  Edit settings
                </Button>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Runtime</CardTitle>
              </CardHeader>
              <CardContent>
                <dl className="grid grid-cols-[8rem_1fr] gap-y-2 text-sm">
                  <dt className="text-muted-foreground">State</dt>
                  <dd>{live.state}</dd>
                  {live.pid ? (
                    <>
                      <dt className="text-muted-foreground">PID</dt>
                      <dd>{live.pid}</dd>
                    </>
                  ) : null}
                  {live.port ? (
                    <>
                      <dt className="text-muted-foreground">Internal port</dt>
                      <dd>{live.port}</dd>
                    </>
                  ) : null}
                  {live.started_at && (
                    <>
                      <dt className="text-muted-foreground">Uptime</dt>
                      <dd>{uptimeSince(live.started_at)}</dd>
                    </>
                  )}
                  {live.details && Object.keys(live.details).length > 0 && (
                    <>
                      <dt className="text-muted-foreground">Details</dt>
                      <dd>
                        <pre className="bg-muted/50 max-h-64 overflow-auto rounded-md p-2 font-mono text-xs">{JSON.stringify(live.details, null, 2)}</pre>
                      </dd>
                    </>
                  )}
                </dl>
              </CardContent>
            </Card>
          </div>
        </TabsContent>
        <TabsContent value="logs" className="mt-2">
          <LogViewer id={model.id} />
        </TabsContent>
        <TabsContent value="api" className="mt-2">
          <ApiExamples model={model} />
        </TabsContent>
      </Tabs>

      <ModelForm open={editOpen} onOpenChange={setEditOpen} model={model} onSaved={setModel} />
    </div>
  )
}

function BackLink() {
  return (
    <Link to="/models" className="text-muted-foreground hover:text-foreground inline-flex w-fit items-center gap-1 text-sm">
      <ArrowLeft className="size-4" /> All models
    </Link>
  )
}

function LogViewer({ id }: { id: string }) {
  const [lines, setLines] = useState<LogLine[]>([])
  const [paused, setPaused] = useState(false)
  const [filter, setFilter] = useState('')
  const box = useRef<HTMLDivElement>(null)
  const pausedRef = useRef(paused)
  pausedRef.current = paused

  useEffect(() => {
    // The stream replays the recent tail before live lines, so no separate fetch is needed.
    setLines([])
    return streamLogs(id, (l) => {
      if (pausedRef.current) return
      setLines((xs) => (xs.length > 3000 ? [...xs.slice(-2500), l] : [...xs, l]))
    })
  }, [id])

  useEffect(() => {
    if (!paused && box.current) box.current.scrollTop = box.current.scrollHeight
  }, [lines, paused])

  const shown = filter ? lines.filter((l) => l.text.toLowerCase().includes(filter.toLowerCase())) : lines

  return (
    <Card className="gap-2 py-3">
      <div className="flex items-center gap-2 px-4">
        <input
          className="bg-transparent text-sm outline-none placeholder:text-muted-foreground flex-1"
          placeholder="Filter log lines…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <span className="text-muted-foreground text-xs">{shown.length} lines</span>
        <Button variant="ghost" size="sm" onClick={() => setPaused((p) => !p)}>
          {paused ? <Play /> : <Pause />} {paused ? 'Resume' : 'Pause'}
        </Button>
        <Button variant="ghost" size="sm" onClick={() => setLines([])}>
          <Trash /> Clear
        </Button>
      </div>
      <div ref={box} className="bg-black/40 mx-4 h-[60vh] overflow-auto rounded-md p-3 font-mono text-xs leading-5">
        {shown.length === 0 && <div className="text-muted-foreground">No log output yet. Load the model to see runtime logs.</div>}
        {shown.map((l, i) => (
          <div key={i} className="flex gap-2 whitespace-pre-wrap break-all">
            <span className="text-muted-foreground shrink-0 select-none">{new Date(l.time).toLocaleTimeString()}</span>
            <span className={cn('shrink-0 select-none w-14', l.source === 'stderr' ? 'text-amber-400/80' : l.source === 'system' ? 'text-violet-300' : 'text-emerald-300/80')}>{l.source}</span>
            <span>{l.text}</span>
          </div>
        ))}
      </div>
    </Card>
  )
}

function ApiExamples({ model }: { model: Model }) {
  const base = window.location.origin
  const n = model.name
  const examples: { title: string; body: string }[] = []
  if (model.task === 'chat') {
    examples.push({
      title: 'Chat completion (OpenAI-compatible)',
      body: `curl ${base}/v1/chat/completions \\\n  -H 'Content-Type: application/json' \\\n  -d '{"model":"${n}","messages":[{"role":"user","content":"Hello!"}]}'`,
    })
  }
  if (model.task === 'completion' || model.task === 'chat') {
    examples.push({
      title: 'Text completion',
      body: `curl ${base}/v1/completions \\\n  -H 'Content-Type: application/json' \\\n  -d '{"model":"${n}","prompt":"Once upon a time","max_tokens":64}'`,
    })
  }
  if (model.task === 'embedding') {
    examples.push({
      title: 'Embeddings (OpenAI-compatible)',
      body: `curl ${base}/v1/embeddings \\\n  -H 'Content-Type: application/json' \\\n  -d '{"model":"${n}","input":["hello world"]}'`,
    })
  }
  const predictInput =
    model.task === 'classification' || model.task === 'embedding'
      ? '"The movie was wonderful"'
      : model.task === 'custom'
        ? '{"input_name":{"shape":[1,4],"data":[1,2,3,4]}}'
        : '"Hello!"'
  examples.push({
    title: 'Generic predict',
    body: `curl ${base}/v1/models/${n}/predict \\\n  -H 'Content-Type: application/json' \\\n  -d '{"input":${predictInput}}'`,
  })
  examples.push({ title: 'Load / unload', body: `curl -X POST ${base}/api/models/${n}/load\ncurl -X POST ${base}/api/models/${n}/unload` })
  return (
    <div className="grid gap-4">
      {examples.map((e) => (
        <Card key={e.title} className="gap-2 py-3">
          <CardHeader className="py-0">
            <CardTitle className="text-sm">{e.title}</CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="bg-black/40 overflow-x-auto rounded-md p-3 font-mono text-xs">{e.body}</pre>
          </CardContent>
        </Card>
      ))}
      <p className="text-muted-foreground text-xs">When an API key is configured, add <code>-H 'Authorization: Bearer &lt;key&gt;'</code>.</p>
    </div>
  )
}
