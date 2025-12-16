import { Router, Request, Response } from 'express';
import * as redis from '../services/redis.js';

const router = Router();

interface SetCacheRequest {
    value: string;
    ttl?: number;
}

/**
 * GET /cache/:key - Get a cached value.
 */
router.get('/:key', async (req: Request, res: Response) => {
    try {
        const { key } = req.params;
        const value = await redis.get(key);

        if (value === null) {
            res.status(404).json({ error: 'Key not found' });
            return;
        }

        res.json({ key, value });
    } catch (error) {
        console.error('Error getting cache:', error);
        res.status(500).json({ error: 'Failed to get cache value' });
    }
});

/**
 * PUT /cache/:key - Set a cached value.
 */
router.put('/:key', async (req: Request, res: Response) => {
    try {
        const { key } = req.params;
        const { value, ttl } = req.body as SetCacheRequest;

        if (value === undefined) {
            res.status(400).json({ error: 'value is required' });
            return;
        }

        await redis.set(key, value, ttl);

        res.status(200).json({
            key,
            value,
            ttl: ttl || null,
            message: 'Cache value set successfully',
        });
    } catch (error) {
        console.error('Error setting cache:', error);
        res.status(500).json({ error: 'Failed to set cache value' });
    }
});

/**
 * DELETE /cache/:key - Delete a cached value.
 */
router.delete('/:key', async (req: Request, res: Response) => {
    try {
        const { key } = req.params;
        const deleted = await redis.del(key);

        if (deleted === 0) {
            res.status(404).json({ error: 'Key not found' });
            return;
        }

        res.status(204).send();
    } catch (error) {
        console.error('Error deleting cache:', error);
        res.status(500).json({ error: 'Failed to delete cache value' });
    }
});

/**
 * HEAD /cache/:key - Check if a key exists.
 */
router.head('/:key', async (req: Request, res: Response) => {
    try {
        const { key } = req.params;
        const exists = await redis.exists(key);

        if (!exists) {
            res.status(404).send();
            return;
        }

        res.status(200).send();
    } catch (error) {
        console.error('Error checking cache:', error);
        res.status(500).send();
    }
});

export default router;
