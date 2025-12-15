/**
 * E2E Test Runner - JUnit XML Reporter
 *
 * Generates JUnit XML format compatible with CI systems (Jenkins, GitHub Actions)
 */

import * as fs from 'fs';
import * as path from 'path';
import type { ReporterConfig, TestSuiteResult, TestExecutionResult, PhaseResult, StepResult } from '../types';
import { BaseReporter, type ReporterOptions } from './base.reporter';

/**
 * JUnit XML reporter for CI integration
 */
export class JUnitReporter extends BaseReporter {
  private startTime: Date | null = null;

  constructor(config: ReporterConfig, options: ReporterOptions = {}) {
    super(config, options);
  }

  get name(): string {
    return 'junit';
  }

  async generateReport(result: TestSuiteResult): Promise<void> {
    const xml = this.buildXML(result);
    const outputPath = this.getOutputPath() || 'e2e-results.xml';

    await this.writeFile(outputPath, xml);
  }

  /**
   * Build complete JUnit XML document
   */
  private buildXML(result: TestSuiteResult): string {
    const lines: string[] = [];
    lines.push('<?xml version="1.0" encoding="UTF-8"?>');
    lines.push(this.buildTestSuites(result));
    return lines.join('\n');
  }

  /**
   * Build testsuites root element
   */
  private buildTestSuites(result: TestSuiteResult): string {
    const timestamp = new Date().toISOString();
    const timeInSeconds = (result.duration / 1000).toFixed(3);

    const attrs = [
      `name="E2E Test Suite"`,
      `tests="${result.total}"`,
      `failures="${result.failed}"`,
      `skipped="${result.skipped}"`,
      `time="${timeInSeconds}"`,
      `timestamp="${timestamp}"`,
    ];

    const lines: string[] = [];
    lines.push(`<testsuites ${attrs.join(' ')}>`);

    // Group test results by source file
    const groupedResults = this.groupBySourceFile(result.results);

    for (const [sourceFile, tests] of Object.entries(groupedResults)) {
      lines.push(this.buildTestSuite(sourceFile, tests));
    }

    lines.push('</testsuites>');
    return lines.join('\n');
  }

  /**
   * Group test results by their source file
   */
  private groupBySourceFile(results: TestExecutionResult[]): Record<string, TestExecutionResult[]> {
    const grouped: Record<string, TestExecutionResult[]> = {};

    for (const testResult of results) {
      // Use test name as fallback for source file grouping
      const key = testResult.name.split(':')[0] || 'default';
      if (!grouped[key]) {
        grouped[key] = [];
      }
      grouped[key].push(testResult);
    }

    return grouped;
  }

  /**
   * Build testsuite element for a group of tests
   */
  private buildTestSuite(name: string, tests: TestExecutionResult[]): string {
    const totalTime = tests.reduce((sum, t) => sum + t.duration, 0);
    const failures = tests.filter((t) => t.status === 'failed' || t.status === 'error').length;
    const skipped = tests.filter((t) => t.status === 'skipped').length;

    const attrs = [
      `name="${this.escapeXML(name)}"`,
      `tests="${tests.length}"`,
      `failures="${failures}"`,
      `skipped="${skipped}"`,
      `time="${(totalTime / 1000).toFixed(3)}"`,
    ];

    const lines: string[] = [];
    lines.push(`  <testsuite ${attrs.join(' ')}>`);

    for (const testResult of tests) {
      lines.push(this.buildTestCase(testResult));
    }

    lines.push('  </testsuite>');
    return lines.join('\n');
  }

  /**
   * Build testcase element
   */
  private buildTestCase(testResult: TestExecutionResult): string {
    const timeInSeconds = (testResult.duration / 1000).toFixed(3);
    const className = testResult.name.replace(/[^a-zA-Z0-9_.-]/g, '_');

    const attrs = [
      `name="${this.escapeXML(testResult.name)}"`,
      `classname="${this.escapeXML(className)}"`,
      `time="${timeInSeconds}"`,
    ];

    const lines: string[] = [];
    lines.push(`    <testcase ${attrs.join(' ')}>`);

    // Add failure or error element
    if (testResult.status === 'failed' || testResult.status === 'error') {
      lines.push(this.buildFailure(testResult));
    }

    // Add skipped element
    if (testResult.status === 'skipped') {
      lines.push('      <skipped/>');
    }

    // Add system-out with phase details (verbose mode)
    if (this.isVerbose() && testResult.phases.length > 0) {
      lines.push(this.buildSystemOut(testResult));
    }

    // Add system-err for errors
    if (testResult.error) {
      lines.push(this.buildSystemErr(testResult.error));
    }

    lines.push('    </testcase>');
    return lines.join('\n');
  }

  /**
   * Build failure element
   */
  private buildFailure(testResult: TestExecutionResult): string {
    const error = this.findFirstError(testResult);
    const message = error?.message || 'Test failed';
    const type = testResult.status === 'error' ? 'Error' : 'AssertionError';

    const lines: string[] = [];
    lines.push(`      <failure message="${this.escapeXML(message)}" type="${type}">`);

    if (error?.stack) {
      lines.push(this.escapeXML(error.stack));
    } else {
      lines.push(this.escapeXML(message));
    }

    lines.push('      </failure>');
    return lines.join('\n');
  }

  /**
   * Find the first error in test result
   */
  private findFirstError(testResult: TestExecutionResult): Error | undefined {
    if (testResult.error) {
      return testResult.error;
    }

    for (const phase of testResult.phases) {
      if (phase.error) {
        return phase.error;
      }
      for (const step of phase.steps) {
        if (step.error) {
          return step.error;
        }
      }
    }

    return undefined;
  }

  /**
   * Build system-out element with phase/step details
   */
  private buildSystemOut(testResult: TestExecutionResult): string {
    const lines: string[] = [];
    lines.push('      <system-out><![CDATA[');

    for (const phase of testResult.phases) {
      lines.push(this.formatPhaseOutput(phase));
    }

    // Include captured values
    if (Object.keys(testResult.capturedValues).length > 0) {
      lines.push('\nCaptured Values:');
      for (const [key, value] of Object.entries(testResult.capturedValues)) {
        lines.push(`  ${key}: ${JSON.stringify(value)}`);
      }
    }

    lines.push(']]></system-out>');
    return lines.join('\n');
  }

  /**
   * Format phase output for system-out
   */
  private formatPhaseOutput(phase: PhaseResult): string {
    const lines: string[] = [];
    const status = phase.status.toUpperCase();
    const duration = this.formatDuration(phase.duration);

    lines.push(`\n[${phase.phase.toUpperCase()}] ${status} (${duration})`);

    for (const step of phase.steps) {
      lines.push(this.formatStepOutput(step));
    }

    return lines.join('\n');
  }

  /**
   * Format step output for system-out
   */
  private formatStepOutput(step: StepResult): string {
    const status = step.status === 'passed' ? 'OK' : step.status.toUpperCase();
    const duration = this.formatDuration(step.duration);
    const description = step.description ? ` - ${step.description}` : '';

    let line = `  [${step.adapter}] ${step.action}${description}: ${status} (${duration})`;

    if (step.error) {
      line += `\n    Error: ${step.error.message}`;
    }

    return line;
  }

  /**
   * Build system-err element
   */
  private buildSystemErr(error: Error): string {
    const lines: string[] = [];
    lines.push('      <system-err><![CDATA[');
    lines.push(error.stack || error.message);
    lines.push(']]></system-err>');
    return lines.join('\n');
  }

  /**
   * Escape XML special characters
   */
  private escapeXML(str: string): string {
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&apos;');
  }

  /**
   * Write XML content to file
   */
  private async writeFile(filePath: string, content: string): Promise<void> {
    const absolutePath = path.isAbsolute(filePath)
      ? filePath
      : path.resolve(process.cwd(), filePath);

    // Ensure directory exists
    const dir = path.dirname(absolutePath);
    if (!fs.existsSync(dir)) {
      fs.mkdirSync(dir, { recursive: true });
    }

    fs.writeFileSync(absolutePath, content, 'utf-8');
  }
}
