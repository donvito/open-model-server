import { useEffect, useLayoutEffect, useState, type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { Boxes, Cpu, FlaskConical, KeyRound, Moon, Server, Sun, ChevronRight, PanelLeftClose, PanelLeftOpen, Terminal, LayoutDashboard } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, getApiKey, onUnauthorized, setApiKey } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const nav = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/models', label: 'Model registry', icon: Boxes },
  { to: '/playground', label: 'Playground', icon: FlaskConical },
  { to: '/system', label: 'System', icon: Cpu },
]

type Theme = 'light' | 'dark'

const themeStorageKey = 'modelserver-theme'

function getInitialTheme(): Theme {
  if (typeof window === 'undefined') return 'light'

  try {
    const saved = window.localStorage.getItem(themeStorageKey)
    if (saved === 'light' || saved === 'dark') return saved
  } catch {
    // Fall back to the OS preference when local storage is unavailable.
  }

  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(theme: Theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.style.colorScheme = theme
}

export function Layout({ children }: { children: ReactNode }) {
  const location = useLocation()
  const [keyOpen, setKeyOpen] = useState(false)
  const [key, setKey] = useState(getApiKey())
  const [online, setOnline] = useState<boolean | null>(null)
  const [theme, setTheme] = useState<Theme>(getInitialTheme)

  const activeNav = nav.find((item) => location.pathname === item.to || location.pathname.startsWith(`${item.to}/`)) ?? nav[0]
  const [collapsed, setCollapsed] = useState(false)
  const nextTheme = theme === 'dark' ? 'light' : 'dark'

  useLayoutEffect(() => {
    applyTheme(theme)
  }, [theme])

  useEffect(() => {
    try {
      window.localStorage.setItem(themeStorageKey, theme)
    } catch {
      // The UI still updates when storage is unavailable.
    }
  }, [theme])

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
    <div className={cn('console-shell', collapsed && 'sidebar-collapsed')}>
      <aside className="console-sidebar">
        <div className="console-brand"><span className="brand-symbol"><Terminal size={19} /></span><span className="sidebar-copy font-semibold tracking-tight">modelserver<span className="ml-2 text-[10px] font-normal text-muted-foreground">/ local</span></span></div>
        <div className="sidebar-copy px-5 pb-2 pt-6 text-[10px] font-semibold uppercase tracking-[.14em] text-muted-foreground">Workspace</div>
        <nav aria-label="Primary navigation" className="flex flex-col gap-1 p-2">
          {nav.map(item => <NavLink key={item.to} to={item.to} title={item.label} aria-label={item.label} className={({ isActive }) => cn('console-nav', isActive && 'is-active')}><item.icon size={17} /><span className="sidebar-copy flex-1">{item.label}</span><ChevronRight size={12} className="sidebar-copy nav-chevron" /></NavLink>)}
        </nav>
        <div className="sidebar-copy mx-4 mt-7 border-t pt-5">
          <div className="mb-3 text-[10px] font-semibold uppercase tracking-[.14em] text-muted-foreground">Environment</div>
          <div className="flex items-center gap-2.5 text-xs"><Server className="size-3.5 text-muted-foreground" /><span>Local server</span><span className={cn('ml-auto size-1.5 rounded-full', online ? 'bg-emerald-500' : 'bg-muted-foreground')} /></div>
          <p className="mt-2 pl-6 text-[11px] text-muted-foreground">{online === null ? 'Connecting…' : online ? 'Connection established' : 'Connection unavailable'}</p>
        </div>
        <div className="mt-auto border-t p-2">
          <button className="console-nav w-full" onClick={() => setKeyOpen(true)} title="API credentials" aria-label="API credentials"><KeyRound size={16} /><span className="sidebar-copy">API credentials</span><span className="sidebar-copy ml-auto font-mono text-[10px] text-muted-foreground">{key ? 'SET' : '—'}</span></button>
          <button className="console-nav w-full" onClick={() => setTheme(nextTheme)} title={`Switch to ${nextTheme} mode`} aria-label={`Switch to ${nextTheme} mode`}>{theme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}<span className="sidebar-copy">{theme === 'dark' ? 'Light appearance' : 'Dark appearance'}</span></button>
        </div>
      </aside>
      <div className="console-body">
        <header className="console-topbar">
          <Button variant="ghost" size="icon-sm" className="hidden md:inline-flex" onClick={() => setCollapsed(!collapsed)} aria-label={collapsed ? 'Expand navigation' : 'Collapse navigation'}>{collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}</Button>
          <span className="hidden text-muted-foreground sm:inline">Workspace</span><ChevronRight className="hidden size-3 text-muted-foreground sm:block" /><span className="font-medium">{activeNav.label}</span>
          <span className="ml-auto flex items-center gap-2 text-[11px] text-muted-foreground" role="status"><span className={cn('size-1.5 rounded-full', online === null ? 'bg-muted-foreground' : online ? 'bg-emerald-500' : 'bg-destructive')} />{online === null ? 'Connecting' : online ? 'Connected' : 'Offline'}</span>
        </header>
        <main className={cn('console-content', location.pathname.startsWith('/playground') && 'console-content-playground')}>{children}</main>
        <footer className="console-statusbar"><span className="flex items-center gap-1.5"><Terminal size={11} /> Local inference</span><span className="ml-auto">modelserver</span></footer>
      </div>
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
