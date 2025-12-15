/**
 * E2E Test Runner - Base Reporter
 *
 * Abstract base class for all reporters with event handlers
 */

import type {
  ReporterConfig,
  ReporterEvent,
  TestSuiteResult,
  TestExecutionResult,
  PhaseResult,
  StepResult,
  TestSuite,
  TestPhase,
  UnifiedTestDefinition,
} from '../types';

/**
 * Reporter options interface
 */
export interface ReporterOptions {
  output?: string;
  verbose?: boolean;
  noColor?: boolean;
  prettyPrint?: boolean;
}

/**
 * Event data types for type-safe event handling
 */
export interface SuiteStartData {
  suite: TestSuite;
  totalTests: number;
  timestamp: Date;
}

export interface SuiteEndData {
  result: TestSuiteResult;
  timestamp: Date;
}

export interface TestStartData {
  test: UnifiedTestDefinition;
  index: number;
  total: number;
  timestamp: Date;
}

export interface TestEndData {
  test: UnifiedTestDefinition;
  result: TestExecutionResult;
  index: number;
  total: number;
  timestamp: Date;
}

export interface PhaseStartData {
  testName: string;
  phase: TestPhase;
  timestamp: Date;
}

export interface PhaseEndData {
  testName: string;
  phase: TestPhase;
  result: PhaseResult;
  timestamp: Date;
}

export interface StepStartData {
  testName: string;
  phase: TestPhase;
  stepId: string;
  action: string;
  adapter: string;
  timestamp: Date;
}

export interface StepEndData {
  testName: string;
  phase: TestPhase;
  stepId: string;
  result: StepResult;
  timestamp: Date;
}

/**
 * Abstract base class for E2E test reporters
 */
export abstract class BaseReporter {
  protected config: ReporterConfig;
  protected options: ReporterOptions;

  constructor(config: ReporterConfig, options: ReporterOptions = {}) {
    this.config = config;
    this.options = {
      output: config.output,
      verbose: config.verbose ?? false,
      noColor: options.noColor ?? false,
      prettyPrint: options.prettyPrint ?? true,
    };
  }

  /**
   * Get the reporter name for identification
   */
  abstract get name(): string;

  /**
   * Called when test suite starts
   */
  onSuiteStart(data: SuiteStartData): void {
    // Default: no-op, subclasses can override
    void data;
  }

  /**
   * Called when test suite ends
   */
  onSuiteEnd(data: SuiteEndData): void {
    // Default: no-op, subclasses can override
    void data;
  }

  /**
   * Called when a test starts
   */
  onTestStart(data: TestStartData): void {
    // Default: no-op, subclasses can override
    void data;
  }

  /**
   * Called when a test ends
   */
  onTestEnd(data: TestEndData): void {
    // Default: no-op, subclasses can override
    void data;
  }

  /**
   * Called when a phase starts
   */
  onPhaseStart(data: PhaseStartData): void {
    // Default: no-op, subclasses can override
    void data;
  }

  /**
   * Called when a phase ends
   */
  onPhaseEnd(data: PhaseEndData): void {
    // Default: no-op, subclasses can override
    void data;
  }

  /**
   * Called when a step starts
   */
  onStepStart(data: StepStartData): void {
    // Default: no-op, subclasses can override
    void data;
  }

  /**
   * Called when a step ends
   */
  onStepEnd(data: StepEndData): void {
    // Default: no-op, subclasses can override
    void data;
  }

  /**
   * Generate the final report
   */
  abstract generateReport(result: TestSuiteResult): Promise<void>;

  /**
   * Handle reporter events (generic event dispatcher)
   */
  handleEvent(event: ReporterEvent): void {
    const { type, data, timestamp } = event;

    switch (type) {
      case 'suite:start':
        this.onSuiteStart({ ...(data as Omit<SuiteStartData, 'timestamp'>), timestamp });
        break;
      case 'suite:end':
        this.onSuiteEnd({ ...(data as Omit<SuiteEndData, 'timestamp'>), timestamp });
        break;
      case 'test:start':
        this.onTestStart({ ...(data as Omit<TestStartData, 'timestamp'>), timestamp });
        break;
      case 'test:end':
        this.onTestEnd({ ...(data as Omit<TestEndData, 'timestamp'>), timestamp });
        break;
      case 'phase:start':
        this.onPhaseStart({ ...(data as Omit<PhaseStartData, 'timestamp'>), timestamp });
        break;
      case 'phase:end':
        this.onPhaseEnd({ ...(data as Omit<PhaseEndData, 'timestamp'>), timestamp });
        break;
      case 'step:start':
        this.onStepStart({ ...(data as Omit<StepStartData, 'timestamp'>), timestamp });
        break;
      case 'step:end':
        this.onStepEnd({ ...(data as Omit<StepEndData, 'timestamp'>), timestamp });
        break;
    }
  }

  /**
   * Get the output path for this reporter
   */
  protected getOutputPath(): string | undefined {
    return this.options.output;
  }

  /**
   * Check if verbose mode is enabled
   */
  protected isVerbose(): boolean {
    return this.options.verbose ?? false;
  }

  /**
   * Format duration in human-readable format
   */
  protected formatDuration(ms: number): string {
    if (ms < 1000) {
      return `${ms}ms`;
    }
    if (ms < 60000) {
      return `${(ms / 1000).toFixed(2)}s`;
    }
    const minutes = Math.floor(ms / 60000);
    const seconds = ((ms % 60000) / 1000).toFixed(1);
    return `${minutes}m ${seconds}s`;
  }

  /**
   * Format timestamp in ISO format
   */
  protected formatTimestamp(date: Date): string {
    return date.toISOString();
  }

  /**
   * Get error message and stack from an error
   */
  protected formatError(error: Error | undefined): { message: string; stack?: string } {
    if (!error) {
      return { message: 'Unknown error' };
    }
    return {
      message: error.message,
      stack: error.stack,
    };
  }

  /**
   * Calculate summary statistics from results
   */
  protected calculateSummary(result: TestSuiteResult): {
    total: number;
    passed: number;
    failed: number;
    skipped: number;
    passRate: number;
    duration: string;
  } {
    const passRate = result.total > 0 ? (result.passed / result.total) * 100 : 0;
    return {
      total: result.total,
      passed: result.passed,
      failed: result.failed,
      skipped: result.skipped,
      passRate: Math.round(passRate * 100) / 100,
      duration: this.formatDuration(result.duration),
    };
  }
}

/**
 * Helper to emit reporter events
 */
export function createReporterEvent(
  type: ReporterEvent['type'],
  data: unknown
): ReporterEvent {
  return {
    type,
    timestamp: new Date(),
    data,
  };
}
