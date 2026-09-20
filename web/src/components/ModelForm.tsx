import { useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import { RUNTIMES, TASKS, type Model, type Runtime, type Task } from '@/lib/types'
import { useToast } from '@/components/Toast'
import { basename } from '@/lib/utils'

const AUTO = '__auto__'

const runtimeHints: Record<Runtime, string> = {
  llamacpp: 'GGUF file served by a llama-server child process. Config keys: context_length, gpu_layers, threads, batch_size, extra_args.',
  onnx: 'ONNX model file or a directory with model.onnx + tokenizer.json. Config keys: model_file, tokenizer_file, max_length, labels, pooling, normalize, threads.',
}

const configTemplates: Record<Runtime, string> = {
  llamacpp: '{\n  "context_length": 4096,\n  "gpu_layers": 0\n}',
  onnx: '{\n  "max_length": 512\n}',
}

export interface ModelFormProps {
  open: boolean
  onOpenChange: (o: boolean) => void
  model?: Model // edit mode when set
  onSaved: (m: Model) => void
}

export function ModelForm({ open, onOpenChange, model, onSaved }: ModelFormProps) {
  const toast = useToast()
  const [path, setPath] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [runtime, setRuntime] = useState<string>(AUTO)
  const [task, setTask] = useState<string>(AUTO)
  const [config, setConfig] = useState('')
  const [loadNow, setLoadNow] = useState(false)
  const [busy, setBusy] = useState(false)
  const [nameTouched, setNameTouched] = useState(false)

  useEffect(() => {
    if (!open) return
    if (model) {
      setPath(model.model_path)
      setName(model.name)
      setDescription(model.description ?? '')
      setRuntime(model.runtime)
      setTask(model.task)
      setConfig(model.config && Object.keys(model.config).length ? JSON.stringify(model.config, null, 2) : '')
      setNameTouched(true)
    } else {
      setPath('')
      setName('')
      setDescription('')
      setRuntime(AUTO)
      setTask(AUTO)
      setConfig('')
      setLoadNow(false)
      setNameTouched(false)
    }
  }, [open, model])

  // Suggest a name from the path until the user edits it.
  useEffect(() => {
    if (nameTouched || model) return
    const b = basename(path)
    setName(b.replace(/\.(gguf|onnx)$/i, '').toLowerCase().replace(/[^a-z0-9._-]+/g, '-'))
  }, [path, nameTouched, model])

  const inferredRuntime = useMemo<Runtime | null>(() => {
    if (runtime !== AUTO) return runtime as Runtime
    if (/\.gguf$/i.test(path)) return 'llamacpp'
    if (/\.onnx$/i.test(path)) return 'onnx'
    return null
  }, [runtime, path])

  const configError = useMemo(() => {
    if (!config.trim()) return null
    try {
      const v = JSON.parse(config)
      if (typeof v !== 'object' || v === null || Array.isArray(v)) return 'config must be a JSON object'
      return null
    } catch (e) {
      return (e as Error).message
    }
  }, [config])

  async function submit() {
    if (configError) return
    setBusy(true)
    try {
      const cfg = config.trim() ? (JSON.parse(config) as Record<string, unknown>) : undefined
      let saved: Model
      if (model) {
        saved = await api.updateModel(model.id, {
          name,
          description,
          runtime: runtime === AUTO ? undefined : runtime,
          task: task === AUTO ? undefined : task,
          model_path: path,
          config: cfg ?? {},
        })
        toast.success(`Saved ${saved.name}`)
      } else {
        saved = await api.createModel({
          name,
          description,
          runtime: runtime === AUTO ? undefined : runtime,
          task: task === AUTO ? undefined : task,
          model_path: path,
          config: cfg,
        })
        toast.success(`Registered ${saved.name}`)
        if (loadNow) {
          try {
            saved = await api.load(saved.id)
          } catch (e) {
            toast.error(`Load failed: ${(e as Error).message}`)
          }
        }
      }
      onSaved(saved)
      onOpenChange(false)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent wide>
        <DialogHeader>
          <DialogTitle>{model ? `Edit ${model.name}` : 'Add model'}</DialogTitle>
          <DialogDescription>
            {model ? 'Changes to path, runtime or config take effect on the next load.' : 'Register a model file that already exists on the server machine. Nothing is copied.'}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="path">Model path</Label>
            <Input id="path" placeholder="/models/gemma-2b-it-q4_k_m.gguf or /models/my-onnx-dir" value={path} onChange={(e) => setPath(e.target.value)} autoFocus />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="grid gap-2">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                value={name}
                onChange={(e) => {
                  setNameTouched(true)
                  setName(e.target.value)
                }}
                placeholder="gemma"
              />
              <p className="text-muted-foreground text-xs">Used as the <code>model</code> in API calls.</p>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="desc">Description</Label>
              <Input id="desc" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="optional" />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="grid gap-2">
              <Label>Runtime</Label>
              <Select value={runtime} onValueChange={setRuntime}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={AUTO}>Auto-detect{inferredRuntime && runtime === AUTO ? ` (${inferredRuntime})` : ''}</SelectItem>
                  {RUNTIMES.map((r) => (
                    <SelectItem key={r} value={r}>
                      {r === 'llamacpp' ? 'llama.cpp (GGUF)' : 'ONNX Runtime'}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label>Task</Label>
              <Select value={task} onValueChange={setTask}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={AUTO}>Auto-detect</SelectItem>
                  {TASKS.map((t: Task) => (
                    <SelectItem key={t} value={t}>
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="grid gap-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="config">Runtime config (JSON)</Label>
              {inferredRuntime && !config && (
                <button type="button" className="text-muted-foreground hover:text-foreground text-xs cursor-pointer" onClick={() => setConfig(configTemplates[inferredRuntime])}>
                  insert template
                </button>
              )}
            </div>
            <Textarea id="config" className="font-mono text-xs" rows={5} value={config} onChange={(e) => setConfig(e.target.value)} placeholder="{}" aria-invalid={Boolean(configError)} />
            {configError ? <p className="text-destructive text-xs">{configError}</p> : inferredRuntime ? <p className="text-muted-foreground text-xs">{runtimeHints[inferredRuntime]}</p> : null}
          </div>
          {!model && (
            <div className="flex items-center gap-2">
              <Switch id="loadnow" checked={loadNow} onCheckedChange={setLoadNow} />
              <Label htmlFor="loadnow">Load immediately after registering</Label>
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={busy || !path || !name || Boolean(configError)}>
            {busy ? 'Saving…' : model ? 'Save' : loadNow ? 'Register & load' : 'Register'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
