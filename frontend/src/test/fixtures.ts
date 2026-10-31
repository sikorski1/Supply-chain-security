import type { Order, OrderStatus } from '@/types/order'
import type { Product } from '@/types/product'
import type { DashboardStats } from '@/types/stats'

export const products: Product[] = [
  { id: 'p1', name: 'Keyboard', priceCents: 7900, stockQuantity: 12 },
  { id: 'p2', name: 'Mouse', priceCents: 2500, stockQuantity: 4 },
  { id: 'p3', name: 'Monitor', priceCents: 24900, stockQuantity: 0 },
]

export const orders: Order[] = [
  {
    id: '001',
    productId: 'p1',
    productName: 'Keyboard',
    quantity: 2,
    unitPriceCents: 7900,
    status: 'completed',
    createdAt: '2026-10-31T12:00:00.000Z',
    updatedAt: '2026-10-31T12:05:00.000Z',
  },
  {
    id: '002',
    productId: 'p2',
    productName: 'Mouse',
    quantity: 1,
    unitPriceCents: 2500,
    status: 'pending',
    createdAt: '2026-10-31T12:00:00.000Z',
    updatedAt: '2026-10-31T12:00:00.000Z',
  },
]

export const stats: DashboardStats = {
  totalOrders: 2,
  pendingOrders: 1,
  processingOrders: 0,
  completedOrders: 1,
  failedOrders: 0,
}

export function makeOrder(overrides: Partial<Order> = {}): Order {
  return {
    id: '001',
    productId: 'p1',
    productName: 'Keyboard',
    quantity: 1,
    unitPriceCents: 7900,
    status: 'pending' satisfies OrderStatus,
    createdAt: '2026-10-31T15:00:00.000Z',
    updatedAt: '2026-10-31T15:00:00.000Z',
    ...overrides,
  }
}
