import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { ensureSchema, insertNotification, migrationStatements, type Sql } from './db.js'
import type { Notification } from './notifications.js'

const notification: Notification = {
  id: '33333333-3333-4333-8333-333333333333',
  eventId: '11111111-1111-4111-8111-111111111111',
  orderId: '22222222-2222-4222-8222-222222222222',
  type: 'orders.created',
  message: 'Order 22222222-2222-4222-8222-222222222222 was created',
}

describe('insertNotification', () => {
  it('inserts a notification idempotently by event id', async () => {
    const queries: Array<{ text: string; values: unknown[] | undefined }> = []
    const db: Sql = {
      async query(text, values) {
        queries.push({ text, values })
        return { rowCount: 1 }
      },
    }

    await expect(insertNotification(db, notification)).resolves.toBe(true)
    expect(queries[0]?.text).toContain('ON CONFLICT (event_id) DO NOTHING')
    expect(queries[0]?.values).toEqual([
      notification.id,
      notification.eventId,
      notification.orderId,
      notification.type,
      notification.message,
    ])
  })

  it('reports an existing event as already stored', async () => {
    const db: Sql = {
      async query() {
        return { rowCount: 0 }
      },
    }

    await expect(insertNotification(db, notification)).resolves.toBe(false)
  })
})

describe('schema', () => {
  it('keeps event id unique and skips the goose down section', () => {
    const sql = `
      -- +goose Up
      CREATE TABLE notifications (
          event_id UUID NOT NULL UNIQUE
      );
      CREATE INDEX idx_notifications_created_at ON notifications (created_at DESC);
      -- +goose Down
      DROP TABLE IF EXISTS notifications;
    `

    const statements = migrationStatements(sql)
    expect(statements).toEqual([
      'CREATE TABLE IF NOT EXISTS notifications (\n          event_id UUID NOT NULL UNIQUE\n      )',
      'CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications (created_at DESC)',
    ])
  })

  it('applies the notifications migration without dropping the table', async () => {
    const queries: string[] = []
    const db: Sql = {
      async query(text) {
        queries.push(text)
        return { rowCount: null }
      },
    }

    await ensureSchema(db)

    const migration = readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), '../migrations/00001_create_notifications.sql'),
      'utf8',
    )
    expect(queries).toEqual(migrationStatements(migration))
    expect(queries.join('\n')).toContain('event_id UUID NOT NULL UNIQUE')
    expect(queries.join('\n')).toContain("'orders.created'")
    expect(queries.join('\n')).toContain("'orders.completed'")
    expect(queries.join('\n')).toContain("'orders.failed'")
    expect(queries.join('\n')).not.toContain('DROP TABLE')
    expect(queries[0]).toContain('CREATE TABLE IF NOT EXISTS notifications')
  })
})
