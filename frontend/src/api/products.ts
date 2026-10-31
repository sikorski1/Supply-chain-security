import type { Product } from '../types/product.ts'
import { apiRequest } from './client.ts'

export function getProducts(): Promise<Product[]> {
  return apiRequest<Product[]>({
    method: 'GET',
    url: '/api/products',
  })
}
