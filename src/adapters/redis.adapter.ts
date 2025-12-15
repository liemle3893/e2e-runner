/**
 * E2E Test Runner - Redis Adapter
 *
 * Redis operations using ioredis
 */

import { AdapterError, AssertionError } from '../errors';
import type { AdapterConfig, AdapterContext, AdapterStepResult, Logger } from '../types';
import { BaseAdapter } from './base.adapter';

// ============================================================================
// Types
// ============================================================================

export interface RedisAssertion {
  equals?: string | number;
  isNull?: boolean;
  isNotNull?: boolean;
  greaterThan?: number;
  lessThan?: number;
  contains?: string;
  length?: number;
}

// ============================================================================
// Redis Adapter
// ============================================================================

export class RedisAdapter extends BaseAdapter {
  private client: import('ioredis').Redis | null = null;

  constructor(config: AdapterConfig, logger: Logger) {
    super(config, logger);
  }

  get name(): string {
    return 'redis';
  }

  async connect(): Promise<void> {
    if (this.connected) return;

    try {
      const Redis = (await import('ioredis')).default;

      this.client = new Redis(this.config.connectionString as string, {
        keyPrefix: this.config.keyPrefix as string | undefined,
        maxRetriesPerRequest: 3,
        lazyConnect: true,
        retryStrategy: (times) => {
          if (times > 3) return null;
          return Math.min(times * 200, 2000);
        },
      });

      await this.client.connect();

      // Test connection
      await this.client.ping();

      this.connected = true;
      this.logger.info('Redis connected');
    } catch (error) {
      throw new AdapterError(
        'redis',
        'connect',
        `Failed to connect: ${error instanceof Error ? error.message : String(error)}`
      );
    }
  }

  async disconnect(): Promise<void> {
    if (this.client) {
      await this.client.quit();
      this.client = null;
      this.connected = false;
      this.logger.info('Redis disconnected');
    }
  }

  async execute(
    action: string,
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    if (!this.client) {
      throw new AdapterError('redis', action, 'Not connected');
    }

    this.logAction(action, params);

    const start = Date.now();

    try {
      let result: unknown;

      switch (action) {
        case 'get':
          result = await this.client.get(params.key as string);
          break;

        case 'set':
          if (params.ttl) {
            await this.client.set(
              params.key as string,
              String(params.value),
              'EX',
              params.ttl as number
            );
          } else {
            await this.client.set(params.key as string, String(params.value));
          }
          result = 'OK';
          break;

        case 'del':
          result = await this.client.del(params.key as string);
          break;

        case 'exists':
          result = await this.client.exists(params.key as string);
          break;

        case 'incr':
          result = await this.client.incr(params.key as string);
          break;

        case 'hget':
          result = await this.client.hget(params.key as string, params.field as string);
          break;

        case 'hset':
          result = await this.client.hset(
            params.key as string,
            params.field as string,
            String(params.value)
          );
          break;

        case 'hgetall':
          result = await this.client.hgetall(params.key as string);
          break;

        case 'keys':
          result = await this.client.keys(params.pattern as string);
          break;

        case 'flushPattern':
          result = await this.flushPattern(params.pattern as string);
          break;

        default:
          throw new AdapterError('redis', action, `Unknown action: ${action}`);
      }

      const duration = Date.now() - start;

      // Handle capture
      if (params.capture) {
        ctx.capture(params.capture as string, result);
      }

      // Handle assertions
      if (params.assert) {
        this.runAssertion(result, params.assert as RedisAssertion);
      }

      this.logResult(action, true, duration);

      // Return rich result with command metadata for reporting
      return this.successResult({
        command: action.toUpperCase(),
        key: (params.key || params.pattern) as string | undefined,
        field: params.field as string | undefined,
        value: params.value,
        result,
      }, duration);
    } catch (error) {
      const duration = Date.now() - start;
      this.logResult(action, false, duration);

      if (error instanceof AssertionError || error instanceof AdapterError) {
        throw error;
      }

      throw new AdapterError(
        'redis',
        action,
        error instanceof Error ? error.message : String(error)
      );
    }
  }

  async healthCheck(): Promise<boolean> {
    if (!this.client) return false;

    try {
      const pong = await this.client.ping();
      return pong === 'PONG';
    } catch {
      return false;
    }
  }

  /**
   * Delete all keys matching a pattern
   */
  private async flushPattern(pattern: string): Promise<number> {
    const keys = await this.client!.keys(pattern);
    if (keys.length === 0) {
      return 0;
    }
    return await this.client!.del(...keys);
  }

  /**
   * Run assertion on Redis value
   */
  private runAssertion(value: unknown, assertion: RedisAssertion): void {
    if (assertion.equals !== undefined && String(value) !== String(assertion.equals)) {
      throw new AssertionError(
        `Value = ${JSON.stringify(value)}, expected ${JSON.stringify(assertion.equals)}`,
        {
          expected: assertion.equals,
          actual: value,
          operator: 'equals',
        }
      );
    }

    if (assertion.isNull && value !== null) {
      throw new AssertionError('Value is not null', {
        actual: value,
        operator: 'isNull',
      });
    }

    if (assertion.isNotNull && value === null) {
      throw new AssertionError('Value is null', {
        operator: 'isNotNull',
      });
    }

    if (assertion.greaterThan !== undefined && Number(value) <= assertion.greaterThan) {
      throw new AssertionError(
        `${value} is not > ${assertion.greaterThan}`,
        {
          expected: `> ${assertion.greaterThan}`,
          actual: value,
          operator: 'greaterThan',
        }
      );
    }

    if (assertion.lessThan !== undefined && Number(value) >= assertion.lessThan) {
      throw new AssertionError(
        `${value} is not < ${assertion.lessThan}`,
        {
          expected: `< ${assertion.lessThan}`,
          actual: value,
          operator: 'lessThan',
        }
      );
    }

    if (assertion.contains && !String(value).includes(assertion.contains)) {
      throw new AssertionError(
        `Value does not contain "${assertion.contains}"`,
        {
          expected: assertion.contains,
          actual: value,
          operator: 'contains',
        }
      );
    }

    if (assertion.length !== undefined) {
      const actualLength = Array.isArray(value) ? value.length : String(value).length;
      if (actualLength !== assertion.length) {
        throw new AssertionError(
          `Length = ${actualLength}, expected ${assertion.length}`,
          {
            expected: assertion.length,
            actual: actualLength,
            operator: 'length',
          }
        );
      }
    }
  }
}
