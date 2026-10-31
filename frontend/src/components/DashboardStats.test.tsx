import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { deferred } from '@/test/deferred'
import { stats } from '@/test/fixtures'
import { renderWithClient } from '@/test/render'
import { server } from '@/test/server'
import { statValue } from '@/test/stats'
import { DashboardStats } from './DashboardStats'

describe('DashboardStats', () => {
  it('shows Connecting until the stats response arrives', async () => {
    const gate = deferred()
    server.use(
      http.get('/api/stats', async () => {
        await gate.promise
        return HttpResponse.json(stats)
      }),
    )

    renderWithClient(<DashboardStats />)

    expect(screen.getByText('Connecting')).toBeInTheDocument()
    expect(document.querySelectorAll('[data-slot="skeleton"]')).toHaveLength(10)

    gate.resolve()

    expect(await screen.findByText('Connected')).toBeInTheDocument()
    expect(statValue('Total orders')).toHaveTextContent('2')
    expect(statValue('Pending')).toHaveTextContent('1')
    expect(statValue('Completed')).toHaveTextContent('1')
    expect(screen.queryByText('Connecting')).not.toBeInTheDocument()
  })

  it('shows Unavailable and the API error', async () => {
    server.use(
      http.get('/api/stats', () => {
        return HttpResponse.json(
          { code: 'STATS_DOWN', message: 'Stats unavailable' },
          { status: 503 },
        )
      }),
    )

    renderWithClient(<DashboardStats />)

    expect(await screen.findByText('Unavailable')).toBeInTheDocument()
    expect(screen.getByText('STATS_DOWN: Stats unavailable')).toBeInTheDocument()
    expect(screen.queryByText('Total orders')).not.toBeInTheDocument()
  })

  it('renders zero for every stat', async () => {
    server.use(
      http.get('/api/stats', () => {
        return HttpResponse.json({
          totalOrders: 0,
          pendingOrders: 0,
          processingOrders: 0,
          completedOrders: 0,
          failedOrders: 0,
        })
      }),
    )

    renderWithClient(<DashboardStats />)

    expect(await screen.findByText('Connected')).toBeInTheDocument()
    expect(screen.getAllByText('0')).toHaveLength(5)
  })

  it('maps every stat field from the response', async () => {
    server.use(
      http.get('/api/stats', () => {
        return HttpResponse.json({
          totalOrders: 10,
          pendingOrders: 4,
          processingOrders: 3,
          completedOrders: 2,
          failedOrders: 1,
        })
      }),
    )

    renderWithClient(<DashboardStats />)

    expect(await screen.findByText('Connected')).toBeInTheDocument()
    expect(statValue('Total orders')).toHaveTextContent('10')
    expect(statValue('Pending')).toHaveTextContent('4')
    expect(statValue('Processing')).toHaveTextContent('3')
    expect(statValue('Completed')).toHaveTextContent('2')
    expect(statValue('Failed')).toHaveTextContent('1')
  })
})
