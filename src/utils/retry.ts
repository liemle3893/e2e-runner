/**
 * E2E Test Runner - Retry Utility
 */

import { TimeoutError } from '../errors';

export interface RetryOptions {
  /** Maximum number of attempts (1 = no retry) */
  maxAttempts: number;
  /** Base delay between retries in milliseconds */
  baseDelay: number;
  /** Maximum delay between retries in milliseconds */
  maxDelay: number;
  /** Whether to use exponential backoff */
  exponentialBackoff: boolean;
  /** Jitter factor (0-1) to randomize delays */
  jitterFactor: number;
  /** Optional function to determine if error is retryable */
  shouldRetry?: (error: Error, attempt: number) => boolean;
  /** Optional callback before each retry */
  onRetry?: (error: Error, attempt: number, delay: number) => void;
}

export const DEFAULT_RETRY_OPTIONS: RetryOptions = {
  maxAttempts: 1,
  baseDelay: 1000,
  maxDelay: 30000,
  exponentialBackoff: true,
  jitterFactor: 0.3,
};

/**
 * Execute a function with retry logic
 */
export async function withRetry<T>(
  fn: () => Promise<T>,
  options: Partial<RetryOptions> = {}
): Promise<T> {
  const config: RetryOptions = { ...DEFAULT_RETRY_OPTIONS, ...options };
  let lastError: Error | undefined;

  for (let attempt = 1; attempt <= config.maxAttempts; attempt++) {
    try {
      return await fn();
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));

      // Check if we should retry
      if (attempt >= config.maxAttempts) {
        break;
      }

      if (config.shouldRetry && !config.shouldRetry(lastError, attempt)) {
        break;
      }

      // Calculate delay
      const delay = calculateDelay(attempt, config);

      // Notify retry callback
      if (config.onRetry) {
        config.onRetry(lastError, attempt, delay);
      }

      // Wait before retrying
      await sleep(delay);
    }
  }

  throw lastError;
}

/**
 * Calculate delay for a retry attempt
 */
export function calculateDelay(attempt: number, options: RetryOptions): number {
  let delay = options.baseDelay;

  if (options.exponentialBackoff) {
    // Exponential backoff: baseDelay * 2^(attempt-1)
    delay = options.baseDelay * Math.pow(2, attempt - 1);
  }

  // Apply jitter
  if (options.jitterFactor > 0) {
    const jitter = delay * options.jitterFactor * Math.random();
    delay += jitter;
  }

  // Cap at max delay
  return Math.min(delay, options.maxDelay);
}

/**
 * Sleep for a specified duration
 */
export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Execute a function with a timeout
 */
export async function withTimeout<T>(
  fn: () => Promise<T>,
  timeoutMs: number,
  operation: string = 'Operation'
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timeoutId = setTimeout(() => {
      reject(new TimeoutError(operation, timeoutMs));
    }, timeoutMs);

    fn()
      .then((result) => {
        clearTimeout(timeoutId);
        resolve(result);
      })
      .catch((error) => {
        clearTimeout(timeoutId);
        reject(error);
      });
  });
}

/**
 * Execute a function with both retry and timeout
 */
export async function withRetryAndTimeout<T>(
  fn: () => Promise<T>,
  options: Partial<RetryOptions> & { timeout?: number; operation?: string } = {}
): Promise<T> {
  const { timeout, operation = 'Operation', ...retryOptions } = options;

  const wrappedFn = timeout ? () => withTimeout(fn, timeout, operation) : fn;

  return withRetry(wrappedFn, retryOptions);
}

/**
 * Create a retry wrapper with preset options
 */
export function createRetryWrapper(
  presetOptions: Partial<RetryOptions>
): <T>(fn: () => Promise<T>, options?: Partial<RetryOptions>) => Promise<T> {
  return (fn, options = {}) => withRetry(fn, { ...presetOptions, ...options });
}

/**
 * Measure execution duration
 */
export async function measureDuration<T>(
  fn: () => Promise<T>
): Promise<{ result: T; duration: number }> {
  const start = Date.now();
  const result = await fn();
  const duration = Date.now() - start;
  return { result, duration };
}

/**
 * Poll until a condition is met
 */
export async function pollUntil<T>(
  fn: () => Promise<T>,
  condition: (result: T) => boolean,
  options: {
    interval?: number;
    timeout?: number;
    operation?: string;
  } = {}
): Promise<T> {
  const { interval = 1000, timeout = 30000, operation = 'Poll' } = options;
  const startTime = Date.now();

  while (true) {
    const result = await fn();

    if (condition(result)) {
      return result;
    }

    const elapsed = Date.now() - startTime;
    if (elapsed >= timeout) {
      throw new TimeoutError(operation, timeout);
    }

    await sleep(interval);
  }
}
