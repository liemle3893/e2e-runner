/**
 * E2E Test Runner - Context Factory
 *
 * Creates test execution contexts with access to adapters and variables
 */

import { AdapterRegistry } from '../adapters'
import type {
    AdapterContext,
    InterpolationContext,
    LoadedConfig,
    Logger,
    UnifiedTestDefinition,
} from '../types'
import { createInterpolationContext } from './variable-interpolator'

// ============================================================================
// Test Context
// ============================================================================

/**
 * Context for a single test execution
 * Provides access to adapters, variables, and captured values
 */
export interface TestContext {
    /** Test definition being executed */
    test: UnifiedTestDefinition
    /** Adapter registry for database and HTTP access */
    adapters: AdapterRegistry
    /** Global and test-level variables merged */
    variables: Record<string, unknown>
    /** Values captured during test execution */
    captured: Record<string, unknown>
    /** Logger instance */
    logger: Logger
    /** Base URL for HTTP requests */
    baseUrl: string
    /** Capture a value for use in subsequent steps */
    capture: (name: string, value: unknown) => void
    /** Get interpolation context for variable substitution */
    getInterpolationContext: () => InterpolationContext
    /** Create adapter context for step execution */
    createAdapterContext: () => AdapterContext
}

// ============================================================================
// Context Factory
// ============================================================================

/**
 * Factory for creating test execution contexts
 * Manages variable scoping and captured values
 */
export class ContextFactory {
    private readonly config: LoadedConfig
    private readonly adapters: AdapterRegistry
    private readonly logger: Logger

    constructor(config: LoadedConfig, adapters: AdapterRegistry, logger: Logger) {
        this.config = config
        this.adapters = adapters
        this.logger = logger
    }

    /**
     * Create a new test context for executing a test
     *
     * @param test - The test definition to create context for
     * @returns A new TestContext instance
     */
    createTestContext(test: UnifiedTestDefinition): TestContext {
        // Merge global variables with test-level variables
        // Test variables override global variables
        const variables: Record<string, unknown> = {
            ...this.config.variables,
            ...test.variables,
        }

        // Initialize captured values storage
        const captured: Record<string, unknown> = {}

        // Create capture function that stores values
        const capture = (name: string, value: unknown): void => {
            captured[name] = value
            this.logger.debug(`Captured "${name}":`, value)
        }

        // Create interpolation context getter
        const getInterpolationContext = (): InterpolationContext => {
            return createInterpolationContext(variables, captured, this.config.environment.baseUrl)
        }

        // Create adapter context getter
        const createAdapterContext = (): AdapterContext => {
            return {
                variables,
                captured,
                baseUrl: this.config.environment.baseUrl,
                logger: this.logger,
                capture,
            }
        }

        return {
            test,
            adapters: this.adapters,
            variables,
            captured,
            logger: this.logger,
            baseUrl: this.config.environment.baseUrl,
            capture,
            getInterpolationContext,
            createAdapterContext,
        }
    }

    /**
     * Get the adapter registry
     */
    getAdapters(): AdapterRegistry {
        return this.adapters
    }

    /**
     * Get the loaded configuration
     */
    getConfig(): LoadedConfig {
        return this.config
    }

    /**
     * Get the logger instance
     */
    getLogger(): Logger {
        return this.logger
    }
}

// ============================================================================
// Factory Functions
// ============================================================================

/**
 * Create a new context factory
 *
 * @param config - Loaded configuration
 * @param adapters - Adapter registry
 * @param logger - Logger instance
 * @returns A new ContextFactory instance
 */
export function createContextFactory(
    config: LoadedConfig,
    adapters: AdapterRegistry,
    logger: Logger,
): ContextFactory {
    return new ContextFactory(config, adapters, logger)
}

/**
 * Create a standalone adapter context for ad-hoc operations
 * Useful for hooks or utility functions that need adapter access
 *
 * @param variables - Variables to include
 * @param captured - Captured values
 * @param baseUrl - Base URL for HTTP
 * @param logger - Logger instance
 * @returns An AdapterContext instance
 */
export function createStandaloneContext(
    variables: Record<string, unknown>,
    captured: Record<string, unknown>,
    baseUrl: string,
    logger: Logger,
): AdapterContext {
    return {
        variables,
        captured,
        baseUrl,
        logger,
        capture: (name: string, value: unknown) => {
            captured[name] = value
        },
    }
}

/**
 * Create a minimal context for quick operations
 * Uses empty variables and captured values
 *
 * @param baseUrl - Base URL for HTTP
 * @param logger - Logger instance
 * @returns An AdapterContext instance with empty state
 */
export function createMinimalContext(baseUrl: string, logger: Logger): AdapterContext {
    const captured: Record<string, unknown> = {}
    return {
        variables: {},
        captured,
        baseUrl,
        logger,
        capture: (name: string, value: unknown) => {
            captured[name] = value
        },
    }
}

// ============================================================================
// Context Utilities
// ============================================================================

/**
 * Merge multiple captured value objects
 *
 * @param sources - Array of captured value objects to merge
 * @returns Merged captured values
 */
export function mergeCapturedValues(
    ...sources: Record<string, unknown>[]
): Record<string, unknown> {
    return Object.assign({}, ...sources)
}

/**
 * Clone a context's captured values
 * Useful for creating snapshots
 *
 * @param captured - Captured values to clone
 * @returns Deep clone of captured values
 */
export function cloneCapturedValues(captured: Record<string, unknown>): Record<string, unknown> {
    return structuredClone(captured)
}

/**
 * Check if a context has a specific captured value
 *
 * @param captured - Captured values object
 * @param name - Name of the value to check
 * @returns True if the value exists
 */
export function hasCapturedValue(captured: Record<string, unknown>, name: string): boolean {
    return name in captured && captured[name] !== undefined
}

/**
 * Get a captured value with type safety
 *
 * @param captured - Captured values object
 * @param name - Name of the value to get
 * @param defaultValue - Default value if not found
 * @returns The captured value or default
 */
export function getCapturedValue<T>(
    captured: Record<string, unknown>,
    name: string,
    defaultValue?: T,
): T | undefined {
    if (name in captured) {
        return captured[name] as T
    }
    return defaultValue
}
