import { Badge } from '@/components/ui/badge'
import type { OrderStatus } from '@/types/order'

const statusClassName: Record<OrderStatus, string> = {
  pending: 'border-amber-200 bg-amber-100 text-amber-950',
  processing: 'border-sky-200 bg-sky-100 text-sky-950',
  completed: 'border-emerald-200 bg-emerald-100 text-emerald-950',
  failed: 'border-red-200 bg-red-100 text-red-950',
}

const statusLabel: Record<OrderStatus, string> = {
  pending: 'Pending',
  processing: 'Processing',
  completed: 'Completed',
  failed: 'Failed',
}

export function StatusBadge({ status }: { status: OrderStatus }) {
  return (
    <Badge variant="outline" className={statusClassName[status]}>
      {statusLabel[status]}
    </Badge>
  )
}
