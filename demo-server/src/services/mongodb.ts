import { MongoClient, Db, Collection, ObjectId } from 'mongodb';

let client: MongoClient | null = null;
let db: Db | null = null;

/**
 * Initialize MongoDB client with timeout.
 */
export async function initMongoDB(): Promise<void> {
    const connectionString = process.env.MONGODB_CONNECTION_STRING ||
        'mongodb://root:root@localhost:27017';
    const dbName = process.env.MONGODB_DATABASE || 'demo';

    client = new MongoClient(connectionString, {
        serverSelectionTimeoutMS: 5000,
        connectTimeoutMS: 5000,
    });
    await client.connect();
    db = client.db(dbName);
}

/**
 * Get the MongoDB database instance.
 */
export function getDb(): Db {
    if (!db) {
        throw new Error('MongoDB not initialized. Call initMongoDB() first.');
    }
    return db;
}

/**
 * Get a collection by name.
 */
export function getCollection<T extends Document = Document>(name: string): Collection<T> {
    return getDb().collection<T>(name);
}

/**
 * Check MongoDB connection health with timeout.
 */
export async function healthCheck(): Promise<boolean> {
    if (!db) {
        console.error('MongoDB health check failed: not initialized');
        return false;
    }

    const timeoutMs = 5000;

    const healthPromise = (async () => {
        try {
            await db!.command({ ping: 1 });
            return true;
        } catch (error) {
            console.error('MongoDB health check failed:', error);
            return false;
        }
    })();

    const timeoutPromise = new Promise<boolean>((resolve) => {
        setTimeout(() => {
            console.error('MongoDB health check timed out');
            resolve(false);
        }, timeoutMs);
    });

    return Promise.race([healthPromise, timeoutPromise]);
}

/**
 * Close MongoDB connection.
 */
export async function closeMongoDB(): Promise<void> {
    if (client) {
        await client.close();
        client = null;
        db = null;
    }
}

/**
 * Insert a document.
 */
export async function insertOne<T extends object>(
    collectionName: string,
    document: T
): Promise<{ insertedId: string }> {
    const collection = getCollection(collectionName);
    const result = await collection.insertOne(document as any);
    return { insertedId: result.insertedId.toString() };
}

/**
 * Find a document by ID.
 */
export async function findById<T = unknown>(
    collectionName: string,
    id: string
): Promise<T | null> {
    const collection = getCollection(collectionName);
    const result = await collection.findOne({ _id: new ObjectId(id) });
    return result as T | null;
}

/**
 * Find documents by filter.
 */
export async function find<T = unknown>(
    collectionName: string,
    filter: object = {}
): Promise<T[]> {
    const collection = getCollection(collectionName);
    const results = await collection.find(filter).toArray();
    return results as T[];
}

/**
 * Delete a document by ID.
 */
export async function deleteById(
    collectionName: string,
    id: string
): Promise<{ deletedCount: number }> {
    const collection = getCollection(collectionName);
    const result = await collection.deleteOne({ _id: new ObjectId(id) });
    return { deletedCount: result.deletedCount };
}

/**
 * Delete documents by filter.
 */
export async function deleteMany(
    collectionName: string,
    filter: object
): Promise<{ deletedCount: number }> {
    const collection = getCollection(collectionName);
    const result = await collection.deleteMany(filter);
    return { deletedCount: result.deletedCount };
}
