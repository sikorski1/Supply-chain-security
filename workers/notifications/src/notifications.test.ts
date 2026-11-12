import { afterEach, describe, expect, it, vi } from 'vitest'
import { insertNotification, type Sql } from './db.js'
import {
  SUBJECTS,
  handleDelivery,
  toNotification,
  type Delivery,
  type Notification,
} from './notifications.js'

const eventId = '11111111-1111-4111-8111-111111111111'
const orderId = '22222222-2222-4222-8222-222222222222'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('toNotification', () => {
  it('builds one notification for each order subject', () => {
    const bodies = {
      'orders.created': `Order ${orderId} was created`,
      'orders.completed': `Order ${orderId} was completed`,
      'orders.failed': `Order ${orderId} failed`,
    } as const

    for (const subject of SUBJECTS) {
      const notification = toNotification(subject, body({ eventId, orderId }))
      expect(notification).toMatchObject({
        eventId,
        orderId,
        type: subject,
        message: bodies[subject],
      })
      expect(notification.id).toMatch(/^[0-9a-f-]{36}$/)
    }
  })

  it('accepts a matching type and lowercase uuid text', () => {
    const notification = toNotification(
      'orders.completed',
      body({ eventId: eventId.toUpperCase(), orderId: `  ${orderId}  `, type: 'orders.completed' }),
    )

    expect(notification.eventId).toBe(eventId)
    expect(notification.orderId).toBe(orderId)
  })

  it.each([
    ['orders.created', 'not-json'],
    ['orders.created', JSON.stringify(['nope'])],
    ['orders.created', JSON.stringify({ orderId })],
    ['orders.created', JSON.stringify({ eventId: 'event-1', orderId })],
    ['orders.created', JSON.stringify({ eventId, orderId: 'order-1' })],
    ['orders.created', JSON.stringify({ eventId, orderId, type: 'orders.failed' })],
    ['orders.cancelled', JSON.stringify({ eventId, orderId })],
  ])('rejects %s with %s', (subject, raw) => {
    expect(() => toNotification(subject, new TextEncoder().encode(raw))).toThrow()
  })
})

describe('handleDelivery', () => {
  it('acknowledges only after the notification is saved', async () => {
    const message = delivery('orders.created', { eventId, orderId })
    let release = () => {}
    const saved = new Promise<void>((resolve) => {
      release = resolve
    })
    let started = false

    const pending = handleDelivery(message, async () => {
      started = true
      await saved
    })

    await vi.waitFor(() => {
      expect(started).toBe(true)
    })
    expect(message.actions).toEqual([])

    release()
    await pending
    expect(message.actions).toEqual(['ack'])
  })

  it('does not acknowledge a failed save', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const message = delivery('orders.failed', { eventId, orderId })

    await handleDelivery(message, async () => {
      throw new Error('database unavailable')
    })

    expect(message.actions).toEqual(['nak'])
  })

  it('terminates a permanently invalid message without saving it', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const message = delivery('orders.created', { eventId: 'bad', orderId })
    const save = vi.fn<(notification: Notification) => Promise<unknown>>()

    await handleDelivery(message, save)

    expect(save).not.toHaveBeenCalled()
    expect(message.actions).toEqual(['term'])
  })

  it('acknowledges a redelivery when the event was already stored', async () => {
    const message = delivery('orders.completed', { eventId, orderId })
    const queries: Array<{ text: string; values: unknown[] | undefined }> = []
    const db: Sql = {
      async query(text, values) {
        queries.push({ text, values })
        return { rowCount: 0 }
      },
    }

    await handleDelivery(message, (notification) => insertNotification(db, notification))

    expect(message.actions).toEqual(['ack'])
    expect(queries).toHaveLength(1)
    expect(queries[0]?.text).toContain('ON CONFLICT (event_id) DO NOTHING')
    expect(queries[0]?.values).toEqual([
      expect.any(String),
      eventId,
      orderId,
      'orders.completed',
      `Order ${orderId} was completed`,
    ])
  })
})

function body(value: unknown): Uint8Array {
  return new TextEncoder().encode(JSON.stringify(value))
}

function delivery(subject: string, value: unknown): Delivery & { actions: string[] } {
  const actions: string[] = []
  return {
    subject,
    data: body(value),
    actions,
    ack() {
      actions.push('ack')
    },
    nak() {
      actions.push('nak')
    },
    term() {
      actions.push('term')
    },
  }
}
