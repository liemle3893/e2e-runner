/**
 * E2E Test Runner - Health Command
 *
 * Check connectivity to all configured adapters
 */

import { createAdapterRegistry, parseAdapterType } from '../adapters'
import { loadConfig } from '../core'
import { isE2ERunnerError, wrapError } from '../errors'
import type { AdapterType, CLIArgs } from '../types'
import { errorCodeToExitCode, EXIT_CODES } from '../utils/exit-codes'
import { createLogger, type LogLevel } from '../utils/logger'
import { printError } from './index'

// ============================================================================
// Types
// ============================================================================

export interface AdapterHealthResult {
    adapter: AdapterType
    healthy: boolean
    latency?: number
    error?: string
}

export interface HealthCommandResult {
    exitCode: number
    results: AdapterHealthResult[]
    allHealthy: boolean
}

// ============================================================================
// ANSI Colors
// ============================================================================

const COLORS = {
    reset: '\u001B[0m',
    bold: '\u001B[1m',
    dim: '\u001B[2m',
    green: '\u001B[32m',
    red: '\u001B[31m',
    yellow: '\u001B[33m',
    cyan: '\u001B[36m',
}

const SYMBOLS = {
    check: '\u2713',
    cross: '\u2717',
    bullet: '\u2022',
    spinner: [
        '\u280B',
        '\u2819',
        '\u2839',
        '\u2838',
        '\u283C',
        '\u2834',
        '\u2826',
        '\u2827',
        '\u2807',
        '\u280F',
    ],
}

function colorize(text: string, color: keyof typeof COLORS, useColors: boolean): string {
    return useColors ? `${COLORS[color]}${text}${COLORS.reset}` : text
}

// ============================================================================
// Health Command
// ============================================================================

/**
 * Execute the health command
 */
export async function healthCommand(args: CLIArgs): Promise<HealthCommandResult> {
    const { options } = args

    // Determine log level based on options
    const logLevel: LogLevel = options.quiet ? 'silent' : (options.verbose ? 'debug' : 'info')

    const logger = createLogger({
        level: logLevel,
        useColors: !options.noColor,
    })

    const useColors = !options.noColor && process.stdout.isTTY !== false
    const results: AdapterHealthResult[] = []

    try {
        // 1. Load configuration
        logger.debug(`Loading config from: ${options.config}`)
        const config = await loadConfig({
            configPath: options.config,
            environment: options.env,
        })

        console.log()
        console.log(colorize('E2E Adapter Health Check', 'bold', useColors))
        console.log('='.repeat(50))
        console.log()
        console.log(`Environment: ${colorize(config.environmentName, 'cyan', useColors)}`)
        console.log()

        // 2. Create adapter registry
        const adapters = createAdapterRegistry(config.environment, logger)

        // 3. Determine which adapters to check
        let adaptersToCheck: AdapterType[]

        if (options.adapter) {
            // Check specific adapter
            try {
                const adapterType = parseAdapterType(options.adapter)
                if (!adapters.has(adapterType)) {
                    printError(
                        `Adapter "${options.adapter}" is not configured for environment "${config.environmentName}"`,
                        'Check your e2e.config.yaml file',
                    )
                    return { exitCode: EXIT_CODES.CONFIG_ERROR, results, allHealthy: false }
                }
                adaptersToCheck = [adapterType]
            } catch (error) {
                const message = error instanceof Error ? error.message : String(error)
                printError(message)
                return { exitCode: EXIT_CODES.CONFIG_ERROR, results, allHealthy: false }
            }
        } else {
            // Check all configured adapters
            adaptersToCheck = adapters.getAvailableAdapters()
        }

        // 4. Check each adapter
        console.log('Checking adapters...')
        console.log()

        for (const adapterType of adaptersToCheck) {
            const result = await checkAdapter(adapters, adapterType, useColors)
            results.push(result)
            printHealthResult(result, useColors)
        }

        // 5. Print summary
        printHealthSummary(results, useColors)

        // 6. Return result
        const allHealthy = results.every((r) => r.healthy)
        const exitCode = allHealthy ? EXIT_CODES.SUCCESS : EXIT_CODES.CONNECTION_ERROR

        return { exitCode, results, allHealthy }
    } catch (error) {
        if (isE2ERunnerError(error)) {
            printError(error.message, error.hint)
            return { exitCode: errorCodeToExitCode(error.code), results, allHealthy: false }
        }

        const wrapped = wrapError(error, 'Health check failed')
        printError(wrapped.message)
        return { exitCode: EXIT_CODES.FATAL, results, allHealthy: false }
    }
}

// ============================================================================
// Health Check Functions
// ============================================================================

/**
 * Check a single adapter's health
 */
async function checkAdapter(
    adapters: ReturnType<typeof createAdapterRegistry>,
    adapterType: AdapterType,
    useColors: boolean,
): Promise<AdapterHealthResult> {
    const startTime = Date.now()

    try {
        // Get the adapter
        const adapter = adapters.get(adapterType)

        // Connect if not already connected
        await adapter.connect()

        // Run health check
        const healthy = await adapter.healthCheck()

        // Calculate latency
        const latency = Date.now() - startTime

        // Disconnect
        await adapter.disconnect()

        return {
            adapter: adapterType,
            healthy,
            latency,
        }
    } catch (error) {
        const latency = Date.now() - startTime
        const errorMessage = error instanceof Error ? error.message : String(error)

        return {
            adapter: adapterType,
            healthy: false,
            latency,
            error: errorMessage,
        }
    }
}

// ============================================================================
// Output Functions
// ============================================================================

/**
 * Print a single health check result
 */
function printHealthResult(result: AdapterHealthResult, useColors: boolean): void {
    const adapterName = formatAdapterName(result.adapter)

    if (result.healthy) {
        const symbol = colorize(SYMBOLS.check, 'green', useColors)
        const status = colorize('HEALTHY', 'green', useColors)
        const latency = colorize(`(${result.latency}ms)`, 'dim', useColors)

        console.log(`  ${symbol} ${adapterName.padEnd(15)} ${status} ${latency}`)
    } else {
        const symbol = colorize(SYMBOLS.cross, 'red', useColors)
        const status = colorize('UNHEALTHY', 'red', useColors)

        console.log(`  ${symbol} ${adapterName.padEnd(15)} ${status}`)

        if (result.error) {
            const errorPrefix = colorize('    Error:', 'dim', useColors)
            console.log(`${errorPrefix} ${result.error}`)
        }
    }
}

/**
 * Format adapter name for display
 */
function formatAdapterName(adapter: AdapterType): string {
    const names: Record<AdapterType, string> = {
        http: 'HTTP',
        postgresql: 'PostgreSQL',
        redis: 'Redis',
        mongodb: 'MongoDB',
        eventhub: 'EventHub',
    }

    return names[adapter] || adapter
}

/**
 * Print health check summary
 */
function printHealthSummary(results: AdapterHealthResult[], useColors: boolean): void {
    console.log()
    console.log(colorize('Summary', 'bold', useColors))
    console.log('-'.repeat(40))

    const healthy = results.filter((r) => r.healthy).length
    const unhealthy = results.length - healthy

    console.log(`  Total adapters: ${results.length}`)
    console.log(`  Healthy:        ${colorize(String(healthy), 'green', useColors)}`)
    console.log(
        `  Unhealthy:      ${colorize(String(unhealthy), unhealthy > 0 ? 'red' : 'dim', useColors)}`,
    )

    // Average latency
    const healthyResults = results.filter((r) => r.healthy && r.latency !== undefined)
    if (healthyResults.length > 0) {
        const avgLatency = Math.round(
            healthyResults.reduce((sum, r) => sum + (r.latency || 0), 0) / healthyResults.length,
        )
        console.log(`  Avg latency:    ${avgLatency}ms`)
    }

    console.log()

    if (unhealthy === 0) {
        console.log(colorize(`${SYMBOLS.check} All adapters are healthy`, 'green', useColors))
    } else {
        console.log(
            colorize(
                `${SYMBOLS.cross} ${unhealthy} adapter(s) failed health check`,
                'red',
                useColors,
            ),
        )

        // List unhealthy adapters
        const unhealthyAdapters = results.filter((r) => !r.healthy)
        for (const r of unhealthyAdapters) {
            console.log(
                `  ${colorize(SYMBOLS.bullet, 'red', useColors)} ${formatAdapterName(r.adapter)}`,
            )
        }
    }
}
