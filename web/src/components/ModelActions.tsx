import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { FlaskConical, MoreHorizontal, Pencil, Play, RotateCw, ScrollText, Square, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { api } from '@/lib/api'
import type { Model } from '@/lib/types'
import { useToast } from '@/components/Toast'

interface Props {
  model: Model
  onChanged: (m?: Model) => void
  onEdit: () => void
  compact?: boolean
}

export function ModelActions({ model, onChanged, onEdit, compact }: Props) {
  const toast = useToast()
  const nav = useNavigate()
  const [busy, setBusy] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const st = model.live.state
  const canLoad = st === 'stopped' || st === 'failed'
  const canUnload = st === 'running' || st === 'failed' || st === 'starting'

  async function run(label: string, fn: () => Promise<Model | void>) {
    setBusy(label)
    try {
      const m = await fn()
      onChanged(m ?? undefined)
      toast.success(`${label} ${model.name}`)
    } catch (e) {
      onChanged()
      toast.error(`${label} ${model.name}: ${(e as Error).message}`)
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="flex items-center gap-1">
      {canLoad ? (
        <Button size={compact ? 'sm' : 'default'} onClick={() => run('Loaded', () => api.load(model.id))} disabled={busy !== null}>
          <Play /> {busy === 'Loaded' ? 'Loading…' : 'Load'}
        </Button>
      ) : (
        <Button size={compact ? 'sm' : 'default'} variant="outline" onClick={() => run('Unloaded', () => api.unload(model.id))} disabled={busy !== null || !canUnload}>
          <Square /> {busy === 'Unloaded' ? 'Stopping…' : 'Unload'}
        </Button>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size={compact ? 'icon-sm' : 'icon'} aria-label="More actions">
            <MoreHorizontal />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => nav(`/playground/${model.name}`)} disabled={st !== 'running'}>
            <FlaskConical /> Playground
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => nav(`/models/${model.id}?tab=logs`)}>
            <ScrollText /> Logs
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => run('Restarted', () => api.restart(model.id))} disabled={busy !== null}>
            <RotateCw /> Restart
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={onEdit}>
            <Pencil /> Edit settings
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setConfirmDelete(true)}>
            <Trash2 /> Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete {model.name}?</DialogTitle>
            <DialogDescription>
              Removes the registry entry{st === 'running' ? ' and stops the running instance' : ''}. The model file on disk is not touched.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                setConfirmDelete(false)
                run('Deleted', () => api.deleteModel(model.id))
              }}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
