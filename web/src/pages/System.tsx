import { useEffect, useState } from 'react'
import { CheckCircle2, XCircle } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { RuntimeLogs } from '@/components/RuntimeLogs'
import { api } from '@/lib/api'
import type { SystemInfo } from '@/lib/types'
import { formatBytes, formatDuration } from '@/lib/utils'

export function SystemPage() {
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const tick = () =>
      api
        .system()
        .then((i) => {
          setInfo(i)
          setError(null)
        })
        .catch((e: Error) => setError(e.message))
    tick()
    const t = setInterval(tick, 5000)
    return () => clearInterval(t)
  }, [])

  if (error)
    return (
      <Card className="border-destructive/40">
        <CardContent className="text-destructive text-sm">{error}</CardContent>
      </Card>
    )
  if (!info) return <div className="text-muted-foreground text-sm">Loading…</div>

  const used = info.memory.total_bytes && info.memory.available_bytes ? info.memory.total_bytes - info.memory.available_bytes : undefined
  const pct = used && info.memory.total_bytes ? Math.round((used / info.memory.total_bytes) * 100) : undefined

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">System</h1>
        <p className="text-muted-foreground text-sm">
          modelserver {info.version} · up {formatDuration(info.uptime_seconds)}
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <Stat label="Host" value={info.hostname || '—'} sub={`${info.os}/${info.arch}`} />
        <Stat label="CPU" value={`${info.cpus} cores`} sub={info.go_version} />
        <Stat
          label="Memory"
          value={info.memory.total_bytes ? `${formatBytes(used)} / ${formatBytes(info.memory.total_bytes)}` : 'n/a'}
          sub={pct !== undefined ? `${pct}% used` : 'not reported on this OS'}
          bar={pct}
        />
        <Stat label="Models" value={`${info.models.total}`} sub={`${info.models.running} running · ${info.models.failed} failed`} />
        <Stat label="Listening" value={`${String(info.server.host)}:${String(info.server.port)}`} sub={`ui ${info.server.ui_enabled ? 'on' : 'off'} · auth ${info.server.auth_enabled ? 'on' : 'off'}`} />
        <Stat label="Database" value={info.paths.database ?? '—'} sub={`models dir ${info.paths.models_directory ?? '—'}`} mono />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Runtimes</CardTitle>
          <CardDescription>Inference backends detected on this machine.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          {info.runtimes.map((r) => (
            <div key={r.name} className="flex items-start gap-3 rounded-lg border p-3">
              {r.available ? <CheckCircle2 className="mt-0.5 size-5 text-emerald-400" /> : <XCircle className="mt-0.5 size-5 text-destructive" />}
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{r.name === 'llamacpp' ? 'llama.cpp' : 'ONNX Runtime'}</span>
                  <Badge variant="outline">{r.name}</Badge>
                  {r.version && <span className="text-muted-foreground text-xs">{r.version}</span>}
                </div>
                {r.available ? (
                  r.details && (
                    <dl className="text-muted-foreground mt-1 grid grid-cols-[8rem_1fr] gap-y-0.5 text-xs">
                      {Object.entries(r.details).map(([k, v]) => (
                        <div key={k} className="contents">
                          <dt>{k}</dt>
                          <dd className="break-all font-mono">{typeof v === 'string' ? v : JSON.stringify(v)}</dd>
                        </div>
                      ))}
                    </dl>
                  )
                ) : (
                  <p className="text-destructive mt-1 text-xs">{r.error}</p>
                )}
                <RuntimeLogs runtime={r.name} />
              </div>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  )
}

function Stat({ label, value, sub, bar, mono }: { label: string; value: string; sub?: string; bar?: number; mono?: boolean }) {
  return (
    <Card className="gap-1 py-4">
      <CardHeader className="pb-0">
        <CardDescription>{label}</CardDescription>
        <CardTitle className={mono ? 'truncate font-mono text-sm' : 'text-lg'} title={value}>
          {value}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {sub && <div className="text-muted-foreground truncate text-xs">{sub}</div>}
        {bar !== undefined && (
          <div className="bg-muted mt-2 h-1.5 overflow-hidden rounded-full">
            <div className="h-full bg-violet-400" style={{ width: `${bar}%` }} />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
