/**
 * E2E Test Runner - List Command
 *
 * List discovered tests with filtering options
 */

import * as path from 'node:path'

import {
    discoverTests,
    filterTestsByGrep,
    filterTestsByPatterns,
    filterTestsByPriority,
    filterTestsByTags,
} from '../core'
import { isE2ERunnerError, wrapError } from '../errors'
import type { CLIArgs, DiscoveredTest, TestPriority } from '../types'
import { errorCodeToExitCode, EXIT_CODES } from '../utils/exit-codes'
import { createLogger, type LogLevel } from '../utils/logger'
import { printError } from './index'

// ============================================================================
// Types
// ============================================================================

export interface TestInfo {
    name: string
    file: string
    type: 'yaml' | 'typescript'
    priority?: TestPriority
    tags: string[]
    skip: boolean
    skipReason?: string
}

export interface ListCommandResult {
    exitCode: number
    tests: TestInfo[]
}

// ============================================================================
// ANSI Colors
// ============================================================================

const COLORS = {
    reset: '\u001B[0m',
    bold: '\u001B[1m',
    dim: '\u001B[2m',
    green: '\u001B[32m',
    yellow: '\u001B[33m',
    blue: '\u001B[34m',
    magenta: '\u001B[35m',
    cyan: '\u001B[36m',
    gray: '\u001B[90m',
}

function colorize(text: string, color: keyof typeof COLORS, useColors: boolean): string {
    return useColors ? `${COLORS[color]}${text}${COLORS.reset}` : text
}

// ============================================================================
// List Command
// ============================================================================

/**
 * Execute the list command
 */
export async function listCommand(args: CLIArgs): Promise<ListCommandResult> {
    const { options, patterns } = args

    // Determine log level based on options
    const logLevel: LogLevel = options.quiet ? 'silent' : (options.verbose ? 'debug' : 'info')

    const logger = createLogger({
        level: logLevel,
        useColors: !options.noColor,
    })

    const useColors = !options.noColor && process.stdout.isTTY !== false

    try {
        // 1. Discover tests
        logger.debug('Discovering test files...')
        let tests = await discoverTests({
            basePath: 'tests/e2e',
            patterns: ['**/*.test.yaml', '**/*.test.ts'],
        })

        logger.debug(`Found ${tests.length} test file(s)`)

        // 2. Apply filters
        tests = await applyFilters(tests, patterns, options, logger)

        if (tests.length === 0) {
            if (!options.quiet) {
                console.log('No tests found matching the specified criteria')
            }
            return { exitCode: EXIT_CODES.SUCCESS, tests: [] }
        }

        // 3. Load metadata for all tests
        const testInfos: TestInfo[] = []

        for (const test of tests) {
            const info = await loadTestInfo(test)
            testInfos.push(info)
        }

        // 4. Output results
        if (options.output === 'json') {
            // JSON output mode
            console.log(JSON.stringify(testInfos, null, 2))
        } else {
            // Table output mode
            printTestTable(testInfos, useColors, options.verbose)
        }

        return { exitCode: EXIT_CODES.SUCCESS, tests: testInfos }
    } catch (error) {
        if (isE2ERunnerError(error)) {
            printError(error.message, error.hint)
            return { exitCode: errorCodeToExitCode(error.code), tests: [] }
        }

        const wrapped = wrapError(error, 'Failed to list tests')
        printError(wrapped.message)
        return { exitCode: EXIT_CODES.FATAL, tests: [] }
    }
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Apply all filters to discovered tests
 */
async function applyFilters(
    tests: DiscoveredTest[],
    patterns: string[],
    options: CLIArgs['options'],
    logger: ReturnType<typeof createLogger>,
): Promise<DiscoveredTest[]> {
    let filtered = tests

    // Filter by patterns
    if (patterns.length > 0) {
        logger.debug(`Filtering by patterns: ${patterns.join(', ')}`)
        filtered = await filterTestsByPatterns(filtered, patterns)
        logger.debug(`After pattern filter: ${filtered.length} test(s)`)
    }

    // Filter by grep (name pattern)
    if (options.grep) {
        logger.debug(`Filtering by grep: ${options.grep}`)
        filtered = filterTestsByGrep(filtered, options.grep)
        logger.debug(`After grep filter: ${filtered.length} test(s)`)
    }

    // Filter by tags
    if (options.tag.length > 0) {
        logger.debug(`Filtering by tags: ${options.tag.join(', ')}`)
        filtered = await filterTestsByTags(filtered, options.tag, loadTestMetadata)
        logger.debug(`After tag filter: ${filtered.length} test(s)`)
    }

    // Filter by priority
    if (options.priority.length > 0) {
        logger.debug(`Filtering by priority: ${options.priority.join(', ')}`)
        filtered = await filterTestsByPriority(filtered, options.priority, loadTestMetadata)
        logger.debug(`After priority filter: ${filtered.length} test(s)`)
    }

    return filtered
}

/**
 * Load test metadata without full definition
 */
async function loadTestMetadata(
    test: DiscoveredTest,
): Promise<{ tags?: string[]; priority?: TestPriority }> {
    try {
        if (test.type === 'yaml') {
            const { getYAMLTestMetadata } = await import('../core/yaml-loader')
            return await getYAMLTestMetadata(test.filePath)
        }
        const { getTSTestMetadata } = await import('../core/ts-loader')
        return await getTSTestMetadata(test.filePath)
    } catch {
        return {}
    }
}

/**
 * Load full test info for display
 */
async function loadTestInfo(test: DiscoveredTest): Promise<TestInfo> {
    try {
        if (test.type === 'yaml') {
            const { loadYAMLTest } = await import('../core/yaml-loader')
            const def = await loadYAMLTest(test.filePath)

            return {
                name: def.name,
                file: path.relative(process.cwd(), test.filePath),
                type: 'yaml',
                priority: def.priority,
                tags: def.tags || [],
                skip: def.skip || false,
                skipReason: def.skipReason,
            }
        }
        const { loadTSTest } = await import('../core/ts-loader')
        const def = await loadTSTest(test.filePath)

        return {
            name: def.name,
            file: path.relative(process.cwd(), test.filePath),
            type: 'typescript',
            priority: def.priority,
            tags: def.tags || [],
            skip: def.skip || false,
            skipReason: def.skipReason,
        }
    } catch {
        // If loading fails, return basic info
        return {
            name: test.name,
            file: path.relative(process.cwd(), test.filePath),
            type: test.type,
            tags: [],
            skip: false,
        }
    }
}

// ============================================================================
// Output Functions
// ============================================================================

/**
 * Print tests in a table format
 */
function printTestTable(tests: TestInfo[], useColors: boolean, verbose: boolean): void {
    console.log()
    console.log(colorize('Discovered E2E Tests', 'bold', useColors))
    console.log('='.repeat(80))
    console.log()

    // Group tests by type
    const yamlTests = tests.filter((t) => t.type === 'yaml')
    const tsTests = tests.filter((t) => t.type === 'typescript')

    // Print column headers
    const nameHeader = 'Name'.padEnd(40)
    const typeHeader = 'Type'.padEnd(12)
    const priorityHeader = 'Priority'.padEnd(10)
    const tagsHeader = 'Tags'

    console.log(
        colorize(`  ${nameHeader}${typeHeader}${priorityHeader}${tagsHeader}`, 'dim', useColors),
    )
    console.log(colorize('  ' + '-'.repeat(78), 'dim', useColors))

    // Print each test
    for (const test of tests) {
        printTestRow(test, useColors, verbose)
    }

    console.log()

    // Print summary
    printSummary(yamlTests.length, tsTests.length, tests, useColors)
}

/**
 * Print a single test row
 */
function printTestRow(test: TestInfo, useColors: boolean, verbose: boolean): void {
    let name = test.name

    // Truncate long names
    if (name.length > 38) {
        name = name.slice(0, 35) + '...'
    }
    name = name.padEnd(40)

    // Format type
    const typeLabel =
        test.type === 'yaml'
            ? colorize('YAML', 'cyan', useColors)
            : colorize('TypeScript', 'magenta', useColors)
    const type = typeLabel.padEnd(useColors ? 12 + 9 : 12) // Account for ANSI codes

    // Format priority
    let priority = test.priority || '-'
    if (test.priority) {
        const priorityColor: Record<string, keyof typeof COLORS> = {
            P0: 'bold',
            P1: 'yellow',
            P2: 'dim',
            P3: 'dim',
        }
        priority = colorize(test.priority, priorityColor[test.priority] || 'dim', useColors)
    }
    priority = priority.padEnd(useColors ? 10 + 4 : 10)

    // Format tags
    const tags =
        test.tags.length > 0
            ? test.tags.map((t) => colorize(t, 'blue', useColors)).join(', ')
            : colorize('-', 'dim', useColors)

    // Skip indicator
    const skipIndicator = test.skip ? colorize(' [SKIP]', 'yellow', useColors) : ''

    console.log(`  ${name}${type}${priority}${tags}${skipIndicator}`)

    // Show file path in verbose mode
    if (verbose) {
        console.log(colorize(`    -> ${test.file}`, 'dim', useColors))
        if (test.skip && test.skipReason) {
            console.log(colorize(`       Reason: ${test.skipReason}`, 'yellow', useColors))
        }
    }
}

/**
 * Print summary statistics
 */
function printSummary(
    yamlCount: number,
    tsCount: number,
    tests: TestInfo[],
    useColors: boolean,
): void {
    console.log(colorize('Summary', 'bold', useColors))
    console.log('-'.repeat(40))

    console.log(`  Total tests:      ${tests.length}`)
    console.log(`  YAML tests:       ${yamlCount}`)
    console.log(`  TypeScript tests: ${tsCount}`)

    // Count by priority
    const byPriority = {
        P0: tests.filter((t) => t.priority === 'P0').length,
        P1: tests.filter((t) => t.priority === 'P1').length,
        P2: tests.filter((t) => t.priority === 'P2').length,
        P3: tests.filter((t) => t.priority === 'P3').length,
        none: tests.filter((t) => !t.priority).length,
    }

    console.log()
    console.log(`  By Priority:`)
    console.log(`    P0 (Critical):  ${byPriority.P0}`)
    console.log(`    P1 (High):      ${byPriority.P1}`)
    console.log(`    P2 (Medium):    ${byPriority.P2}`)
    console.log(`    P3 (Low):       ${byPriority.P3}`)
    if (byPriority.none > 0) {
        console.log(`    Unspecified:    ${byPriority.none}`)
    }

    // Count skipped
    const skipped = tests.filter((t) => t.skip).length
    if (skipped > 0) {
        console.log()
        console.log(colorize(`  Skipped:          ${skipped}`, 'yellow', useColors))
    }

    // Collect unique tags
    const allTags = new Set<string>()
    for (const t of tests) for (const tag of t.tags) allTags.add(tag)

    if (allTags.size > 0) {
        console.log()
        console.log(`  Tags:`)
        console.log(`    ${[...allTags].sort().join(', ')}`)
    }
}
