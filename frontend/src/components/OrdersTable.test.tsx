import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { formatPrice } from '@/lib/format'
import { makeOrder, orders } from '@/test/fixtures'
import { renderWithClient } from '@/test/render'
import { server } from '@/test/server'
import { OrdersTable } from './OrdersTable'

describe('OrdersTable', () => {
  it('shows the refresh note and column headers', async () => {
    renderWithClient(<OrdersTable />)

    expect(screen.getByRole('heading', { name: 'Recent Orders' })).toBeInTheDocument()
    expect(screen.getByText('Auto-refresh: 5s')).toBeInTheDocument()
    expect(await screen.findByRole('cell', { name: '001' })).toBeInTheDocument()

    for (const header of ['ID', 'Product', 'Qty', 'Price', 'Status', 'Created']) {
      expect(screen.getByRole('columnheader', { name: header })).toBeInTheDocument()
    }
  })

  it('renders order rows with price, status, and date', async () => {
    renderWithClient(<OrdersTable />)

    expect(await screen.findByRole('cell', { name: 'Keyboard' })).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: 'Mouse' })).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: formatPrice(orders[0].unitPriceCents) })).toBeInTheDocument()
    expect(screen.getByText('Completed')).toBeInTheDocument()
    expect(screen.getByText('Pending')).toBeInTheDocument()
    expect(screen.getAllByText(/Oct 31, 2026/).length).toBeGreaterThan(0)
  })

  it('shows an empty message when there are no orders', async () => {
    server.use(http.get('/api/orders', () => HttpResponse.json([])))

    renderWithClient(<OrdersTable />)

    expect(await screen.findByText('No orders yet.')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('shows a load error', async () => {
    server.use(
      http.get('/api/orders', () => {
        return HttpResponse.json(
          { code: 'ORDERS_DOWN', message: 'Orders unavailable' },
          { status: 503 },
        )
      }),
    )

    renderWithClient(<OrdersTable />)

    expect(await screen.findByText('ORDERS_DOWN: Orders unavailable')).toBeInTheDocument()
    expect(screen.queryByText('No orders yet.')).not.toBeInTheDocument()
  })

  it('renders every order status', async () => {
    server.use(
      http.get('/api/orders', () => {
        return HttpResponse.json([
          makeOrder({ id: '101', status: 'pending', productName: 'Keyboard' }),
          makeOrder({ id: '102', status: 'processing', productName: 'Mouse' }),
          makeOrder({ id: '103', status: 'completed', productName: 'Monitor' }),
          makeOrder({ id: '104', status: 'failed', productName: 'Cable' }),
        ])
      }),
    )

    renderWithClient(<OrdersTable />)

    expect(await screen.findByText('Processing')).toBeInTheDocument()
    expect(screen.getByText('Pending')).toBeInTheDocument()
    expect(screen.getByText('Completed')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
  })
})
