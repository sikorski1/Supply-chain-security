import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, type RenderOptions } from '@testing-library/react'
import userEvent, { type UserEvent } from '@testing-library/user-event'
import type { ReactElement, ReactNode } from 'react'

export function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: false,
      },
      mutations: {
        retry: false,
      },
    },
  })
}

export function createQueryWrapper(queryClient = createQueryClient()) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

export function renderWithClient(ui: ReactElement, options?: Omit<RenderOptions, 'wrapper'>) {
  const queryClient = createQueryClient()
  const user = userEvent.setup()

  return {
    user,
    queryClient,
    ...render(ui, {
      wrapper: createQueryWrapper(queryClient),
      ...options,
    }),
  }
}

export type { UserEvent }
