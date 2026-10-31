import { formatApiError } from '@/api/client'
import { StatusBadge } from '@/components/StatusBadge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useOrders } from '@/hooks/useOrders'
import { formatDateTime, formatPrice } from '@/lib/format'

export function OrdersTable() {
  const orders = useOrders()

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <h2>Recent Orders</h2>
        </CardTitle>
        <CardDescription>Auto-refresh: 5s</CardDescription>
      </CardHeader>
      <CardContent>
        {orders.isError ? (
          <Alert variant="destructive">
            <AlertTitle>Could not load orders</AlertTitle>
            <AlertDescription>
              {formatApiError(orders.error, 'The orders service is unavailable.')}
            </AlertDescription>
          </Alert>
        ) : null}

        {orders.isPending ? (
          <div className="flex flex-col gap-3">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : null}

        {orders.data && orders.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">No orders yet.</p>
        ) : null}

        {orders.data && orders.data.length > 0 ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Product</TableHead>
                <TableHead>Qty</TableHead>
                <TableHead>Price</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {orders.data.map((order) => (
                <TableRow key={order.id}>
                  <TableCell className="font-mono text-xs">{order.id}</TableCell>
                  <TableCell>{order.productName}</TableCell>
                  <TableCell>{order.quantity}</TableCell>
                  <TableCell>{formatPrice(order.unitPriceCents)}</TableCell>
                  <TableCell>
                    <StatusBadge status={order.status} />
                  </TableCell>
                  <TableCell>{formatDateTime(order.createdAt)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : null}
      </CardContent>
    </Card>
  )
}
