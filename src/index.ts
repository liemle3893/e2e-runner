#!/usr/bin/env node
/**
 * E2E Test Runner - Main Entry Point
 *
 * Command-line interface for running E2E tests
 */

import { parseArgs, printError, printHelp, validateArgs } from './cli'
import { healthCommand } from './cli/health.command'
import { initCommand } from './cli/init.command'
import { listCommand } from './cli/list.command'
import { runCommand } from './cli/run.command'
import { validateCommand } from './cli/validate.command'
import { isE2ERunnerError } from './errors'
import type { CLIArgs, TestSuiteResult } from './types'
import { errorCodeToExitCode, EXIT_CODES } from './utils/exit-codes'

// ============================================================================
// Main Entry Point
// ============================================================================

/**
 * Main function - entry point for CLI
 */
async function main(): Promise<void> {
    let exitCode: number = EXIT_CODES.SUCCESS

    try {
        // Parse command line arguments
        const args = parseArgs()

        // Validate arguments
        const validation = validateArgs(args)
        if (!validation.valid) {
            for (const error of validation.errors) console.error(`Error: ${error}`)
            printHelp()
            process.exit(EXIT_CODES.FATAL)
        }

        // Route to appropriate command
        exitCode = await routeCommand(args)
    } catch (error) {
        if (isE2ERunnerError(error)) {
            printError(error.message, error.hint)
            exitCode = errorCodeToExitCode(error.code)
        } else {
            const message = error instanceof Error ? error.message : String(error)
            printError(`Unexpected error: ${message}`)
            exitCode = EXIT_CODES.FATAL
        }
    }

    process.exit(exitCode)
}

/**
 * Route to the appropriate command handler
 */
async function routeCommand(args: CLIArgs): Promise<number> {
    switch (args.command) {
        case 'run': {
            const result = await runCommand(args)
            return result.exitCode
        }

        case 'validate': {
            const result = await validateCommand(args)
            return result.exitCode
        }

        case 'list': {
            const result = await listCommand(args)
            return result.exitCode
        }

        case 'health': {
            const result = await healthCommand(args)
            return result.exitCode
        }

        case 'init': {
            const result = await initCommand(args)
            return result.exitCode
        }

        default: {
            // Should not reach here due to validation
            printError(`Unknown command: ${args.command}`)
            printHelp()
            return EXIT_CODES.FATAL
        }
    }
}

// ============================================================================
// Programmatic API
// ============================================================================

/**
 * Run E2E tests programmatically
 *
 * @param options - Test execution options
 * @returns Test suite result
 */
export async function runTests(
    options: Partial<CLIArgs['options']> = {},
): Promise<TestSuiteResult | null> {
    const defaultOptions = {
        config: 'e2e.config.yaml',
        env: 'local',
        testDir: '.',
        reportDir: './reports',
        verbose: false,
        quiet: false,
        noColor: true,
        parallel: 1,
        timeout: 30_000,
        retries: 0,
        bail: false,
        watch: false,
        grep: '',
        tag: [],
        priority: [],
        skipSetup: false,
        skipTeardown: false,
        dryRun: false,
        reporter: ['console'],
        output: '',
        debug: false,
        stepByStep: false,
        captureTraffic: false,
        adapter: '',
    }

    const args: CLIArgs = {
        command: 'run',
        patterns: [],
        options: { ...defaultOptions, ...options } as CLIArgs['options'],
    }

    const result = await runCommand(args)
    return result.result || null
}

/**
 * Validate E2E test files programmatically
 *
 * @param options - Validation options
 * @returns Validation result
 */
export async function validateTests(
    options: Partial<CLIArgs['options']> = {},
): Promise<{ valid: boolean; errors: string[] }> {
    const defaultOptions = {
        config: 'e2e.config.yaml',
        env: 'local',
        testDir: '.',
        reportDir: './reports',
        verbose: false,
        quiet: true,
        noColor: true,
        parallel: 1,
        timeout: 30_000,
        retries: 0,
        bail: false,
        watch: false,
        grep: '',
        tag: [],
        priority: [],
        skipSetup: false,
        skipTeardown: false,
        dryRun: false,
        reporter: [],
        output: '',
        debug: false,
        stepByStep: false,
        captureTraffic: false,
        adapter: '',
    }

    const args: CLIArgs = {
        command: 'validate',
        patterns: [],
        options: { ...defaultOptions, ...options } as CLIArgs['options'],
    }

    const result = await validateCommand(args)
    const errors = result.results.filter((r) => !r.valid).flatMap((r) => r.errors)

    return {
        valid: result.summary.invalid === 0,
        errors,
    }
}

/**
 * List E2E tests programmatically
 *
 * @param options - List options
 * @returns Array of test information
 */
export async function listTests(
    options: Partial<CLIArgs['options']> = {},
): Promise<Array<{ name: string; file: string; type: string; tags: string[] }>> {
    const defaultOptions = {
        config: 'e2e.config.yaml',
        env: 'local',
        testDir: '.',
        reportDir: './reports',
        verbose: false,
        quiet: true,
        noColor: true,
        parallel: 1,
        timeout: 30_000,
        retries: 0,
        bail: false,
        watch: false,
        grep: '',
        tag: [],
        priority: [],
        skipSetup: false,
        skipTeardown: false,
        dryRun: false,
        reporter: [],
        output: '',
        debug: false,
        stepByStep: false,
        captureTraffic: false,
        adapter: '',
    }

    const args: CLIArgs = {
        command: 'list',
        patterns: [],
        options: { ...defaultOptions, ...options } as CLIArgs['options'],
    }

    const result = await listCommand(args)
    return result.tests
}

/**
 * Check E2E adapter health programmatically
 *
 * @param options - Health check options
 * @returns Health check results
 */
export async function checkHealth(
    options: Partial<CLIArgs['options']> = {},
): Promise<{ healthy: boolean; results: Record<string, boolean> }> {
    const defaultOptions = {
        config: 'e2e.config.yaml',
        env: 'local',
        testDir: '.',
        reportDir: './reports',
        verbose: false,
        quiet: true,
        noColor: true,
        parallel: 1,
        timeout: 30_000,
        retries: 0,
        bail: false,
        watch: false,
        grep: '',
        tag: [],
        priority: [],
        skipSetup: false,
        skipTeardown: false,
        dryRun: false,
        reporter: [],
        output: '',
        debug: false,
        stepByStep: false,
        captureTraffic: false,
        adapter: '',
    }

    const args: CLIArgs = {
        command: 'health',
        patterns: [],
        options: { ...defaultOptions, ...options } as CLIArgs['options'],
    }

    const result = await healthCommand(args)
    const results: Record<string, boolean> = {}

    for (const r of result.results) {
        results[r.adapter] = r.healthy
    }

    return {
        healthy: result.allHealthy,
        results,
    }
}

// ============================================================================
// Re-exports for programmatic usage
// ============================================================================

// Core functionality
export { loadConfig, mergeConfigWithOptions } from './core'
export { discoverTests } from './core'
export { loadTSTest, loadYAMLTest } from './core'
export { createAndInitializeRunner, createOrchestrator } from './core'

// Adapters
export { createAdapterRegistry } from './adapters'

// Reporters
export { createReporter, createReporterManager } from './reporters'

// Types
export type {
    CLIArgs,
    CLICommand,
    CLIOptions,
    E2EConfig,
    LoadedConfig,
    TestExecutionResult,
    TestSuiteResult,
    UnifiedTestDefinition,
} from './types'

// Exit codes
export { EXIT_CODES } from './utils/exit-codes'

// ============================================================================
// Run Main
// ============================================================================

// Only run main when executed directly
if (require.main === module) {
    main().catch((error) => {
        console.error('Fatal error:', error)
        process.exit(EXIT_CODES.FATAL)
    })
}
