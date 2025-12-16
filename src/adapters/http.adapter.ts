/**
 * E2E Test Runner - HTTP Adapter
 *
 * HTTP client for REST API testing
 */

import { AdapterError, AssertionError } from '../errors';
import type { AdapterConfig, AdapterContext, AdapterStepResult } from '../types';
import { BaseAdapter } from './base.adapter';
import { evaluateJSONPath as evaluateJSONPathFull } from '../assertions/jsonpath';
import { runAssertion, type BaseAssertion } from '../assertions/assertion-runner';

// ============================================================================
// Types
// ============================================================================

export interface HTTPRequestParams {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'HEAD' | 'OPTIONS';
  url: string;
  headers?: Record<string, string>;
  body?: unknown;
  query?: Record<string, string>;
  timeout?: number;
  followRedirects?: boolean;
  capture?: Record<string, string>;
  assert?: HTTPAssertion;
}

export interface HTTPResponse {
  status: number;
  statusText: string;
  headers: Record<string, string>;
  body: unknown;
  duration: number;
}

export interface HTTPAssertion {
  status?: number | number[];
  statusRange?: [number, number];
  headers?: Record<string, string | RegExp>;
  json?: JSONPathAssertion[];
  body?: {
    contains?: string;
    matches?: string;
    equals?: string;
  };
  duration?: {
    lessThan?: number;
    greaterThan?: number;
  };
}

export interface JSONPathAssertion extends BaseAssertion {
  path: string;
}

// ============================================================================
// HTTP Adapter
// ============================================================================

export class HTTPAdapter extends BaseAdapter {
  private baseUrl: string;
  private defaultHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  private defaultTimeout: number = 30000;

  constructor(config: AdapterConfig, logger: import('../types').Logger) {
    super(config, logger);
    this.baseUrl = config.baseUrl || '';
    if (config.defaultHeaders) {
      this.defaultHeaders = {
        ...this.defaultHeaders,
        ...(config.defaultHeaders as Record<string, string>),
      };
    }
    if (config.timeout) {
      this.defaultTimeout = config.timeout as number;
    }
  }

  get name(): string {
    return 'http';
  }

  async connect(): Promise<void> {
    // HTTP adapter doesn't need connection
    this.connected = true;
  }

  async disconnect(): Promise<void> {
    this.connected = false;
  }

  async execute(
    action: string,
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    this.logAction(action, params);

    if (action !== 'request') {
      throw new AdapterError('http', action, `Unknown action: ${action}`);
    }

    return this.request(params as unknown as HTTPRequestParams, ctx);
  }

  async healthCheck(): Promise<boolean> {
    if (!this.baseUrl) {
      return true; // No base URL to check
    }

    try {
      const response = await fetch(this.baseUrl, {
        method: 'HEAD',
        signal: AbortSignal.timeout(5000),
      });
      return response.ok || response.status < 500;
    } catch {
      return false;
    }
  }

  /**
   * Execute an HTTP request
   */
  async request(
    params: HTTPRequestParams,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    const start = Date.now();

    try {
      const url = this.buildUrl(params.url, params.query, ctx.baseUrl);
      const method = params.method || 'GET';
      const headers = this.buildHeaders(params.headers, ctx);
      const timeout = params.timeout || this.defaultTimeout;

      const fetchOptions: RequestInit = {
        method,
        headers,
        signal: AbortSignal.timeout(timeout),
        redirect: params.followRedirects === false ? 'manual' : 'follow',
      };

      // Add body for non-GET requests
      if (params.body && method !== 'GET' && method !== 'HEAD') {
        fetchOptions.body =
          typeof params.body === 'string'
            ? params.body
            : JSON.stringify(params.body);
      }

      const response = await fetch(url, fetchOptions);
      const duration = Date.now() - start;

      // Parse response body
      let responseBody: unknown;
      const contentType = response.headers.get('content-type') || '';

      if (contentType.includes('application/json')) {
        try {
          responseBody = await response.json();
        } catch {
          responseBody = await response.text();
        }
      } else {
        responseBody = await response.text();
      }

      const httpResponse: HTTPResponse = {
        status: response.status,
        statusText: response.statusText,
        headers: Object.fromEntries(response.headers.entries()),
        body: responseBody,
        duration,
      };

      // Log HTTP response in debug mode
      this.logHttpResponse(method, url, httpResponse);

      // Handle captures
      if (params.capture) {
        this.handleCaptures(
          responseBody,
          params.capture as Record<string, string>,
          ctx
        );
      }

      // Handle assertions
      if (params.assert) {
        this.runAssertions(httpResponse, params.assert as HTTPAssertion);
      }

      this.logResult('request', true, duration);

      // Return rich result with request + response details for reporting
      return this.successResult({
        request: {
          method,
          url,
          headers,
          body: params.body,
        },
        response: httpResponse,
      }, duration);
    } catch (error) {
      const duration = Date.now() - start;
      this.logResult('request', false, duration);

      if (error instanceof AssertionError) {
        throw error;
      }

      const message =
        error instanceof Error ? error.message : String(error);
      throw new AdapterError('http', 'request', message);
    }
  }

  /**
   * Build full URL with query parameters
   */
  private buildUrl(
    path: string,
    query?: Record<string, string>,
    contextBaseUrl?: string
  ): string {
    const baseUrl = contextBaseUrl || this.baseUrl;

    // If path is already a full URL, use it directly
    let url: URL;
    if (path.startsWith('http://') || path.startsWith('https://')) {
      url = new URL(path);
    } else {
      url = new URL(path, baseUrl);
    }

    // Add query parameters
    if (query) {
      for (const [key, value] of Object.entries(query)) {
        url.searchParams.set(key, value);
      }
    }

    return url.toString();
  }

  /**
   * Format and log HTTP response for debug mode
   */
  private logHttpResponse(
    method: string,
    url: string,
    response: HTTPResponse
  ): void {
    const statusLine = `HTTP/1.1 ${response.statusText} ${response.status}`;

    // Format headers
    const headerLines = Object.entries(response.headers)
      .map(([key, value]) => `${key}: ${value}`)
      .join('\n');

    // Format body
    let bodyStr: string;
    if (response.body === null || response.body === undefined) {
      bodyStr = '';
    } else if (typeof response.body === 'string') {
      bodyStr = response.body;
    } else {
      try {
        bodyStr = JSON.stringify(response.body, null, 2);
      } catch {
        bodyStr = String(response.body);
      }
    }

    // Build full response output
    const output = [
      `\n┌─── HTTP Response ───────────────────────────────────────`,
      `│ ${method} ${url}`,
      `├─── Status ──────────────────────────────────────────────`,
      `│ ${statusLine}`,
      `├─── Headers ─────────────────────────────────────────────`,
      ...headerLines.split('\n').map(line => `│ ${line}`),
      `├─── Body (${response.duration}ms) ────────────────────────────────────`,
      ...bodyStr.split('\n').map(line => `│ ${line}`),
      `└─────────────────────────────────────────────────────────\n`,
    ].join('\n');

    this.logger.debug(output);
  }

  /**
   * Build request headers
   */
  private buildHeaders(
    requestHeaders?: Record<string, string>,
    ctx?: AdapterContext
  ): Record<string, string> {
    const headers = { ...this.defaultHeaders };

    if (requestHeaders) {
      for (const [key, value] of Object.entries(requestHeaders)) {
        headers[key] = value;
      }
    }

    return headers;
  }

  /**
   * Handle value captures from response
   */
  private handleCaptures(
    body: unknown,
    capture: Record<string, string>,
    ctx: AdapterContext
  ): void {
    for (const [varName, jsonPath] of Object.entries(capture)) {
      const value = this.evaluateJSONPath(body, jsonPath);
      ctx.capture(varName, value);
      this.logger.debug(`Captured ${varName} = ${JSON.stringify(value)}`);
    }
  }

  /**
   * Evaluate a JSONPath expression
   * Supports full JSONPath syntax including array indexing: $.errors[0].code
   */
  private evaluateJSONPath(data: unknown, path: string): unknown {
    return evaluateJSONPathFull(data, path);
  }

  /**
   * Run assertions on HTTP response
   */
  private runAssertions(response: HTTPResponse, assertion: HTTPAssertion): void {
    // Status assertions
    if (assertion.status !== undefined) {
      const expectedStatuses = Array.isArray(assertion.status)
        ? assertion.status
        : [assertion.status];

      if (!expectedStatuses.includes(response.status)) {
        throw new AssertionError(
          `Expected status ${expectedStatuses.join(' or ')}, got ${response.status}`,
          {
            expected: expectedStatuses,
            actual: response.status,
            operator: 'status',
          }
        );
      }
    }

    // Status range assertion
    if (assertion.statusRange) {
      const [min, max] = assertion.statusRange;
      if (response.status < min || response.status > max) {
        throw new AssertionError(
          `Expected status in range [${min}, ${max}], got ${response.status}`,
          {
            expected: assertion.statusRange,
            actual: response.status,
            operator: 'statusRange',
          }
        );
      }
    }

    // Header assertions
    if (assertion.headers) {
      for (const [name, expected] of Object.entries(assertion.headers)) {
        const actual = response.headers[name.toLowerCase()];

        if (expected instanceof RegExp) {
          if (!actual || !expected.test(actual)) {
            throw new AssertionError(
              `Header "${name}" does not match pattern ${expected}`,
              {
                expected: expected.toString(),
                actual,
                path: `headers.${name}`,
                operator: 'matches',
              }
            );
          }
        } else if (actual !== expected) {
          throw new AssertionError(
            `Header "${name}" expected "${expected}", got "${actual}"`,
            {
              expected,
              actual,
              path: `headers.${name}`,
              operator: 'equals',
            }
          );
        }
      }
    }

    // JSON assertions
    if (assertion.json) {
      for (const jsonAssertion of assertion.json) {
        this.runJSONAssertion(response.body, jsonAssertion);
      }
    }

    // Body assertions
    if (assertion.body) {
      const bodyStr =
        typeof response.body === 'string'
          ? response.body
          : JSON.stringify(response.body);

      if (
        assertion.body.contains &&
        !bodyStr.includes(assertion.body.contains)
      ) {
        throw new AssertionError(
          `Body does not contain "${assertion.body.contains}"`,
          {
            expected: assertion.body.contains,
            actual: bodyStr.slice(0, 200),
            operator: 'contains',
          }
        );
      }

      if (
        assertion.body.matches &&
        !new RegExp(assertion.body.matches).test(bodyStr)
      ) {
        throw new AssertionError(
          `Body does not match pattern "${assertion.body.matches}"`,
          {
            expected: assertion.body.matches,
            actual: bodyStr.slice(0, 200),
            operator: 'matches',
          }
        );
      }

      if (assertion.body.equals && bodyStr !== assertion.body.equals) {
        throw new AssertionError(`Body does not equal expected value`, {
          expected: assertion.body.equals,
          actual: bodyStr.slice(0, 200),
          operator: 'equals',
        });
      }
    }

    // Duration assertions
    if (assertion.duration) {
      if (
        assertion.duration.lessThan !== undefined &&
        response.duration >= assertion.duration.lessThan
      ) {
        throw new AssertionError(
          `Response time ${response.duration}ms exceeds limit ${assertion.duration.lessThan}ms`,
          {
            expected: `< ${assertion.duration.lessThan}`,
            actual: response.duration,
            operator: 'lessThan',
          }
        );
      }

      if (
        assertion.duration.greaterThan !== undefined &&
        response.duration <= assertion.duration.greaterThan
      ) {
        throw new AssertionError(
          `Response time ${response.duration}ms below minimum ${assertion.duration.greaterThan}ms`,
          {
            expected: `> ${assertion.duration.greaterThan}`,
            actual: response.duration,
            operator: 'greaterThan',
          }
        );
      }
    }
  }

  /**
   * Run a single JSON path assertion using shared runner
   */
  private runJSONAssertion(data: unknown, assertion: JSONPathAssertion): void {
    const value = this.evaluateJSONPath(data, assertion.path);
    runAssertion(value, assertion, assertion.path);
  }
}
