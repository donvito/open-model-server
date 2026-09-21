import { useEffect, useRef, useState } from 'react'
import { Pause, Play, Terminal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { streamRuntimeLogs, type RuntimeLogsStreamStatus } from '@/lib/api'
import type { LogLine, Runtime } from '@/lib/types'
import { cn } from '@/lib/utils'

const MAX_LINES = 500

interface RuntimeLogsProps {
  runtime: Runtime
  defaultOpen?: boolean
  visible?: boolean
}

export function RuntimeLogs({ runtime, defaultOpen = true, visible }: RuntimeLogsProps) {
  const controlled = visible !== undefined
  const [localOpen, setLocalOpen] = useState(defaultOpen)
  const open = visible ?? localOpen
  const [paused, setPaused] = useState(false)
  const [lines, setLines] = useState<LogLine[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(open)
  const [status, setStatus] = useState<RuntimeLogsStreamStatus>('closed')
  const [retryToken, setRetryToken] = useState(0)
  const scroller = useRef<HTMLDivElement>(null)
  const follow = useRef(true)

  useEffect(() => {
    if (!open || paused) {
      setStatus('closed')
      setLoading(false)
      return
    }

    setLines([])
    setError('')
    setLoading(true)
    const stop = streamRuntimeLogs(runtime, {
      onSnapshot: (snapshot) => {
        setLines(snapshot.slice(-MAX_LINES))
        setLoading(false)
        setError('')
      },
      onLine: (line) => {
        setLines((current) => [...current, line].slice(-MAX_LINES))
        setLoading(false)
      },
      onError: (_, next) => {
        setError(next === 'closed' ? 'Stream closed; check server availability or API credentials.' : 'Connection interrupted; retrying. Check server availability or API credentials if this persists.')
      },
      onStatus: (next) => {
        setStatus(next)
        if (next === 'connecting') setLoading(true)
        if (next === 'open') setError('')
        if (next === 'closed') setLoading(false)
      },
    })
    return stop
  }, [runtime, open, paused, retryToken])

  useEffect(() => {
    if (follow.current && scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight
  }, [lines, open])

  if (controlled && !open) return null

  const connectionLabel = paused
    ? 'Paused'
    : status === 'open'
      ? 'Live'
      : status === 'reconnecting'
        ? 'Reconnecting…'
        : status === 'connecting'
          ? 'Connecting…'
          : 'Disconnected'

  return (
    <div className="mt-3">
      <div className="flex items-center gap-3">
        {!controlled ? (
          <Button variant="outline" size="sm" aria-expanded={open} aria-controls={`runtime-logs-${runtime}`} onClick={() => setLocalOpen(!open)}>
            <Terminal /> {open ? 'Hide logs' : 'Show logs'}
          </Button>
        ) : (
          <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
            <Terminal className="size-3.5" aria-hidden="true" /> Runtime logs
          </span>
        )}
        {open && (
          <>
            <Button variant="ghost" size="sm" onClick={() => setPaused(!paused)}>
              {paused ? <Play /> : <Pause />} {paused ? 'Resume' : 'Pause'}
            </Button>
            <span className="text-muted-foreground text-xs" role="status" aria-live="polite">{connectionLabel}</span>
          </>
        )}
      </div>
      {open && (
        <div id={`runtime-logs-${runtime}`} className="mt-2 space-y-2">
          <p className="text-muted-foreground text-xs">
            {paused ? 'Paused' : connectionLabel} · Latest 500 lines · Kept in memory until server restart
          </p>
          {runtime === 'onnx' && <p className="text-muted-foreground text-xs">Model lifecycle and load errors. Native ONNX stderr is not captured.</p>}
          {error && (
            <div className="flex items-center gap-2">
              <p role="status" aria-live="polite" className="text-muted-foreground text-xs">{error}</p>
              {status === 'closed' && !paused && <Button variant="ghost" size="sm" onClick={() => setRetryToken((current) => current + 1)}>Retry</Button>}
            </div>
          )}
          <div ref={scroller} tabIndex={0} role="region" aria-label={`${runtime} logs`} className="bg-background max-h-80 min-h-24 overflow-auto rounded-md border p-3 font-mono text-xs" onScroll={(e) => {
            const el = e.currentTarget
            follow.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
          }}>
            {lines.length === 0 ? (
              <p className="text-muted-foreground">{loading ? 'Connecting to logs…' : 'No runtime logs yet. Load a model to see activity.'}</p>
            ) : lines.map((line, index) => (
              <div key={index} className="whitespace-pre-wrap break-all leading-5">
                <span className="text-muted-foreground">{new Date(line.time).toLocaleTimeString()} </span>
                <span className={cn(line.source === 'stderr' ? 'text-amber-700 dark:text-amber-400' : 'text-muted-foreground')}>[{line.source}] </span>
                {line.model && <span className="text-primary">[{line.model}] </span>}
                {line.text}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
