import { screen, within } from '@testing-library/react'

export function statValue(label: string) {
  const region = screen.getByRole('region', { name: 'Order statistics' })
  const labelNode = within(region).getByText(label)
  const value = labelNode.previousElementSibling

  if (!(value instanceof HTMLElement)) {
    throw new Error(`Missing stat value for ${label}`)
  }

  return value
}
