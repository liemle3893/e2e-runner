/**
 * E2E Test Runner - Custom Error Classes
 */

/**
 * Base error class for E2E runner errors
 */
export class E2ERunnerError extends Error {
  public readonly code: string;
  public readonly hint?: string;

  constructor(message: string, code: string, hint?: string) {
    super(message);
    this.name = 'E2ERunnerError';
    this.code = code;
    this.hint = hint;
    Error.captureStackTrace(this, this.constructor);
  }
}

/**
 * Configuration-related errors
 */
export class ConfigurationError extends E2ERunnerError {
  constructor(message: string, hint?: string) {
    super(message, 'CONFIG_ERROR', hint);
    this.name = 'ConfigurationError';
  }
}

/**
 * Test validation errors
 */
export class ValidationError extends E2ERunnerError {
  public readonly filePath?: string;
  public readonly errors: SchemaError[];

  constructor(message: string, errors: SchemaError[] = [], filePath?: string) {
    super(message, 'VALIDATION_ERROR');
    this.name = 'ValidationError';
    this.filePath = filePath;
    this.errors = errors;
  }
}

export interface SchemaError {
  path: string;
  message: string;
  keyword?: string;
}

/**
 * Adapter connection errors
 */
export class ConnectionError extends E2ERunnerError {
  public readonly adapter: string;

  constructor(adapter: string, message: string, hint?: string) {
    super(`[${adapter}] ${message}`, 'CONNECTION_ERROR', hint);
    this.name = 'ConnectionError';
    this.adapter = adapter;
  }
}

/**
 * Test execution errors
 */
export class ExecutionError extends E2ERunnerError {
  public readonly testName?: string;
  public readonly phase?: string;
  public readonly stepId?: string;

  constructor(
    message: string,
    options: { testName?: string; phase?: string; stepId?: string } = {}
  ) {
    super(message, 'EXECUTION_ERROR');
    this.name = 'ExecutionError';
    this.testName = options.testName;
    this.phase = options.phase;
    this.stepId = options.stepId;
  }
}

/**
 * Assertion errors
 */
export class AssertionError extends E2ERunnerError {
  public readonly expected: unknown;
  public readonly actual: unknown;
  public readonly path?: string;
  public readonly operator?: string;

  constructor(
    message: string,
    options: {
      expected?: unknown;
      actual?: unknown;
      path?: string;
      operator?: string;
    } = {}
  ) {
    super(message, 'ASSERTION_ERROR');
    this.name = 'AssertionError';
    this.expected = options.expected;
    this.actual = options.actual;
    this.path = options.path;
    this.operator = options.operator;
  }

  /**
   * Format the assertion error with details
   */
  toDetailedString(): string {
    const lines = [this.message];

    if (this.path) {
      lines.push(`  Path: ${this.path}`);
    }
    if (this.operator) {
      lines.push(`  Operator: ${this.operator}`);
    }
    if (this.expected !== undefined) {
      lines.push(`  Expected: ${formatValue(this.expected)}`);
    }
    if (this.actual !== undefined) {
      lines.push(`  Actual: ${formatValue(this.actual)}`);
    }

    return lines.join('\n');
  }
}

/**
 * Timeout errors
 */
export class TimeoutError extends E2ERunnerError {
  public readonly timeoutMs: number;
  public readonly operation: string;

  constructor(operation: string, timeoutMs: number) {
    super(`Operation "${operation}" timed out after ${timeoutMs}ms`, 'TIMEOUT_ERROR');
    this.name = 'TimeoutError';
    this.timeoutMs = timeoutMs;
    this.operation = operation;
  }
}

/**
 * Variable interpolation errors
 */
export class InterpolationError extends E2ERunnerError {
  public readonly expression: string;

  constructor(message: string, expression: string) {
    super(message, 'INTERPOLATION_ERROR');
    this.name = 'InterpolationError';
    this.expression = expression;
  }
}

/**
 * Test loader errors
 */
export class LoaderError extends E2ERunnerError {
  public readonly filePath: string;

  constructor(message: string, filePath: string, hint?: string) {
    super(message, 'LOADER_ERROR', hint);
    this.name = 'LoaderError';
    this.filePath = filePath;
  }
}

/**
 * Adapter execution errors
 */
export class AdapterError extends E2ERunnerError {
  public readonly adapter: string;
  public readonly action: string;

  constructor(adapter: string, action: string, message: string) {
    super(`[${adapter}.${action}] ${message}`, 'ADAPTER_ERROR');
    this.name = 'AdapterError';
    this.adapter = adapter;
    this.action = action;
  }
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Format a value for display in error messages
 */
function formatValue(value: unknown): string {
  if (value === undefined) return 'undefined';
  if (value === null) return 'null';
  if (typeof value === 'string') return `"${value}"`;
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }
  return String(value);
}

/**
 * Check if an error is an E2E runner error
 */
export function isE2ERunnerError(error: unknown): error is E2ERunnerError {
  return error instanceof E2ERunnerError;
}

/**
 * Wrap an unknown error in an E2ERunnerError
 */
export function wrapError(error: unknown, context?: string): E2ERunnerError {
  if (error instanceof E2ERunnerError) {
    return error;
  }

  const message = error instanceof Error ? error.message : String(error);
  const fullMessage = context ? `${context}: ${message}` : message;

  return new E2ERunnerError(fullMessage, 'UNKNOWN_ERROR');
}
