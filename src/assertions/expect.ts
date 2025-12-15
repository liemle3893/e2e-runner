/**
 * E2E Test Runner - Expect Function
 *
 * Fluent assertion API with .not modifier support.
 * Throws AssertionError on failure with detailed information.
 */

import { AssertionError } from '../errors'
import {
    type MatcherResult,
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
// Types
// ============================================================================

/**
 * Expectation interface with all matcher methods
 */
export interface Expectation<T> {
    /**
     * Negates the following matcher
     */
    not: Expectation<T>

    /**
     * Strict equality (===)
     */
    toBe(expected: T): void

    /**
     * Deep equality comparison
     */
    toEqual(expected: T): void

    /**
     * Check if value is one of the expected values
     */
    toBeOneOf(expected: T[]): void

    /**
     * Check if value is defined (not undefined)
     */
    toBeDefined(): void

    /**
     * Check if value is undefined
     */
    toBeUndefined(): void

    /**
     * Check if value is null
     */
    toBeNull(): void

    /**
     * Check if value is not null
     */
    toBeNotNull(): void

    /**
     * Check if value is truthy
     */
    toBeTruthy(): void

    /**
     * Check if value is falsy
     */
    toBeFalsy(): void

    /**
     * Check if array/string contains an item
     */
    toContain(item: unknown): void

    /**
     * Check if array/string/object has specific length
     */
    toHaveLength(length: number): void

    /**
     * Check if string matches a pattern
     */
    toMatch(pattern: RegExp | string): void

    /**
     * Check if number is greater than value
     */
    toBeGreaterThan(value: number): void

    /**
     * Check if number is greater than or equal to value
     */
    toBeGreaterThanOrEqual(value: number): void

    /**
     * Check if number is less than value
     */
    toBeLessThan(value: number): void

    /**
     * Check if number is less than or equal to value
     */
    toBeLessThanOrEqual(value: number): void

    /**
     * Check if object has a property at the given path
     */
    toHaveProperty(path: string, value?: unknown): void

    /**
     * Check if value is of a specific type
     */
    toBeType(
        type:
            | 'string'
            | 'number'
            | 'boolean'
            | 'object'
            | 'array'
            | 'function'
            | 'undefined'
            | 'null',
    ): void
}

/**
 * The expect function signature
 */
export type ExpectFunction = <T>(actual: T) => Expectation<T>

// ============================================================================
// Implementation
// ============================================================================

/**
 * Create the expectation object with all matchers
 */
function createExpectation<T>(actual: T, negated: boolean): Expectation<T> {
    /**
     * Process a matcher result and throw if assertion fails
     */
    function processResult(result: MatcherResult, operator: string): void {
        const shouldPass = negated ? !result.pass : result.pass

        if (!shouldPass) {
            throw new AssertionError(result.message, {
                actual,
                operator: negated ? `not.${operator}` : operator,
            })
        }
    }

    const expectation: Expectation<T> = {
        get not(): Expectation<T> {
            if (negated) {
                throw new Error('Cannot chain multiple .not modifiers')
            }
            return createExpectation(actual, true)
        },

        toBe(expected: T): void {
            processResult(toBe(actual, expected), 'toBe')
        },

        toEqual(expected: T): void {
            processResult(toEqual(actual, expected), 'toEqual')
        },

        toBeOneOf(expected: T[]): void {
            processResult(toBeOneOf(actual, expected), 'toBeOneOf')
        },

        toBeDefined(): void {
            processResult(toBeDefined(actual), 'toBeDefined')
        },

        toBeUndefined(): void {
            processResult(toBeUndefined(actual), 'toBeUndefined')
        },

        toBeNull(): void {
            processResult(toBeNull(actual), 'toBeNull')
        },

        toBeNotNull(): void {
            processResult(toBeNotNull(actual), 'toBeNotNull')
        },

        toBeTruthy(): void {
            processResult(toBeTruthy(actual), 'toBeTruthy')
        },

        toBeFalsy(): void {
            processResult(toBeFalsy(actual), 'toBeFalsy')
        },

        toContain(item: unknown): void {
            processResult(toContain(actual, item), 'toContain')
        },

        toHaveLength(length: number): void {
            processResult(toHaveLength(actual, length), 'toHaveLength')
        },

        toMatch(pattern: RegExp | string): void {
            processResult(toMatch(actual, pattern), 'toMatch')
        },

        toBeGreaterThan(value: number): void {
            processResult(toBeGreaterThan(actual, value), 'toBeGreaterThan')
        },

        toBeGreaterThanOrEqual(value: number): void {
            processResult(toBeGreaterThanOrEqual(actual, value), 'toBeGreaterThanOrEqual')
        },

        toBeLessThan(value: number): void {
            processResult(toBeLessThan(actual, value), 'toBeLessThan')
        },

        toBeLessThanOrEqual(value: number): void {
            processResult(toBeLessThanOrEqual(actual, value), 'toBeLessThanOrEqual')
        },

        toHaveProperty(path: string, value?: unknown): void {
            if (arguments.length === 1) {
                processResult(toHaveProperty(actual, path), 'toHaveProperty')
            } else {
                processResult(toHaveProperty(actual, path, value), 'toHaveProperty')
            }
        },

        toBeType(type: string): void {
            processResult(toBeType(actual, type), 'toBeType')
        },
    }

    return expectation
}

/**
 * Create an expectation for a value
 *
 * @param actual - The value to test
 * @returns An expectation object with assertion methods
 *
 * @example
 * expect(5).toBe(5);
 * expect('hello').toContain('ell');
 * expect(value).not.toBeNull();
 */
export function expect<T>(actual: T): Expectation<T> {
    return createExpectation(actual, false)
}

// ============================================================================
// Additional Assertion Utilities
// ============================================================================

/**
 * Assert that a condition is true
 *
 * @param condition - The condition to check
 * @param message - Optional message on failure
 * @throws AssertionError if condition is false
 */
export function assert(condition: boolean, message?: string): asserts condition {
    if (!condition) {
        throw new AssertionError(message || 'Assertion failed', {
            actual: condition,
            expected: true,
            operator: 'assert',
        })
    }
}

/**
 * Assert that a condition is false
 *
 * @param condition - The condition to check
 * @param message - Optional message on failure
 * @throws AssertionError if condition is true
 */
export function assertFalse(condition: boolean, message?: string): void {
    if (condition) {
        throw new AssertionError(message || 'Expected condition to be false', {
            actual: condition,
            expected: false,
            operator: 'assertFalse',
        })
    }
}

/**
 * Force a test to fail with a message
 *
 * @param message - The failure message
 * @throws AssertionError always
 */
export function fail(message: string): never {
    throw new AssertionError(message, {
        operator: 'fail',
    })
}

/**
 * Assert that code throws an error
 *
 * @param fn - The function to execute
 * @param expected - Optional expected error message, class, or regex
 * @throws AssertionError if function doesn't throw or throws wrong error
 */
// eslint-disable-next-line sonarjs/cognitive-complexity -- Error matching requires handling multiple expected value types
export function assertThrows(
    fn: () => unknown,
    expected?: string | RegExp | (new (...args: unknown[]) => Error),
): void {
    let thrown = false
    let error: unknown

    try {
        fn()
    } catch (error_) {
        thrown = true
        error = error_
    }

    if (!thrown) {
        throw new AssertionError('Expected function to throw an error', {
            operator: 'assertThrows',
        })
    }

    if (expected === undefined) {
        return
    }

    if (typeof expected === 'string') {
        const message = error instanceof Error ? error.message : String(error)
        if (!message.includes(expected)) {
            throw new AssertionError(
                `Expected error message to contain "${expected}", but got "${message}"`,
                {
                    expected,
                    actual: message,
                    operator: 'assertThrows',
                },
            )
        }
    } else if (expected instanceof RegExp) {
        const message = error instanceof Error ? error.message : String(error)
        if (!expected.test(message)) {
            throw new AssertionError(
                `Expected error message to match ${expected}, but got "${message}"`,
                {
                    expected: expected.toString(),
                    actual: message,
                    operator: 'assertThrows',
                },
            )
        }
    } else if (typeof expected === 'function' && !(error instanceof expected)) {
        const actualName = error instanceof Error ? error.constructor.name : typeof error
        throw new AssertionError(
            `Expected error to be instance of ${expected.name}, but got ${actualName}`,
            {
                expected: expected.name,
                actual: actualName,
                operator: 'assertThrows',
            },
        )
    }
}

/**
 * Assert that async code throws an error
 *
 * @param fn - The async function to execute
 * @param expected - Optional expected error message, class, or regex
 * @throws AssertionError if function doesn't throw or throws wrong error
 */
// eslint-disable-next-line sonarjs/cognitive-complexity -- Error matching requires handling multiple expected value types
export async function assertThrowsAsync(
    fn: () => Promise<unknown>,
    expected?: string | RegExp | (new (...args: unknown[]) => Error),
): Promise<void> {
    let thrown = false
    let error: unknown

    try {
        await fn()
    } catch (error_) {
        thrown = true
        error = error_
    }

    if (!thrown) {
        throw new AssertionError('Expected async function to throw an error', {
            operator: 'assertThrowsAsync',
        })
    }

    if (expected === undefined) {
        return
    }

    if (typeof expected === 'string') {
        const message = error instanceof Error ? error.message : String(error)
        if (!message.includes(expected)) {
            throw new AssertionError(
                `Expected error message to contain "${expected}", but got "${message}"`,
                {
                    expected,
                    actual: message,
                    operator: 'assertThrowsAsync',
                },
            )
        }
    } else if (expected instanceof RegExp) {
        const message = error instanceof Error ? error.message : String(error)
        if (!expected.test(message)) {
            throw new AssertionError(
                `Expected error message to match ${expected}, but got "${message}"`,
                {
                    expected: expected.toString(),
                    actual: message,
                    operator: 'assertThrowsAsync',
                },
            )
        }
    } else if (typeof expected === 'function' && !(error instanceof expected)) {
        const actualName = error instanceof Error ? error.constructor.name : typeof error
        throw new AssertionError(
            `Expected error to be instance of ${expected.name}, but got ${actualName}`,
            {
                expected: expected.name,
                actual: actualName,
                operator: 'assertThrowsAsync',
            },
        )
    }
}

// Re-export formatValue for use in custom assertions

export { formatValue } from './matchers'
