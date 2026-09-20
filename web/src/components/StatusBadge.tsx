import { Loader2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import type { State } from '@/lib/types'

export function StatusBadge({ state }: { state: State }) {
  switch (state) {
    case 'running':
      return <Badge variant="success">running</Badge>
    case 'starting':
    case 'stopping':
      return (
        <Badge variant="warning">
          <Loader2 className="animate-spin" /> {state}
        </Badge>
      )
    case 'failed':
      return <Badge variant="destructive">failed</Badge>
    default:
      return <Badge variant="muted">stopped</Badge>
  }
}
