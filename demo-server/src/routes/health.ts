import { Router, Request, Response } from 'express';
import * as postgres from '../services/postgres.js';
import * as redis from '../services/redis.js';
import * as mongodb from '../services/mongodb.js';
import * as eventhub from '../services/eventhub.js';

const router = Router();

interface HealthStatus {
    status: 'healthy' | 'unhealthy';
    services: {
        postgres: boolean;
        redis: boolean;
        mongodb: boolean;
        eventhub: boolean;
    };
    timestamp: string;
}

/**
 * GET /health - Check all service connections.
 */
router.get('/', async (_req: Request, res: Response) => {
    const [postgresOk, redisOk, mongodbOk, eventhubOk] = await Promise.all([
        postgres.healthCheck(),
        redis.healthCheck(),
        mongodb.healthCheck(),
        eventhub.healthCheck(),
    ]);

    const allHealthy = postgresOk && redisOk && mongodbOk && eventhubOk;

    const status: HealthStatus = {
        status: allHealthy ? 'healthy' : 'unhealthy',
        services: {
            postgres: postgresOk,
            redis: redisOk,
            mongodb: mongodbOk,
            eventhub: eventhubOk,
        },
        timestamp: new Date().toISOString(),
    };

    res.status(allHealthy ? 200 : 503).json(status);
});

/**
 * GET /health/:service - Check a specific service.
 */
router.get('/:service', async (req: Request, res: Response) => {
    const { service } = req.params;

    let healthy = false;
    switch (service) {
        case 'postgres':
            healthy = await postgres.healthCheck();
            break;
        case 'redis':
            healthy = await redis.healthCheck();
            break;
        case 'mongodb':
            healthy = await mongodb.healthCheck();
            break;
        case 'eventhub':
            healthy = await eventhub.healthCheck();
            break;
        default:
            res.status(404).json({ error: `Unknown service: ${service}` });
            return;
    }

    res.status(healthy ? 200 : 503).json({
        service,
        status: healthy ? 'healthy' : 'unhealthy',
        timestamp: new Date().toISOString(),
    });
});

export default router;
