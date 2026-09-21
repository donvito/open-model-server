import { useCallback, useSyncExternalStore, type Dispatch, type SetStateAction } from 'react'
import { getChatSession, type ChatSessionState } from '@/lib/playgroundSessions'

export function useChatSessionState<K extends keyof ChatSessionState>(modelId: string, key: K): [ChatSessionState[K], Dispatch<SetStateAction<ChatSessionState[K]>>] {
  const session = getChatSession(modelId)
  const state = useSyncExternalStore(session.subscribe, session.getSnapshot)
  const set = useCallback((value: SetStateAction<ChatSessionState[K]>) => session.set(key, value), [session, key])
  return [state[key], set]
}
