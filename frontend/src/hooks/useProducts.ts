import { useQuery } from '@tanstack/react-query'
import { getProducts } from '../api/products.ts'

export function useProducts() {
  return useQuery({
    queryKey: ['products'],
    queryFn: getProducts,
    staleTime: 30_000,
  })
}
