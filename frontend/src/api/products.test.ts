import { describe, expect, it } from 'vitest'
import { products as productFixtures } from '@/test/fixtures'
import { getProducts } from './products'

describe('getProducts', () => {
  it('loads the product catalog', async () => {
    await expect(getProducts()).resolves.toEqual(productFixtures)
  })
})
