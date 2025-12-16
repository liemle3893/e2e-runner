import express, { Express, Request, Response, NextFunction } from 'express';
import { initPostgres, closePostgres } from './services/postgres.js';
import { initRedis, closeRedis } from './services/redis.js';
import { initMongoDB, closeMongoDB } from './services/mongodb.js';
import { initEventHub, closeEventHub } from './services/eventhub.js';
import healthRoutes from './routes/health.js';
import usersRoutes from './routes/users.js';
import cacheRoutes from './routes/cache.js';
import documentsRoutes from './routes/documents.js';
import eventsRoutes from './routes/events.js';

const app: Express = express();
const PORT = process.env.PORT || 3000;

// Middleware
app.use(express.json());
app.use(express.urlencoded({ extended: true }));

// Request logging
app.use((req: Request, _res: Response, next: NextFunction) => {
    console.log(`${new Date().toISOString()} ${req.method} ${req.path}`);
    next();
});

// Routes
app.use('/health', healthRoutes);
app.use('/users', usersRoutes);
app.use('/cache', cacheRoutes);
app.use('/documents', documentsRoutes);
app.use('/events', eventsRoutes);

// Root endpoint
app.get('/', (_req: Request, res: Response) => {
    res.json({
        name: 'Demo Server',
        version: '1.0.0',
        endpoints: {
            health: '/health',
            users: '/users',
            cache: '/cache/:key',
            documents: '/documents',
            events: '/events',
        },
    });
});

// Error handling
app.use((err: Error, _req: Request, res: Response, _next: NextFunction) => {
    console.error('Unhandled error:', err);
    res.status(500).json({ error: 'Internal server error' });
});

// Graceful shutdown
async function shutdown(): Promise<void> {
    console.log('\nShutting down...');

    await Promise.all([
        closePostgres(),
        closeRedis(),
        closeMongoDB(),
        closeEventHub(),
    ]);

    console.log('All connections closed');
    process.exit(0);
}

process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);

// Initialize services and start server
async function main(): Promise<void> {
    console.log('Initializing services...');

    // Initialize services - continue even if some fail
    // PostgreSQL (sync init, connection is lazy)
    try {
        initPostgres();
        console.log('PostgreSQL initialized');
    } catch (error) {
        console.warn('PostgreSQL initialization failed:', error);
    }

    // Redis (sync init, connection is lazy)
    try {
        initRedis();
        console.log('Redis initialized');
    } catch (error) {
        console.warn('Redis initialization failed:', error);
    }

    // MongoDB (async init, requires connection)
    try {
        await initMongoDB();
        console.log('MongoDB initialized');
    } catch (error) {
        console.warn('MongoDB initialization failed (service may be unavailable):', (error as Error).message);
    }

    // EventHub (sync init, connection is lazy)
    try {
        initEventHub();
        console.log('EventHub initialized');
    } catch (error) {
        console.warn('EventHub initialization failed:', error);
    }

    // Start server regardless of service availability
    app.listen(PORT, () => {
        console.log(`Server running on http://localhost:${PORT}`);
        console.log('Available endpoints:');
        console.log('  GET  /health          - Health check');
        console.log('  POST /users           - Create user');
        console.log('  GET  /users/:id       - Get user');
        console.log('  PUT  /users/:id       - Update user');
        console.log('  DELETE /users/:id     - Delete user');
        console.log('  GET  /cache/:key      - Get cache value');
        console.log('  PUT  /cache/:key      - Set cache value');
        console.log('  DELETE /cache/:key    - Delete cache value');
        console.log('  POST /documents       - Create document');
        console.log('  GET  /documents       - List documents');
        console.log('  GET  /documents/:id   - Get document');
        console.log('  DELETE /documents/:id - Delete document');
        console.log('  POST /events          - Publish event');
        console.log('  GET  /events/consume  - Consume events (test)');
    });
}

main();
