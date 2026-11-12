import { AckPolicy, DeliverPolicy, connect, type JetStreamManager } from 'nats'
import { createDb, ensureSchema, insertNotification, type Database } from './db.js'
import { SUBJECTS, handleDelivery } from './notifications.js'

const streamName = 'ORDERS'
const consumerName = 'notifications'

async function main(): Promise<void> {
  const db = createDb()
  try {
    await ensureSchema(db)
    console.log('database connected')
    await consume(db)
  } finally {
    await db.end()
  }
}

async function consume(db: Database): Promise<void> {
  const servers = process.env.NATS_URL || 'nats://127.0.0.1:4222'
  const connection = await connect({ servers })
  try {
    const manager = await connection.jetstreamManager()
    await ensureStream(manager)
    await ensureConsumer(manager)

    const consumer = await connection.jetstream().consumers.get(streamName, consumerName)
    const messages = await consumer.consume()
    const stop = () => {
      messages.stop()
    }
    process.once('SIGINT', stop)
    process.once('SIGTERM', stop)

    console.log(`notifications worker consuming ${SUBJECTS.join(', ')}`)
    for await (const message of messages) {
      await handleDelivery(message, (notification) => insertNotification(db, notification))
    }
  } finally {
    await connection.drain()
  }
}

async function ensureStream(manager: JetStreamManager): Promise<void> {
  try {
    await manager.streams.info(streamName)
  } catch (error) {
    if (!isMissing(error)) throw error
    await manager.streams.add({
      name: streamName,
      subjects: ['orders.>'],
    })
  }
}

async function ensureConsumer(manager: JetStreamManager): Promise<void> {
  try {
    const info = await manager.consumers.info(streamName, consumerName)
    if (info.config.ack_policy !== AckPolicy.Explicit) {
      throw new Error(`consumer ${consumerName} must acknowledge messages explicitly`)
    }
  } catch (error) {
    if (!isMissing(error)) throw error
    await manager.consumers.add(streamName, {
      durable_name: consumerName,
      ack_policy: AckPolicy.Explicit,
      deliver_policy: DeliverPolicy.All,
      filter_subjects: [...SUBJECTS],
    })
  }
}

function isMissing(error: unknown): boolean {
  if (typeof error !== 'object' || error === null || !('code' in error)) {
    return false
  }

  const code = (error as { code: unknown }).code
  return code === 404 || code === '404' || code === 10059 || code === 10014
}

main().catch((error: unknown) => {
  console.error('notifications worker failed', error)
  process.exitCode = 1
})
