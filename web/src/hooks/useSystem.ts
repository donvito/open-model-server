import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import type { SystemInfo } from '@/lib/types'

/** Poll sequentially so slow hardware queries cannot pile up. */
export function useSystem(intervalMs = 3000) {
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const alive = useRef(false)
  const pending = useRef<Promise<void> | null>(null)
  const refresh = useCallback((): Promise<void> => {
    if (pending.current) return pending.current
    if (alive.current) setRefreshing(true)
    pending.current = api.system().then(result => {
      if (!alive.current) return
      setInfo(result)
      setError(null)
      setUpdatedAt(new Date())
    }).catch((failure: Error) => {
      if (alive.current) setError(failure.message)
    }).finally(() => {
      pending.current = null
      if (alive.current) setRefreshing(false)
    })
    return pending.current
  }, [])
  useEffect(() => {
    alive.current = true
    let active = true
    let timer: ReturnType<typeof setTimeout>
    const poll = async () => {
      await refresh()
      if (active) timer = setTimeout(poll, intervalMs)
    }
    void poll()
    return () => { active = false; alive.current = false; clearTimeout(timer) }
  }, [intervalMs, refresh])
  return { info, error, updatedAt, refreshing, refresh }
}
