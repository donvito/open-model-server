import { type ComponentType, type ReactNode } from 'react'
import { CheckCircle2, CircleAlert, Cpu, Database, HardDrive, LoaderCircle, RefreshCw, Server, XCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { LiveLogsPanel } from '@/components/LiveLogsPanel'
import { ResourceMetrics } from '@/components/ResourceMetrics'
import { useSystem } from '@/hooks/useSystem'
import type { RuntimeInfo, SystemInfo } from '@/lib/types'
import { cn, formatBytes, formatDuration } from '@/lib/utils'

export function SystemPage() {
  const { info, error, refreshing, refresh: handleRefresh } = useSystem()

  const memoryTotal = info?.memory.total_bytes
  const memoryAvailable = info?.memory.available_bytes
  const memoryUsed = typeof memoryTotal === 'number' && typeof memoryAvailable === 'number' ? memoryTotal - memoryAvailable : undefined
  const memoryPercent = memoryUsed !== undefined && memoryTotal ? Math.round((memoryUsed / memoryTotal) * 100) : undefined

  return (
    <div className="flex min-h-0 flex-col gap-4">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b pb-4">
        <div className="min-w-0">
          <div className="flex items-baseline gap-2">
            <h1 className="text-xl font-semibold tracking-tight">System</h1>
            <span className="font-mono text-[11px] text-muted-foreground">telemetry</span>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {info ? `modelserver ${info.version} · uptime ${formatDuration(info.uptime_seconds)}` : 'Host, runtime, and server status.'}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <div className="hidden items-center gap-1.5 text-[11px] text-muted-foreground sm:flex" role="status" aria-live="polite">
            <span className={cn('size-1.5', info && !error ? 'bg-emerald-500' : 'bg-destructive')} aria-hidden="true" />
            {error ? 'Telemetry stale · retrying' : info ? 'Live · refreshes every 3s' : 'Connecting…'}
          </div>
          <Button variant="outline" size="icon-sm" onClick={() => void handleRefresh()} disabled={refreshing} aria-label="Refresh system telemetry" title="Refresh system telemetry">
            <RefreshCw className={cn(refreshing && 'animate-spin')} />
          </Button>
        </div>
      </header>

      {error && (
        <div role="alert" className="flex items-start gap-2 border border-destructive/40 bg-destructive/5 px-3 py-2.5 text-sm">
          <CircleAlert className="mt-0.5 size-4 shrink-0 text-destructive" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <p className="font-medium text-destructive">Unable to refresh system telemetry</p>
            <p className="mt-0.5 break-words text-xs text-muted-foreground">{error}</p>
          </div>
          <Button variant="ghost" size="sm" onClick={() => void handleRefresh()} disabled={refreshing}>Retry</Button>
        </div>
      )}

      {!info ? (
        <SystemState icon={error ? <CircleAlert className="size-5 text-destructive" /> : <LoaderCircle className="size-5 animate-spin text-primary" />} title={error ? "System telemetry unavailable" : "Loading system telemetry"} detail={error ? "Retry when the server is reachable." : "Waiting for the server response."} />
      ) : (
        <>
          <ResourceMetrics info={info} />
          <SystemSummary info={info} memoryUsed={memoryUsed} memoryPercent={memoryPercent} />
          <RuntimePanel runtimes={info.runtimes} />
          <LiveLogsPanel runtimes={info.runtimes} />
        </>
      )}
    </div>
  )
}

function SystemSummary({ info, memoryUsed, memoryPercent }: { info: SystemInfo; memoryUsed?: number; memoryPercent?: number }) {
  const address = `${String(info.server.host ?? '—')}:${String(info.server.port ?? '—')}`
  const databasePath = info.paths.database ?? '—'

  return (
    <section aria-label="System summary" className="reference-panel overflow-hidden">
      <div className="grid divide-y divide-border sm:grid-cols-2 sm:divide-x sm:divide-y-0 lg:grid-cols-3 xl:grid-cols-6">
        <SystemMetric icon={Server} label="Host" value={info.hostname || '—'} detail={`${info.os}/${info.arch}`} />
        <SystemMetric icon={Cpu} label="CPU" value={`${info.cpus} cores`} detail={info.go_version} />
        <SystemMetric
          icon={HardDrive}
          label="Memory"
          value={memoryUsed !== undefined && info.memory.total_bytes ? `${formatBytes(memoryUsed)} / ${formatBytes(info.memory.total_bytes)}` : 'n/a'}
          detail={memoryPercent !== undefined ? `${memoryPercent}% used` : 'not reported'}
          bar={memoryPercent}
        />
        <SystemMetric icon={Database} label="Models" value={String(info.models.total)} detail={`${info.models.running} running · ${info.models.failed} failed`} />
        <SystemMetric icon={Server} label="Listening" value={address} detail={`ui ${info.server.ui_enabled ? 'on' : 'off'} · auth ${info.server.auth_enabled ? 'on' : 'off'}`} mono />
        <SystemMetric icon={Database} label="Database" value={databasePath} detail={`models ${info.paths.models_directory ?? '—'}`} mono />
      </div>
    </section>
  )
}

function SystemMetric({ icon: Icon, label, value, detail, bar, mono }: { icon: ComponentType<{ className?: string; 'aria-hidden'?: boolean }>; label: string; value: string; detail?: string; bar?: number; mono?: boolean }) {
  return (
    <div className="min-w-0 px-3 py-3">
      <div className="flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-[0.1em] text-muted-foreground">
        <Icon className="size-3.5 text-primary" aria-hidden={true} />
        {label}
      </div>
      <div className={cn('mt-1 truncate text-sm font-semibold', mono && 'font-mono text-xs')} title={value}>{value}</div>
      {detail && <div className="mt-0.5 truncate text-[11px] text-muted-foreground" title={detail}>{detail}</div>}
      {bar !== undefined && <div className="mt-2 h-1 overflow-hidden bg-muted"><div className="h-full bg-primary" style={{ width: `${Math.min(100, Math.max(0, bar))}%` }} /></div>}
    </div>
  )
}

function RuntimePanel({ runtimes }: { runtimes: RuntimeInfo[] }) {
  const available = runtimes.filter((runtime) => runtime.available).length

  return (
    <section className="reference-panel overflow-hidden" aria-label="Inference runtimes">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2.5">
        <div>
          <h2 className="text-xs font-semibold uppercase tracking-[0.12em] text-muted-foreground">Runtimes</h2>
          <p className="mt-0.5 text-[11px] text-muted-foreground">Detected inference backends and runtime logs.</p>
        </div>
        <span className="font-mono text-[11px] text-muted-foreground">{available}/{runtimes.length} available</span>
      </div>

      {runtimes.length ? (
        <div className="divide-y divide-border">
          {runtimes.map((runtime) => <RuntimeRow key={runtime.name} runtime={runtime} />)}
        </div>
      ) : (
        <SystemState title="No runtime records" detail="The server did not report any inference backends." />
      )}
    </section>
  )
}

function RuntimeRow({ runtime }: { runtime: RuntimeInfo }) {
  const label = runtime.name === 'llamacpp' ? 'llama.cpp' : 'ONNX Runtime'
  const StatusIcon = runtime.available ? CheckCircle2 : XCircle

  return (
    <div className="grid gap-3 px-3 py-3 lg:grid-cols-[minmax(12rem,0.4fr)_minmax(0,1fr)]">
      <div className="flex min-w-0 items-start gap-2.5">
        <StatusIcon className={cn('mt-0.5 size-4 shrink-0', runtime.available ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive')} aria-hidden="true" />
        <div className="min-w-0">
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
            <h3 className="text-sm font-medium">{label}</h3>
            <code className="text-[10px] text-muted-foreground">{runtime.name}</code>
            {runtime.version && <span className="font-mono text-[10px] text-muted-foreground">{runtime.version}</span>}
          </div>
          <p className={cn('mt-1 text-xs', runtime.available ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive')}>
            {runtime.available ? 'available' : runtime.error || 'unavailable'}
          </p>
        </div>
      </div>

      <div className="min-w-0">
        {runtime.available && runtime.details && (
          <dl className="grid gap-x-4 gap-y-1 text-xs sm:grid-cols-[max-content_minmax(0,1fr)]">
            {Object.entries(runtime.details).map(([key, value]) => (
              <div key={key} className="contents">
                <dt className="text-muted-foreground">{key}</dt>
                <dd className="break-all font-mono">{formatDetailValue(value)}</dd>
              </div>
            ))}
          </dl>
        )}
      </div>
    </div>
  )
}

function SystemState({ icon, title, detail }: { icon?: ReactNode; title: string; detail: string }) {
  return (
    <div className="flex min-h-36 flex-col items-center justify-center gap-2 border border-dashed bg-card px-4 py-8 text-center" aria-live="polite">
      {icon}
      <h2 className="text-sm font-semibold">{title}</h2>
      <p className="text-xs text-muted-foreground">{detail}</p>
    </div>
  )
}

function formatDetailValue(value: unknown) {
  if (typeof value === 'string') return value
  const serialized = JSON.stringify(value)
  return serialized === undefined ? String(value) : serialized
}
