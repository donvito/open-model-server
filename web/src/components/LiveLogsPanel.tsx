import { Terminal } from 'lucide-react'
import { RuntimeLogs } from '@/components/RuntimeLogs'
import { Switch } from '@/components/ui/switch'
import { useLogVisibility } from '@/hooks/useLogVisibility'
import type { RuntimeInfo } from '@/lib/types'

export function LiveLogsPanel({ runtimes }: { runtimes: RuntimeInfo[] }) {
  const [visible, setVisible] = useLogVisibility()
  return <section className="reference-panel overflow-hidden" aria-label="Live runtime logs">
    <div className="flex flex-wrap items-center justify-between gap-3 border-b px-5 py-3">
      <div className="flex items-center gap-2"><Terminal className="size-4 text-primary" /><h2 className="text-sm font-medium">Live logs</h2></div>
      <label className="flex cursor-pointer items-center gap-2 text-xs text-muted-foreground" htmlFor="show-live-logs">Show logs by default<Switch id="show-live-logs" checked={visible} onCheckedChange={setVisible} /></label>
    </div>
    {visible ? <div className="grid gap-4 p-4 xl:grid-cols-2">
      {runtimes.length ? runtimes.map(runtime => <div key={runtime.name} className="min-w-0"><div className="flex items-center gap-2 text-xs font-medium"><span className="size-1.5 rounded-full bg-primary" />{runtime.name === 'llamacpp' ? 'llama.cpp' : 'ONNX Runtime'}</div><RuntimeLogs runtime={runtime.name} visible /></div>) : <p className="text-xs text-muted-foreground">Waiting for runtime information…</p>}
    </div> : <p className="px-5 py-3 text-xs text-muted-foreground">Logs are hidden. Enable the switch to resume live streaming. Your choice is saved in this browser.</p>}
  </section>
}
