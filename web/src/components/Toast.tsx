import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { AlertCircle, CheckCircle2, X } from 'lucide-react'
import { cn } from '@/lib/utils'

interface Toast {
  id: number
  kind: 'success' | 'error'
  message: string
}

interface ToastApi {
  success: (m: string) => void
  error: (m: string) => void
}

const Ctx = createContext<ToastApi>({ success: () => {}, error: () => {} })

export function useToast() {
  return useContext(Ctx)
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([])
  const push = useCallback((kind: Toast['kind'], message: string) => {
    const id = Date.now() + Math.random()
    setItems((xs) => [...xs, { id, kind, message }])
    setTimeout(() => setItems((xs) => xs.filter((x) => x.id !== id)), kind === 'error' ? 8000 : 3500)
  }, [])
  const api = useMemo<ToastApi>(() => ({ success: (m) => push('success', m), error: (m) => push('error', m) }), [push])
  return (
    <Ctx.Provider value={api}>
      {children}
      <div className="pointer-events-none fixed right-4 bottom-4 z-[100] flex w-96 max-w-[calc(100vw-2rem)] flex-col gap-2">
        {items.map((t) => (
          <div
            key={t.id}
            role="status"
            className={cn(
              'pointer-events-auto flex items-start gap-2 rounded-lg border px-3 py-2 text-sm shadow-lg bg-card',
              t.kind === 'error' ? 'border-destructive/40' : 'border-emerald-500/40',
            )}
          >
            {t.kind === 'error' ? <AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" /> : <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-400" />}
            <span className="flex-1 break-words">{t.message}</span>
            <button className="text-muted-foreground hover:text-foreground cursor-pointer" onClick={() => setItems((xs) => xs.filter((x) => x.id !== t.id))}>
              <X className="size-4" />
            </button>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  )
}
