import axios from 'axios'
import type { AxiosRequestConfig } from 'axios'
import type { ApiError } from '../types/api.ts'

export const apiClient = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL ?? '',
  headers: {
    Accept: 'application/json',
  },
})

export class ApiClientError extends Error {
  readonly code: string

  constructor(error: ApiError) {
    super(error.message)
    this.name = 'ApiClientError'
    this.code = error.code
  }
}

export function formatApiError(error: unknown, fallback: string): string {
  if (error instanceof ApiClientError) {
    return `${error.code}: ${error.message}`
  }

  return fallback
}

function isApiError(value: unknown): value is ApiError {
  if (typeof value !== 'object' || value === null) {
    return false
  }

  const record = value as Record<string, unknown>
  return typeof record.code === 'string' && typeof record.message === 'string'
}

function toApiClientError(error: unknown): ApiClientError {
  if (axios.isAxiosError(error)) {
    const data: unknown = error.response?.data
    if (isApiError(data)) {
      return new ApiClientError(data)
    }

    return new ApiClientError({
      code: error.code ?? 'request_failed',
      message: error.message,
    })
  }

  if (error instanceof Error) {
    return new ApiClientError({
      code: 'request_failed',
      message: error.message,
    })
  }

  return new ApiClientError({
    code: 'request_failed',
    message: 'Request failed',
  })
}

export async function apiRequest<T>(config: AxiosRequestConfig): Promise<T> {
  try {
    const response = await apiClient.request<T>(config)
    return response.data
  } catch (error) {
    throw toApiClientError(error)
  }
}
