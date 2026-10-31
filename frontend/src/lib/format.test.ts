import { describe, expect, it } from 'vitest'
import { formatDateTime, formatPrice } from './format'

describe('formatPrice', () => {
  it('formats cent amounts as USD', () => {
    expect(formatPrice(7900)).toBe('$79.00')
    expect(formatPrice(1)).toBe('$0.01')
  })

  it('formats zero cents', () => {
    expect(formatPrice(0)).toBe('$0.00')
  })
})

describe('formatDateTime', () => {
  it('includes the calendar date for a valid timestamp', () => {
    expect(formatDateTime('2026-10-31T12:00:00.000Z')).toContain('Oct 31, 2026')
  })

  it('returns an invalid timestamp unchanged', () => {
    expect(formatDateTime('not-a-date')).toBe('not-a-date')
  })
})
