import { Link } from 'react-router-dom'
import { ArrowUpRight, Boxes, CircleAlert, Cpu, RefreshCw, Server } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/StatusBadge'
import { ResourceMetrics } from '@/components/ResourceMetrics'
import { LiveLogsPanel } from '@/components/LiveLogsPanel'
import { useSystem } from '@/hooks/useSystem'
import { useModels } from '@/hooks/useModels'
import { formatDuration, uptimeSince } from '@/lib/utils'

export function DashboardPage() {
  const { info, error, updatedAt, refreshing, refresh } = useSystem()
  const { models, error: modelError, refresh: refreshModels } = useModels(3000)
  const loaded = models?.filter(model => model.live.state === 'running') ?? []
  return <div className="flex flex-col gap-5">
    <header className="flex flex-wrap items-center justify-between gap-4">
      <div><h1>Dashboard</h1><p className="mt-2 text-xs text-muted-foreground">{info ? `${info.hostname} · ${info.os}/${info.arch} · up ${formatDuration(info.uptime_seconds)}` : 'Your local inference system, at a glance.'}</p></div>
      <div className="flex items-center gap-3"><span className="text-[11px] text-muted-foreground">{error ? 'Telemetry stale' : updatedAt ? `Updated ${updatedAt.toLocaleTimeString()}` : 'Connecting…'}</span><Button variant="outline" size="icon-sm" disabled={refreshing} onClick={() => { void refresh(); void refreshModels() }} aria-label="Refresh dashboard"><RefreshCw className={refreshing ? 'animate-spin' : ''} /></Button></div>
    </header>
    {(error || modelError) && <div role="alert" className="flex items-center gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive"><CircleAlert className="size-4 shrink-0" />{error || modelError}{info || models ? ' · Showing last received data.' : ''}</div>}
    <ResourceMetrics info={info} />
    <div className="grid gap-4 lg:grid-cols-[minmax(0,.8fr)_minmax(0,1.2fr)]">
      <section className="reference-panel registry-highlight p-5" aria-label="Model inventory summary">
        <div className="flex items-center justify-between"><span className="reference-tag"><Boxes className="size-3.5" /> Model inventory</span><Link to="/models" className="rounded p-1 hover:bg-white/10 focus-visible:outline-2" aria-label="View model registry"><ArrowUpRight className="size-4" /></Link></div>
        <div className="mt-6 grid grid-cols-3 gap-3">
          {[{ label: 'Registered', value: models?.length }, { label: 'Loaded', value: models ? loaded.length : undefined }, { label: 'Failed', value: models?.filter(model => model.live.state === 'failed').length }].map(item => <div key={item.label}><div className="reference-title text-4xl">{item.value ?? '—'}</div><div className="mt-1 text-[11px] text-white/80">{item.label}</div></div>)}
        </div>
        <div className="mt-5 flex items-center gap-2 border-t border-white/15 pt-3 text-[11px] text-white/80"><Server className="size-3.5" />{info ? `${info.runtimes.filter(runtime => runtime.available).length} inference backends available` : 'Checking runtime availability…'}</div>
      </section>
      <section className="reference-panel overflow-hidden" aria-label="Loaded models">
        <div className="flex items-center justify-between border-b px-5 py-3"><h2 className="flex items-center gap-2 text-sm font-medium"><Cpu className="size-4 text-primary" /> Loaded models</h2><span className="text-[11px] text-muted-foreground">{models ? loaded.length : '—'} serving</span></div>
        <div className="max-h-64 overflow-auto divide-y divide-border">
          {loaded.map(model => <Link key={model.id} to={`/models/${model.id}`} className="flex items-center gap-3 px-5 py-3 hover:bg-accent/40 focus-visible:outline-2 focus-visible:outline-ring"><span className="size-2 shrink-0 rounded-full bg-emerald-500" /><span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium">{model.name}</span><span className="mt-1 block text-[11px] text-muted-foreground">{model.runtime === 'llamacpp' ? 'llama.cpp' : 'ONNX'} · {model.task} · up {uptimeSince(model.live.started_at)}</span></span><ArrowUpRight className="size-3.5 shrink-0 text-muted-foreground" /></Link>)}
          {!loaded.length && <p className="px-5 py-8 text-xs text-muted-foreground">{models ? 'No models loaded. Load one from the model registry.' : modelError ? 'Model list unavailable.' : 'Loading model list…'}</p>}
        </div>
      </section>
    </div>
    <section className="reference-panel overflow-hidden" aria-label="Registered model overview">
      <div className="flex items-center justify-between border-b px-5 py-3"><h2 className="text-sm font-medium">Registered models</h2><Link to="/models" className="flex items-center gap-1 text-xs text-primary hover:underline">Manage models<ArrowUpRight className="size-3" /></Link></div>
      <div className="grid divide-y divide-border sm:grid-cols-2 xl:grid-cols-3">
        {models?.map(model => <Link key={model.id} to={`/models/${model.id}`} className="flex min-w-0 items-center gap-3 px-5 py-3 hover:bg-accent/30"><Boxes className="size-4 shrink-0 text-primary" /><span className="min-w-0 flex-1"><span className="block truncate text-xs font-medium">{model.name}</span><span className="text-[10px] text-muted-foreground">{model.runtime} · {model.task}</span></span><StatusBadge state={model.live.state} /></Link>)}
        {!models?.length && <p className="p-5 text-xs text-muted-foreground">{models ? 'No models registered yet.' : modelError ? 'Model list unavailable.' : 'Loading registry…'}</p>}
      </div>
    </section>
    <LiveLogsPanel runtimes={info?.runtimes ?? []} />
  </div>
}
