export const SUBJECTS = ['orders.created', 'orders.completed', 'orders.failed'] as const

export type NotificationType = (typeof SUBJECTS)[number]

export type Notification = {
  id: string
  eventId: string
  orderId: string
  type: NotificationType
  message: string
}

export type Delivery = {
  subject: string
  data: Uint8Array
  ack: () => void
  nak: (delay?: number) => void
  term: (reason?: string) => void
}

export class PermanentError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'PermanentError'
  }
}

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

const messages: Record<NotificationType, (orderId: string) => string> = {
  'orders.created': (orderId) => `Order ${orderId} was created`,
  'orders.completed': (orderId) => `Order ${orderId} was completed`,
  'orders.failed': (orderId) => `Order ${orderId} failed`,
}

export function toNotification(subject: string, data: Uint8Array): Notification {
  const type = notificationType(subject)
  const body = readBody(data)
  if (body.type !== undefined && body.type !== type) {
    throw new PermanentError('event type does not match subject')
  }

  const orderId = readUuid(body.orderId, 'orderId')
  return {
    id: crypto.randomUUID(),
    eventId: readUuid(body.eventId, 'eventId'),
    orderId,
    type,
    message: messages[type](orderId),
  }
}

export async function handleDelivery(
  delivery: Delivery,
  save: (notification: Notification) => Promise<unknown>,
): Promise<void> {
  try {
    const notification = toNotification(delivery.subject, delivery.data)
    await save(notification)
    delivery.ack()
  } catch (error) {
    if (error instanceof PermanentError) {
      console.error('dropping notification event', error.message)
      delivery.term(error.message)
      return
    }

    console.error('notification event will be retried', error)
    delivery.nak(1000)
  }
}

function notificationType(subject: string): NotificationType {
  if (!SUBJECTS.includes(subject as NotificationType)) {
    throw new PermanentError(`unsupported subject ${subject}`)
  }
  return subject as NotificationType
}

function readBody(data: Uint8Array): Record<string, unknown> {
  let parsed: unknown
  try {
    parsed = JSON.parse(new TextDecoder().decode(data))
  } catch {
    throw new PermanentError('event body must be JSON')
  }

  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
    throw new PermanentError('event body must be a JSON object')
  }

  return parsed as Record<string, unknown>
}

function readUuid(value: unknown, field: string): string {
  if (typeof value !== 'string' || !uuidPattern.test(value.trim())) {
    throw new PermanentError(`${field} must be a UUID`)
  }
  return value.trim().toLowerCase()
}
