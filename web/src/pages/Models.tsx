import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Plus, RefreshCw, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { StatusBadge } from '@/components/StatusBadge'
import { ModelForm } from '@/components/ModelForm'
import { ModelActions } from '@/components/ModelActions'
import { useModels } from '@/hooks/useModels'
import type { Model } from '@/lib/types'
import { uptimeSince } from '@/lib/utils'

export function ModelsPage() {
  const { models, error, refresh } = useModels()
  const [q, setQ] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Model | undefined>()

  const filtered = (models ?? []).filter((m) => {
    const s = q.toLowerCase()
    return !s || m.name.toLowerCase().includes(s) || m.runtime.includes(s) || m.task.includes(s) || m.model_path.toLowerCase().includes(s)
  })

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Models</h1>
          <p className="text-muted-foreground text-sm">
            {models ? `${models.length} registered · ${models.filter((m) => m.live.state === 'running').length} running` : 'Loading…'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="text-muted-foreground absolute top-2.5 left-2.5 size-4" />
            <Input className="w-64 pl-8" placeholder="Filter models…" value={q} onChange={(e) => setQ(e.target.value)} />
          </div>
          <Button variant="outline" size="icon" onClick={() => refresh()} aria-label="Refresh">
            <RefreshCw />
          </Button>
          <Button
            onClick={() => {
              setEditing(undefined)
              setFormOpen(true)
            }}
          >
            <Plus /> Add model
          </Button>
        </div>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="text-destructive text-sm">Could not reach the server: {error}</CardContent>
        </Card>
      )}

      {models && models.length === 0 && !error && (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-10 text-center">
            <div className="text-lg font-medium">No models yet</div>
            <p className="text-muted-foreground max-w-md text-sm">
              Register a GGUF file (served with llama.cpp) or an ONNX model directory to get started. You can also use the CLI:{' '}
              <code className="bg-muted rounded px-1 py-0.5">modelserver models add ./model.gguf</code>
            </p>
            <Button
              onClick={() => {
                setEditing(undefined)
                setFormOpen(true)
              }}
            >
              <Plus /> Add model
            </Button>
          </CardContent>
        </Card>
      )}

      {filtered.length > 0 && (
        <div className="overflow-hidden rounded-xl border">
          <table className="w-full text-sm">
            <thead className="bg-muted/40 text-muted-foreground text-left text-xs uppercase tracking-wide">
              <tr>
                <th className="px-4 py-2.5 font-medium">Name</th>
                <th className="px-4 py-2.5 font-medium">Runtime</th>
                <th className="px-4 py-2.5 font-medium">Task</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
                <th className="px-4 py-2.5 font-medium">Path</th>
                <th className="px-4 py-2.5 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((m) => (
                <tr key={m.id} className="border-t hover:bg-accent/30">
                  <td className="px-4 py-3">
                    <Link to={`/models/${m.id}`} className="font-medium hover:underline">
                      {m.name}
                    </Link>
                    {m.description && <div className="text-muted-foreground line-clamp-1 text-xs">{m.description}</div>}
                  </td>
                  <td className="px-4 py-3">
                    <Badge variant="outline">{m.runtime}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    <Badge variant="secondary">{m.task}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-col gap-1">
                      <StatusBadge state={m.live.state} />
                      {m.live.state === 'running' && (
                        <span className="text-muted-foreground text-xs">
                          up {uptimeSince(m.live.started_at)}
                          {m.live.port ? ` · :${m.live.port}` : ''}
                        </span>
                      )}
                      {m.live.state === 'failed' && m.live.error && (
                        <span className="text-destructive line-clamp-2 max-w-xs text-xs" title={m.live.error}>
                          {m.live.error}
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="text-muted-foreground max-w-xs truncate px-4 py-3 font-mono text-xs" title={m.model_path}>
                    {m.model_path}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex justify-end">
                      <ModelActions
                        model={m}
                        compact
                        onChanged={() => refresh()}
                        onEdit={() => {
                          setEditing(m)
                          setFormOpen(true)
                        }}
                      />
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <ModelForm open={formOpen} onOpenChange={setFormOpen} model={editing} onSaved={() => refresh()} />
    </div>
  )
}
