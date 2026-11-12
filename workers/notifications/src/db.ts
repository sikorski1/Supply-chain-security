import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { Pool } from "pg";
import type { Notification } from "./notifications.js";

const defaultDsn = "postgresql://app:app@localhost:5432/supply_chain?sslmode=disable";

export type Sql = {
	query: (text: string, values?: unknown[]) => Promise<{ rowCount: number | null }>;
};

export type Database = Sql & {
	end: () => Promise<void>;
};

export function createDb(connectionString = process.env.GOOSE_DB_STRING || defaultDsn): Database {
	const pool = new Pool({ connectionString });
	pool.on("error", (error: Error) => {
		console.error("database connection error", error);
	});

	return {
		query(text, values) {
			return pool.query(text, values);
		},
		end() {
			return pool.end();
		},
	};
}

export function migrationStatements(sql: string): string[] {
	const up = sql.split("-- +goose Down")[0] ?? sql;
	const body = up
		.split("\n")
		.filter(line => !line.trim().startsWith("--"))
		.join("\n")
		.replaceAll("CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ")
		.replaceAll("CREATE INDEX ", "CREATE INDEX IF NOT EXISTS ");

	return body
		.split(";")
		.map(statement => statement.trim())
		.filter(statement => statement.length > 0);
}

export async function ensureSchema(db: Sql): Promise<void> {
	const path = join(dirname(fileURLToPath(import.meta.url)), "../migrations/00001_create_notifications.sql");
	const sql = readFileSync(path, "utf8");
	for (const statement of migrationStatements(sql)) {
		await db.query(statement);
	}
}

export async function insertNotification(db: Sql, notification: Notification): Promise<boolean> {
	const result = await db.query(
		`INSERT INTO notifications (id, event_id, order_id, type, message)
     VALUES ($1, $2, $3, $4, $5)
     ON CONFLICT (event_id) DO NOTHING`,
		[notification.id, notification.eventId, notification.orderId, notification.type, notification.message],
	);
	return (result.rowCount ?? 0) > 0;
}
