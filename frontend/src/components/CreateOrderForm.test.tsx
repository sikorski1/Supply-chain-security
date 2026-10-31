import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { deferred } from '@/test/deferred'
import { selectProduct, setQuantity, submitOrder } from '@/test/form'
import { makeOrder } from '@/test/fixtures'
import { renderWithClient } from '@/test/render'
import { server } from '@/test/server'
import { CreateOrderForm } from './CreateOrderForm'

describe('CreateOrderForm', () => {
  it('shows the price and stock for the selected product', async () => {
    const { user } = renderWithClient(<CreateOrderForm />)

    await selectProduct(user, 'Mouse')

    expect(screen.getByText('Price: $25.00 · In stock: 4')).toBeInTheDocument()
  })

  it('asks for a product when none is selected', async () => {
    const { user } = renderWithClient(<CreateOrderForm />)

    await submitOrder(user)

    expect(await screen.findByText('Select a product.')).toBeInTheDocument()
  })

  it('rejects quantities outside available stock and fractional amounts', async () => {
    const { user } = renderWithClient(<CreateOrderForm />)

    await selectProduct(user, 'Mouse')
    await setQuantity(user, '9')
    await submitOrder(user)

    expect(await screen.findByText('Quantity must be between 1 and 4.')).toBeInTheDocument()

    await setQuantity(user, '1.5')
    await submitOrder(user)

    expect(screen.getByText('Quantity must be a whole number.')).toBeInTheDocument()
  })

  it('rejects an out-of-stock product', async () => {
    const { user } = renderWithClient(<CreateOrderForm />)

    await selectProduct(user, 'Monitor')
    await submitOrder(user)

    expect(await screen.findByText('This product is out of stock.')).toBeInTheDocument()
  })

  it('shows an empty catalog and disables submit', async () => {
    server.use(http.get('/api/products', () => HttpResponse.json([])))

    renderWithClient(<CreateOrderForm />)

    expect(await screen.findByText('No products available.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create Order' })).toBeDisabled()
    expect(screen.getByRole('combobox', { name: 'Product' })).toBeDisabled()
  })

  it('shows a product service error', async () => {
    server.use(
      http.get('/api/products', () => {
        return HttpResponse.json(
          { code: 'PRODUCTS_DOWN', message: 'Catalog unavailable' },
          { status: 503 },
        )
      }),
    )

    renderWithClient(<CreateOrderForm />)

    expect(await screen.findByText('PRODUCTS_DOWN: Catalog unavailable')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create Order' })).toBeDisabled()
  })

  it('shows INSUFFICIENT_STOCK from the API', async () => {
    server.use(
      http.post('/api/orders', () => {
        return HttpResponse.json(
          { code: 'INSUFFICIENT_STOCK', message: 'Not enough stock for this order.' },
          { status: 409 },
        )
      }),
    )
    const { user } = renderWithClient(<CreateOrderForm />)

    await selectProduct(user, 'Keyboard')
    await submitOrder(user)

    expect(
      await screen.findByText('INSUFFICIENT_STOCK: Not enough stock for this order.'),
    ).toBeInTheDocument()
  })

  it('shows Creating... and resets the form after the order is accepted', async () => {
    const gate = deferred()
    server.use(
      http.post('/api/orders', async () => {
        await gate.promise
        return HttpResponse.json(
          makeOrder({
            id: '003',
            productName: 'Keyboard',
            quantity: 1,
            status: 'pending',
          }),
          { status: 201 },
        )
      }),
    )
    const { user } = renderWithClient(<CreateOrderForm />)

    await selectProduct(user, 'Keyboard')
    await submitOrder(user)

    expect(await screen.findByRole('button', { name: 'Creating...' })).toBeDisabled()
    expect(screen.getByRole('combobox', { name: 'Product' })).toBeDisabled()

    gate.resolve()

    expect(await screen.findByRole('button', { name: 'Create Order' })).toBeEnabled()
    expect(screen.getByRole('combobox', { name: 'Product' })).toHaveTextContent('Select product')
    expect(screen.queryByText(/In stock:/)).not.toBeInTheDocument()
  })
})
