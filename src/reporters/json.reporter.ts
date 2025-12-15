/**
 * E2E Test Runner - JSON Reporter
 *
 * Outputs full TestSuiteResult as JSON to file or stdout
 */

import * as fs from 'fs';
import * as path from 'path';
import type { ReporterConfig, TestSuiteResult, TestExecutionResult, PhaseResult, StepResult } from '../types';
import { BaseReporter, type ReporterOptions } from './base.reporter';

/**
 * JSON output structure with metadata
 */
export interface JSONReportOutput {
  metadata: {
    version: string;
    timestamp: string;
    environment?: string;
    runner: string;
  };
  summary: {
    total: number;
    passed: number;
    failed: number;
    skipped: number;
    passRate: number;
    duration: number;
    durationFormatted: string;
    success: boolean;
  };
  tests: JSONTestResult[];
}

/**
 * Test result in JSON format
 */
export interface JSONTestResult {
  name: string;
  status: string;
  duration: number;
  durationFormatted: string;
  retryCount: number;
  error?: {
    message: string;
    stack?: string;
  };
  phases: JSONPhaseResult[];
  capturedValues: Record<string, unknown>;
}

/**
 * Phase result in JSON format
 */
export interface JSONPhaseResult {
  phase: string;
  status: string;
  duration: number;
  durationFormatted: string;
  error?: {
    message: string;
    stack?: string;
  };
  steps: JSONStepResult[];
}

/**
 * Step result in JSON format
 */
export interface JSONStepResult {
  stepId: string;
  adapter: string;
  action: string;
  description?: string;
  status: string;
  duration: number;
  durationFormatted: string;
  retryCount: number;
  data?: unknown;
  error?: {
    message: string;
    stack?: string;
  };
}

/**
 * JSON reporter for structured output
 */
export class JSONReporter extends BaseReporter {
  private prettyPrint: boolean;
  private environmentName: string | undefined;

  constructor(config: ReporterConfig, options: ReporterOptions = {}) {
    super(config, options);
    this.prettyPrint = options.prettyPrint ?? true;
    this.environmentName = undefined;
  }

  get name(): string {
    return 'json';
  }

  /**
   * Set the environment name for metadata
   */
  setEnvironment(envName: string): void {
    this.environmentName = envName;
  }

  async generateReport(result: TestSuiteResult): Promise<void> {
    const output = this.buildJSONOutput(result);
    const json = this.prettyPrint
      ? JSON.stringify(output, null, 2)
      : JSON.stringify(output);

    const outputPath = this.getOutputPath();

    if (outputPath) {
      await this.writeFile(outputPath, json);
    } else {
      // Write to stdout
      console.log(json);
    }
  }

  /**
   * Build the complete JSON output structure
   */
  private buildJSONOutput(result: TestSuiteResult): JSONReportOutput {
    const summary = this.calculateSummary(result);

    return {
      metadata: {
        version: '1.0',
        timestamp: new Date().toISOString(),
        environment: this.environmentName,
        runner: 'e2e-runner',
      },
      summary: {
        total: summary.total,
        passed: summary.passed,
        failed: summary.failed,
        skipped: summary.skipped,
        passRate: summary.passRate,
        duration: result.duration,
        durationFormatted: summary.duration,
        success: result.success,
      },
      tests: result.results.map((test) => this.formatTestResult(test)),
    };
  }

  /**
   * Format a test execution result
   */
  private formatTestResult(test: TestExecutionResult): JSONTestResult {
    return {
      name: test.name,
      status: test.status,
      duration: test.duration,
      durationFormatted: this.formatDuration(test.duration),
      retryCount: test.retryCount,
      error: test.error ? this.formatErrorForJSON(test.error) : undefined,
      phases: test.phases.map((phase) => this.formatPhaseResult(phase)),
      capturedValues: test.capturedValues,
    };
  }

  /**
   * Format a phase result
   */
  private formatPhaseResult(phase: PhaseResult): JSONPhaseResult {
    return {
      phase: phase.phase,
      status: phase.status,
      duration: phase.duration,
      durationFormatted: this.formatDuration(phase.duration),
      error: phase.error ? this.formatErrorForJSON(phase.error) : undefined,
      steps: phase.steps.map((step) => this.formatStepResult(step)),
    };
  }

  /**
   * Format a step result
   */
  private formatStepResult(step: StepResult): JSONStepResult {
    return {
      stepId: step.stepId,
      adapter: step.adapter,
      action: step.action,
      description: step.description,
      status: step.status,
      duration: step.duration,
      durationFormatted: this.formatDuration(step.duration),
      retryCount: step.retryCount,
      data: this.sanitizeData(step.data),
      error: step.error ? this.formatErrorForJSON(step.error) : undefined,
    };
  }

  /**
   * Format error for JSON output
   */
  private formatErrorForJSON(error: Error): { message: string; stack?: string } {
    return {
      message: error.message,
      stack: error.stack,
    };
  }

  /**
   * Sanitize data to ensure it is JSON-serializable
   */
  private sanitizeData(data: unknown): unknown {
    if (data === undefined || data === null) {
      return undefined;
    }

    try {
      // Attempt to serialize and parse to ensure JSON compatibility
      return JSON.parse(JSON.stringify(data));
    } catch {
      // If serialization fails, convert to string
      return String(data);
    }
  }

  /**
   * Write JSON content to file
   */
  private async writeFile(filePath: string, content: string): Promise<void> {
    const absolutePath = path.isAbsolute(filePath)
      ? filePath
      : path.resolve(process.cwd(), filePath);

    const dir = path.dirname(absolutePath);
    if (!fs.existsSync(dir)) {
      fs.mkdirSync(dir, { recursive: true });
    }

    fs.writeFileSync(absolutePath, content, 'utf-8');
  }
}

/**
 * Create a minimal JSON output (for piping to other tools)
 */
export function createMinimalJSONOutput(result: TestSuiteResult): string {
  return JSON.stringify({
    success: result.success,
    total: result.total,
    passed: result.passed,
    failed: result.failed,
    skipped: result.skipped,
    duration: result.duration,
  });
}
