import { Pool, PoolClient } from 'pg';

let pool: Pool | null = null;

/**
 * Initialize PostgreSQL connection pool.
 */
export function initPostgres(): void {
    pool = new Pool({
        connectionString: process.env.POSTGRES_CONNECTION_STRING ||
            'postgresql://fortune:fortune@localhost:5432/fortune_play',
        max: 10,
        idleTimeoutMillis: 30000,
        connectionTimeoutMillis: 5000,
    });
}

/**
 * Get the PostgreSQL pool instance.
 */
export function getPool(): Pool {
    if (!pool) {
        throw new Error('PostgreSQL pool not initialized. Call initPostgres() first.');
    }
    return pool;
}

/**
 * Check PostgreSQL connection health with timeout.
 */
export async function healthCheck(): Promise<boolean> {
    if (!pool) {
        console.error('PostgreSQL health check failed: not initialized');
        return false;
    }

    const timeoutMs = 5000;

    const healthPromise = (async () => {
        try {
            const client = await pool!.connect();
            await client.query('SELECT 1');
            client.release();
            return true;
        } catch (error) {
            console.error('PostgreSQL health check failed:', error);
            return false;
        }
    })();

    const timeoutPromise = new Promise<boolean>((resolve) => {
        setTimeout(() => {
            console.error('PostgreSQL health check timed out');
            resolve(false);
        }, timeoutMs);
    });

    return Promise.race([healthPromise, timeoutPromise]);
}

/**
 * Close PostgreSQL connection pool.
 */
export async function closePostgres(): Promise<void> {
    if (pool) {
        await pool.end();
        pool = null;
    }
}

/**
 * Execute a query with parameters.
 */
export async function query<T = unknown>(
    sql: string,
    params?: unknown[]
): Promise<T[]> {
    const result = await getPool().query(sql, params);
    return result.rows as T[];
}

/**
 * Execute a query and return the first row.
 */
export async function queryOne<T = unknown>(
    sql: string,
    params?: unknown[]
): Promise<T | null> {
    const rows = await query<T>(sql, params);
    return rows[0] || null;
}

/**
 * Execute a non-SELECT query (INSERT, UPDATE, DELETE).
 */
export async function execute(
    sql: string,
    params?: unknown[]
): Promise<{ rowCount: number }> {
    const result = await getPool().query(sql, params);
    return { rowCount: result.rowCount || 0 };
}
