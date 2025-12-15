/**
 * E2E Test Runner - Matcher Implementations
 *
 * Each matcher function returns { pass: boolean, message: string }
 * where message describes the assertion failure if pass is false.
 */

import { getByPath } from './jsonpath'

// ============================================================================
// Types
// ============================================================================

/**
 * Result of a matcher evaluation
 */
export interface MatcherResult {
    pass: boolean
    message: string
}

/**
 * A matcher function that compares actual value against expected
 */
export type MatcherFunction<T = unknown> = (actual: unknown, expected?: T) => MatcherResult

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Format a value for display in error messages
 */
export function formatValue(value: unknown, maxLength = 100): string {
    if (value === undefined) return 'undefined'
    if (value === null) return 'null'
    if (typeof value === 'string') return `"${value}"`
    if (typeof value === 'number' || typeof value === 'boolean') return String(value)
    if (typeof value === 'function') return `[Function: ${value.name || 'anonymous'}]`
    if (typeof value === 'symbol') return value.toString()
    if (value instanceof RegExp) return value.toString()
    if (value instanceof Date) return value.toISOString()
    if (value instanceof Error) return `[Error: ${value.message}]`

    try {
        // Using undefined instead of null for the replacer parameter
        const json = JSON.stringify(value, undefined, 2)
        if (json.length > maxLength) {
            return json.slice(0, maxLength) + '...'
        }
        return json
    } catch {
        return String(value)
    }
}

/**
 * Get the type name of a value
 */
export function getTypeName(value: unknown): string {
    if (value === null) return 'null'
    if (value === undefined) return 'undefined'
    if (Array.isArray(value)) return 'array'
    return typeof value
}

/**
 * Deep equality comparison
 */
// eslint-disable-next-line sonarjs/cognitive-complexity -- Deep equality requires handling multiple types
export function deepEqual(a: unknown, b: unknown): boolean {
    // Same reference or primitive
    if (a === b) return true

    // Handle null/undefined
    if (a === null || b === null) return a === b
    if (a === undefined || b === undefined) return a === b

    // Type check
    if (typeof a !== typeof b) return false

    // Handle dates
    if (a instanceof Date && b instanceof Date) {
        return a.getTime() === b.getTime()
    }

    // Handle RegExp
    if (a instanceof RegExp && b instanceof RegExp) {
        return a.toString() === b.toString()
    }

    // Handle arrays
    if (Array.isArray(a) && Array.isArray(b)) {
        if (a.length !== b.length) return false
        // eslint-disable-next-line unicorn/no-for-loop -- Need index access for parallel comparison
        for (let i = 0; i < a.length; i++) {
            if (!deepEqual(a[i], b[i])) return false
        }
        return true
    }

    // Handle objects
    if (typeof a === 'object' && typeof b === 'object') {
        const keysA = Object.keys(a as Record<string, unknown>)
        const keysB = Object.keys(b as Record<string, unknown>)

        if (keysA.length !== keysB.length) return false

        for (const key of keysA) {
            if (!keysB.includes(key)) return false
            if (
                !deepEqual((a as Record<string, unknown>)[key], (b as Record<string, unknown>)[key])
            ) {
                return false
            }
        }

        return true
    }

    return false
}

// ============================================================================
// Equality Matchers
// ============================================================================

/**
 * Strict equality matcher (===)
 */
export function toBe(actual: unknown, expected: unknown): MatcherResult {
    const pass = actual === expected
    const message = pass
        ? `Expected ${formatValue(actual)} not to be ${formatValue(expected)}`
        : `Expected ${formatValue(actual)} to be ${formatValue(expected)}`

    return { pass, message }
}

/**
 * Deep equality matcher
 */
export function toEqual(actual: unknown, expected: unknown): MatcherResult {
    const pass = deepEqual(actual, expected)
    const message = pass
        ? `Expected ${formatValue(actual)} not to deeply equal ${formatValue(expected)}`
        : `Expected ${formatValue(actual)} to deeply equal ${formatValue(expected)}`

    return { pass, message }
}

/**
 * Check if actual is one of the expected values
 */
export function toBeOneOf(actual: unknown, expected: unknown[]): MatcherResult {
    if (!Array.isArray(expected)) {
        return {
            pass: false,
            message: `Expected array of values, got ${formatValue(expected)}`,
        }
    }

    const pass = expected.some((e) => deepEqual(actual, e))
    const message = pass
        ? `Expected ${formatValue(actual)} not to be one of ${formatValue(expected)}`
        : `Expected ${formatValue(actual)} to be one of ${formatValue(expected)}`

    return { pass, message }
}

// ============================================================================
// Truthiness Matchers
// ============================================================================

/**
 * Check if value is defined (not undefined)
 */
export function toBeDefined(actual: unknown): MatcherResult {
    const pass = actual !== undefined
    const message = pass ? 'Expected value to be undefined' : 'Expected value to be defined'

    return { pass, message }
}

/**
 * Check if value is undefined
 */
export function toBeUndefined(actual: unknown): MatcherResult {
    const pass = actual === undefined
    const message = pass
        ? `Expected ${formatValue(actual)} not to be undefined`
        : `Expected ${formatValue(actual)} to be undefined`

    return { pass, message }
}

/**
 * Check if value is null
 */
export function toBeNull(actual: unknown): MatcherResult {
    const pass = actual === null
    const message = pass
        ? 'Expected value not to be null'
        : `Expected ${formatValue(actual)} to be null`

    return { pass, message }
}

/**
 * Check if value is not null
 */
export function toBeNotNull(actual: unknown): MatcherResult {
    const pass = actual !== null
    const message = pass
        ? 'Expected value to be null'
        : 'Expected value not to be null, but got null'

    return { pass, message }
}

/**
 * Check if value is truthy
 */
export function toBeTruthy(actual: unknown): MatcherResult {
    const pass = Boolean(actual)
    const message = pass
        ? `Expected ${formatValue(actual)} to be falsy`
        : `Expected ${formatValue(actual)} to be truthy`

    return { pass, message }
}

/**
 * Check if value is falsy
 */
export function toBeFalsy(actual: unknown): MatcherResult {
    const pass = !actual
    const message = pass
        ? `Expected ${formatValue(actual)} to be truthy`
        : `Expected ${formatValue(actual)} to be falsy`

    return { pass, message }
}

// ============================================================================
// Collection Matchers
// ============================================================================

/**
 * Check if array/string contains an item
 */
export function toContain(actual: unknown, item: unknown): MatcherResult {
    if (typeof actual === 'string') {
        if (typeof item !== 'string') {
            return {
                pass: false,
                message: `Expected string to contain ${formatValue(item)}, but item is not a string`,
            }
        }
        const pass = actual.includes(item)
        const message = pass
            ? `Expected "${actual}" not to contain "${item}"`
            : `Expected "${actual}" to contain "${item}"`
        return { pass, message }
    }

    if (Array.isArray(actual)) {
        const pass = actual.some((el) => deepEqual(el, item))
        const message = pass
            ? `Expected ${formatValue(actual)} not to contain ${formatValue(item)}`
            : `Expected ${formatValue(actual)} to contain ${formatValue(item)}`
        return { pass, message }
    }

    return {
        pass: false,
        message: `Expected an array or string, but got ${getTypeName(actual)}`,
    }
}

/**
 * Check if array/string/object has specific length
 */
export function toHaveLength(actual: unknown, length: number): MatcherResult {
    if (typeof length !== 'number') {
        return {
            pass: false,
            message: `Expected length to be a number, got ${formatValue(length)}`,
        }
    }

    let actualLength: number | undefined

    if (typeof actual === 'string' || Array.isArray(actual)) {
        actualLength = actual.length
    } else if (actual && typeof actual === 'object') {
        actualLength = Object.keys(actual).length
    }

    if (actualLength === undefined) {
        return {
            pass: false,
            message: `Expected a string, array, or object, but got ${getTypeName(actual)}`,
        }
    }

    const pass = actualLength === length
    const message = pass
        ? `Expected length not to be ${length}`
        : `Expected length to be ${length}, but got ${actualLength}`

    return { pass, message }
}

/**
 * Check if string matches a pattern
 */
export function toMatch(actual: unknown, pattern: RegExp | string): MatcherResult {
    if (typeof actual !== 'string') {
        return {
            pass: false,
            message: `Expected a string, but got ${getTypeName(actual)}`,
        }
    }

    const regex = pattern instanceof RegExp ? pattern : new RegExp(pattern)
    const pass = regex.test(actual)
    const message = pass
        ? `Expected "${actual}" not to match ${regex}`
        : `Expected "${actual}" to match ${regex}`

    return { pass, message }
}

// ============================================================================
// Numeric Matchers
// ============================================================================

/**
 * Check if actual is greater than value
 */
export function toBeGreaterThan(actual: unknown, value: number): MatcherResult {
    if (typeof actual !== 'number') {
        return {
            pass: false,
            message: `Expected a number, but got ${getTypeName(actual)}`,
        }
    }

    if (typeof value !== 'number') {
        return {
            pass: false,
            message: `Expected comparison value to be a number, got ${getTypeName(value)}`,
        }
    }

    const pass = actual > value
    const message = pass
        ? `Expected ${actual} not to be greater than ${value}`
        : `Expected ${actual} to be greater than ${value}`

    return { pass, message }
}

/**
 * Check if actual is greater than or equal to value
 */
export function toBeGreaterThanOrEqual(actual: unknown, value: number): MatcherResult {
    if (typeof actual !== 'number') {
        return {
            pass: false,
            message: `Expected a number, but got ${getTypeName(actual)}`,
        }
    }

    if (typeof value !== 'number') {
        return {
            pass: false,
            message: `Expected comparison value to be a number, got ${getTypeName(value)}`,
        }
    }

    const pass = actual >= value
    const message = pass
        ? `Expected ${actual} not to be greater than or equal to ${value}`
        : `Expected ${actual} to be greater than or equal to ${value}`

    return { pass, message }
}

/**
 * Check if actual is less than value
 */
export function toBeLessThan(actual: unknown, value: number): MatcherResult {
    if (typeof actual !== 'number') {
        return {
            pass: false,
            message: `Expected a number, but got ${getTypeName(actual)}`,
        }
    }

    if (typeof value !== 'number') {
        return {
            pass: false,
            message: `Expected comparison value to be a number, got ${getTypeName(value)}`,
        }
    }

    const pass = actual < value
    const message = pass
        ? `Expected ${actual} not to be less than ${value}`
        : `Expected ${actual} to be less than ${value}`

    return { pass, message }
}

/**
 * Check if actual is less than or equal to value
 */
export function toBeLessThanOrEqual(actual: unknown, value: number): MatcherResult {
    if (typeof actual !== 'number') {
        return {
            pass: false,
            message: `Expected a number, but got ${getTypeName(actual)}`,
        }
    }

    if (typeof value !== 'number') {
        return {
            pass: false,
            message: `Expected comparison value to be a number, got ${getTypeName(value)}`,
        }
    }

    const pass = actual <= value
    const message = pass
        ? `Expected ${actual} not to be less than or equal to ${value}`
        : `Expected ${actual} to be less than or equal to ${value}`

    return { pass, message }
}

// ============================================================================
// Object Matchers
// ============================================================================

/**
 * Check if object has a property, optionally with a specific value
 */
export function toHaveProperty(
    actual: unknown,
    path: string,
    expectedValue?: unknown,
): MatcherResult {
    if (actual === null || actual === undefined || typeof actual !== 'object') {
        return {
            pass: false,
            message: `Expected an object, but got ${getTypeName(actual)}`,
        }
    }

    if (typeof path !== 'string') {
        return {
            pass: false,
            message: `Expected path to be a string, got ${getTypeName(path)}`,
        }
    }

    const value = getByPath(actual, path)
    const hasProperty = value !== undefined

    // Check property existence only
    if (arguments.length === 2) {
        const pass = hasProperty
        const message = pass
            ? `Expected object not to have property "${path}"`
            : `Expected object to have property "${path}"`
        return { pass, message }
    }

    // Check property value
    if (!hasProperty) {
        return {
            pass: false,
            message: `Expected object to have property "${path}"`,
        }
    }

    const pass = deepEqual(value, expectedValue)
    const message = pass
        ? `Expected property "${path}" not to be ${formatValue(expectedValue)}`
        : `Expected property "${path}" to be ${formatValue(expectedValue)}, but got ${formatValue(value)}`

    return { pass, message }
}

/**
 * Check if value is of a specific type
 */
export function toBeType(actual: unknown, expectedType: string): MatcherResult {
    const validTypes = [
        'string',
        'number',
        'boolean',
        'object',
        'array',
        'function',
        'undefined',
        'null',
        'symbol',
        'bigint',
    ]

    if (!validTypes.includes(expectedType)) {
        return {
            pass: false,
            message: `Invalid type "${expectedType}". Valid types: ${validTypes.join(', ')}`,
        }
    }

    let actualType: string

    if (actual === null) {
        actualType = 'null'
    } else if (Array.isArray(actual)) {
        actualType = 'array'
    } else {
        actualType = typeof actual
    }

    const pass = actualType === expectedType
    const message = pass
        ? `Expected ${formatValue(actual)} not to be of type "${expectedType}"`
        : `Expected ${formatValue(actual)} to be of type "${expectedType}", but got "${actualType}"`

    return { pass, message }
}

// ============================================================================
// Matcher Registry
// ============================================================================

/**
 * Registry of all available matchers
 */
export const matchers = {
    toBe,
    toEqual,
    toBeOneOf,
    toBeDefined,
    toBeUndefined,
    toBeNull,
    toBeNotNull,
    toBeTruthy,
    toBeFalsy,
    toContain,
    toHaveLength,
    toMatch,
    toBeGreaterThan,
    toBeGreaterThanOrEqual,
    toBeLessThan,
    toBeLessThanOrEqual,
    toHaveProperty,
    toBeType,
} as const

export type MatcherName = keyof typeof matchers
