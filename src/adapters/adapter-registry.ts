/**
 * E2E Test Runner - Adapter Registry
 *
 * Manages adapter lifecycle and provides unified access
 */

import { ConnectionError } from '../errors';
import type {
  AdapterContext,
  AdapterType,
  EnvironmentConfig,
  Logger,
  UnifiedTestDefinition,
} from '../types';
import { BaseAdapter } from './base.adapter';
import { HTTPAdapter } from './http.adapter';
import { PostgreSQLAdapter } from './postgresql.adapter';
import { RedisAdapter } from './redis.adapter';
import { MongoDBAdapter } from './mongodb.adapter';
import { EventHubAdapter } from './eventhub.adapter';

// ============================================================================
// Types
// ============================================================================

export interface AdapterRegistryOptions {
  /** Only initialize these adapters (if not provided, all configured adapters are initialized) */
  requiredAdapters?: Set<AdapterType>;
}

// ============================================================================
// Adapter Registry
// ============================================================================

export class AdapterRegistry {
  private adapters: Map<AdapterType, BaseAdapter> = new Map();
  private logger: Logger;
  private config: EnvironmentConfig;
  private requiredAdapters?: Set<AdapterType>;

  constructor(config: EnvironmentConfig, logger: Logger, options?: AdapterRegistryOptions) {
    this.config = config;
    this.logger = logger;
    this.requiredAdapters = options?.requiredAdapters;
    this.initializeAdapters();
  }

  /**
   * Check if an adapter is required (or if no filter, all configured adapters are required)
   */
  private isRequired(type: AdapterType): boolean {
    if (!this.requiredAdapters) {
      return true; // No filter = all configured adapters
    }
    return this.requiredAdapters.has(type);
  }

  /**
   * Initialize adapters that are both configured AND required
   */
  private initializeAdapters(): void {
    // Always create HTTP adapter (no connection cost)
    if (this.isRequired('http')) {
      this.adapters.set(
        'http',
        new HTTPAdapter({ baseUrl: this.config.baseUrl }, this.logger)
      );
    }

    // Create PostgreSQL adapter if configured AND required
    if (this.isRequired('postgresql') && this.config.adapters?.postgresql) {
      this.adapters.set(
        'postgresql',
        new PostgreSQLAdapter(
          {
            connectionString: this.config.adapters.postgresql.connectionString,
            poolMin: 2,
            poolMax: this.config.adapters.postgresql.poolSize || 5,
          },
          this.logger
        )
      );
    }

    // Create Redis adapter if configured AND required
    if (this.isRequired('redis') && this.config.adapters?.redis) {
      this.adapters.set(
        'redis',
        new RedisAdapter(
          {
            connectionString: this.config.adapters.redis.connectionString,
            keyPrefix: this.config.adapters.redis.keyPrefix,
          },
          this.logger
        )
      );
    }

    // Create MongoDB adapter if configured AND required
    if (this.isRequired('mongodb') && this.config.adapters?.mongodb) {
      this.adapters.set(
        'mongodb',
        new MongoDBAdapter(
          {
            connectionString: this.config.adapters.mongodb.connectionString,
            database: this.config.adapters.mongodb.database,
          },
          this.logger
        )
      );
    }

    // Create EventHub adapter if configured AND required
    if (this.isRequired('eventhub') && this.config.adapters?.eventhub) {
      this.adapters.set(
        'eventhub',
        new EventHubAdapter(
          {
            connectionString: this.config.adapters.eventhub.connectionString,
            consumerGroup: this.config.adapters.eventhub.consumerGroup,
          },
          this.logger
        )
      );
    }
  }

  /**
   * Connect all adapters
   */
  async connectAll(): Promise<void> {
    const connectPromises: Promise<void>[] = [];

    for (const [name, adapter] of this.adapters) {
      connectPromises.push(
        adapter.connect().catch((error) => {
          throw new ConnectionError(
            name,
            error instanceof Error ? error.message : String(error)
          );
        })
      );
    }

    await Promise.all(connectPromises);
    this.logger.info(`Connected ${this.adapters.size} adapter(s)`);
  }

  /**
   * Disconnect all adapters
   */
  async disconnectAll(): Promise<void> {
    const disconnectPromises: Promise<void>[] = [];

    for (const [, adapter] of this.adapters) {
      disconnectPromises.push(
        adapter.disconnect().catch((error) => {
          this.logger.warn(
            `Failed to disconnect adapter: ${error instanceof Error ? error.message : String(error)}`
          );
        })
      );
    }

    await Promise.all(disconnectPromises);
    this.logger.info('All adapters disconnected');
  }

  /**
   * Get an adapter by type
   */
  get(type: AdapterType): BaseAdapter {
    const adapter = this.adapters.get(type);
    if (!adapter) {
      throw new Error(`Adapter not configured: ${type}`);
    }
    return adapter;
  }

  /**
   * Get HTTP adapter
   */
  getHTTP(): HTTPAdapter {
    return this.get('http') as HTTPAdapter;
  }

  /**
   * Get PostgreSQL adapter
   */
  getPostgreSQL(): PostgreSQLAdapter {
    return this.get('postgresql') as PostgreSQLAdapter;
  }

  /**
   * Get Redis adapter
   */
  getRedis(): RedisAdapter {
    return this.get('redis') as RedisAdapter;
  }

  /**
   * Get MongoDB adapter
   */
  getMongoDB(): MongoDBAdapter {
    return this.get('mongodb') as MongoDBAdapter;
  }

  /**
   * Get EventHub adapter
   */
  getEventHub(): EventHubAdapter {
    return this.get('eventhub') as EventHubAdapter;
  }

  /**
   * Check if an adapter is available
   */
  has(type: AdapterType): boolean {
    return this.adapters.has(type);
  }

  /**
   * Get all available adapter types
   */
  getAvailableAdapters(): AdapterType[] {
    return Array.from(this.adapters.keys());
  }

  /**
   * Health check all adapters
   */
  async healthCheckAll(): Promise<Map<AdapterType, boolean>> {
    const results = new Map<AdapterType, boolean>();

    for (const [name, adapter] of this.adapters) {
      try {
        const healthy = await adapter.healthCheck();
        results.set(name, healthy);
      } catch {
        results.set(name, false);
      }
    }

    return results;
  }

  /**
   * Health check a specific adapter
   */
  async healthCheck(type: AdapterType): Promise<boolean> {
    const adapter = this.adapters.get(type);
    if (!adapter) {
      return false;
    }
    return adapter.healthCheck();
  }

  /**
   * Create an adapter context for test execution
   */
  createContext(
    variables: Record<string, unknown>,
    captured: Record<string, unknown>,
    baseUrl: string
  ): AdapterContext {
    return {
      variables,
      captured,
      baseUrl,
      logger: this.logger,
      capture: (name: string, value: unknown) => {
        captured[name] = value;
      },
    };
  }
}

// ============================================================================
// Factory Functions
// ============================================================================

/**
 * Create an adapter registry from environment config
 */
export function createAdapterRegistry(
  config: EnvironmentConfig,
  logger: Logger,
  options?: AdapterRegistryOptions
): AdapterRegistry {
  return new AdapterRegistry(config, logger, options);
}

/**
 * Analyze test definitions and extract the set of required adapters
 */
export function getRequiredAdapters(tests: UnifiedTestDefinition[]): Set<AdapterType> {
  const adapters = new Set<AdapterType>();

  for (const test of tests) {
    // Collect all steps from all phases
    const allSteps = [
      ...(test.setup || []),
      ...test.execute,
      ...(test.verify || []),
      ...(test.teardown || []),
    ];

    // Extract adapter types from steps
    for (const step of allSteps) {
      adapters.add(step.adapter as AdapterType);
    }
  }

  return adapters;
}

/**
 * Get adapter type from string
 */
export function parseAdapterType(type: string): AdapterType {
  const validTypes: AdapterType[] = [
    'postgresql',
    'redis',
    'mongodb',
    'eventhub',
    'http',
  ];

  if (validTypes.includes(type as AdapterType)) {
    return type as AdapterType;
  }

  throw new Error(`Invalid adapter type: ${type}. Valid types: ${validTypes.join(', ')}`);
}
