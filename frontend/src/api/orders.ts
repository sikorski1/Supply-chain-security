import type { CreateOrderRequest, Order } from '../types/order.ts'
import { apiRequest } from './client.ts'

export function getOrders(): Promise<Order[]> {
  return apiRequest<Order[]>({
    method: 'GET',
    url: '/api/orders',
  })
}

export function getOrder(id: string): Promise<Order> {
  return apiRequest<Order>({
    method: 'GET',
    url: `/api/orders/${encodeURIComponent(id)}`,
  })
}

export function createOrder(request: CreateOrderRequest): Promise<Order> {
  return apiRequest<Order>({
    method: 'POST',
    url: '/api/orders',
    data: request,
  })
}
