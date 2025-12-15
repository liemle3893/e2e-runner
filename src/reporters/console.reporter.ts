/**
 * E2E Test Runner - Console Reporter
 *
 * Real-time terminal output with colors and progress indicators
 */

import type { ReporterConfig, TestSuiteResult, TestStatus, PhaseStatus, StepStatus } from '../types';
import {
  BaseReporter,
  type ReporterOptions,
  type SuiteStartData,
  type SuiteEndData,
  type TestStartData,
  type TestEndData,
  type PhaseStartData,
  type PhaseEndData,
  type StepStartData,
  type StepEndData,
} from './base.reporter';

/**
 * ANSI color codes for terminal output
 */
const COLORS = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  dim: '\x1b[2m',
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  cyan: '\x1b[36m',
  gray: '\x1b[90m',
  bgRed: '\x1b[41m',
  bgGreen: '\x1b[42m',
  bgYellow: '\x1b[43m',
};

/**
 * Status symbols for terminal display
 */
const SYMBOLS = {
  pass: '\u2713', // checkmark
  fail: '\u2717', // X mark
  skip: '\u25CB', // circle
  error: '\u2716', // heavy X
  pending: '\u25CB', // circle
  arrow: '\u2192', // arrow
  bullet: '\u2022', // bullet
};

/**
 * Console reporter for terminal output
 */
export class ConsoleReporter extends BaseReporter {
  private useColors: boolean;
  private startTime: Date | null = null;
  private currentTest: string | null = null;

  constructor(config: ReporterConfig, options: ReporterOptions = {}) {
    super(config, options);
    this.useColors = !options.noColor && process.stdout.isTTY !== false;
  }

  get name(): string {
    return 'console';
  }

  /**
   * Apply color to text if colors are enabled
   */
  private colorize(text: string, color: keyof typeof COLORS): string {
    if (!this.useColors) return text;
    return `${COLORS[color]}${text}${COLORS.reset}`;
  }

  /**
   * Get status symbol with color
   */
  private getStatusSymbol(status: TestStatus | PhaseStatus | StepStatus): string {
    switch (status) {
      case 'passed':
        return this.colorize(SYMBOLS.pass, 'green');
      case 'failed':
        return this.colorize(SYMBOLS.fail, 'red');
      case 'skipped':
        return this.colorize(SYMBOLS.skip, 'yellow');
      case 'error':
        return this.colorize(SYMBOLS.error, 'red');
      default:
        return this.colorize(SYMBOLS.pending, 'gray');
    }
  }

  /**
   * Get status text with color
   */
  private getStatusText(status: TestStatus | PhaseStatus | StepStatus): string {
    switch (status) {
      case 'passed':
        return this.colorize('PASSED', 'green');
      case 'failed':
        return this.colorize('FAILED', 'red');
      case 'skipped':
        return this.colorize('SKIPPED', 'yellow');
      case 'error':
        return this.colorize('ERROR', 'red');
      default: {
        // Handle any unexpected status value
        const unknownStatus: string = status;
        return this.colorize(unknownStatus.toUpperCase(), 'gray');
      }
    }
  }

  /**
   * Print a line to stdout
   */
  private print(message: string = '', indent: number = 0): void {
    const prefix = '  '.repeat(indent);
    console.log(`${prefix}${message}`);
  }

  /**
   * Print a divider line
   */
  private printDivider(char: string = '-', length: number = 60): void {
    this.print(this.colorize(char.repeat(length), 'dim'));
  }

  override onSuiteStart(data: SuiteStartData): void {
    this.startTime = data.timestamp;
    this.print();
    this.printDivider('=');
    this.print(this.colorize(this.colorize('E2E Test Suite', 'bold'), 'cyan'));
    this.printDivider('=');
    this.print();
    this.print(`${this.colorize(SYMBOLS.bullet, 'blue')} Total tests: ${this.colorize(String(data.totalTests), 'bold')}`);
    this.print(`${this.colorize(SYMBOLS.bullet, 'blue')} Started at: ${this.formatTimestamp(data.timestamp)}`);
    this.print();
    this.printDivider();
    this.print();
  }

  override onSuiteEnd(data: SuiteEndData): void {
    // Summary is printed in generateReport
    void data;
  }

  override onTestStart(data: TestStartData): void {
    this.currentTest = data.test.name;
    const progress = this.colorize(`[${data.index + 1}/${data.total}]`, 'dim');
    this.print(`${progress} ${this.colorize(SYMBOLS.arrow, 'blue')} ${data.test.name}`);
  }

  override onTestEnd(data: TestEndData): void {
    const { result } = data;
    const symbol = this.getStatusSymbol(result.status);
    const duration = this.colorize(`(${this.formatDuration(result.duration)})`, 'dim');

    this.print(`   ${symbol} ${this.getStatusText(result.status)} ${duration}`);

    // Show error details for failed tests
    if (result.status === 'failed' || result.status === 'error') {
      if (result.error) {
        this.print();
        this.print(this.colorize(`   Error: ${result.error.message}`, 'red'));
        if (this.isVerbose() && result.error.stack) {
          const stackLines = result.error.stack.split('\n').slice(1, 4);
          stackLines.forEach((line) => {
            this.print(this.colorize(`   ${line.trim()}`, 'dim'));
          });
        }
      }
    }

    // Show retry count if retries occurred
    if (result.retryCount > 0) {
      this.print(`   ${this.colorize(SYMBOLS.bullet, 'yellow')} Retries: ${result.retryCount}`);
    }

    this.print();
    this.currentTest = null;
  }

  override onPhaseStart(data: PhaseStartData): void {
    if (this.isVerbose()) {
      const phaseName = data.phase.charAt(0).toUpperCase() + data.phase.slice(1);
      this.print(`   ${this.colorize(SYMBOLS.arrow, 'cyan')} ${phaseName} phase`, 1);
    }
  }

  override onPhaseEnd(data: PhaseEndData): void {
    if (this.isVerbose()) {
      const { result } = data;
      const symbol = this.getStatusSymbol(result.status);
      const duration = this.colorize(`(${this.formatDuration(result.duration)})`, 'dim');
      const phaseName = data.phase.charAt(0).toUpperCase() + data.phase.slice(1);
      this.print(`   ${symbol} ${phaseName} ${duration}`, 1);

      if (result.error && result.status === 'failed') {
        this.print(this.colorize(`      Error: ${result.error.message}`, 'red'));
      }
    }
  }

  override onStepStart(data: StepStartData): void {
    if (this.isVerbose()) {
      const adapter = this.colorize(`[${data.adapter}]`, 'magenta');
      this.print(`      ${adapter} ${data.action}`, 1);
    }
  }

  override onStepEnd(data: StepEndData): void {
    if (this.isVerbose()) {
      const { result } = data;
      const symbol = this.getStatusSymbol(result.status);
      const duration = this.colorize(`(${this.formatDuration(result.duration)})`, 'dim');
      this.print(`      ${symbol} ${data.stepId} ${duration}`, 1);

      if (result.error && result.status === 'failed') {
        this.print(this.colorize(`         Error: ${result.error.message}`, 'red'));
      }
    }
  }

  async generateReport(result: TestSuiteResult): Promise<void> {
    this.printDivider();
    this.print();
    this.print(this.colorize(this.colorize('Test Summary', 'bold'), 'cyan'));
    this.print();

    const summary = this.calculateSummary(result);

    // Results bar
    const passedBar = this.colorize(`${SYMBOLS.pass} ${summary.passed} passed`, 'green');
    const failedBar = summary.failed > 0
      ? this.colorize(`${SYMBOLS.fail} ${summary.failed} failed`, 'red')
      : this.colorize(`${SYMBOLS.fail} ${summary.failed} failed`, 'dim');
    const skippedBar = summary.skipped > 0
      ? this.colorize(`${SYMBOLS.skip} ${summary.skipped} skipped`, 'yellow')
      : this.colorize(`${SYMBOLS.skip} ${summary.skipped} skipped`, 'dim');

    this.print(`  ${passedBar}  |  ${failedBar}  |  ${skippedBar}`);
    this.print();

    // Pass rate with visual bar
    const passRate = summary.passRate;
    const barLength = 30;
    const filledLength = Math.round((passRate / 100) * barLength);
    const emptyLength = barLength - filledLength;
    const passRateColor = passRate === 100 ? 'green' : passRate >= 80 ? 'yellow' : 'red';

    const filledBar = this.colorize('\u2588'.repeat(filledLength), passRateColor);
    const emptyBar = this.colorize('\u2591'.repeat(emptyLength), 'dim');
    this.print(`  Pass rate: [${filledBar}${emptyBar}] ${passRate}%`);
    this.print();

    // Duration
    this.print(`  ${this.colorize(SYMBOLS.bullet, 'blue')} Total duration: ${summary.duration}`);
    this.print(`  ${this.colorize(SYMBOLS.bullet, 'blue')} Total tests: ${summary.total}`);
    this.print();

    // List failed tests
    if (summary.failed > 0) {
      this.print(this.colorize('Failed Tests:', 'red'));
      result.results
        .filter((r) => r.status === 'failed' || r.status === 'error')
        .forEach((r) => {
          this.print(`  ${this.colorize(SYMBOLS.fail, 'red')} ${r.name}`);
          if (r.error) {
            this.print(`    ${this.colorize(r.error.message, 'dim')}`);
          }
        });
      this.print();
    }

    // Final status
    this.printDivider('=');
    if (result.success) {
      this.print(this.colorize(this.colorize('  ALL TESTS PASSED  ', 'bold'), 'green'));
    } else {
      this.print(this.colorize(this.colorize('  TESTS FAILED  ', 'bold'), 'red'));
    }
    this.printDivider('=');
    this.print();
  }
}
