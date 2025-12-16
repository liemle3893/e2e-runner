import { EventHubProducerClient, EventHubConsumerClient, ReceivedEventData } from '@azure/event-hubs';

let producer: EventHubProducerClient | null = null;
let consumer: EventHubConsumerClient | null = null;

/**
 * Get the EventHub connection string.
 * For the emulator, use a specific format.
 */
function getConnectionString(): string {
    return process.env.EVENTHUB_CONNECTION_STRING ||
        'Endpoint=sb://localhost;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=SAS_KEY_VALUE;UseDevelopmentEmulator=true;';
}

/**
 * Get the EventHub name.
 */
function getEventHubName(): string {
    return process.env.EVENTHUB_NAME || 'fortune-events';
}

/**
 * Get the consumer group.
 */
function getConsumerGroup(): string {
    return process.env.EVENTHUB_CONSUMER_GROUP || '$Default';
}

/**
 * Initialize EventHub producer client.
 */
export function initEventHub(): void {
    producer = new EventHubProducerClient(getConnectionString(), getEventHubName());
}

/**
 * Get the EventHub producer client.
 */
export function getProducer(): EventHubProducerClient {
    if (!producer) {
        throw new Error('EventHub producer not initialized. Call initEventHub() first.');
    }
    return producer;
}

/**
 * Check EventHub connection health with timeout.
 */
export async function healthCheck(): Promise<boolean> {
    const timeoutMs = 5000;

    const healthPromise = (async () => {
        try {
            const props = await getProducer().getEventHubProperties();
            return !!props.name;
        } catch (error) {
            console.error('EventHub health check failed:', error);
            return false;
        }
    })();

    const timeoutPromise = new Promise<boolean>((resolve) => {
        setTimeout(() => {
            console.error('EventHub health check timed out');
            resolve(false);
        }, timeoutMs);
    });

    return Promise.race([healthPromise, timeoutPromise]);
}

/**
 * Close EventHub connections.
 */
export async function closeEventHub(): Promise<void> {
    if (producer) {
        await producer.close();
        producer = null;
    }
    if (consumer) {
        await consumer.close();
        consumer = null;
    }
}

/**
 * Publish an event to EventHub.
 */
export async function publish(
    eventData: object,
    partitionKey?: string
): Promise<void> {
    const batch = await getProducer().createBatch({
        partitionKey,
    });

    const added = batch.tryAdd({
        body: eventData,
        properties: {
            timestamp: new Date().toISOString(),
        }
    });

    if (!added) {
        throw new Error('Event too large for batch');
    }

    await getProducer().sendBatch(batch);
}

/**
 * Consume events from EventHub (for testing).
 * Returns events received within the timeout period.
 */
export async function consume(
    timeoutMs: number = 5000,
    maxEvents: number = 10
): Promise<ReceivedEventData[]> {
    const events: ReceivedEventData[] = [];

    const tempConsumer = new EventHubConsumerClient(
        getConsumerGroup(),
        getConnectionString(),
        getEventHubName()
    );

    return new Promise((resolve) => {
        const timeout = setTimeout(async () => {
            await tempConsumer.close();
            resolve(events);
        }, timeoutMs);

        const subscription = tempConsumer.subscribe({
            processEvents: async (receivedEvents) => {
                events.push(...receivedEvents);
                if (events.length >= maxEvents) {
                    clearTimeout(timeout);
                    await subscription.close();
                    await tempConsumer.close();
                    resolve(events);
                }
            },
            processError: async (err) => {
                console.error('EventHub consumer error:', err);
            }
        }, {
            startPosition: { enqueuedOn: new Date(Date.now() - 60000) }
        });
    });
}
