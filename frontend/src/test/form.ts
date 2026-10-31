import { screen, waitFor } from '@testing-library/react'
import type { UserEvent } from '@testing-library/user-event'
import { expect } from 'vitest'

export async function selectProduct(user: UserEvent, name: string) {
  const combobox = await screen.findByRole('combobox', { name: 'Product' })
  await waitFor(() => expect(combobox).toBeEnabled())
  await user.click(combobox)
  await user.click(await screen.findByRole('option', { name }))
}

export async function submitOrder(user: UserEvent) {
  const submit = await screen.findByRole('button', { name: 'Create Order' })
  await waitFor(() => expect(submit).toBeEnabled())
  await user.click(submit)
}

export async function setQuantity(user: UserEvent, value: string) {
  const field = screen.getByRole('spinbutton', { name: 'Quantity' })
  await user.clear(field)
  if (value !== '') {
    await user.type(field, value)
  }
}
