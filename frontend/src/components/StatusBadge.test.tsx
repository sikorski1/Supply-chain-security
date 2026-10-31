import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { OrderStatus } from '@/types/order'
import { StatusBadge } from './StatusBadge'

const statuses = [
  ['pending', 'Pending', 'bg-amber-100'],
  ['processing', 'Processing', 'bg-sky-100'],
  ['completed', 'Completed', 'bg-emerald-100'],
  ['failed', 'Failed', 'bg-red-100'],
] as const satisfies ReadonlyArray<readonly [OrderStatus, string, string]>

describe('StatusBadge', () => {
  it.each(statuses)('renders %s as %s', (status, label, className) => {
    render(<StatusBadge status={status} />)

    expect(screen.getByText(label)).toHaveClass(className)
  })
})
