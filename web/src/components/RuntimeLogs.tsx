import { useEffect, useRef, useState } from 'react'
import { Pause, Play, Terminal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import type { LogLine, Runtime } from '@/lib/types'
import { cn } from '@/lib/utils'

export function RuntimeLogs({ runtime }: { runtime: Runtime }) {
  const [open, setOpen] = useState(false)
  const [paused, setPaused] = useState(false)
  const [lines, setLines] = useState<LogLine[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const scroller = useRef<HTMLDivElement>(null)
  const follow = useRef(true)

  useEffect(() => {
    if (!open || paused) return
    const ctrl = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const result = await api.runtimeLogs(runtime, 500, ctrl.signal)
        if (ctrl.signal.aborted) return
        setLines(result)
        setError('')
      } catch (e) {
        if (!ctrl.signal.aborted) setError((e as Error).message)
      } finally {
        if (!ctrl.signal.aborted) {
          setLoading(false)
          timer = setTimeout(refresh, 2000)
        }
      }
    }
    void refresh()
    return () => { ctrl.abort(); clearTimeout(timer) }
  }, [runtime, open, paused])

  useEffect(() => {
    if (follow.current && scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight
  }, [lines, open])

  return (
    <div className="mt-3">
      <div className="flex items-center gap-3">
        <Button variant="outline" size="sm" aria-expanded={open} aria-controls={`runtime-logs-${runtime}`} onClick={() => setOpen(!open)}>
          <Terminal /> {open ? 'Hide logs' : 'Show logs'}
        </Button>
        {open && (
          <Button variant="ghost" size="sm" onClick={() => setPaused(!paused)}>
            {paused ? <Play /> : <Pause />} {paused ? 'Resume' : 'Pause'}
          </Button>
        )}
      </div>
      {open && (
        <div id={`runtime-logs-${runtime}`} className="mt-2 space-y-2">
          <p className="text-muted-foreground text-xs">
            {paused ? 'Paused' : 'Refreshes every 2 seconds'} · Latest 500 lines · Kept in memory until server restart
          </p>
          {runtime === 'onnx' && <p className="text-muted-foreground text-xs">Model lifecycle and load errors. Native ONNX stderr is not captured.</p>}
          {error && <p role="alert" className="text-destructive text-xs">Could not fetch logs: {error}</p>}
          <div ref={scroller} tabIndex={0} role="region" aria-label={`${runtime} logs`} className="bg-black/40 max-h-80 min-h-24 overflow-auto rounded-md border p-3 font-mono text-xs" onScroll={(e) => {
            const el = e.currentTarget
            follow.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
          }}>
            {lines.length === 0 ? (
              <p className="text-muted-foreground">{loading ? 'Loading logs…' : 'No runtime logs yet. Load a model to see activity.'}</p>
            ) : lines.map((line, index) => (
              <div key={index} className="whitespace-pre-wrap break-all leading-5">
                <span className="text-muted-foreground">{new Date(line.time).toLocaleTimeString()} </span>
                <span className={cn(line.source === 'stderr' ? 'text-amber-400' : 'text-muted-foreground')}>[{line.source}] </span>
                {line.model && <span className="text-violet-400">[{line.model}] </span>}
                {line.text}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
