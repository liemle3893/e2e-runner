/**
 * E2E Test Runner - JSONPath Evaluator
 *
 * Evaluates JSONPath expressions to extract values from objects/arrays.
 * Supports common JSONPath syntax: $, ., [], [*], recursive descent (..)
 */

// ============================================================================
// Types
// ============================================================================

/**
 * Result of a JSONPath evaluation
 */
export interface JSONPathResult {
    value: unknown
    found: boolean
    path: string
}

/**
 * Token types for path parsing
 */
type TokenType = 'root' | 'property' | 'index' | 'wildcard' | 'recursive'

interface PathToken {
    type: TokenType
    value: string | number
}

// ============================================================================
// Core Functions
// ============================================================================

/**
 * Evaluate a JSONPath expression against an object
 *
 * @param obj - The object to query
 * @param path - JSONPath expression (e.g., "$.data.items[0].name")
 * @returns The value at the path or undefined if not found
 *
 * @example
 * const obj = { data: { items: [{ name: 'test' }] } };
 * evaluateJSONPath(obj, '$.data.items[0].name'); // 'test'
 * evaluateJSONPath(obj, '$.data.items[*].name'); // ['test']
 */
export function evaluateJSONPath(obj: unknown, path: string): unknown {
    const result = evaluateJSONPathWithMeta(obj, path)
    return result.found ? result.value : undefined
}

/**
 * Evaluate JSONPath with metadata about the result
 *
 * @param obj - The object to query
 * @param path - JSONPath expression
 * @returns Result with value, found flag, and normalized path
 */
export function evaluateJSONPathWithMeta(obj: unknown, path: string): JSONPathResult {
    if (!path || path.trim() === '') {
        return { value: undefined, found: false, path: '' }
    }

    const normalizedPath = normalizePath(path)
    const tokens = tokenizePath(normalizedPath)

    if (tokens.length === 0) {
        return { value: obj, found: true, path: '$' }
    }

    const result = evaluateTokens(obj, tokens)

    return {
        value: result.value,
        found: result.found,
        path: normalizedPath,
    }
}

/**
 * Check if a value exists at the given path
 *
 * @param obj - The object to query
 * @param path - JSONPath expression
 * @returns true if a value exists at the path
 */
export function hasJSONPath(obj: unknown, path: string): boolean {
    const result = evaluateJSONPathWithMeta(obj, path)
    return result.found
}

/**
 * Get all values matching a JSONPath with wildcards
 *
 * @param obj - The object to query
 * @param path - JSONPath expression with wildcards
 * @returns Array of all matching values
 */
export function queryJSONPath(obj: unknown, path: string): unknown[] {
    const normalizedPath = normalizePath(path)
    const tokens = tokenizePath(normalizedPath)

    if (tokens.length === 0) {
        return obj === undefined ? [] : [obj]
    }

    return queryTokens(obj, tokens)
}

// ============================================================================
// Path Parsing
// ============================================================================

/**
 * Normalize a JSONPath expression
 * Handles both dot notation and bracket notation
 */
function normalizePath(path: string): string {
    let normalized = path.trim()

    // Ensure path starts with $ or is treated as relative to root
    if (!normalized.startsWith('$')) {
        normalized = '$.' + normalized
    }

    return normalized
}

/**
 * Tokenize a JSONPath expression into segments
 */
// eslint-disable-next-line sonarjs/cognitive-complexity -- JSONPath parsing requires handling multiple syntax cases
function tokenizePath(path: string): PathToken[] {
    const tokens: PathToken[] = []

    // Remove leading $
    let remaining = path.startsWith('$') ? path.slice(1) : path

    while (remaining.length > 0) {
        // Handle recursive descent (..)
        if (remaining.startsWith('..')) {
            remaining = remaining.slice(2)
            const { property, rest } = extractNextProperty(remaining)
            if (property !== undefined) {
                tokens.push({ type: 'recursive', value: property })
                remaining = rest
            }
            continue
        }

        // Handle dot notation
        if (remaining.startsWith('.')) {
            remaining = remaining.slice(1)
            const { property, rest } = extractNextProperty(remaining)
            if (property !== undefined) {
                tokens.push({ type: 'property', value: property })
                remaining = rest
            }
            continue
        }

        // Handle bracket notation
        if (remaining.startsWith('[')) {
            const closeIdx = findMatchingBracket(remaining)
            if (closeIdx === -1) {
                break // Invalid path
            }

            const bracketContent = remaining.slice(1, closeIdx)
            remaining = remaining.slice(closeIdx + 1)

            // Wildcard [*]
            if (bracketContent === '*') {
                tokens.push({ type: 'wildcard', value: '*' })
                continue
            }

            // Index [0]
            const indexRegex = /^(\d+)$/
            const indexMatch = indexRegex.exec(bracketContent)
            if (indexMatch) {
                tokens.push({ type: 'index', value: Number.parseInt(indexMatch[1], 10) })
                continue
            }

            // Property ['name'] or ["name"]
            const propRegex = /^['"](.+?)['"]$/
            const propMatch = propRegex.exec(bracketContent)
            if (propMatch) {
                tokens.push({ type: 'property', value: propMatch[1] })
                continue
            }

            // Bare property name in brackets
            tokens.push({ type: 'property', value: bracketContent })
            continue
        }

        // Skip unrecognized characters
        remaining = remaining.slice(1)
    }

    return tokens
}

/**
 * Extract the next property name from a path segment
 */
function extractNextProperty(path: string): { property: string | undefined; rest: string } {
    // Handle wildcard
    if (path.startsWith('*')) {
        return { property: '*', rest: path.slice(1) }
    }

    // Match property name (alphanumeric, underscore, hyphen)
    const propertyRegex = /^([a-zA-Z_$][a-zA-Z0-9_$-]*)/
    const match = propertyRegex.exec(path)
    if (match) {
        return { property: match[1], rest: path.slice(match[1].length) }
    }

    return { property: undefined, rest: path }
}

/**
 * Find the matching closing bracket
 */
// eslint-disable-next-line sonarjs/cognitive-complexity -- Bracket matching with string handling requires nested conditions
function findMatchingBracket(str: string): number {
    if (str[0] !== '[') return -1

    let depth = 0
    let inString = false
    let stringChar = ''

    for (let i = 0; i < str.length; i++) {
        const char = str[i]

        if (inString) {
            if (char === stringChar && str[i - 1] !== '\\') {
                inString = false
            }
            continue
        }

        if (char === '"' || char === "'") {
            inString = true
            stringChar = char
            continue
        }

        if (char === '[') {
            depth++
        } else if (char === ']') {
            depth--
            if (depth === 0) {
                return i
            }
        }
    }

    return -1
}

// ============================================================================
// Token Evaluation
// ============================================================================

interface EvaluationResult {
    value: unknown
    found: boolean
}

/**
 * Evaluate tokens against an object
 */
// eslint-disable-next-line sonarjs/cognitive-complexity -- Token evaluation must handle multiple token types with different logic
function evaluateTokens(obj: unknown, tokens: PathToken[]): EvaluationResult {
    let current: unknown = obj

    for (let i = 0; i < tokens.length; i++) {
        const token = tokens[i]

        if (current === undefined || current === null) {
            return { value: undefined, found: false }
        }

        switch (token.type) {
            case 'property': {
                if (typeof current !== 'object') {
                    return { value: undefined, found: false }
                }
                const propValue = (current as Record<string, unknown>)[token.value as string]
                if (
                    propValue === undefined &&
                    !(token.value in (current as Record<string, unknown>))
                ) {
                    return { value: undefined, found: false }
                }
                current = propValue
                break
            }

            case 'index': {
                if (!Array.isArray(current)) {
                    return { value: undefined, found: false }
                }
                const idx = token.value as number
                if (idx < 0 || idx >= current.length) {
                    return { value: undefined, found: false }
                }
                current = current[idx]
                break
            }

            case 'wildcard': {
                // Wildcard returns first element for single value evaluation
                // Use queryTokens for all matches
                if (Array.isArray(current)) {
                    if (current.length === 0) {
                        return { value: undefined, found: false }
                    }
                    // Return array of all values with remaining tokens applied
                    const remainingTokens = tokens.slice(i + 1)
                    if (remainingTokens.length === 0) {
                        return { value: current, found: true }
                    }
                    const results = current
                        .map((item) => evaluateTokens(item, remainingTokens))
                        .filter((r) => r.found)
                        .map((r) => r.value)
                    return { value: results, found: results.length > 0 }
                }
                // eslint-disable-next-line sonarjs/different-types-comparison -- typeof null === 'object' in JS
                if (typeof current === 'object' && current !== null) {
                    const values = Object.values(current as Record<string, unknown>)
                    if (values.length === 0) {
                        return { value: undefined, found: false }
                    }
                    const remainingTokens = tokens.slice(i + 1)
                    if (remainingTokens.length === 0) {
                        return { value: values, found: true }
                    }
                    const results = values
                        .map((item) => evaluateTokens(item, remainingTokens))
                        .filter((r) => r.found)
                        .map((r) => r.value)
                    return { value: results, found: results.length > 0 }
                }
                return { value: undefined, found: false }
            }

            case 'recursive': {
                const propName = token.value as string
                const results = recursiveSearch(current, propName)
                const remainingTokens = tokens.slice(i + 1)
                if (remainingTokens.length === 0) {
                    return { value: results, found: results.length > 0 }
                }
                const finalResults = results
                    .map((item) => evaluateTokens(item, remainingTokens))
                    .filter((r) => r.found)
                    .map((r) => r.value)
                return { value: finalResults, found: finalResults.length > 0 }
            }
        }
    }

    return { value: current, found: true }
}

/**
 * Query all values matching tokens (supports wildcards)
 */
// eslint-disable-next-line sonarjs/cognitive-complexity -- Query logic handles multiple token types
function queryTokens(obj: unknown, tokens: PathToken[]): unknown[] {
    let current: unknown[] = [obj]

    for (const token of tokens) {
        const next: unknown[] = []

        for (const item of current) {
            if (item === undefined || item === null) {
                continue
            }

            switch (token.type) {
                case 'property': {
                    if (typeof item === 'object') {
                        const propValue = (item as Record<string, unknown>)[token.value as string]
                        if (propValue !== undefined) {
                            next.push(propValue)
                        }
                    }
                    break
                }

                case 'index': {
                    if (Array.isArray(item)) {
                        const idx = token.value as number
                        if (idx >= 0 && idx < item.length) {
                            next.push(item[idx])
                        }
                    }
                    break
                }

                case 'wildcard': {
                    if (Array.isArray(item)) {
                        next.push(...item)
                    } else if (typeof item === 'object') {
                        next.push(...Object.values(item as Record<string, unknown>))
                    }
                    break
                }

                case 'recursive': {
                    const propName = token.value as string
                    const results = recursiveSearch(item, propName)
                    next.push(...results)
                    break
                }
            }
        }

        current = next
    }

    return current
}

/**
 * Recursively search for a property in an object tree
 */
function recursiveSearch(obj: unknown, propertyName: string): unknown[] {
    const results: unknown[] = []

    function search(current: unknown): void {
        if (current === null || current === undefined) {
            return
        }

        if (typeof current !== 'object') {
            return
        }

        if (Array.isArray(current)) {
            for (const item of current) {
                search(item)
            }
            return
        }

        const record = current as Record<string, unknown>

        // Check if current object has the property
        if (propertyName in record) {
            results.push(record[propertyName])
        }

        // Recursively search child objects
        for (const value of Object.values(record)) {
            search(value)
        }
    }

    search(obj)
    return results
}

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Parse a path string into a simple property path array
 * This is a simplified helper for common cases
 *
 * @example
 * parseSimplePath('data.items.0.name') // ['data', 'items', '0', 'name']
 */
// eslint-disable-next-line sonarjs/cognitive-complexity -- Path parsing requires handling brackets and dots
export function parseSimplePath(path: string): string[] {
    // Remove leading $ and .
    const normalized = path.replace(/^\$\.?/, '')

    // Split on . but not inside brackets
    const parts: string[] = []
    let current = ''
    let inBracket = false

    for (const char of normalized) {
        if (char === '[') {
            if (current) {
                parts.push(current)
                current = ''
            }
            inBracket = true
            continue
        }
        if (char === ']') {
            if (current) {
                // Remove quotes from bracket content
                // eslint-disable-next-line unicorn/prefer-string-replace-all -- replaceAll requires ES2021
                parts.push(current.replace(/(?:^['"])|(?:['"]$)/g, ''))
                current = ''
            }
            inBracket = false
            continue
        }
        if (char === '.' && !inBracket) {
            if (current) {
                parts.push(current)
                current = ''
            }
            continue
        }
        current += char
    }

    if (current) {
        parts.push(current)
    }

    return parts
}

/**
 * Get a value from an object using a simple dot-notation path
 * More performant than full JSONPath for simple cases
 *
 * @example
 * getByPath({ a: { b: { c: 1 } } }, 'a.b.c') // 1
 */
export function getByPath(obj: unknown, path: string): unknown {
    const parts = parseSimplePath(path)
    let current: unknown = obj

    for (const part of parts) {
        if (current === null || current === undefined) {
            return undefined
        }

        if (typeof current !== 'object') {
            return undefined
        }

        if (Array.isArray(current)) {
            const index = Number.parseInt(part, 10)
            if (Number.isNaN(index)) {
                return undefined
            }
            current = current[index]
        } else {
            current = (current as Record<string, unknown>)[part]
        }
    }

    return current
}

/**
 * Check if a path is a valid JSONPath expression
 */
export function isValidJSONPath(path: string): boolean {
    if (!path || typeof path !== 'string') {
        return false
    }

    // Must start with $ or be a relative path
    const normalized = path.trim()

    // Basic validation - check for obvious syntax errors
    let bracketDepth = 0
    for (const char of normalized) {
        if (char === '[') bracketDepth++
        if (char === ']') bracketDepth--
        if (bracketDepth < 0) return false
    }

    return bracketDepth === 0
}
