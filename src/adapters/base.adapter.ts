/**
 * E2E Test Runner - Base Adapter
 *
 * Abstract base class for all adapters
 */

import type {
  AdapterConfig,
  AdapterContext,
  AdapterStepResult,
  Logger,
} from '../types';

/**
 * Abstract base class for E2E test adapters
 */
export abstract class BaseAdapter {
  protected config: AdapterConfig;
  protected logger: Logger;
  protected connected: boolean = false;

  constructor(config: AdapterConfig, logger: Logger) {
    this.config = config;
    this.logger = logger;
  }

  /**
   * Get the adapter name for logging
   */
  abstract get name(): string;

  /**
   * Connect to the underlying service
   */
  abstract connect(): Promise<void>;

  /**
   * Disconnect from the underlying service
   */
  abstract disconnect(): Promise<void>;

  /**
   * Execute an action on the adapter
   */
  abstract execute(
    action: string,
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult>;

  /**
   * Check if the service is healthy
   */
  abstract healthCheck(): Promise<boolean>;

  /**
   * Check if the adapter is connected
   */
  isConnected(): boolean {
    return this.connected;
  }

  /**
   * Execute with duration measurement
   */
  protected async measureDuration<T>(
    fn: () => Promise<T>
  ): Promise<{ result: T; duration: number }> {
    const start = Date.now();
    const result = await fn();
    const duration = Date.now() - start;
    return { result, duration };
  }

  /**
   * Create a successful result
   */
  protected successResult(
    data: unknown,
    duration: number
  ): AdapterStepResult {
    return {
      success: true,
      data,
      duration,
    };
  }

  /**
   * Create a failed result
   */
  protected failResult(
    error: Error,
    duration: number
  ): AdapterStepResult {
    return {
      success: false,
      error,
      duration,
    };
  }

  /**
   * Log adapter action
   */
  protected logAction(action: string, params?: Record<string, unknown>): void {
    this.logger.debug(`[${this.name}] Executing ${action}`, params || {});
  }

  /**
   * Log action result
   */
  protected logResult(
    action: string,
    success: boolean,
    duration: number
  ): void {
    if (success) {
      this.logger.debug(`[${this.name}] ${action} completed in ${duration}ms`);
    } else {
      this.logger.error(`[${this.name}] ${action} failed after ${duration}ms`);
    }
  }
}

/**
 * Helper to run assertions on adapter results
 */
export function runAdapterAssertions(
  data: unknown,
  assertions: unknown,
  assertionRunner: (data: unknown, assertions: unknown) => void
): void {
  if (!assertions) return;
  assertionRunner(data, assertions);
}

/**
 * Helper to capture values from adapter results
 */
export function captureValues(
  data: unknown,
  capture: Record<string, string> | undefined,
  ctx: AdapterContext,
  getValueFn: (data: unknown, path: string) => unknown
): void {
  if (!capture) return;

  for (const [varName, path] of Object.entries(capture)) {
    const value = getValueFn(data, path);
    ctx.capture(varName, value);
  }
}
