import { http, HttpResponse } from 'msw'
import type { CreateOrderRequest } from '@/types/order'
import { makeOrder, orders, products, stats } from './fixtures'

export const handlers = [
  http.get('/api/products', () => HttpResponse.json(products)),

  http.get('/api/orders', () => HttpResponse.json(orders)),

  http.get('/api/orders/:id', ({ params }) => {
    const id = String(params.id)
    const order = orders.find((item) => item.id === id) ?? makeOrder({ id })
    return HttpResponse.json(order)
  }),

  http.post('/api/orders', async ({ request }) => {
    const body = (await request.json()) as CreateOrderRequest
    const product = products.find((item) => item.id === body.productId)

    return HttpResponse.json(
      makeOrder({
        id: '003',
        productId: body.productId,
        productName: product?.name ?? 'Unknown',
        quantity: body.quantity,
        unitPriceCents: product?.priceCents ?? 0,
        status: 'pending',
      }),
      { status: 201 },
    )
  }),

  http.get('/api/stats', () => HttpResponse.json(stats)),
]
