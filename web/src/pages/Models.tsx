import { useMemo, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Activity, Boxes, CircleAlert, Cpu, FlaskConical, LoaderCircle, Plus, RefreshCw, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { StatusBadge } from '@/components/StatusBadge'
import { ModelForm } from '@/components/ModelForm'
import { ModelActions } from '@/components/ModelActions'
import { useModels } from '@/hooks/useModels'
import type { Model, State } from '@/lib/types'
import { cn, uptimeSince } from '@/lib/utils'

type ModelFilter = 'all' | 'running' | 'stopped' | 'failed'
type ModelCounts = Record<ModelFilter, number>
type RuntimeSummary = Record<Model['runtime'], { registered: number; running: number }>

const filters: { key: ModelFilter; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'running', label: 'Running' },
  { key: 'stopped', label: 'Stopped' },
  { key: 'failed', label: 'Failed' },
]

export function ModelsPage() {
  const { models, error, refresh } = useModels()
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<ModelFilter>('all')
  const [refreshing, setRefreshing] = useState(false)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Model | undefined>()

  const counts = useMemo(() => {
    if (!models) return null

    return {
      all: models.length,
      running: models.filter((model) => isRunning(model.live.state)).length,
      stopped: models.filter((model) => model.live.state === 'stopped').length,
      failed: models.filter((model) => model.live.state === 'failed').length,
    } satisfies ModelCounts
  }, [models])

  const runtimeSummary = useMemo(() => {
    if (!models) return null

    return models.reduce<RuntimeSummary>(
      (summary, model) => {
        summary[model.runtime].registered += 1
        if (isRunning(model.live.state)) summary[model.runtime].running += 1
        return summary
      },
      {
        llamacpp: { registered: 0, running: 0 },
        onnx: { registered: 0, running: 0 },
      },
    )
  }, [models])

  const filtered = useMemo(() => {
    if (!models) return []

    const normalizedQuery = query.trim().toLowerCase()

    return models.filter((model) => {
      const matchesFilter = filter === 'all' || matchesModelFilter(model, filter)
      if (!matchesFilter) return false
      if (!normalizedQuery) return true

      return [model.name, model.description, model.runtime, model.task, model.model_path].some((value) => value.toLowerCase().includes(normalizedQuery))
    })
  }, [filter, models, query])

  async function handleRefresh() {
    setRefreshing(true)
    try {
      await refresh()
    } finally {
      setRefreshing(false)
    }
  }

  function openNewModelForm() {
    setEditing(undefined)
    setFormOpen(true)
  }

  return (
    <div className="flex min-h-0 flex-col gap-5">
      <header className="flex flex-wrap items-center justify-between gap-4 border-b border-border/70 pb-5">
        <div className="min-w-0">
          <div className="flex items-baseline gap-2">
            <h1 className="font-serif text-3xl font-medium leading-none tracking-[-0.04em]">Models</h1>
            <span className="reference-tag border-violet-400/25 bg-violet-500/10 text-violet-700 dark:text-violet-300">registry</span>
          </div>
          <p className="mt-2 text-sm text-muted-foreground">Registered models and runtime state.</p>
        </div>

        <div className="flex items-center gap-2">
          <Button className="rounded-full" variant="outline" size="icon-sm" onClick={() => void handleRefresh()} disabled={refreshing} aria-label="Refresh model registry" title="Refresh model registry">
            <RefreshCw className={cn(refreshing && 'animate-spin')} />
          </Button>
          <Button className="rounded-full px-4" size="sm" onClick={openNewModelForm}>
            <Plus /> Add model
          </Button>
        </div>
      </header>

      <div className="flex flex-col gap-3 border-b border-border/70 pb-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex w-fit items-center gap-1 rounded-full border bg-card/70 p-1" role="group" aria-label="Filter models by status">
          {filters.map((item) => (
            <button
              key={item.key}
              type="button"
              aria-pressed={filter === item.key}
              onClick={() => setFilter(item.key)}
              className={cn(
                'h-7 rounded-full px-3 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
                filter === item.key ? 'bg-primary text-primary-foreground shadow-sm' : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
              )}
            >
              {item.label}
              <span className={cn('ml-1 font-mono text-[10px]', filter === item.key ? 'text-primary-foreground/75' : 'text-muted-foreground/75')}>
                {counts ? counts[item.key] : '—'}
              </span>
            </button>
          ))}
        </div>

        <div className="relative w-full sm:max-w-xs">
          <Search className="absolute top-2.5 left-3 size-3.5 text-muted-foreground" aria-hidden="true" />
          <Input className="h-9 rounded-full border bg-card/70 pl-9 text-xs" placeholder="Search name, path, runtime…" value={query} onChange={(event) => setQuery(event.target.value)} aria-label="Search models" />
        </div>
      </div>

      {error && models && (
        <div role="alert" className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2.5 text-sm">
          <CircleAlert className="mt-0.5 size-4 shrink-0 text-destructive" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <p className="font-medium text-destructive">Unable to refresh model registry</p>
            <p className="mt-0.5 break-words text-xs text-muted-foreground">{error}</p>
          </div>
          <Button variant="ghost" size="sm" onClick={() => void handleRefresh()} disabled={refreshing}>
            Retry
          </Button>
        </div>
      )}

      {models && models.length > 0 && counts && runtimeSummary && <ModelsOverview counts={counts} runtimeSummary={runtimeSummary} />}

      {!models ? (
        error ? (
          <StatePanel
            icon={<CircleAlert className="size-5 text-destructive" />}
            title="Model registry unavailable"
            detail={error}
            action={<Button variant="outline" size="sm" onClick={() => void handleRefresh()} disabled={refreshing}>Retry</Button>}
          />
        ) : (
          <StatePanel icon={<LoaderCircle className="size-5 animate-spin text-primary" />} title="Loading model registry" detail="Waiting for the server response." />
        )
      ) : models.length === 0 ? (
        <StatePanel
          icon={<Boxes className="size-5 text-primary" />}
          title="No models registered"
          detail="Register a model path to make it available to the server."
          action={<Button size="sm" onClick={openNewModelForm}><Plus /> Add model</Button>}
        />
      ) : filtered.length === 0 ? (
        <StatePanel
          icon={<Search className="size-5 text-muted-foreground" />}
          title="No matching models"
          detail={query.trim() ? `No models match “${query.trim()}”.` : `No models are ${filter}.`}
          action={
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setQuery('')
                setFilter('all')
              }}
            >
              Clear filters
            </Button>
          }
        />
      ) : (
        <ModelsTable models={filtered} onRefresh={refresh} onEdit={(model) => { setEditing(model); setFormOpen(true) }} />
      )}

      <ModelForm open={formOpen} onOpenChange={setFormOpen} model={editing} onSaved={() => void refresh()} />
    </div>
  )
}

function ModelsOverview({ counts, runtimeSummary }: { counts: ModelCounts; runtimeSummary: RuntimeSummary }) {
  const runtimeTotal = counts.all
  const runtimeRows: { key: Model['runtime']; label: string; icon: typeof Boxes; tone: string }[] = [
    { key: 'llamacpp', label: 'llama.cpp', icon: Boxes, tone: 'from-violet-400 to-indigo-300' },
    { key: 'onnx', label: 'ONNX Runtime', icon: Cpu, tone: 'from-sky-400 to-cyan-300' },
  ]

  return (
    <section className="grid gap-3 lg:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)]" aria-label="Model registry overview">
      <div className="reference-panel registry-highlight relative isolate min-h-[168px] overflow-hidden px-4 py-4 sm:px-5">
        <div className="pointer-events-none absolute -top-24 right-[-2rem] -z-10 size-64 rounded-full bg-violet-500/20 blur-3xl" aria-hidden="true" />
        <div className="relative flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-violet-700 dark:text-violet-300">
              <Activity className="size-3.5" aria-hidden="true" />
              Registry pulse
            </div>
            <p className="mt-1 text-xs text-muted-foreground">A live view of the registered model inventory.</p>
          </div>
          <span className="reference-tag border-violet-400/25 bg-violet-500/10 text-violet-700 dark:text-violet-200">{counts.all} registered</span>
        </div>

        <div className="relative mt-6 grid grid-cols-3 divide-x divide-violet-300/15">
          <OverviewMetric label="Registered" value={counts.all} />
          <OverviewMetric label="Running" value={counts.running} tone="text-violet-700 dark:text-violet-200" />
          <OverviewMetric label="Failed" value={counts.failed} tone={counts.failed ? 'text-destructive' : undefined} />
        </div>
      </div>

      <div className="reference-panel min-h-[168px] bg-card/80 px-4 py-4 sm:px-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-[10px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">Runtime distribution</h2>
            <p className="mt-1 text-xs text-muted-foreground">Registered models by backend.</p>
          </div>
          <span className="font-mono text-[10px] text-muted-foreground">{runtimeTotal} total</span>
        </div>

        <div className="mt-4 space-y-3">
          {runtimeRows.map(({ key, label, icon: Icon, tone }) => {
            const summary = runtimeSummary[key]
            const share = runtimeTotal ? Math.round((summary.registered / runtimeTotal) * 100) : 0

            return (
              <div key={key}>
                <div className="flex items-center justify-between gap-3 text-xs">
                  <div className="flex min-w-0 items-center gap-2">
                    <Icon className="size-3.5 shrink-0 text-primary" aria-hidden="true" />
                    <span className="truncate font-medium">{label}</span>
                  </div>
                  <span className="shrink-0 font-mono text-[10px] text-muted-foreground">
                    {summary.registered} registered · {summary.running} running
                  </span>
                </div>
                <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted" aria-label={`${label}: ${share}% of registered models`} role="img">
                  <div className={cn('h-full rounded-full bg-gradient-to-r', tone)} style={{ width: `${share}%` }} />
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </section>
  )
}

function OverviewMetric({ label, value, tone }: { label: string; value: number; tone?: string }) {
  return (
    <div className="px-3 first:pl-0 last:pr-0">
      <div className={cn('font-mono text-2xl font-medium tracking-[-0.04em]', tone)}>{value}</div>
      <div className="mt-1 text-[10px] uppercase tracking-[0.14em] text-muted-foreground">{label}</div>
    </div>
  )
}

function ModelsTable({ models, onRefresh, onEdit }: { models: Model[]; onRefresh: () => Promise<Model[] | null>; onEdit: (model: Model) => void }) {
  return (
    <section className="reference-panel overflow-hidden bg-card/80" aria-label="Registered models">
      <div className="flex items-center justify-between border-b px-4 py-3">
        <h2 className="text-xs font-semibold uppercase tracking-[0.12em] text-muted-foreground">Registered models</h2>
        <span className="font-mono text-[11px] text-muted-foreground">{models.length} shown</span>
      </div>
      <div className="max-w-full overflow-x-auto">
        <table className="w-full min-w-[760px] table-fixed text-[13px]">
          <thead className="bg-muted/30 text-left text-[10px] uppercase tracking-[0.12em] text-muted-foreground">
            <tr className="border-b">
              <th className="px-3 py-2 font-medium">Model</th>
              <th className="w-32 px-3 py-2 font-medium">Runtime</th>
              <th className="w-28 px-3 py-2 font-medium">Task</th>
              <th className="w-36 px-3 py-2 font-medium">Status</th>
              <th className="w-44 px-3 py-2 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {models.map((model) => (
              <tr key={model.id} className="border-b last:border-b-0 hover:bg-violet-500/5">
                <td className="max-w-0 px-3 py-2.5">
                  <div className="flex min-w-0 items-center gap-2.5">
                    <RuntimeIcon runtime={model.runtime} />
                    <div className="min-w-0">
                      <Link to={`/models/${model.id}`} className="block truncate font-medium text-foreground hover:text-primary hover:underline">
                        {model.name}
                      </Link>
                      <div className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground" title={model.model_path}>
                        {model.model_path}
                      </div>
                    </div>
                  </div>
                </td>
                <td className="max-w-0 px-3 py-2.5">
                  <div className="font-medium">{model.runtime === 'llamacpp' ? 'llama.cpp' : 'ONNX Runtime'}</div>
                  <div className="font-mono text-[10px] text-muted-foreground">{model.runtime}</div>
                </td>
                <td className="px-3 py-2.5 font-mono text-xs text-muted-foreground">{model.task}</td>
                <td className="max-w-0 px-3 py-2.5">
                  <div className="flex min-w-0 flex-col items-start gap-1">
                    <StatusBadge state={model.live.state} />
                    <StatusDetail model={model} />
                  </div>
                </td>
                <td className="px-3 py-2.5">
                  <div className="flex items-center justify-end gap-1">
                    {model.live.state === 'running' && (
                      <Button asChild variant="ghost" size="icon-sm" aria-label={`Open playground for ${model.name}`} title={`Open playground for ${model.name}`}>
                        <Link to={`/playground/${model.name}`}>
                          <FlaskConical aria-hidden="true" />
                        </Link>
                      </Button>
                    )}
                    <ModelActions model={model} compact onChanged={() => void onRefresh()} onEdit={() => onEdit(model)} />
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="flex justify-end border-t px-3 py-1.5 text-[11px] text-muted-foreground">Updates automatically</div>
    </section>
  )
}

function RuntimeIcon({ runtime }: { runtime: Model['runtime'] }) {
  const Icon = runtime === 'llamacpp' ? Boxes : Cpu

  return (
    <div className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-violet-400/20 bg-violet-500/5 text-primary" title={runtime === 'llamacpp' ? 'llama.cpp' : 'ONNX Runtime'}>
      <Icon className="size-4" aria-hidden="true" />
    </div>
  )
}

function StatusDetail({ model }: { model: Model }) {
  if (model.live.state === 'running') {
    return <span className="truncate text-[11px] text-muted-foreground">up {uptimeSince(model.live.started_at)}{model.live.port ? ` · :${model.live.port}` : ''}</span>
  }

  if (model.live.state === 'failed' && model.live.error) {
    return <span className="line-clamp-1 max-w-[15rem] text-[11px] text-destructive" title={model.live.error}>{model.live.error}</span>
  }

  return <span className="text-[11px] text-muted-foreground">{model.live.state === 'stopped' ? 'idle' : 'transitioning'}</span>
}

function StatePanel({ icon, title, detail, action }: { icon: ReactNode; title: string; detail: string; action?: ReactNode }) {
  return (
    <section className="reference-panel flex min-h-40 flex-col items-center justify-center gap-2 border-dashed bg-card px-4 py-10 text-center" aria-live="polite">
      {icon}
      <h2 className="text-sm font-semibold">{title}</h2>
      <p className="max-w-md text-xs text-muted-foreground">{detail}</p>
      {action}
    </section>
  )
}

function isRunning(state: State) {
  return state === 'running'
}

function matchesModelFilter(model: Model, filter: ModelFilter) {
  if (filter === 'running') return isRunning(model.live.state)
  if (filter === 'stopped') return model.live.state === 'stopped'
  if (filter === 'failed') return model.live.state === 'failed'
  return true
}
