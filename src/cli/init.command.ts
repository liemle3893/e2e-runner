/**
 * E2E Test Runner - Init Command
 *
 * Initialize E2E test configuration and example files
 */

import * as fs from 'node:fs'
import * as path from 'node:path'

import { createDefaultConfig } from '../core'
import { isE2ERunnerError, wrapError } from '../errors'
import type { CLIArgs } from '../types'
import { errorCodeToExitCode, EXIT_CODES } from '../utils/exit-codes'
import { createLogger, type LogLevel } from '../utils/logger'
import { printError } from './index'
import {
    CONFIG_SCHEMA,
    ENV_EXAMPLE,
    EXAMPLE_TS_TEST,
    EXAMPLE_YAML_TEST,
    TEST_SCHEMA,
} from './init-templates'

// ============================================================================
// Types
// ============================================================================

export interface InitCommandResult {
    exitCode: number
    filesCreated: string[]
    directoriesCreated: string[]
}

// ============================================================================
// ANSI Colors
// ============================================================================

const COLORS = {
    reset: '\u001B[0m',
    bold: '\u001B[1m',
    dim: '\u001B[2m',
    green: '\u001B[32m',
}

const SYMBOLS = { check: '\u2713', plus: '+' }

function colorize(text: string, color: keyof typeof COLORS, useColors: boolean): string {
    return useColors ? `${COLORS[color]}${text}${COLORS.reset}` : text
}

// ============================================================================
// Init Command
// ============================================================================

/**
 * Execute the init command
 */
export async function initCommand(args: CLIArgs): Promise<InitCommandResult> {
    const { options } = args
    const logLevel: LogLevel = options.quiet ? 'silent' : (options.verbose ? 'debug' : 'info')

    createLogger({ level: logLevel, useColors: !options.noColor })

    const useColors = !options.noColor && process.stdout.isTTY !== false
    const filesCreated: string[] = []
    const directoriesCreated: string[] = []

    try {
        console.log()
        console.log(colorize('E2E Test Runner - Project Initialization', 'bold', useColors))
        console.log('='.repeat(50))
        console.log()

        const baseDir = process.cwd()

        // 1. Create directories
        createDirectories(baseDir, directoriesCreated, useColors)

        // 2. Create configuration file
        console.log()
        console.log('Creating configuration files...')
        createFile(
            baseDir,
            'tests/e2e/e2e.config.yaml',
            createDefaultConfig(),
            filesCreated,
            useColors,
        )

        // 3. Create example test files
        createFile(
            baseDir,
            'tests/e2e/examples/TC-EXAMPLE-001.test.yaml',
            EXAMPLE_YAML_TEST,
            filesCreated,
            useColors,
        )
        createFile(
            baseDir,
            'tests/e2e/examples/TC-EXAMPLE-002.test.ts',
            EXAMPLE_TS_TEST,
            filesCreated,
            useColors,
        )

        // 4. Create .env.e2e.example
        createFile(baseDir, '.env.e2e.example', ENV_EXAMPLE, filesCreated, useColors)

        // 5. Create JSON schemas for validation
        createSchemaFiles(baseDir, filesCreated, useColors)

        console.log()

        // 6. Print summary and next steps
        printInitSummary(filesCreated, directoriesCreated, useColors)
        printNextSteps(useColors)

        return { exitCode: EXIT_CODES.SUCCESS, filesCreated, directoriesCreated }
    } catch (error) {
        if (isE2ERunnerError(error)) {
            printError(error.message, error.hint)
            return { exitCode: errorCodeToExitCode(error.code), filesCreated, directoriesCreated }
        }

        const wrapped = wrapError(error, 'Initialization failed')
        printError(wrapped.message)
        return { exitCode: EXIT_CODES.FATAL, filesCreated, directoriesCreated }
    }
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Create required directories
 */
function createDirectories(baseDir: string, created: string[], useColors: boolean): void {
    const directories = [
        'tests/e2e',
        'tests/e2e/schemas',
        'tests/e2e/examples',
        'tests/e2e/reports',
        'tests/e2e/fixtures',
    ]

    console.log('Creating directories...')
    for (const dir of directories) {
        const fullPath = path.join(baseDir, dir)
        if (fs.existsSync(fullPath)) {
            printExists(dir, useColors)
        } else {
            fs.mkdirSync(fullPath, { recursive: true })
            created.push(dir)
            printCreated(dir, useColors)
        }
    }
}

/**
 * Create a file if it doesn't exist
 */
function createFile(
    baseDir: string,
    filePath: string,
    content: string,
    created: string[],
    useColors: boolean,
): void {
    const fullPath = path.join(baseDir, filePath)
    if (fs.existsSync(fullPath)) {
        printExists(filePath, useColors)
    } else {
        fs.writeFileSync(fullPath, content, 'utf8')
        created.push(filePath)
        printCreated(filePath, useColors)
    }
}

/**
 * Create JSON schema files for validation
 */
function createSchemaFiles(baseDir: string, created: string[], useColors: boolean): void {
    const configSchemaPath = 'tests/e2e/schemas/e2e-config.schema.json'
    const configSchemaFullPath = path.join(baseDir, configSchemaPath)

    if (fs.existsSync(configSchemaFullPath)) {
        printExists(configSchemaPath, useColors)
    } else {
        fs.writeFileSync(configSchemaFullPath, JSON.stringify(CONFIG_SCHEMA, null, 2), 'utf8')
        created.push(configSchemaPath)
        printCreated(configSchemaPath, useColors)
    }

    const testSchemaPath = 'tests/e2e/schemas/e2e-test.schema.json'
    const testSchemaFullPath = path.join(baseDir, testSchemaPath)

    if (fs.existsSync(testSchemaFullPath)) {
        printExists(testSchemaPath, useColors)
    } else {
        fs.writeFileSync(testSchemaFullPath, JSON.stringify(TEST_SCHEMA, null, 2), 'utf8')
        created.push(testSchemaPath)
        printCreated(testSchemaPath, useColors)
    }
}

// ============================================================================
// Output Functions
// ============================================================================

function printCreated(filePath: string, useColors: boolean): void {
    const symbol = colorize(SYMBOLS.plus, 'green', useColors)
    const label = colorize('created', 'green', useColors)
    console.log(`  ${symbol} ${label} ${filePath}`)
}

function printExists(filePath: string, useColors: boolean): void {
    const symbol = colorize(SYMBOLS.check, 'dim', useColors)
    const label = colorize('exists', 'dim', useColors)
    console.log(`  ${symbol} ${label}  ${filePath}`)
}

function printInitSummary(filesCreated: string[], dirCreated: string[], useColors: boolean): void {
    console.log(colorize('Summary', 'bold', useColors))
    console.log('-'.repeat(40))
    console.log(`  Directories created: ${dirCreated.length}`)
    console.log(`  Files created:       ${filesCreated.length}`)
}

function printNextSteps(useColors: boolean): void {
    console.log()
    console.log(colorize('Next Steps:', 'bold', useColors))
    console.log('-'.repeat(40))
    console.log()
    console.log('  1. Configure your environment:')
    console.log('     cp .env.e2e.example .env.e2e')
    console.log('     # Edit .env.e2e with your connection strings')
    console.log()
    console.log('  2. Update e2e.config.yaml with your settings')
    console.log()
    console.log('  3. Run the health check:')
    console.log('     yarn e2e health')
    console.log()
    console.log('  4. Run the example tests:')
    console.log('     yarn e2e run --tag example')
    console.log()
    console.log('  5. Create your own tests in tests/e2e/')
    console.log()
    console.log(colorize('Documentation:', 'dim', useColors))
    console.log('  See .claude/docs/testing/test-runner-engine-spec.md')
}
