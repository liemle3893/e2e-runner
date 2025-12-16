import Redis from 'ioredis';

let client: Redis | null = null;

/**
 * Initialize Redis client.
 */
export function initRedis(): void {
    const connectionString = process.env.REDIS_CONNECTION_STRING || 'redis://localhost:6379';
    client = new Redis(connectionString, {
        maxRetriesPerRequest: 3,
        retryDelayOnFailover: 100,
        lazyConnect: true,
    });
}

/**
 * Get the Redis client instance.
 */
export function getClient(): Redis {
    if (!client) {
        throw new Error('Redis client not initialized. Call initRedis() first.');
    }
    return client;
}

/**
 * Check Redis connection health with timeout.
 */
export async function healthCheck(): Promise<boolean> {
    const timeoutMs = 5000;

    const healthPromise = (async () => {
        try {
            const cli = getClient();
            // Only connect if not already connected
            if (cli.status === 'wait') {
                await cli.connect();
            }
            const result = await cli.ping();
            return result === 'PONG';
        } catch (error) {
            console.error('Redis health check failed:', error);
            return false;
        }
    })();

    const timeoutPromise = new Promise<boolean>((resolve) => {
        setTimeout(() => {
            console.error('Redis health check timed out');
            resolve(false);
        }, timeoutMs);
    });

    return Promise.race([healthPromise, timeoutPromise]);
}

/**
 * Close Redis connection.
 */
export async function closeRedis(): Promise<void> {
    if (client) {
        await client.quit();
        client = null;
    }
}

/**
 * Get a value by key.
 */
export async function get(key: string): Promise<string | null> {
    return getClient().get(key);
}

/**
 * Set a value with optional TTL in seconds.
 */
export async function set(
    key: string,
    value: string,
    ttlSeconds?: number
): Promise<void> {
    if (ttlSeconds) {
        await getClient().setex(key, ttlSeconds, value);
    } else {
        await getClient().set(key, value);
    }
}

/**
 * Delete a key.
 */
export async function del(key: string): Promise<number> {
    return getClient().del(key);
}

/**
 * Check if a key exists.
 */
export async function exists(key: string): Promise<boolean> {
    const result = await getClient().exists(key);
    return result === 1;
}
