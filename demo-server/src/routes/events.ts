import { Router, Request, Response } from 'express';
import * as eventhub from '../services/eventhub.js';

const router = Router();

interface PublishEventRequest {
    type: string;
    data: Record<string, unknown>;
    partitionKey?: string;
}

/**
 * POST /events - Publish an event to EventHub.
 */
router.post('/', async (req: Request, res: Response) => {
    try {
        const { type, data, partitionKey } = req.body as PublishEventRequest;

        if (!type || !data) {
            res.status(400).json({ error: 'type and data are required' });
            return;
        }

        const event = {
            type,
            data,
            timestamp: new Date().toISOString(),
        };

        await eventhub.publish(event, partitionKey);

        res.status(202).json({
            message: 'Event published successfully',
            event,
        });
    } catch (error) {
        console.error('Error publishing event:', error);
        res.status(500).json({ error: 'Failed to publish event' });
    }
});

/**
 * GET /events/consume - Consume events from EventHub (for testing).
 */
router.get('/consume', async (req: Request, res: Response) => {
    try {
        const timeout = parseInt(req.query.timeout as string) || 5000;
        const maxEvents = parseInt(req.query.maxEvents as string) || 10;

        const events = await eventhub.consume(timeout, maxEvents);

        res.json({
            count: events.length,
            events: events.map(e => ({
                body: e.body,
                properties: e.properties,
                enqueuedTimeUtc: e.enqueuedTimeUtc,
                offset: e.offset,
                sequenceNumber: e.sequenceNumber,
                partitionKey: e.partitionKey,
            })),
        });
    } catch (error) {
        console.error('Error consuming events:', error);
        res.status(500).json({ error: 'Failed to consume events' });
    }
});

export default router;
