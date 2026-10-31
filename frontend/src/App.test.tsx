import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { selectProduct, submitOrder } from '@/test/form'
import { makeOrder, orders, stats } from '@/test/fixtures'
import { renderWithClient } from '@/test/render'
import { server } from '@/test/server'
import { statValue } from '@/test/stats'
import App from '@/App'
import { CreateOrderForm } from '@/components/CreateOrderForm'
import { DashboardStats } from '@/components/DashboardStats'
import { OrdersTable } from '@/components/OrdersTable'

describe('App', () => {
  it('renders the dashboard from products, orders, and stats', async () => {
    renderWithClient(<App />)

    expect(screen.getByRole('heading', { level: 1, name: 'Orders Dashboard' })).toBeInTheDocument()
    expect(await screen.findByText('Connected')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Recent Orders' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Create New Order' })).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: 'Keyboard' })).toBeInTheDocument()
    expect(statValue('Total orders')).toHaveTextContent('2')
    expect(await screen.findByRole('combobox', { name: 'Product' })).toBeEnabled()
  })

  it('refreshes stats and the order table after an order is created', async () => {
    const created = makeOrder({
      id: '003',
      productId: 'p1',
      productName: 'Keyboard',
      quantity: 1,
      unitPriceCents: 7900,
      status: 'pending',
    })
    let accepted = false

    server.use(
      http.get('/api/orders', () => HttpResponse.json(accepted ? [created, ...orders] : orders)),
      http.get('/api/stats', () => {
        return HttpResponse.json(
          accepted
            ? { ...stats, totalOrders: 3, pendingOrders: 2 }
            : stats,
        )
      }),
      http.post('/api/orders', () => {
        accepted = true
        return HttpResponse.json(created, { status: 201 })
      }),
    )

    const { user } = renderWithClient(
      <>
        <DashboardStats />
        <OrdersTable />
        <CreateOrderForm />
      </>,
    )

    expect(await screen.findByRole('cell', { name: '001' })).toBeInTheDocument()
    await selectProduct(user, 'Keyboard')
    await submitOrder(user)

    expect(await screen.findByRole('cell', { name: '003' })).toBeInTheDocument()
    await waitFor(() => expect(statValue('Total orders')).toHaveTextContent('3'))
    expect(statValue('Pending')).toHaveTextContent('2')
    expect(within(screen.getByRole('table')).getAllByText('Pending').length).toBeGreaterThan(0)
    expect(screen.getByRole('combobox', { name: 'Product' })).toHaveTextContent('Select product')
  })
})
