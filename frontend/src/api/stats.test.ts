import { describe, expect, it } from 'vitest'
import { stats as statsFixture } from '@/test/fixtures'
import { getStats } from './stats'

describe('getStats', () => {
  it('loads dashboard stats', async () => {
    await expect(getStats()).resolves.toEqual(statsFixture)
  })
})
