import type { DashboardStats } from '../types/stats.ts'
import { apiRequest } from './client.ts'

export function getStats(): Promise<DashboardStats> {
  return apiRequest<DashboardStats>({
    method: 'GET',
    url: '/api/stats',
  })
}
