/**
 * E2E Test Runner - Run Command
 *
 * Execute E2E tests with filtering and reporting
 */

import { createAdapterRegistry, getRequiredAdapters } from '../adapters'
import {
    createOrchestrator,
    discoverTests,
    filterTestsByGrep,
    filterTestsByPatterns,
    filterTestsByPriority,
    filterTestsByTags,
    loadConfig,
    loadTSTest,
    loadYAMLTest,
    mergeConfigWithOptions,
    validateAdapterConnectionStrings,
} from '../core'
import { isE2ERunnerError, wrapError } from '../errors'
import { createReporterManager } from '../reporters'
import type {
    CLIArgs,
    DiscoveredTest,
    TestPriority,
    TestSuiteResult,
    UnifiedTestDefinition,
} from '../types'
import { errorCodeToExitCode, EXIT_CODES } from '../utils/exit-codes'
import { createLogger, type LogLevel } from '../utils/logger'
import { printError } from './index'

// ============================================================================
// Types
// ============================================================================

export interface RunCommandResult {
    exitCode: number
    result?: TestSuiteResult
}

// ============================================================================
// Run Command
// ============================================================================

/**
 * Execute the run command
 */
export async function runCommand(args: CLIArgs): Promise<RunCommandResult> {
    const { options, patterns } = args

    // Determine log level based on options
    const logLevel: LogLevel = options.quiet
        ? 'error'
        : options.debug
          ? 'debug'
          : options.verbose
            ? 'info'
            : 'info'

    const logger = createLogger({
        level: logLevel,
        useColors: !options.noColor,
        timestamp: options.debug,
    })

    logger.info('Starting E2E test run')

    try {
        // 1. Load configuration
        logger.debug(`Loading config from: ${options.config}`)
        const baseConfig = await loadConfig({
            configPath: options.config,
            environment: options.env,
        })

        // 2. Merge CLI options with config
        const config = mergeConfigWithOptions(baseConfig, {
            timeout: options.timeout,
            retries: options.retries,
            parallel: options.parallel,
            reporter: options.reporter.length > 0 ? options.reporter : undefined,
        })

        logger.debug(`Using environment: ${config.environmentName}`)

        // 3. Discover tests
        logger.debug('Discovering tests...')
        let tests = await discoverTests({
            basePath: 'tests/e2e',
            patterns: ['**/*.test.yaml', '**/*.test.ts'],
        })

        logger.debug(`Discovered ${tests.length} test(s)`)

        // 4. Apply filters
        tests = await applyFilters(tests, patterns, options, logger)

        if (tests.length === 0) {
            logger.warn('No tests found matching the specified criteria')
            return { exitCode: EXIT_CODES.SUCCESS }
        }

        logger.info(`Running ${tests.length} test(s)`)

        // 5. Load test definitions
        const definitions = await loadTestDefinitions(tests, logger)

        if (definitions.length === 0) {
            logger.warn('No valid test definitions loaded')
            return { exitCode: EXIT_CODES.VALIDATION_ERROR }
        }

        // 6. Dry run mode - just show what would run
        if (options.dryRun) {
            return performDryRun(definitions, logger)
        }

        // 7. Analyze which adapters are required by the tests
        const requiredAdapters = getRequiredAdapters(definitions)
        logger.debug(`Required adapters: ${[...requiredAdapters].join(', ') || 'none'}`)

        // 7.5 Validate that required adapters have resolved connection strings
        validateAdapterConnectionStrings(
            config.environment,
            [...requiredAdapters]
        )

        // 8. Create adapters (only required ones) and connect
        const adapters = createAdapterRegistry(config.environment, logger, { requiredAdapters })

        try {
            logger.debug('Connecting adapters...')
            await adapters.connectAll()
        } catch (error) {
            const e = wrapError(error, 'Failed to connect adapters')
            printError(e.message, e.hint)
            return { exitCode: EXIT_CODES.CONNECTION_ERROR }
        }

        // 9. Create reporters
        const reporterManager = createReporterManager(config.reporters, {
            verbose: options.verbose,
            noColor: options.noColor,
            environmentName: config.environmentName,
        })

        // 10. Create orchestrator and run tests
        const orchestrator = createOrchestrator(config, adapters, logger, {
            parallel: options.parallel,
            timeout: options.timeout,
            retries: options.retries,
            skipSetup: options.skipSetup,
            skipTeardown: options.skipTeardown,
            bail: options.bail,
            dryRun: false,
        })

        // Connect orchestrator events to reporters
        orchestrator.addEventListener((event, data) => {
            switch (event) {
                case 'suite:start': {
                    reporterManager.onSuiteStart(
                        data as Parameters<typeof reporterManager.onSuiteStart>[0],
                    )
                    break
                }
                case 'suite:end': {
                    reporterManager.onSuiteEnd(
                        data as Parameters<typeof reporterManager.onSuiteEnd>[0],
                    )
                    break
                }
                case 'test:start': {
                    reporterManager.onTestStart(
                        data as Parameters<typeof reporterManager.onTestStart>[0],
                    )
                    break
                }
                case 'test:end': {
                    reporterManager.onTestEnd(
                        data as Parameters<typeof reporterManager.onTestEnd>[0],
                    )
                    break
                }
                case 'phase:start': {
                    reporterManager.onPhaseStart(
                        data as Parameters<typeof reporterManager.onPhaseStart>[0],
                    )
                    break
                }
                case 'phase:end': {
                    reporterManager.onPhaseEnd(
                        data as Parameters<typeof reporterManager.onPhaseEnd>[0],
                    )
                    break
                }
                case 'step:start': {
                    reporterManager.onStepStart(
                        data as Parameters<typeof reporterManager.onStepStart>[0],
                    )
                    break
                }
                case 'step:end': {
                    reporterManager.onStepEnd(
                        data as Parameters<typeof reporterManager.onStepEnd>[0],
                    )
                    break
                }
            }
        })

        // 11. Execute tests
        let result: TestSuiteResult
        try {
            // Emit suite start event for reporters
            reporterManager.onSuiteStart({
                suite: { name: 'E2E Tests', tests: definitions, config: config.raw },
                totalTests: definitions.length,
                timestamp: new Date(),
            })

            result = await orchestrator.runSuite(definitions)

            // Emit suite end event
            reporterManager.onSuiteEnd({
                result,
                timestamp: new Date(),
            })
        } finally {
            // Always disconnect adapters
            logger.debug('Disconnecting adapters...')
            await adapters.disconnectAll()
        }

        // 12. Generate reports
        await reporterManager.generateReports(result)

        // 13. Return result
        const exitCode = result.success ? EXIT_CODES.SUCCESS : EXIT_CODES.TEST_FAILURE

        return { exitCode, result }
    } catch (error) {
        if (isE2ERunnerError(error)) {
            printError(error.message, error.hint)
            return { exitCode: errorCodeToExitCode(error.code) }
        }

        const wrapped = wrapError(error, 'Unexpected error during test run')
        printError(wrapped.message)
        return { exitCode: EXIT_CODES.FATAL }
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
        filtered = await filterTestsByTags(filtered, options.tag, async (test) => {
            return await loadTestMetadata(test)
        })
        logger.debug(`After tag filter: ${filtered.length} test(s)`)
    }

    // Filter by priority
    if (options.priority.length > 0) {
        logger.debug(`Filtering by priority: ${options.priority.join(', ')}`)
        filtered = await filterTestsByPriority(filtered, options.priority, async (test) => {
            return await loadTestMetadata(test)
        })
        logger.debug(`After priority filter: ${filtered.length} test(s)`)
    }

    return filtered
}

/**
 * Load test metadata without full definition
 */
async function loadTestMetadata(
    test: DiscoveredTest,
): Promise<{ tags?: string[]; priority?: TestPriority | undefined }> {
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
 * Load full test definitions
 */
async function loadTestDefinitions(
    tests: DiscoveredTest[],
    logger: ReturnType<typeof createLogger>,
): Promise<UnifiedTestDefinition[]> {
    const definitions: UnifiedTestDefinition[] = []
    const errors: string[] = []

    for (const test of tests) {
        try {
            let definition: UnifiedTestDefinition

            definition = await (test.type === 'yaml'
                ? loadYAMLTest(test.filePath, logger)
                : loadTSTest(test.filePath, logger))

            definitions.push(definition)
        } catch (error) {
            const message = error instanceof Error ? error.message : String(error)
            errors.push(`${test.name}: ${message}`)
            logger.warn(`Failed to load test: ${test.name}`)
        }
    }

    if (errors.length > 0) {
        logger.warn(`Failed to load ${errors.length} test(s):`)
        for (const e of errors) logger.warn(`  - ${e}`)
    }

    return definitions
}

/**
 * Perform dry run without executing tests
 */
function performDryRun(
    definitions: UnifiedTestDefinition[],
    logger: ReturnType<typeof createLogger>,
): RunCommandResult {
    logger.info('Dry run mode - showing tests that would be executed:')
    console.log()

    for (const [index, def] of definitions.entries()) {
        const priority = def.priority ? `[${def.priority}]` : ''
        const tags = def.tags?.length ? `tags: ${def.tags.join(', ')}` : ''
        const skip = def.skip ? ' (SKIPPED)' : ''

        console.log(`  ${index + 1}. ${def.name} ${priority}${skip}`)
        if (tags) {
            console.log(`     ${tags}`)
        }
        console.log(`     Source: ${def.sourceFile}`)

        // Show phases
        const phases: string[] = []
        if (def.setup?.length) phases.push(`setup(${def.setup.length})`)
        phases.push(`execute(${def.execute.length})`)
        if (def.verify?.length) phases.push(`verify(${def.verify.length})`)
        if (def.teardown?.length) phases.push(`teardown(${def.teardown.length})`)
        console.log(`     Phases: ${phases.join(' -> ')}`)
        console.log()
    }

    console.log(`Total: ${definitions.length} test(s) would be executed`)

    return { exitCode: EXIT_CODES.SUCCESS }
}

/**
 * Merge loaded config with CLI options (re-export)
 */

export { mergeConfigWithOptions } from '../core'
