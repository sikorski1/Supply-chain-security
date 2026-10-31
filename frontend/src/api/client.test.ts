import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/server'
import { stats } from '@/test/fixtures'
import { ApiClientError, apiRequest, formatApiError } from './client'

describe('apiRequest', () => {
  it('returns JSON from the gateway', async () => {
    await expect(apiRequest({ method: 'GET', url: '/api/stats' })).resolves.toEqual(stats)
  })

  it('throws ApiClientError when the body is an ApiError', async () => {
    server.use(
      http.get('/api/products', () => {
        return HttpResponse.json({ code: 'NOPE', message: 'Down' }, { status: 500 })
      }),
    )

    await expect(apiRequest({ method: 'GET', url: '/api/products' })).rejects.toMatchObject({
      name: 'ApiClientError',
      code: 'NOPE',
      message: 'Down',
    })
  })

  it('wraps a non-JSON error response', async () => {
    server.use(
      http.get('/api/stats', () => HttpResponse.text('nope', { status: 500 })),
    )

    await expect(apiRequest({ method: 'GET', url: '/api/stats' })).rejects.toMatchObject({
      name: 'ApiClientError',
      message: expect.stringMatching(/500/),
    })
  })

  it('wraps a network failure', async () => {
    server.use(http.get('/api/stats', () => HttpResponse.error()))

    await expect(apiRequest({ method: 'GET', url: '/api/stats' })).rejects.toMatchObject({
      name: 'ApiClientError',
      code: 'ERR_NETWORK',
      message: 'Network Error',
    })
  })
})

describe('formatApiError', () => {
  it('includes the code and message', () => {
    const error = new ApiClientError({ code: 'INSUFFICIENT_STOCK', message: 'Not enough stock' })

    expect(formatApiError(error, 'fallback')).toBe('INSUFFICIENT_STOCK: Not enough stock')
  })

  it('returns the fallback for unknown values', () => {
    expect(formatApiError('down', 'The service is unavailable.')).toBe(
      'The service is unavailable.',
    )
  })
})
