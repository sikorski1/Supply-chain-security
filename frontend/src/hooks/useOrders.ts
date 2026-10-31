import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createOrder, getOrder, getOrders } from '../api/orders.ts'
import type { CreateOrderRequest } from '../types/order.ts'

export const orderKeys = {
  all: ['orders'] as const,
  detail: (id: string) => ['orders', id] as const,
}

export function useOrders() {
  return useQuery({
    queryKey: orderKeys.all,
    queryFn: getOrders,
    refetchInterval: 5000,
  })
}

export function useOrder(id: string) {
  return useQuery({
    queryKey: orderKeys.detail(id),
    queryFn: () => getOrder(id),
    enabled: Boolean(id),
  })
}

export function useCreateOrder() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (data: CreateOrderRequest) => createOrder(data),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: orderKeys.all,
      })
      queryClient.invalidateQueries({
        queryKey: ['stats'],
      })
      queryClient.invalidateQueries({
        queryKey: ['products'],
      })
    },
  })
}
