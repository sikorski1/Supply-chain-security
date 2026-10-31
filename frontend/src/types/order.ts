export type OrderStatus = 'pending' | 'processing' | 'completed' | 'failed'

export interface Order {
  id: string
  productId: string
  productName: string
  quantity: number
  unitPriceCents: number
  status: OrderStatus
  createdAt: string
  updatedAt: string
}

export interface CreateOrderRequest {
  productId: string
  quantity: number
}
