/**
 * E2E Test Runner - Shared Assertion Runner
 *
 * Unified assertion logic for all adapters
 */

import { AssertionError } from '../errors';

// ============================================================================
// Types
// ============================================================================

/**
 * Base assertion interface with all supported operators
 * All adapters should extend or use this interface
 */
export interface BaseAssertion {
  // Equality
  equals?: unknown;

  // String operations
  contains?: string;
  matches?: string;

  // Existence
  exists?: boolean;

  // Type checking
  type?: string;

  // Length/size
  length?: number;
  notEmpty?: boolean;
  isEmpty?: boolean;

  // Numeric comparisons
  greaterThan?: number;
  lessThan?: number;

  // Null checks
  isNull?: boolean;
  isNotNull?: boolean;
}

// ============================================================================
// Core Functions
// ============================================================================

/**
 * Run all assertions on a value
 *
 * @param value - The value to assert against
 * @param assertion - The assertion operators to apply
 * @param path - Optional path for error messages (e.g., "$.errors[0].code")
 */
export function runAssertion(
  value: unknown,
  assertion: BaseAssertion,
  path?: string
): void {
  const pathPrefix = path ? `${path} ` : '';

  // exists
  if (assertion.exists === true && value === undefined) {
    throw new AssertionError(`${pathPrefix}does not exist`, {
      path,
      operator: 'exists',
    });
  }

  if (assertion.exists === false && value !== undefined) {
    throw new AssertionError(`${pathPrefix}exists but should not`, {
      path,
      actual: value,
      operator: 'notExists',
    });
  }

  // equals
  if (assertion.equals !== undefined && value !== assertion.equals) {
    // Handle toString comparison for ObjectId etc.
    const actualStr = value?.toString ? value.toString() : value;
    const expectedStr = assertion.equals?.toString
      ? assertion.equals.toString()
      : assertion.equals;

    if (actualStr !== expectedStr) {
      throw new AssertionError(
        `${pathPrefix}= ${JSON.stringify(value)}, expected ${JSON.stringify(assertion.equals)}`,
        {
          path,
          expected: assertion.equals,
          actual: value,
          operator: 'equals',
        }
      );
    }
  }

  // contains
  if (assertion.contains !== undefined && !String(value).includes(assertion.contains)) {
    throw new AssertionError(
      `${pathPrefix}does not contain "${assertion.contains}"`,
      {
        path,
        expected: assertion.contains,
        actual: value,
        operator: 'contains',
      }
    );
  }

  // matches
  if (assertion.matches !== undefined && !new RegExp(assertion.matches).test(String(value))) {
    throw new AssertionError(
      `${pathPrefix}does not match /${assertion.matches}/`,
      {
        path,
        expected: assertion.matches,
        actual: value,
        operator: 'matches',
      }
    );
  }

  // type
  if (assertion.type !== undefined) {
    const actualType = getValueType(value);
    if (actualType !== assertion.type) {
      throw new AssertionError(
        `${pathPrefix}type is ${actualType}, expected ${assertion.type}`,
        {
          path,
          expected: assertion.type,
          actual: actualType,
          operator: 'type',
        }
      );
    }
  }

  // length
  if (assertion.length !== undefined) {
    const len = getValueLength(value);
    if (len !== assertion.length) {
      throw new AssertionError(
        `${pathPrefix}length is ${len}, expected ${assertion.length}`,
        {
          path,
          expected: assertion.length,
          actual: len,
          operator: 'length',
        }
      );
    }
  }

  // greaterThan
  if (assertion.greaterThan !== undefined && Number(value) <= assertion.greaterThan) {
    throw new AssertionError(
      `${pathPrefix}= ${value}, expected > ${assertion.greaterThan}`,
      {
        path,
        expected: `> ${assertion.greaterThan}`,
        actual: value,
        operator: 'greaterThan',
      }
    );
  }

  // lessThan
  if (assertion.lessThan !== undefined && Number(value) >= assertion.lessThan) {
    throw new AssertionError(
      `${pathPrefix}= ${value}, expected < ${assertion.lessThan}`,
      {
        path,
        expected: `< ${assertion.lessThan}`,
        actual: value,
        operator: 'lessThan',
      }
    );
  }

  // notEmpty
  if (assertion.notEmpty === true) {
    const len = getValueLength(value);
    if (len === 0) {
      throw new AssertionError(
        `${pathPrefix}is empty, expected not empty`,
        {
          path,
          expected: 'not empty',
          actual: value,
          operator: 'notEmpty',
        }
      );
    }
  }

  // isEmpty
  if (assertion.isEmpty === true) {
    const len = getValueLength(value);
    if (len !== 0) {
      throw new AssertionError(
        `${pathPrefix}is not empty (length: ${len}), expected empty`,
        {
          path,
          expected: 'empty',
          actual: value,
          operator: 'isEmpty',
        }
      );
    }
  }

  // isNull
  if (assertion.isNull === true && value !== null && value !== undefined) {
    throw new AssertionError(`${pathPrefix}is not null`, {
      path,
      actual: value,
      operator: 'isNull',
    });
  }

  // isNotNull
  if (assertion.isNotNull === true && (value === null || value === undefined)) {
    throw new AssertionError(`${pathPrefix}is null`, {
      path,
      operator: 'isNotNull',
    });
  }
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Get the length of a value (array, string, or object keys)
 */
export function getValueLength(value: unknown): number {
  if (value === null || value === undefined) {
    return 0;
  }
  if (Array.isArray(value)) {
    return value.length;
  }
  if (typeof value === 'string') {
    return value.length;
  }
  if (typeof value === 'object') {
    return Object.keys(value).length;
  }
  return -1;
}

/**
 * Get the type of a value (with special handling for arrays and null)
 */
export function getValueType(value: unknown): string {
  if (value === null) {
    return 'null';
  }
  if (Array.isArray(value)) {
    return 'array';
  }
  return typeof value;
}
