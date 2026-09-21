import { useState } from 'react'

const key = 'modelserver-show-live-logs'
export function useLogVisibility() {
  const [visible, setVisible] = useState(() => {
    try { return localStorage.getItem(key) !== 'false' } catch { return true }
  })
  function changeVisible(next: boolean) {
    setVisible(next)
    try { localStorage.setItem(key, String(next)) } catch { /* Visibility still works without storage. */ }
  }
  return [visible, changeVisible] as const
}
