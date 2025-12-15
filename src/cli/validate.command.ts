/**
 * E2E Test Runner - Validate Command
 *
 * Validates test files without executing them
 */

import { discoverTests, filterTestsByPatterns, validateYAMLWithSchema } from '../core'
import { loadConfig } from '../core/config-loader'
import { isValidTSTestFile } from '../core/ts-loader'
import { isE2ERunnerError, wrapError } from '../errors'
import type { CLIArgs, DiscoveredTest } from '../types'
import { errorCodeToExitCode, EXIT_CODES } from '../utils/exit-codes'
import { createLogger, type LogLevel } from '../utils/logger'
import { printError } from './index'

// ============================================================================
// Types
// ============================================================================

export interface ValidationResult {
    file: string
    type: 'yaml' | 'typescript' | 'config'
    valid: boolean
    errors: string[]
}

export interface ValidateCommandResult {
    exitCode: number
    results: ValidationResult[]
    summary: {
        total: number
        valid: number
        invalid: number
    }
}

// ============================================================================
// ANSI Colors
// ============================================================================

const COLORS = {
    reset: '\u001B[0m',
    green: '\u001B[32m',
    red: '\u001B[31m',
    yellow: '\u001B[33m',
    dim: '\u001B[2m',
    bold: '\u001B[1m',
}

function colorize(text: string, color: keyof typeof COLORS, useColors: boolean): string {
    return useColors ? `${COLORS[color]}${text}${COLORS.reset}` : text
}

// ============================================================================
// Validate Command
// ============================================================================

/**
 * Execute the validate command
 */
export async function validateCommand(args: CLIArgs): Promise<ValidateCommandResult> {
    const { options, patterns } = args

    // Determine log level based on options
    const logLevel: LogLevel = options.quiet ? 'error' : (options.verbose ? 'debug' : 'info')

    const logger = createLogger({
        level: logLevel,
        useColors: !options.noColor,
    })

    const useColors = !options.noColor && process.stdout.isTTY !== false

    logger.info('Validating E2E test files')

    const results: ValidationResult[] = []

    try {
        // 1. Validate configuration file
        logger.debug(`Validating config: ${options.config}`)
        const configResult = await validateConfigFile(options.config, options.env)
        results.push(configResult)

        if (configResult.valid) {
            printValidationResult(configResult, useColors, options.verbose)
        } else {
            printValidationResult(configResult, useColors, options.verbose)
            // Continue validation even if config has issues
        }

        // 2. Discover test files
        logger.debug('Discovering test files...')
        let tests = await discoverTests({
            basePath: 'tests/e2e',
            patterns: ['**/*.test.yaml', '**/*.test.ts'],
        })

        // Apply pattern filter if specified
        if (patterns.length > 0) {
            tests = await filterTestsByPatterns(tests, patterns)
        }

        logger.debug(`Found ${tests.length} test file(s) to validate`)

        // 3. Validate each test file
        for (const test of tests) {
            const result = await validateTestFile(test)
            results.push(result)
            printValidationResult(result, useColors, options.verbose)
        }

        // 4. Print summary
        const summary = calculateSummary(results)
        printSummary(summary, useColors)

        const exitCode = summary.invalid === 0 ? EXIT_CODES.SUCCESS : EXIT_CODES.VALIDATION_ERROR

        return { exitCode, results, summary }
    } catch (error) {
        if (isE2ERunnerError(error)) {
            printError(error.message, error.hint)
            return {
                exitCode: errorCodeToExitCode(error.code),
                results,
                summary: calculateSummary(results),
            }
        }

        const wrapped = wrapError(error, 'Validation failed')
        printError(wrapped.message)
        return {
            exitCode: EXIT_CODES.FATAL,
            results,
            summary: calculateSummary(results),
        }
    }
}

// ============================================================================
// Validation Functions
// ============================================================================

/**
 * Validate the configuration file
 */
async function validateConfigFile(
    configPath: string,
    environment: string,
): Promise<ValidationResult> {
    const errors: string[] = []

    try {
        await loadConfig({
            configPath,
            environment,
        })
    } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        errors.push(message)
    }

    return {
        file: configPath,
        type: 'config',
        valid: errors.length === 0,
        errors,
    }
}

/**
 * Validate a single test file
 */
async function validateTestFile(test: DiscoveredTest): Promise<ValidationResult> {
    if (test.type === 'yaml') {
        return validateYAMLTestFile(test)
    }
    return validateTSTestFile(test)
}

/**
 * Validate a YAML test file
 */
async function validateYAMLTestFile(test: DiscoveredTest): Promise<ValidationResult> {
    const errors: string[] = []

    try {
        // Validate against JSON schema
        const schemaResult = await validateYAMLWithSchema(test.filePath)
        if (!schemaResult.valid) {
            errors.push(...schemaResult.errors)
        }

        // Try to load the test to check for structural issues
        const { loadYAMLTest } = await import('../core/yaml-loader')
        await loadYAMLTest(test.filePath)
    } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        // Avoid duplicate error messages
        if (!errors.some((e) => e.includes(message))) {
            errors.push(message)
        }
    }

    return {
        file: test.filePath,
        type: 'yaml',
        valid: errors.length === 0,
        errors,
    }
}

/**
 * Validate a TypeScript test file
 */
async function validateTSTestFile(test: DiscoveredTest): Promise<ValidationResult> {
    const errors: string[] = []

    try {
        // Quick syntax check
        if (!isValidTSTestFile(test.filePath)) {
            errors.push(
                'File does not appear to be a valid E2E test file (missing export default or execute)',
            )
        }

        // Try to load the test to check for TypeScript/import errors
        const { loadTSTest } = await import('../core/ts-loader')
        await loadTSTest(test.filePath)
    } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        errors.push(message)
    }

    return {
        file: test.filePath,
        type: 'typescript',
        valid: errors.length === 0,
        errors,
    }
}

// ============================================================================
// Output Functions
// ============================================================================

/**
 * Print a single validation result
 */
function printValidationResult(
    result: ValidationResult,
    useColors: boolean,
    verbose: boolean,
): void {
    const symbol = result.valid
        ? colorize('\u2713', 'green', useColors)
        : colorize('\u2717', 'red', useColors)

    const status = result.valid
        ? colorize('VALID', 'green', useColors)
        : colorize('INVALID', 'red', useColors)

    const typeLabel = colorize(`[${result.type.toUpperCase()}]`, 'dim', useColors)

    console.log(`${symbol} ${typeLabel} ${result.file} - ${status}`)

    // Show errors for invalid files
    if (!result.valid && result.errors.length > 0) {
        for (const error of result.errors) {
            const errorPrefix = colorize('  -', 'red', useColors)
            console.log(`${errorPrefix} ${error}`)
        }
        console.log()
    } else if (verbose && result.valid) {
        console.log()
    }
}

/**
 * Calculate validation summary
 */
function calculateSummary(results: ValidationResult[]): ValidateCommandResult['summary'] {
    const total = results.length
    const valid = results.filter((r) => r.valid).length
    const invalid = total - valid

    return { total, valid, invalid }
}

/**
 * Print validation summary
 */
function printSummary(summary: ValidateCommandResult['summary'], useColors: boolean): void {
    console.log()
    console.log(colorize('Validation Summary', 'bold', useColors))
    console.log('-'.repeat(40))

    console.log(`Total files:  ${summary.total}`)
    console.log(`Valid:        ${colorize(String(summary.valid), 'green', useColors)}`)
    console.log(
        `Invalid:      ${colorize(String(summary.invalid), summary.invalid > 0 ? 'red' : 'dim', useColors)}`,
    )

    console.log()

    if (summary.invalid === 0) {
        console.log(colorize('\u2713 All files passed validation', 'green', useColors))
    } else {
        console.log(
            colorize(`\u2717 ${summary.invalid} file(s) failed validation`, 'red', useColors),
        )
    }
}
