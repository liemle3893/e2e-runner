/**
 * E2E Test Runner - EventHub Adapter
 *
 * Azure Event Hub operations
 */

import { AdapterError, AssertionError, TimeoutError } from '../errors';
import type { AdapterConfig, AdapterContext, AdapterStepResult, Logger } from '../types';
import { BaseAdapter } from './base.adapter';

// ============================================================================
// Types
// ============================================================================

export interface EventHubAssertion {
  path: string;
  equals?: unknown;
  contains?: string;
  matches?: string;
  exists?: boolean;
}

// ============================================================================
// EventHub Adapter
// ============================================================================

export class EventHubAdapter extends BaseAdapter {
  private producerClient: import('@azure/event-hubs').EventHubProducerClient | null = null;
  private consumerClient: import('@azure/event-hubs').EventHubConsumerClient | null = null;
  private receivedEvents: Map<string, unknown[]> = new Map();

  constructor(config: AdapterConfig, logger: Logger) {
    super(config, logger);
  }

  get name(): string {
    return 'eventhub';
  }

  async connect(): Promise<void> {
    if (this.connected) return;

    try {
      const { EventHubProducerClient, EventHubConsumerClient } = await import(
        '@azure/event-hubs'
      );

      const connectionString = this.config.connectionString as string;
      const consumerGroup = (this.config.consumerGroup as string) || '$Default';

      // Create producer for sending events
      this.producerClient = new EventHubProducerClient(connectionString);

      // Create consumer for receiving events
      this.consumerClient = new EventHubConsumerClient(consumerGroup, connectionString);

      // Test connection by getting properties
      await this.producerClient.getEventHubProperties();

      this.connected = true;
      this.logger.info('EventHub connected');
    } catch (error) {
      throw new AdapterError(
        'eventhub',
        'connect',
        `Failed to connect: ${error instanceof Error ? error.message : String(error)}`
      );
    }
  }

  async disconnect(): Promise<void> {
    if (this.producerClient) {
      await this.producerClient.close();
      this.producerClient = null;
    }
    if (this.consumerClient) {
      await this.consumerClient.close();
      this.consumerClient = null;
    }
    this.receivedEvents.clear();
    this.connected = false;
    this.logger.info('EventHub disconnected');
  }

  async execute(
    action: string,
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    if (!this.producerClient || !this.consumerClient) {
      throw new AdapterError('eventhub', action, 'Not connected');
    }

    this.logAction(action, { topic: params.topic });

    const start = Date.now();

    try {
      let result: AdapterStepResult;

      switch (action) {
        case 'publish':
          result = await this.publish(params, ctx);
          break;

        case 'waitFor':
          result = await this.waitFor(params, ctx);
          break;

        case 'consume':
          result = await this.consume(params, ctx);
          break;

        case 'clear':
          this.receivedEvents.clear();
          result = this.successResult(null, Date.now() - start);
          break;

        default:
          throw new AdapterError('eventhub', action, `Unknown action: ${action}`);
      }

      this.logResult(action, true, result.duration);
      return result;
    } catch (error) {
      const duration = Date.now() - start;
      this.logResult(action, false, duration);

      if (
        error instanceof AssertionError ||
        error instanceof AdapterError ||
        error instanceof TimeoutError
      ) {
        throw error;
      }

      throw new AdapterError(
        'eventhub',
        action,
        error instanceof Error ? error.message : String(error)
      );
    }
  }

  async healthCheck(): Promise<boolean> {
    if (!this.producerClient) return false;

    try {
      await this.producerClient.getEventHubProperties();
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Publish message(s) to EventHub
   */
  private async publish(
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    const start = Date.now();

    const batch = await this.producerClient!.createBatch({
      partitionKey: params.partitionKey as string | undefined,
    });

    const messages = params.messages || [params.message];
    for (const msg of messages as Record<string, unknown>[]) {
      if (!batch.tryAdd({ body: msg })) {
        throw new AdapterError('eventhub', 'publish', 'Event too large for batch');
      }
    }

    await this.producerClient!.sendBatch(batch);

    return this.successResult(
      { count: (messages as unknown[]).length },
      Date.now() - start
    );
  }

  /**
   * Wait for a message matching filter
   */
  private async waitFor(
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    const start = Date.now();
    const timeout = (params.timeout as number) || 30000;
    const filter = params.filter as Record<string, unknown>;

    return new Promise<AdapterStepResult>((resolve) => {
      let subscription: { close: () => Promise<void> } | null = null;

      const cleanup = async () => {
        if (subscription) {
          try {
            await subscription.close();
          } catch {
            // Ignore cleanup errors
          }
        }
      };

      // Set timeout handler
      const timeoutId = setTimeout(async () => {
        await cleanup();
        resolve(
          this.failResult(
            new TimeoutError(
              `Waiting for event matching filter: ${JSON.stringify(filter)}`,
              timeout
            ),
            Date.now() - start
          )
        );
      }, timeout);

      // Subscribe to events
      subscription = this.consumerClient!.subscribe({
        processEvents: async (events) => {
          for (const event of events) {
            const body = event.body as Record<string, unknown>;

            // Check if event matches filter
            if (this.matchesFilter(body, filter)) {
              clearTimeout(timeoutId);

              // Handle captures
              if (params.capture) {
                for (const [varName, path] of Object.entries(
                  params.capture as Record<string, string>
                )) {
                  ctx.capture(varName, this.getNestedValue(body, path));
                }
              }

              // Handle assertions
              if (params.assert) {
                try {
                  this.runAssertions(body, params.assert as EventHubAssertion[]);
                } catch (error) {
                  await cleanup();
                  resolve(this.failResult(error as Error, Date.now() - start));
                  return;
                }
              }

              await cleanup();
              resolve(this.successResult(body, Date.now() - start));
              return;
            }
          }
        },
        processError: async (error) => {
          clearTimeout(timeoutId);
          await cleanup();
          resolve(this.failResult(error, Date.now() - start));
        },
      });
    });
  }

  /**
   * Consume messages from EventHub
   */
  private async consume(
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    const start = Date.now();
    const count = (params.count as number) || 1;
    const timeout = (params.timeout as number) || 10000;
    const events: unknown[] = [];

    return new Promise<AdapterStepResult>((resolve) => {
      let subscription: { close: () => Promise<void> } | null = null;

      const cleanup = async () => {
        if (subscription) {
          try {
            await subscription.close();
          } catch {
            // Ignore cleanup errors
          }
        }
      };

      // Set timeout handler - resolve with whatever we have
      const timeoutId = setTimeout(async () => {
        await cleanup();
        resolve(this.successResult(events, Date.now() - start));
      }, timeout);

      subscription = this.consumerClient!.subscribe({
        processEvents: async (receivedEvents) => {
          events.push(...receivedEvents.map((e) => e.body));

          if (events.length >= count) {
            clearTimeout(timeoutId);
            await cleanup();
            resolve(this.successResult(events, Date.now() - start));
          }
        },
        processError: async (error) => {
          clearTimeout(timeoutId);
          await cleanup();
          resolve(this.failResult(error, Date.now() - start));
        },
      });
    });
  }

  /**
   * Check if event matches filter
   */
  private matchesFilter(
    event: Record<string, unknown>,
    filter: Record<string, unknown>
  ): boolean {
    if (!filter) return true;

    for (const [key, expected] of Object.entries(filter)) {
      const actual = this.getNestedValue(event, key);
      if (actual !== expected) {
        return false;
      }
    }
    return true;
  }

  /**
   * Get nested value using dot notation
   */
  private getNestedValue(obj: unknown, path: string): unknown {
    if (!path) return obj;

    return path.split('.').reduce((current, key) => {
      if (current && typeof current === 'object') {
        return (current as Record<string, unknown>)[key];
      }
      return undefined;
    }, obj);
  }

  /**
   * Run assertions on event data
   */
  private runAssertions(data: unknown, assertions: EventHubAssertion[]): void {
    for (const assertion of assertions) {
      const value = this.getNestedValue(data, assertion.path);

      if (assertion.exists === true && value === undefined) {
        throw new AssertionError(`${assertion.path} does not exist`, {
          path: assertion.path,
          operator: 'exists',
        });
      }

      if (assertion.equals !== undefined && value !== assertion.equals) {
        throw new AssertionError(
          `${assertion.path} = ${JSON.stringify(value)}, expected ${JSON.stringify(assertion.equals)}`,
          {
            path: assertion.path,
            expected: assertion.equals,
            actual: value,
            operator: 'equals',
          }
        );
      }

      if (assertion.contains && !String(value).includes(assertion.contains)) {
        throw new AssertionError(
          `${assertion.path} does not contain "${assertion.contains}"`,
          {
            path: assertion.path,
            expected: assertion.contains,
            actual: value,
            operator: 'contains',
          }
        );
      }

      if (assertion.matches && !new RegExp(assertion.matches).test(String(value))) {
        throw new AssertionError(
          `${assertion.path} does not match /${assertion.matches}/`,
          {
            path: assertion.path,
            expected: assertion.matches,
            actual: value,
            operator: 'matches',
          }
        );
      }
    }
  }
}
