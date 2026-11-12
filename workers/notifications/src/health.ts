import { connect } from 'nats'
import { createDb } from './db.js'

const db = createDb()
try {
  await db.query('SELECT 1 FROM notifications LIMIT 0')
  const nc = await connect({ servers: process.env.NATS_URL || 'nats://localhost:4222', timeout: 2000 })
  try {
    const manager = await nc.jetstreamManager()
    await manager.consumers.info('ORDERS', 'notifications')
  } finally {
    await nc.close()
  }
} finally {
  await db.end()
}
