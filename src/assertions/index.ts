/**
 * E2E Test Runner - Assertions Module
 *
 * Provides assertion utilities for E2E tests including:
 * - JSONPath evaluation for extracting values from objects
 * - Matcher functions for comparisons
 * - Fluent expect() API with .not modifier
 */

// ============================================================================
// JSONPath Exports
// ============================================================================

export {
    evaluateJSONPath,
    evaluateJSONPathWithMeta,
    getByPath,
    hasJSONPath,
    isValidJSONPath,
    type JSONPathResult,
    parseSimplePath,
    queryJSONPath,
} from './jsonpath'

// ============================================================================
// Matcher Exports
// ============================================================================

export {
    // Utility functions
    deepEqual,
    formatValue,
    getTypeName,
    // Types
    type MatcherFunction,
    type MatcherName,
    type MatcherResult,
    // Matchers object
    matchers,
    // Equality matchers
    toBe,
    toBeDefined,
    toBeFalsy,
    toBeGreaterThan,
    toBeGreaterThanOrEqual,
    toBeLessThan,
    toBeLessThanOrEqual,
    toBeNotNull,
    toBeNull,
    toBeOneOf,
    toBeTruthy,
    toBeType,
    toBeUndefined,
    toContain,
    toEqual,
    toHaveLength,
    toHaveProperty,
    toMatch,
} from './matchers'

// ============================================================================
// Expect Exports
// ============================================================================

export {
    // Additional assertion utilities
    assert,
    assertFalse,
    assertThrows,
    assertThrowsAsync,
    // Main expect function
    expect,
    // Types
    type Expectation,
    type ExpectFunction,
    fail,
} from './expect'

// ============================================================================
// Assertion Runner Exports
// ============================================================================

export {
    // Core function
    runAssertion,
    // Types
    type BaseAssertion,
    // Helpers
    getValueLength,
    getValueType,
} from './assertion-runner'
