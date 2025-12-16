/**
 * E2E Test Runner - CLI Parser
 *
 * Parses command line arguments and routes to appropriate commands
 */

import type { CLIArgs, CLICommand, CLIOptions, TestPriority } from '../types'
import { DEFAULT_CLI_OPTIONS } from '../types'

// ============================================================================
// Constants
// ============================================================================

const VERSION = '1.0.0'

const VALID_COMMANDS: CLICommand[] = ['run', 'validate', 'list', 'health', 'init', 'test']

const HELP_TEXT = `E2E Test Runner - End-to-End Testing Framework

USAGE: e2e <command> [options] [patterns...]

COMMANDS:
  run         Run E2E tests (default)    validate    Validate test files
  list        List discovered tests      health      Check adapter connectivity
  init        Initialize config files    test        Create test files

TEST SUBCOMMANDS:
  test create <name>                Create new test from template
    --template, -t <type>           Template: api|crud|integration|event-driven|db-verification
    --description, -D <text>        Test description
    --output, -o <path>             Output directory (default: testDir from config)
    --priority <level>              Priority: P0|P1|P2|P3 (default: P0)
    --tags <tags>                   Comma-separated tags (default: e2e)
  test list-templates               List available templates

OPTIONS:
  -c, --config <path>    Config file path (default: e2e.config.yaml)
  -e, --env <name>       Environment (default: local)
  -d, --test-dir <path>  Test directory (default: current directory)
  --report-dir <path>    Report output directory (default: ./reports)
  -v, --verbose          Verbose output          -q, --quiet    Errors only
  --no-color             Disable colors          -h, --help     Show help

Run Command:
  -p, --parallel <n>     Parallel tests (default: 1)
  -t, --timeout <ms>     Timeout (default: 30000)
  -r, --retries <n>      Retries (default: 0)
  --bail                 Stop on first failure   --watch        Watch mode
  -g, --grep <pattern>   Filter by name          --tag <tag>    Filter by tag
  --priority <level>     Filter by P0/P1/P2/P3   --dry-run      Show only
  --skip-setup           Skip setup              --skip-teardown  Skip teardown

Report: --reporter <type>  console|junit|html|json   -o, --output <path>
Debug:  --debug  --step-by-step  --capture-traffic
Health: --adapter <type>  Check specific adapter

EXAMPLES:
  e2e run                           e2e run --tag smoke
  e2e run -d ./tests                e2e validate
  e2e list --tag integration        e2e health
  e2e init

ENV VARS: E2E_CONFIG, E2E_ENV, E2E_TEST_DIR, E2E_REPORT_DIR, E2E_VERBOSE, NO_COLOR
`

// ============================================================================
// Argument Parsing
// ============================================================================

/**
 * Parse command line arguments
 */
export function parseArgs(argv: string[] = process.argv.slice(2)): CLIArgs {
    const args: string[] = [...argv]
    const patterns: string[] = []

    // Check for help/version first
    if (args.includes('-h') || args.includes('--help')) {
        printHelp()
        process.exit(0)
    }

    if (args.includes('--version')) {
        printVersion()
        process.exit(0)
    }

    // Extract command (first non-option argument)
    let command: CLICommand = 'run'
    const firstArg = args[0]

    if (firstArg && !firstArg.startsWith('-') && isValidCommand(firstArg)) {
        command = firstArg as CLICommand
        args.shift()
    }

    // Initialize options with defaults
    const options: CLIOptions = {
        ...DEFAULT_CLI_OPTIONS,
        config: process.env.E2E_CONFIG || DEFAULT_CLI_OPTIONS.config!,
        env: process.env.E2E_ENV || DEFAULT_CLI_OPTIONS.env!,
        testDir: process.env.E2E_TEST_DIR || DEFAULT_CLI_OPTIONS.testDir!,
        reportDir: process.env.E2E_REPORT_DIR || DEFAULT_CLI_OPTIONS.reportDir!,
        verbose: process.env.E2E_VERBOSE === '1' || process.env.E2E_VERBOSE === 'true',
        noColor: process.env.NO_COLOR === '1' || process.env.NO_COLOR === 'true',
        tag: [],
        priority: [],
        reporter: [],
    } as CLIOptions

    // Parse remaining arguments
    let i = 0
    while (i < args.length) {
        const arg = args[i]

        // Handle options
        if (arg.startsWith('-')) {
            const { consumed, key, value } = parseOption(arg, args.slice(i + 1))
            i += consumed

            if (key) {
                applyOption(options, key, value)
            }
        } else {
            // Non-option argument is a pattern
            patterns.push(arg)
        }

        i++
    }

    return { command, patterns, options }
}

/**
 * Check if a string is a valid command
 */
function isValidCommand(str: string): str is CLICommand {
    return VALID_COMMANDS.includes(str as CLICommand)
}

/**
 * Parse a single option and return consumed argument count
 */
function parseOption(
    arg: string,
    remaining: string[],
): { consumed: number; key: string | null; value: string | boolean } {
    // Handle long options with =
    if (arg.includes('=')) {
        const [key, ...valueParts] = arg.split('=')
        const value = valueParts.join('=')
        return { consumed: 0, key: normalizeLongOption(key), value }
    }

    // Handle short options
    if (arg.startsWith('-') && !arg.startsWith('--')) {
        return parseShortOption(arg, remaining)
    }

    // Handle long options
    return parseLongOption(arg, remaining)
}

/**
 * Parse short option (e.g., -v, -c <value>)
 */
function parseShortOption(
    arg: string,
    remaining: string[],
): { consumed: number; key: string | null; value: string | boolean } {
    const shortMap: Record<string, string> = {
        '-c': 'config',
        '-e': 'env',
        '-d': 'testDir',
        '-v': 'verbose',
        '-q': 'quiet',
        '-p': 'parallel',
        '-t': 'timeout',
        '-r': 'retries',
        '-g': 'grep',
        '-o': 'output',
        '-h': 'help',
        '-D': 'testDescription',
        '-T': 'testTemplate',
    }

    const key = shortMap[arg]
    if (!key) {
        return { consumed: 0, key: null, value: false }
    }

    // Boolean options
    if (['verbose', 'quiet', 'help'].includes(key)) {
        return { consumed: 0, key, value: true }
    }

    // Options with values
    const nextArg = remaining[0]
    if (nextArg && !nextArg.startsWith('-')) {
        return { consumed: 1, key, value: nextArg }
    }

    return { consumed: 0, key, value: '' }
}

/**
 * Parse long option (e.g., --verbose, --config <value>)
 */
function parseLongOption(
    arg: string,
    remaining: string[],
): { consumed: number; key: string | null; value: string | boolean } {
    const key = normalizeLongOption(arg)
    if (!key) {
        return { consumed: 0, key: null, value: false }
    }

    // Boolean options (flags)
    const booleanOptions = [
        'verbose',
        'quiet',
        'noColor',
        'bail',
        'watch',
        'skipSetup',
        'skipTeardown',
        'dryRun',
        'debug',
        'stepByStep',
        'captureTraffic',
        'help',
        'version',
    ]

    if (booleanOptions.includes(key)) {
        return { consumed: 0, key, value: true }
    }

    // Options with values
    const nextArg = remaining[0]
    if (nextArg && !nextArg.startsWith('-')) {
        return { consumed: 1, key, value: nextArg }
    }

    return { consumed: 0, key, value: '' }
}

/**
 * Normalize long option name to camelCase key
 */
function normalizeLongOption(arg: string): string | null {
    const optionName = arg.replace(/^--/, '')

    // Map kebab-case to camelCase
    const keyMap: Record<string, string> = {
        config: 'config',
        env: 'env',
        'test-dir': 'testDir',
        'report-dir': 'reportDir',
        verbose: 'verbose',
        quiet: 'quiet',
        'no-color': 'noColor',
        parallel: 'parallel',
        timeout: 'timeout',
        retries: 'retries',
        bail: 'bail',
        watch: 'watch',
        grep: 'grep',
        tag: 'tag',
        priority: 'priority',
        'skip-setup': 'skipSetup',
        'skip-teardown': 'skipTeardown',
        'dry-run': 'dryRun',
        reporter: 'reporter',
        output: 'output',
        debug: 'debug',
        'step-by-step': 'stepByStep',
        'capture-traffic': 'captureTraffic',
        adapter: 'adapter',
        help: 'help',
        version: 'version',
        // Test command options
        template: 'testTemplate',
        'test-template': 'testTemplate',
        description: 'testDescription',
        'test-description': 'testDescription',
        'test-priority': 'testPriority',
        'test-tags': 'testTags',
    }

    return keyMap[optionName] || null
}

/** Apply parsed option to options object */
function applyOption(options: CLIOptions, key: string, value: string | boolean): void {
    const opts = options as unknown as Record<string, unknown>
    const STRING_OPTS = ['config', 'env', 'testDir', 'reportDir', 'grep', 'output', 'adapter', 'testTemplate', 'testDescription', 'testPriority', 'testTags']
    const NUMBER_OPTS = ['parallel', 'timeout', 'retries']
    const BOOL_OPTS = [
        'verbose',
        'quiet',
        'noColor',
        'bail',
        'watch',
        'skipSetup',
        'skipTeardown',
        'dryRun',
        'debug',
        'stepByStep',
        'captureTraffic',
    ]

    if (STRING_OPTS.includes(key)) opts[key] = value
    else if (NUMBER_OPTS.includes(key)) opts[key] = Number.parseInt(String(value), 10)
    else if (BOOL_OPTS.includes(key)) opts[key] = true
    else if (key === 'tag') options.tag.push(String(value))
    else if (key === 'priority' && isValidPriority(String(value)))
        options.priority.push(String(value) as TestPriority)
    else if (key === 'reporter') options.reporter.push(String(value))
}

/**
 * Check if a string is a valid priority
 */
function isValidPriority(str: string): str is TestPriority {
    return ['P0', 'P1', 'P2', 'P3'].includes(str)
}

// ============================================================================
// Output Functions
// ============================================================================

/**
 * Print help message
 */
export function printHelp(): void {
    console.log(HELP_TEXT)
}

/**
 * Print version
 */
export function printVersion(): void {
    console.log(`e2e-runner v${VERSION}`)
}

/**
 * Print error message
 */
export function printError(message: string, hint?: string): void {
    console.error(`Error: ${message}`)
    if (hint) {
        console.error(`Hint: ${hint}`)
    }
}

/**
 * Print success message
 */
export function printSuccess(message: string): void {
    console.log(`✓ ${message}`)
}

// ============================================================================
// Validation Functions
// ============================================================================

/**
 * Validate CLI arguments
 */
export function validateArgs(args: CLIArgs): { valid: boolean; errors: string[] } {
    const errors: string[] = []

    // Validate command
    if (!VALID_COMMANDS.includes(args.command)) {
        errors.push(
            `Invalid command: ${args.command}. Valid commands: ${VALID_COMMANDS.join(', ')}`,
        )
    }

    // Validate numeric options
    if (args.options.parallel !== undefined && args.options.parallel < 1) {
        errors.push('Parallel must be at least 1')
    }

    if (args.options.timeout !== undefined && args.options.timeout < 1000) {
        errors.push('Timeout must be at least 1000ms')
    }

    if (args.options.retries !== undefined && args.options.retries < 0) {
        errors.push('Retries cannot be negative')
    }

    // Validate priorities
    for (const priority of args.options.priority) {
        if (!isValidPriority(priority)) {
            errors.push(`Invalid priority: ${priority}. Valid values: P0, P1, P2, P3`)
        }
    }

    // Validate reporter types
    const validReporters = ['console', 'junit', 'html', 'json']
    for (const reporter of args.options.reporter) {
        if (!validReporters.includes(reporter)) {
            errors.push(`Invalid reporter: ${reporter}. Valid types: ${validReporters.join(', ')}`)
        }
    }

    return { valid: errors.length === 0, errors }
}

// ============================================================================
// Exports
// ============================================================================

export { VALID_COMMANDS, VERSION }
