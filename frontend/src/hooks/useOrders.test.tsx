import { renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { createQueryWrapper } from '@/test/render'
import { useOrder } from './useOrders'

describe('useOrder', () => {
  it('stays idle without an id and loads an order when an id is set', async () => {
    const { result, rerender } = renderHook(({ id }: { id: string }) => useOrder(id), {
      wrapper: createQueryWrapper(),
      initialProps: { id: '' },
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(result.current.data).toBeUndefined()

    rerender({ id: '001' })

    await waitFor(() => expect(result.current.data?.productName).toBe('Keyboard'))
    expect(result.current.data?.status).toBe('completed')
  })
})
