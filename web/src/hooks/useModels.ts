import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import type { Model } from '@/lib/types'

/** Polls the model list; polls faster while anything is transitioning. */
export function useModels(intervalMs = 5000) {
  const [models, setModels] = useState<Model[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const refresh = useCallback(async () => {
    try {
      const ms = await api.listModels()
      setModels(ms)
      setError(null)
      return ms
    } catch (e) {
      setError((e as Error).message)
      return null
    }
  }, [])

  useEffect(() => {
    let alive = true
    const loop = async () => {
      const ms = await refresh()
      if (!alive) return
      const busy = ms?.some((m) => m.live.state === 'starting' || m.live.state === 'stopping')
      timer.current = setTimeout(loop, busy ? 1000 : intervalMs)
    }
    loop()
    return () => {
      alive = false
      if (timer.current) clearTimeout(timer.current)
    }
  }, [refresh, intervalMs])

  return { models, error, refresh, setModels }
}
