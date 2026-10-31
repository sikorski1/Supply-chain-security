import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { orders as orderFixtures } from '@/test/fixtures'
import { makeOrder } from '@/test/fixtures'
import { server } from '@/test/server'
import { createOrder, getOrder, getOrders } from './orders'

describe('order api', () => {
  it('loads the order list', async () => {
    await expect(getOrders()).resolves.toEqual(orderFixtures)
  })

  it('encodes order ids in the request path', async () => {
    let pathname = ''
    server.use(
      http.get('/api/orders/:id', ({ request }) => {
        pathname = new URL(request.url).pathname
        return HttpResponse.json(makeOrder({ id: 'a/b' }))
      }),
    )

    await expect(getOrder('a/b')).resolves.toMatchObject({ id: 'a/b' })
    expect(pathname).toBe('/api/orders/a%2Fb')
  })

  it('posts the create-order body', async () => {
    let body: unknown
    server.use(
      http.post('/api/orders', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(makeOrder({ id: '009', status: 'pending' }), { status: 201 })
      }),
    )

    const order = await createOrder({ productId: 'p2', quantity: 3 })

    expect(body).toEqual({ productId: 'p2', quantity: 3 })
    expect(order.id).toBe('009')
  })

  it('surfaces INSUFFICIENT_STOCK', async () => {
    server.use(
      http.post('/api/orders', () => {
        return HttpResponse.json(
          { code: 'INSUFFICIENT_STOCK', message: 'Not enough stock for this order.' },
          { status: 409 },
        )
      }),
    )

    await expect(createOrder({ productId: 'p2', quantity: 4 })).rejects.toMatchObject({
      name: 'ApiClientError',
      code: 'INSUFFICIENT_STOCK',
      message: 'Not enough stock for this order.',
    })
  })
})
