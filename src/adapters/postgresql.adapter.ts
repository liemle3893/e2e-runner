/**
 * E2E Test Runner - PostgreSQL Adapter
 *
 * Database operations using pg pool
 */

import { AdapterError, AssertionError } from '../errors';
import type { AdapterConfig, AdapterContext, AdapterStepResult, Logger } from '../types';
import { BaseAdapter } from './base.adapter';

// ============================================================================
// Types
// ============================================================================

export interface PostgreSQLAssertion {
  row?: number;
  column: string;
  equals?: unknown;
  contains?: string;
  matches?: string;
  isNull?: boolean;
  isNotNull?: boolean;
  greaterThan?: number;
  lessThan?: number;
}

// ============================================================================
// PostgreSQL Adapter
// ============================================================================

export class PostgreSQLAdapter extends BaseAdapter {
  private pool: import('pg').Pool | null = null;

  constructor(config: AdapterConfig, logger: Logger) {
    super(config, logger);
  }

  get name(): string {
    return 'postgresql';
  }

  async connect(): Promise<void> {
    if (this.connected) return;

    try {
      const { Pool } = await import('pg');

      this.pool = new Pool({
        connectionString: this.config.connectionString,
        min: (this.config.poolMin as number) ?? 2,
        max: (this.config.poolMax as number) ?? 5,
        idleTimeoutMillis: 30000,
        connectionTimeoutMillis: 10000,
      });

      // Test connection
      const client = await this.pool.connect();
      await client.query('SELECT 1');
      client.release();

      this.connected = true;
      this.logger.info('PostgreSQL connected');
    } catch (error) {
      throw new AdapterError(
        'postgresql',
        'connect',
        `Failed to connect: ${error instanceof Error ? error.message : String(error)}`
      );
    }
  }

  async disconnect(): Promise<void> {
    if (this.pool) {
      await this.pool.end();
      this.pool = null;
      this.connected = false;
      this.logger.info('PostgreSQL disconnected');
    }
  }

  async execute(
    action: string,
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    if (!this.pool) {
      throw new AdapterError('postgresql', action, 'Not connected');
    }

    this.logAction(action, { sql: params.sql });

    const start = Date.now();

    try {
      let result: AdapterStepResult;

      switch (action) {
        case 'execute':
          result = await this.executeSQL(params);
          break;
        case 'query':
          result = await this.querySQL(params, ctx);
          break;
        case 'queryOne':
          result = await this.queryOneSQL(params, ctx);
          break;
        case 'count':
          result = await this.countSQL(params);
          break;
        default:
          throw new AdapterError('postgresql', action, `Unknown action: ${action}`);
      }

      this.logResult(action, true, result.duration);
      return result;
    } catch (error) {
      const duration = Date.now() - start;
      this.logResult(action, false, duration);

      if (error instanceof AssertionError || error instanceof AdapterError) {
        throw error;
      }

      throw new AdapterError(
        'postgresql',
        action,
        error instanceof Error ? error.message : String(error)
      );
    }
  }

  async healthCheck(): Promise<boolean> {
    if (!this.pool) return false;

    try {
      const client = await this.pool.connect();
      await client.query('SELECT 1');
      client.release();
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Execute SQL without returning results
   */
  private async executeSQL(params: Record<string, unknown>): Promise<AdapterStepResult> {
    const { sql, params: queryParams } = params;
    const start = Date.now();

    const result = await this.pool!.query(
      sql as string,
      (queryParams as unknown[]) || []
    );

    // Return rich result with query metadata for reporting
    return this.successResult({
      query: sql as string,
      params: queryParams || [],
      rowCount: result.rowCount || 0,
      command: result.command,
    }, Date.now() - start);
  }

  /**
   * Query SQL and return all rows
   */
  private async querySQL(
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    const { sql, params: queryParams, capture, assert } = params;
    const start = Date.now();

    const result = await this.pool!.query(
      sql as string,
      (queryParams as unknown[]) || []
    );

    const rows = result.rows;
    const duration = Date.now() - start;

    // Handle captures
    if (capture && rows.length > 0) {
      for (const [varName, column] of Object.entries(capture as Record<string, string>)) {
        ctx.capture(varName, rows[0][column]);
      }
    }

    // Handle assertions
    if (assert) {
      this.runAssertions(rows, assert as PostgreSQLAssertion[]);
    }

    // Return rich result with query metadata for reporting
    return this.successResult({
      query: sql as string,
      params: queryParams || [],
      rowCount: rows.length,
      rows: rows.slice(0, 5),  // Preview first 5 rows
    }, duration);
  }

  /**
   * Query SQL and return exactly one row
   */
  private async queryOneSQL(
    params: Record<string, unknown>,
    ctx: AdapterContext
  ): Promise<AdapterStepResult> {
    const { sql, params: queryParams, capture, assert } = params;
    const start = Date.now();

    const result = await this.pool!.query(
      sql as string,
      (queryParams as unknown[]) || []
    );

    if (result.rows.length === 0) {
      throw new AdapterError('postgresql', 'queryOne', 'Expected exactly one row, got 0');
    }

    const row = result.rows[0];
    const duration = Date.now() - start;

    // Handle captures
    if (capture) {
      for (const [varName, column] of Object.entries(capture as Record<string, string>)) {
        ctx.capture(varName, row[column]);
      }
    }

    // Handle assertions (wrap single row in array)
    if (assert) {
      this.runAssertions([row], assert as PostgreSQLAssertion[]);
    }

    // Return rich result with query metadata for reporting
    return this.successResult({
      query: sql as string,
      params: queryParams || [],
      rowCount: 1,
      rows: [row],
    }, duration);
  }

  /**
   * Count rows matching query
   */
  private async countSQL(params: Record<string, unknown>): Promise<AdapterStepResult> {
    const { sql, params: queryParams } = params;
    const start = Date.now();

    const result = await this.pool!.query(
      sql as string,
      (queryParams as unknown[]) || []
    );

    const count = result.rows.length > 0 ? parseInt(result.rows[0].count, 10) : 0;

    // Return rich result with query metadata for reporting
    return this.successResult({
      query: sql as string,
      params: queryParams || [],
      count,
    }, Date.now() - start);
  }

  /**
   * Run assertions on query results
   */
  private runAssertions(rows: Record<string, unknown>[], assertions: PostgreSQLAssertion[]): void {
    for (const assertion of assertions) {
      const rowIndex = assertion.row ?? 0;

      if (rowIndex >= rows.length) {
        throw new AssertionError(
          `Row ${rowIndex} does not exist (only ${rows.length} rows)`,
          {
            expected: `row ${rowIndex}`,
            actual: `${rows.length} rows`,
            operator: 'rowExists',
          }
        );
      }

      const value = rows[rowIndex][assertion.column];

      if (assertion.equals !== undefined && value !== assertion.equals) {
        throw new AssertionError(
          `${assertion.column} = ${JSON.stringify(value)}, expected ${JSON.stringify(assertion.equals)}`,
          {
            path: `row[${rowIndex}].${assertion.column}`,
            expected: assertion.equals,
            actual: value,
            operator: 'equals',
          }
        );
      }

      if (assertion.isNull && value !== null) {
        throw new AssertionError(`${assertion.column} is not null`, {
          path: `row[${rowIndex}].${assertion.column}`,
          expected: null,
          actual: value,
          operator: 'isNull',
        });
      }

      if (assertion.isNotNull && value === null) {
        throw new AssertionError(`${assertion.column} is null`, {
          path: `row[${rowIndex}].${assertion.column}`,
          operator: 'isNotNull',
        });
      }

      if (assertion.contains && !String(value).includes(assertion.contains)) {
        throw new AssertionError(
          `${assertion.column} does not contain "${assertion.contains}"`,
          {
            path: `row[${rowIndex}].${assertion.column}`,
            expected: assertion.contains,
            actual: value,
            operator: 'contains',
          }
        );
      }

      if (assertion.matches && !new RegExp(assertion.matches).test(String(value))) {
        throw new AssertionError(
          `${assertion.column} does not match /${assertion.matches}/`,
          {
            path: `row[${rowIndex}].${assertion.column}`,
            expected: assertion.matches,
            actual: value,
            operator: 'matches',
          }
        );
      }

      if (assertion.greaterThan !== undefined && Number(value) <= assertion.greaterThan) {
        throw new AssertionError(
          `${assertion.column} = ${value}, expected > ${assertion.greaterThan}`,
          {
            path: `row[${rowIndex}].${assertion.column}`,
            expected: `> ${assertion.greaterThan}`,
            actual: value,
            operator: 'greaterThan',
          }
        );
      }

      if (assertion.lessThan !== undefined && Number(value) >= assertion.lessThan) {
        throw new AssertionError(
          `${assertion.column} = ${value}, expected < ${assertion.lessThan}`,
          {
            path: `row[${rowIndex}].${assertion.column}`,
            expected: `< ${assertion.lessThan}`,
            actual: value,
            operator: 'lessThan',
          }
        );
      }
    }
  }

  /**
   * Tagged template literal for execute
   */
  async executeTemplate(
    strings: TemplateStringsArray,
    ...values: unknown[]
  ): Promise<void> {
    const { sql, params } = this.taggedToQuery(strings, values);
    await this.pool!.query(sql, params);
  }

  /**
   * Tagged template literal for query
   */
  async queryTemplate(
    strings: TemplateStringsArray,
    ...values: unknown[]
  ): Promise<Record<string, unknown>[]> {
    const { sql, params } = this.taggedToQuery(strings, values);
    const result = await this.pool!.query(sql, params);
    return result.rows;
  }

  /**
   * Tagged template literal for queryOne
   */
  async queryOneTemplate(
    strings: TemplateStringsArray,
    ...values: unknown[]
  ): Promise<Record<string, unknown>> {
    const { sql, params } = this.taggedToQuery(strings, values);
    const result = await this.pool!.query(sql, params);
    if (result.rows.length === 0) {
      throw new AdapterError('postgresql', 'queryOne', 'Expected exactly one row, got 0');
    }
    return result.rows[0];
  }

  /**
   * Convert tagged template to parameterized query
   */
  private taggedToQuery(
    strings: TemplateStringsArray,
    values: unknown[]
  ): { sql: string; params: unknown[] } {
    const sql = strings.reduce((acc, str, i) => {
      return acc + str + (i < values.length ? `$${i + 1}` : '');
    }, '');
    return { sql, params: values };
  }
}
