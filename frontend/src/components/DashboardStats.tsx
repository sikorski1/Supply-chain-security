import { formatApiError } from '@/api/client'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useStats } from '@/hooks/useStats'
import type { DashboardStats as DashboardStatsData } from '@/types/stats'

const statCards: {
  label: string
  value: (stats: DashboardStatsData) => number
}[] = [
  { label: 'Total orders', value: (stats) => stats.totalOrders },
  { label: 'Pending', value: (stats) => stats.pendingOrders },
  { label: 'Processing', value: (stats) => stats.processingOrders },
  { label: 'Completed', value: (stats) => stats.completedOrders },
  { label: 'Failed', value: (stats) => stats.failedOrders },
]

export function DashboardStats() {
  const stats = useStats()

  return (
    <section className="flex flex-col gap-4" aria-label="Order statistics">
      <div>
        {stats.isPending ? (
          <Badge variant="outline">Connecting</Badge>
        ) : stats.isError ? (
          <Badge variant="destructive">Unavailable</Badge>
        ) : (
          <Badge
            variant="outline"
            className="border-emerald-200 bg-emerald-50 text-emerald-950"
          >
            Connected
          </Badge>
        )}
      </div>

      {stats.isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load statistics</AlertTitle>
          <AlertDescription>
            {formatApiError(stats.error, 'The stats service is unavailable.')}
          </AlertDescription>
        </Alert>
      ) : null}

      {stats.isPending ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
          {statCards.map((card) => (
            <Card key={card.label}>
              <CardHeader>
                <Skeleton className="h-9 w-16" />
                <Skeleton className="h-4 w-24" />
              </CardHeader>
            </Card>
          ))}
        </div>
      ) : null}

      {stats.data ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
          {statCards.map((card) => (
            <Card key={card.label}>
              <CardHeader>
                <CardTitle className="text-3xl tabular-nums">
                  {card.value(stats.data)}
                </CardTitle>
                <CardDescription>{card.label}</CardDescription>
              </CardHeader>
            </Card>
          ))}
        </div>
      ) : null}
    </section>
  )
}
