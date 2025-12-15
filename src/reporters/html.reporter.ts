/**
 * E2E Test Runner - HTML Reporter
 *
 * Generates self-contained HTML report with embedded CSS
 */

import * as fs from 'fs';
import * as path from 'path';
import type { ReporterConfig, TestSuiteResult, TestExecutionResult, PhaseResult, StepResult } from '../types';
import { BaseReporter, type ReporterOptions } from './base.reporter';

/**
 * HTML reporter for visual test results
 */
export class HTMLReporter extends BaseReporter {
  constructor(config: ReporterConfig, options: ReporterOptions = {}) {
    super(config, options);
  }

  get name(): string {
    return 'html';
  }

  async generateReport(result: TestSuiteResult): Promise<void> {
    const html = this.buildHTML(result);
    const outputPath = this.getOutputPath() || 'e2e-report.html';

    await this.writeFile(outputPath, html);
  }

  /**
   * Build complete HTML document
   */
  private buildHTML(result: TestSuiteResult): string {
    const summary = this.calculateSummary(result);
    const timestamp = new Date().toISOString();

    return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>E2E Test Report - ${timestamp}</title>
  ${this.getStyles()}
</head>
<body>
  <div class="container">
    ${this.buildHeader(result, timestamp)}
    ${this.buildSummaryDashboard(summary)}
    ${this.buildCharts(summary)}
    ${this.buildTestList(result)}
  </div>
  ${this.getScripts()}
</body>
</html>`;
  }

  /**
   * Get embedded CSS styles
   */
  private getStyles(): string {
    return `<style>
  :root {
    --color-pass: #22c55e;
    --color-fail: #ef4444;
    --color-skip: #f59e0b;
    --color-bg: #f8fafc;
    --color-card: #ffffff;
    --color-text: #1e293b;
    --color-text-light: #64748b;
    --color-border: #e2e8f0;
    --shadow: 0 1px 3px rgba(0,0,0,0.1);
  }

  * { box-sizing: border-box; margin: 0; padding: 0; }

  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: var(--color-bg);
    color: var(--color-text);
    line-height: 1.6;
  }

  .container {
    max-width: 1200px;
    margin: 0 auto;
    padding: 2rem;
  }

  .header {
    text-align: center;
    margin-bottom: 2rem;
  }

  .header h1 {
    font-size: 2rem;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }

  .header .timestamp {
    color: var(--color-text-light);
    font-size: 0.875rem;
  }

  .status-badge {
    display: inline-block;
    padding: 0.5rem 1.5rem;
    border-radius: 9999px;
    font-weight: 600;
    font-size: 1rem;
    margin-top: 1rem;
  }

  .status-badge.pass { background: var(--color-pass); color: white; }
  .status-badge.fail { background: var(--color-fail); color: white; }

  .dashboard {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
    margin-bottom: 2rem;
  }

  .stat-card {
    background: var(--color-card);
    border-radius: 8px;
    padding: 1.5rem;
    box-shadow: var(--shadow);
    text-align: center;
  }

  .stat-card .value {
    font-size: 2.5rem;
    font-weight: 700;
  }

  .stat-card .label {
    color: var(--color-text-light);
    font-size: 0.875rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .stat-card.passed .value { color: var(--color-pass); }
  .stat-card.failed .value { color: var(--color-fail); }
  .stat-card.skipped .value { color: var(--color-skip); }

  .charts {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: 1rem;
    margin-bottom: 2rem;
  }

  .chart-card {
    background: var(--color-card);
    border-radius: 8px;
    padding: 1.5rem;
    box-shadow: var(--shadow);
  }

  .chart-card h3 {
    font-size: 1rem;
    margin-bottom: 1rem;
    color: var(--color-text-light);
  }

  .progress-bar {
    height: 24px;
    background: var(--color-border);
    border-radius: 12px;
    overflow: hidden;
    display: flex;
  }

  .progress-bar .segment {
    height: 100%;
    transition: width 0.3s ease;
  }

  .progress-bar .passed { background: var(--color-pass); }
  .progress-bar .failed { background: var(--color-fail); }
  .progress-bar .skipped { background: var(--color-skip); }

  .legend {
    display: flex;
    justify-content: center;
    gap: 1.5rem;
    margin-top: 1rem;
    font-size: 0.875rem;
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .legend-dot {
    width: 12px;
    height: 12px;
    border-radius: 50%;
  }

  .legend-dot.passed { background: var(--color-pass); }
  .legend-dot.failed { background: var(--color-fail); }
  .legend-dot.skipped { background: var(--color-skip); }

  .test-list {
    background: var(--color-card);
    border-radius: 8px;
    box-shadow: var(--shadow);
    overflow: hidden;
  }

  .test-list h2 {
    padding: 1rem 1.5rem;
    border-bottom: 1px solid var(--color-border);
    font-size: 1.25rem;
  }

  .test-item {
    border-bottom: 1px solid var(--color-border);
  }

  .test-item:last-child { border-bottom: none; }

  .test-header {
    display: flex;
    align-items: center;
    padding: 1rem 1.5rem;
    cursor: pointer;
    gap: 1rem;
  }

  .test-header:hover {
    background: var(--color-bg);
  }

  .test-status {
    width: 24px;
    height: 24px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.75rem;
    color: white;
    flex-shrink: 0;
  }

  .test-status.passed { background: var(--color-pass); }
  .test-status.failed { background: var(--color-fail); }
  .test-status.skipped { background: var(--color-skip); }
  .test-status.error { background: var(--color-fail); }

  .test-name {
    flex: 1;
    font-weight: 500;
  }

  .test-duration {
    color: var(--color-text-light);
    font-size: 0.875rem;
  }

  .test-toggle {
    color: var(--color-text-light);
    transition: transform 0.2s;
  }

  .test-item.expanded .test-toggle {
    transform: rotate(90deg);
  }

  .test-details {
    display: none;
    padding: 0 1.5rem 1rem;
    background: var(--color-bg);
  }

  .test-item.expanded .test-details {
    display: block;
  }

  .phase {
    margin-top: 1rem;
  }

  .phase-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem;
    background: var(--color-card);
    border-radius: 4px;
    font-weight: 500;
    font-size: 0.875rem;
  }

  .phase-status {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }

  .phase-status.passed { background: var(--color-pass); }
  .phase-status.failed { background: var(--color-fail); }
  .phase-status.skipped { background: var(--color-skip); }

  .steps {
    margin-left: 1rem;
    padding-left: 1rem;
    border-left: 2px solid var(--color-border);
  }

  .step {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    padding: 0.5rem 0;
    font-size: 0.875rem;
  }

  .step-status {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    margin-top: 0.5rem;
    flex-shrink: 0;
  }

  .step-status.passed { background: var(--color-pass); }
  .step-status.failed { background: var(--color-fail); }
  .step-status.skipped { background: var(--color-skip); }

  .step-info {
    flex: 1;
  }

  .step-action {
    font-family: monospace;
    background: var(--color-card);
    padding: 0.125rem 0.375rem;
    border-radius: 4px;
  }

  .step-adapter {
    color: var(--color-text-light);
    font-size: 0.75rem;
  }

  .error-box {
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 4px;
    padding: 1rem;
    margin-top: 0.5rem;
    font-family: monospace;
    font-size: 0.8rem;
    white-space: pre-wrap;
    word-break: break-all;
    color: var(--color-fail);
  }

  /* Step details panel styles */
  .step-summary {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.25rem;
    font-size: 0.8rem;
    color: var(--color-text-light);
  }

  .step-summary .method {
    font-weight: 600;
    color: var(--color-text);
  }

  .step-summary .url {
    color: #3b82f6;
    word-break: break-all;
  }

  .step-summary .status {
    padding: 0.125rem 0.375rem;
    border-radius: 4px;
    font-weight: 500;
    font-size: 0.75rem;
  }

  .step-summary .status.success {
    background: #dcfce7;
    color: #166534;
  }

  .step-summary .status.error {
    background: #fee2e2;
    color: #991b1b;
  }

  .step-details-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    margin-top: 0.5rem;
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
    color: var(--color-text-light);
    background: var(--color-card);
    border: 1px solid var(--color-border);
    border-radius: 4px;
    cursor: pointer;
  }

  .step-details-toggle:hover {
    background: var(--color-bg);
  }

  .step-details-toggle .arrow {
    transition: transform 0.2s;
  }

  .step-details-content {
    display: none;
    margin-top: 0.5rem;
    padding: 0.75rem;
    background: #f1f5f9;
    border-radius: 4px;
    font-size: 0.8rem;
  }

  .step-details-content.expanded {
    display: block;
  }

  .step-details-content pre {
    margin: 0;
    padding: 0.5rem;
    background: var(--color-card);
    border-radius: 4px;
    overflow-x: auto;
    font-size: 0.75rem;
    line-height: 1.4;
  }

  .step-details-content .section-label {
    font-weight: 600;
    color: var(--color-text);
    margin-bottom: 0.25rem;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .step-details-content .section {
    margin-bottom: 0.75rem;
  }

  .step-details-content .section:last-child {
    margin-bottom: 0;
  }

  .query-text {
    font-family: monospace;
    color: #7c3aed;
  }

  .row-count {
    color: var(--color-text-light);
    font-size: 0.75rem;
  }

  /* Test description styles */
  .test-description {
    color: var(--color-text-light);
    font-size: 0.875rem;
    font-weight: 400;
    margin-top: 0.25rem;
  }

  .test-info {
    flex: 1;
    min-width: 0;
  }

  /* Search box styles */
  .search-container {
    padding: 1rem 1.5rem;
    border-bottom: 1px solid var(--color-border);
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }

  .search-input {
    flex: 1;
    padding: 0.625rem 1rem;
    font-size: 0.875rem;
    border: 1px solid var(--color-border);
    border-radius: 6px;
    outline: none;
    transition: border-color 0.2s, box-shadow 0.2s;
  }

  .search-input:focus {
    border-color: #3b82f6;
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
  }

  .search-input::placeholder {
    color: var(--color-text-light);
  }

  .search-count {
    color: var(--color-text-light);
    font-size: 0.875rem;
    white-space: nowrap;
  }

  .test-item.hidden {
    display: none;
  }

  .no-results {
    padding: 2rem;
    text-align: center;
    color: var(--color-text-light);
    display: none;
  }

  .no-results.visible {
    display: block;
  }
</style>`;
  }

  /**
   * Get embedded JavaScript
   */
  private getScripts(): string {
    return `<script>
  // Toggle test details
  document.querySelectorAll('.test-header').forEach(header => {
    header.addEventListener('click', () => {
      header.parentElement.classList.toggle('expanded');
    });
  });

  // Toggle step details
  document.querySelectorAll('.step-details-toggle').forEach(toggle => {
    toggle.addEventListener('click', (e) => {
      e.stopPropagation();
      const content = toggle.nextElementSibling;
      const arrow = toggle.querySelector('.arrow');
      if (content) {
        content.classList.toggle('expanded');
        if (arrow) {
          arrow.style.transform = content.classList.contains('expanded') ? 'rotate(90deg)' : '';
        }
      }
    });
  });

  // Search functionality
  const searchInput = document.getElementById('test-search');
  const searchCount = document.getElementById('search-count');
  const noResults = document.getElementById('no-results');
  const testItems = document.querySelectorAll('.test-item');
  const totalTests = testItems.length;

  function updateSearch() {
    const query = searchInput.value.toLowerCase().trim();
    let visibleCount = 0;

    testItems.forEach(item => {
      const name = item.getAttribute('data-name') || '';
      const description = item.getAttribute('data-description') || '';
      const searchText = (name + ' ' + description).toLowerCase();

      if (!query || searchText.includes(query)) {
        item.classList.remove('hidden');
        visibleCount++;
      } else {
        item.classList.add('hidden');
      }
    });

    // Update count display
    if (query) {
      searchCount.textContent = visibleCount + ' of ' + totalTests + ' tests';
    } else {
      searchCount.textContent = totalTests + ' tests';
    }

    // Show/hide no results message
    if (visibleCount === 0 && query) {
      noResults.classList.add('visible');
    } else {
      noResults.classList.remove('visible');
    }
  }

  if (searchInput) {
    searchInput.addEventListener('input', updateSearch);
    // Initialize count
    searchCount.textContent = totalTests + ' tests';
  }
</script>`;
  }

  /**
   * Build header section
   */
  private buildHeader(result: TestSuiteResult, timestamp: string): string {
    const statusClass = result.success ? 'pass' : 'fail';
    const statusText = result.success ? 'ALL TESTS PASSED' : 'TESTS FAILED';

    return `<header class="header">
  <h1>E2E Test Report</h1>
  <div class="timestamp">Generated: ${timestamp}</div>
  <div class="status-badge ${statusClass}">${statusText}</div>
</header>`;
  }

  /**
   * Build summary dashboard
   */
  private buildSummaryDashboard(summary: ReturnType<BaseReporter['calculateSummary']>): string {
    return `<div class="dashboard">
  <div class="stat-card">
    <div class="value">${summary.total}</div>
    <div class="label">Total Tests</div>
  </div>
  <div class="stat-card passed">
    <div class="value">${summary.passed}</div>
    <div class="label">Passed</div>
  </div>
  <div class="stat-card failed">
    <div class="value">${summary.failed}</div>
    <div class="label">Failed</div>
  </div>
  <div class="stat-card skipped">
    <div class="value">${summary.skipped}</div>
    <div class="label">Skipped</div>
  </div>
  <div class="stat-card">
    <div class="value">${summary.duration}</div>
    <div class="label">Duration</div>
  </div>
</div>`;
  }

  /**
   * Build charts section
   */
  private buildCharts(summary: ReturnType<BaseReporter['calculateSummary']>): string {
    const total = summary.total || 1;
    const passedPct = (summary.passed / total) * 100;
    const failedPct = (summary.failed / total) * 100;
    const skippedPct = (summary.skipped / total) * 100;

    return `<div class="charts">
  <div class="chart-card">
    <h3>Test Results Distribution</h3>
    <div class="progress-bar">
      <div class="segment passed" style="width: ${passedPct}%"></div>
      <div class="segment failed" style="width: ${failedPct}%"></div>
      <div class="segment skipped" style="width: ${skippedPct}%"></div>
    </div>
    <div class="legend">
      <div class="legend-item">
        <span class="legend-dot passed"></span>
        <span>Passed (${summary.passed})</span>
      </div>
      <div class="legend-item">
        <span class="legend-dot failed"></span>
        <span>Failed (${summary.failed})</span>
      </div>
      <div class="legend-item">
        <span class="legend-dot skipped"></span>
        <span>Skipped (${summary.skipped})</span>
      </div>
    </div>
  </div>
  <div class="chart-card">
    <h3>Pass Rate</h3>
    <div class="progress-bar">
      <div class="segment passed" style="width: ${summary.passRate}%"></div>
    </div>
    <div class="legend">
      <div class="legend-item">
        <span>${summary.passRate}% pass rate</span>
      </div>
    </div>
  </div>
</div>`;
  }

  /**
   * Build test list section
   */
  private buildTestList(result: TestSuiteResult): string {
    const tests = result.results.map((test) => this.buildTestItem(test)).join('');

    return `<div class="test-list">
  <h2>Test Results</h2>
  <div class="search-container">
    <input type="text" id="test-search" class="search-input" placeholder="Search tests by name or description..." />
    <span id="search-count" class="search-count"></span>
  </div>
  <div id="no-results" class="no-results">No tests match your search.</div>
  ${tests}
</div>`;
  }

  /**
   * Build individual test item
   */
  private buildTestItem(test: TestExecutionResult): string {
    const statusIcon = this.getStatusIcon(test.status);
    const description = test.description ? `<div class="test-description">${this.escapeHTML(test.description)}</div>` : '';
    const dataName = this.escapeHTML(test.name);
    const dataDescription = test.description ? this.escapeHTML(test.description) : '';

    return `<div class="test-item" data-name="${dataName}" data-description="${dataDescription}">
  <div class="test-header">
    <div class="test-status ${test.status}">${statusIcon}</div>
    <div class="test-info">
      <div class="test-name">${this.escapeHTML(test.name)}</div>
      ${description}
    </div>
    <div class="test-duration">${this.formatDuration(test.duration)}</div>
    <div class="test-toggle">&#9654;</div>
  </div>
  <div class="test-details">
    ${this.buildPhaseDetails(test)}
    ${test.error ? this.buildErrorBox(test.error) : ''}
  </div>
</div>`;
  }

  /**
   * Build phase details for a test
   */
  private buildPhaseDetails(test: TestExecutionResult): string {
    return test.phases
      .map((phase) => {
        const steps = phase.steps.map((step) => this.buildStepItem(step)).join('');
        return `<div class="phase">
  <div class="phase-header">
    <span class="phase-status ${phase.status}"></span>
    <span>${phase.phase.toUpperCase()}</span>
    <span style="margin-left: auto; color: var(--color-text-light);">
      ${this.formatDuration(phase.duration)}
    </span>
  </div>
  <div class="steps">${steps}</div>
  ${phase.error ? this.buildErrorBox(phase.error) : ''}
</div>`;
      })
      .join('');
  }

  /**
   * Build step item
   */
  private buildStepItem(step: StepResult): string {
    const description = step.description ? ` - ${this.escapeHTML(step.description)}` : '';
    const adapterDetails = step.data ? this.formatAdapterDetails(step.adapter, step.data) : '';

    return `<div class="step">
  <span class="step-status ${step.status}"></span>
  <div class="step-info">
    <span class="step-adapter">[${step.adapter}]</span>
    <span class="step-action">${this.escapeHTML(step.action)}</span>${description}
    <span style="color: var(--color-text-light);"> (${this.formatDuration(step.duration)})</span>
    ${adapterDetails}
    ${step.error ? `<div class="error-box">${this.escapeHTML(step.error.message)}</div>` : ''}
  </div>
</div>`;
  }

  /**
   * Format adapter-specific details
   */
  private formatAdapterDetails(adapter: string, data: unknown): string {
    if (!data || typeof data !== 'object') return '';

    switch (adapter) {
      case 'http':
        return this.formatHTTPDetails(data as Record<string, unknown>);
      case 'postgresql':
        return this.formatPostgreSQLDetails(data as Record<string, unknown>);
      case 'redis':
        return this.formatRedisDetails(data as Record<string, unknown>);
      default:
        return '';
    }
  }

  /**
   * Format HTTP request/response details
   */
  private formatHTTPDetails(data: Record<string, unknown>): string {
    const request = data.request as Record<string, unknown> | undefined;
    const response = data.response as Record<string, unknown> | undefined;

    if (!request || !response) return '';

    const method = request.method as string || 'GET';
    const url = request.url as string || '';
    const status = response.status as number || 0;
    const statusClass = status >= 200 && status < 300 ? 'success' : 'error';

    // Build summary line
    const summary = `<div class="step-summary">
      <span class="method">${method}</span>
      <span class="url">${this.escapeHTML(this.truncateUrl(url))}</span>
      <span>→</span>
      <span class="status ${statusClass}">${status}</span>
    </div>`;

    // Build expandable details
    const requestHeaders = request.headers ? this.formatJSON(request.headers) : '{}';
    const requestBody = request.body ? this.formatJSON(request.body) : null;
    const responseBody = response.body ? this.formatJSON(response.body) : null;

    const details = `
    <div class="step-details-toggle"><span class="arrow">▶</span> Details</div>
    <div class="step-details-content">
      <div class="section">
        <div class="section-label">Request Headers</div>
        <pre>${this.escapeHTML(requestHeaders)}</pre>
      </div>
      ${requestBody ? `<div class="section">
        <div class="section-label">Request Body</div>
        <pre>${this.escapeHTML(requestBody)}</pre>
      </div>` : ''}
      ${responseBody ? `<div class="section">
        <div class="section-label">Response Body</div>
        <pre>${this.escapeHTML(responseBody)}</pre>
      </div>` : ''}
    </div>`;

    return summary + details;
  }

  /**
   * Format PostgreSQL query details
   */
  private formatPostgreSQLDetails(data: Record<string, unknown>): string {
    const query = data.query as string | undefined;
    const params = data.params as unknown[] | undefined;
    const rowCount = data.rowCount as number | undefined;
    const rows = data.rows as unknown[] | undefined;
    const count = data.count as number | undefined;
    const command = data.command as string | undefined;

    if (!query) return '';

    // Build summary line
    const truncatedQuery = this.truncateQuery(query);
    const resultInfo = count !== undefined
      ? `→ ${count}`
      : rowCount !== undefined
        ? `→ ${rowCount} row${rowCount !== 1 ? 's' : ''}`
        : command
          ? `→ ${command}`
          : '';

    const summary = `<div class="step-summary">
      <span class="query-text">${this.escapeHTML(truncatedQuery)}</span>
      <span class="row-count">${resultInfo}</span>
    </div>`;

    // Build expandable details
    const paramsStr = params && params.length > 0 ? this.formatJSON(params) : null;
    const rowsStr = rows && rows.length > 0 ? this.formatJSON(rows) : null;

    const details = `
    <div class="step-details-toggle"><span class="arrow">▶</span> Details</div>
    <div class="step-details-content">
      <div class="section">
        <div class="section-label">Query</div>
        <pre>${this.escapeHTML(query)}</pre>
      </div>
      ${paramsStr ? `<div class="section">
        <div class="section-label">Parameters</div>
        <pre>${this.escapeHTML(paramsStr)}</pre>
      </div>` : ''}
      ${rowsStr ? `<div class="section">
        <div class="section-label">Result (first ${rows!.length} rows)</div>
        <pre>${this.escapeHTML(rowsStr)}</pre>
      </div>` : ''}
    </div>`;

    return summary + details;
  }

  /**
   * Format Redis command details
   */
  private formatRedisDetails(data: Record<string, unknown>): string {
    const command = data.command as string | undefined;
    const key = data.key as string | undefined;
    const field = data.field as string | undefined;
    const value = data.value;
    const result = data.result;

    if (!command) return '';

    // Build summary line
    const keyStr = key ? ` ${key}` : '';
    const fieldStr = field ? ` ${field}` : '';
    const resultStr = result !== undefined
      ? ` → ${this.truncateValue(result)}`
      : '';

    const summary = `<div class="step-summary">
      <span class="method">${command}</span>
      <span class="query-text">${this.escapeHTML(keyStr + fieldStr)}</span>
      <span class="row-count">${this.escapeHTML(resultStr)}</span>
    </div>`;

    // Build expandable details if there's meaningful data
    const hasDetails = value !== undefined || (result !== undefined && typeof result === 'object');
    if (!hasDetails) return summary;

    const details = `
    <div class="step-details-toggle"><span class="arrow">▶</span> Details</div>
    <div class="step-details-content">
      ${value !== undefined ? `<div class="section">
        <div class="section-label">Value</div>
        <pre>${this.escapeHTML(this.formatJSON(value))}</pre>
      </div>` : ''}
      ${result !== undefined ? `<div class="section">
        <div class="section-label">Result</div>
        <pre>${this.escapeHTML(this.formatJSON(result))}</pre>
      </div>` : ''}
    </div>`;

    return summary + details;
  }

  /**
   * Truncate URL for display
   */
  private truncateUrl(url: string): string {
    if (url.length <= 80) return url;
    try {
      const parsed = new URL(url);
      const path = parsed.pathname + parsed.search;
      if (path.length <= 60) return path;
      return path.slice(0, 57) + '...';
    } catch {
      return url.slice(0, 77) + '...';
    }
  }

  /**
   * Truncate SQL query for display
   */
  private truncateQuery(query: string): string {
    const normalized = query.replace(/\s+/g, ' ').trim();
    if (normalized.length <= 60) return normalized;
    return normalized.slice(0, 57) + '...';
  }

  /**
   * Truncate value for display
   */
  private truncateValue(value: unknown): string {
    const str = typeof value === 'string' ? value : JSON.stringify(value);
    if (str.length <= 30) return str;
    return str.slice(0, 27) + '...';
  }

  /**
   * Format value as pretty JSON
   */
  private formatJSON(value: unknown): string {
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }

  /**
   * Build error box
   */
  private buildErrorBox(error: Error): string {
    const stack = error.stack ? `\n\n${error.stack}` : '';
    return `<div class="error-box">${this.escapeHTML(error.message)}${this.escapeHTML(stack)}</div>`;
  }

  /**
   * Get status icon character
   */
  private getStatusIcon(status: string): string {
    switch (status) {
      case 'passed':
        return '&#10003;'; // checkmark
      case 'failed':
      case 'error':
        return '&#10007;'; // X
      case 'skipped':
        return '&#8211;'; // dash
      default:
        return '?';
    }
  }

  /**
   * Escape HTML special characters
   */
  private escapeHTML(str: string): string {
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  /**
   * Write HTML content to file
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
