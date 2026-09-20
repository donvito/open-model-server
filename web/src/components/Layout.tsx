import { useEffect, useState, type ReactNode } from 'react'
import { NavLink } from 'react-router-dom'
import { Boxes, Cpu, FlaskConical, KeyRound, Server } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, getApiKey, onUnauthorized, setApiKey } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const nav = [
  { to: '/models', label: 'Models', icon: Boxes },
  { to: '/playground', label: 'Playground', icon: FlaskConical },
  { to: '/system', label: 'System', icon: Cpu },
]

export function Layout({ children }: { children: ReactNode }) {
  const [keyOpen, setKeyOpen] = useState(false)
  const [key, setKey] = useState(getApiKey())
  const [online, setOnline] = useState<boolean | null>(null)

  useEffect(() => onUnauthorized(() => setKeyOpen(true)), [])
  useEffect(() => {
    let alive = true
    const tick = () =>
      api
        .health()
        .then(() => alive && setOnline(true))
        .catch(() => alive && setOnline(false))
    tick()
    const t = setInterval(tick, 10000)
    return () => {
      alive = false
      clearInterval(t)
    }
  }, [])

  return (
    <div className="flex min-h-screen">
      <aside className="flex w-56 shrink-0 flex-col border-r bg-card/40">
        <div className="flex items-center gap-2 px-5 py-5">
          <div className="flex size-8 items-center justify-center rounded-lg bg-violet-500/20 text-violet-300">
            <Server className="size-4" />
          </div>
          <div className="leading-tight">
            <div className="font-semibold">modelserver</div>
            <div className="text-muted-foreground text-[11px]">local model server</div>
          </div>
        </div>
        <nav className="flex flex-col gap-1 px-3">
          {nav.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                  isActive ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-accent/60 hover:text-foreground',
                )
              }
            >
              <n.icon className="size-4" />
              {n.label}
            </NavLink>
          ))}
        </nav>
        <div className="mt-auto flex flex-col gap-2 px-3 pb-4">
          <Button variant="ghost" size="sm" className="justify-start text-muted-foreground" onClick={() => setKeyOpen(true)}>
            <KeyRound /> API key {key ? '(set)' : ''}
          </Button>
          <div className="flex items-center gap-2 px-3 text-xs text-muted-foreground">
            <span className={cn('size-2 rounded-full', online === null ? 'bg-muted-foreground' : online ? 'bg-emerald-400' : 'bg-destructive')} />
            {online === null ? 'connecting…' : online ? 'server online' : 'server unreachable'}
          </div>
        </div>
      </aside>
      <main className="flex-1 min-w-0 px-8 py-6">{children}</main>

      <Dialog open={keyOpen} onOpenChange={setKeyOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>API key</DialogTitle>
            <DialogDescription>Required only when the server is started with an API key. Stored in this browser.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="apikey">Key</Label>
            <Input id="apikey" type="password" value={key} onChange={(e) => setKey(e.target.value)} placeholder="sk-…" autoFocus />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setKey('')
                setApiKey('')
                setKeyOpen(false)
              }}
            >
              Clear
            </Button>
            <Button
              onClick={() => {
                setApiKey(key)
                setKeyOpen(false)
                window.location.reload()
              }}
            >
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
