/**
 * E2E Test Runner - Reporters Index
 *
 * Barrel exports and factory function for creating reporters
 */

import type { ReporterConfig } from '../types';
import { BaseReporter, type ReporterOptions } from './base.reporter';
import { ConsoleReporter } from './console.reporter';
import { JUnitReporter } from './junit.reporter';
import { HTMLReporter } from './html.reporter';
import { JSONReporter } from './json.reporter';

// Re-export all reporters
export { BaseReporter, createReporterEvent } from './base.reporter';
export type {
  ReporterOptions,
  SuiteStartData,
  SuiteEndData,
  TestStartData,
  TestEndData,
  PhaseStartData,
  PhaseEndData,
  StepStartData,
  StepEndData,
} from './base.reporter';
export { ConsoleReporter } from './console.reporter';
export { JUnitReporter } from './junit.reporter';
export { HTMLReporter } from './html.reporter';
export { JSONReporter, createMinimalJSONOutput } from './json.reporter';
export type { JSONReportOutput, JSONTestResult, JSONPhaseResult, JSONStepResult } from './json.reporter';

/**
 * Reporter type union
 */
export type ReporterType = 'console' | 'junit' | 'html' | 'json';

/**
 * Reporter factory options
 */
export interface ReporterFactoryOptions extends ReporterOptions {
  environmentName?: string;
}

/**
 * Factory function to create reporter instances
 *
 * @param type - The type of reporter to create
 * @param config - Reporter configuration from e2e.config.yaml
 * @param options - Additional options for the reporter
 * @returns Reporter instance
 */
export function createReporter(
  type: ReporterType | string,
  config: Partial<ReporterConfig> = {},
  options: ReporterFactoryOptions = {}
): BaseReporter {
  const fullConfig: ReporterConfig = {
    type: type as ReporterConfig['type'],
    output: config.output,
    verbose: config.verbose ?? options.verbose ?? false,
  };

  switch (type) {
    case 'console':
      return new ConsoleReporter(fullConfig, options);

    case 'junit':
      return new JUnitReporter(fullConfig, options);

    case 'html':
      return new HTMLReporter(fullConfig, options);

    case 'json': {
      const reporter = new JSONReporter(fullConfig, options);
      if (options.environmentName) {
        reporter.setEnvironment(options.environmentName);
      }
      return reporter;
    }

    default:
      throw new Error(`Unknown reporter type: ${type}. Available types: console, junit, html, json`);
  }
}

/**
 * Create multiple reporters from configuration
 *
 * @param configs - Array of reporter configurations
 * @param options - Shared options for all reporters
 * @returns Array of reporter instances
 */
export function createReporters(
  configs: ReporterConfig[],
  options: ReporterFactoryOptions = {}
): BaseReporter[] {
  if (!configs || configs.length === 0) {
    // Default to console reporter
    return [createReporter('console', {}, options)];
  }

  return configs.map((config) => createReporter(config.type, config, options));
}

/**
 * Reporter manager for handling multiple reporters
 */
export class ReporterManager {
  private reporters: BaseReporter[] = [];

  constructor(reporters: BaseReporter[] = []) {
    this.reporters = reporters;
  }

  /**
   * Add a reporter
   */
  addReporter(reporter: BaseReporter): void {
    this.reporters.push(reporter);
  }

  /**
   * Get all reporters
   */
  getReporters(): BaseReporter[] {
    return [...this.reporters];
  }

  /**
   * Broadcast an event to all reporters
   */
  broadcast<K extends keyof BaseReporter>(
    method: K,
    data: Parameters<BaseReporter[K] extends (...args: infer P) => unknown ? (...args: P) => unknown : never>[0]
  ): void {
    for (const reporter of this.reporters) {
      const fn = reporter[method];
      if (typeof fn === 'function') {
        (fn as (data: unknown) => void).call(reporter, data);
      }
    }
  }

  /**
   * Emit suite start event to all reporters
   */
  onSuiteStart(data: Parameters<BaseReporter['onSuiteStart']>[0]): void {
    for (const reporter of this.reporters) {
      reporter.onSuiteStart(data);
    }
  }

  /**
   * Emit suite end event to all reporters
   */
  onSuiteEnd(data: Parameters<BaseReporter['onSuiteEnd']>[0]): void {
    for (const reporter of this.reporters) {
      reporter.onSuiteEnd(data);
    }
  }

  /**
   * Emit test start event to all reporters
   */
  onTestStart(data: Parameters<BaseReporter['onTestStart']>[0]): void {
    for (const reporter of this.reporters) {
      reporter.onTestStart(data);
    }
  }

  /**
   * Emit test end event to all reporters
   */
  onTestEnd(data: Parameters<BaseReporter['onTestEnd']>[0]): void {
    for (const reporter of this.reporters) {
      reporter.onTestEnd(data);
    }
  }

  /**
   * Emit phase start event to all reporters
   */
  onPhaseStart(data: Parameters<BaseReporter['onPhaseStart']>[0]): void {
    for (const reporter of this.reporters) {
      reporter.onPhaseStart(data);
    }
  }

  /**
   * Emit phase end event to all reporters
   */
  onPhaseEnd(data: Parameters<BaseReporter['onPhaseEnd']>[0]): void {
    for (const reporter of this.reporters) {
      reporter.onPhaseEnd(data);
    }
  }

  /**
   * Emit step start event to all reporters
   */
  onStepStart(data: Parameters<BaseReporter['onStepStart']>[0]): void {
    for (const reporter of this.reporters) {
      reporter.onStepStart(data);
    }
  }

  /**
   * Emit step end event to all reporters
   */
  onStepEnd(data: Parameters<BaseReporter['onStepEnd']>[0]): void {
    for (const reporter of this.reporters) {
      reporter.onStepEnd(data);
    }
  }

  /**
   * Generate reports from all reporters
   */
  async generateReports(result: Parameters<BaseReporter['generateReport']>[0]): Promise<void> {
    const promises = this.reporters.map((reporter) =>
      reporter.generateReport(result).catch((err) => {
        console.error(`Reporter ${reporter.name} failed to generate report:`, err);
      })
    );

    await Promise.all(promises);
  }
}

/**
 * Create a reporter manager from configuration
 */
export function createReporterManager(
  configs: ReporterConfig[],
  options: ReporterFactoryOptions = {}
): ReporterManager {
  const reporters = createReporters(configs, options);
  return new ReporterManager(reporters);
}

/**
 * Get available reporter types
 */
export function getAvailableReporterTypes(): ReporterType[] {
  return ['console', 'junit', 'html', 'json'];
}

/**
 * Validate reporter type
 */
export function isValidReporterType(type: string): type is ReporterType {
  return getAvailableReporterTypes().includes(type as ReporterType);
}
