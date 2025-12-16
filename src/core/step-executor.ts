/**
 * E2E Test Runner - Step Executor
 *
 * Executes individual test steps against appropriate adapters
 */

import { AdapterRegistry } from '../adapters'
import { AdapterError, ExecutionError } from '../errors'
import type {
    AdapterContext,
    AdapterStepResult,
    AdapterType,
    InterpolationContext,
    Logger,
    StepResult,
    StepStatus,
    UnifiedStep,
} from '../types'
import { sleep, withRetry } from '../utils/retry'
import { interpolateObject } from './variable-interpolator'

// ============================================================================
// Types
// ============================================================================

/**
 * Options for step execution
 */
export interface StepExecutorOptions {
    /** Default retry count for steps without explicit retry */
    defaultRetries: number
    /** Base delay between retries in milliseconds */
    retryDelay: number
    /** Logger instance */
    logger: Logger
}

/**
 * TypeScript function step params
 */
interface TypeScriptFunctionParams {
    __function: (ctx: AdapterContext) => Promise<unknown>
    [key: string]: unknown
}

/**
 * Special action identifier for TypeScript function steps
 */
export const TYPESCRIPT_FUNCTION_ACTION = '__typescript_function__'

// ============================================================================
// Step Executor
// ============================================================================

/**
 * Executes individual test steps with retry logic
 */
export class StepExecutor {
    private readonly adapters: AdapterRegistry
    private readonly options: StepExecutorOptions
    private readonly logger: Logger

    constructor(adapters: AdapterRegistry, options: StepExecutorOptions) {
        this.adapters = adapters
        this.options = options
        this.logger = options.logger
    }

    /**
     * Execute a single step
     *
     * @param step - The step to execute
     * @param context - Adapter context with variables and captured values
     * @param interpolationContext - Context for variable interpolation
     * @returns Step execution result
     */
    async executeStep(
        step: UnifiedStep,
        context: AdapterContext,
        interpolationContext: InterpolationContext,
    ): Promise<StepResult> {
        const startTime = Date.now()
        let retryCount = 0

        this.logger.debug(`Executing step: ${step.id} (${step.adapter}.${step.action})`)

        // Apply delay before step if specified
        if (step.delay && step.delay > 0) {
            this.logger.debug(`Waiting ${step.delay}ms before step execution`)
            await sleep(step.delay)
        }

        try {
            // Determine retry count
            const maxAttempts = (step.retry ?? this.options.defaultRetries) + 1

            // Execute with retry logic
            const adapterResult = await withRetry(
                async () => {
                    return this.executeStepOnce(step, context, interpolationContext)
                },
                {
                    maxAttempts,
                    baseDelay: this.options.retryDelay,
                    exponentialBackoff: true,
                    onRetry: (error, attempt) => {
                        retryCount = attempt
                        this.logger.warn(
                            `Step ${step.id} failed (attempt ${attempt}/${maxAttempts - 1}): ${error.message}`,
                        )
                    },
                },
            )

            // Handle assertions from step result
            if (step.assert && adapterResult.data !== undefined) {
                this.validateAssertions(step.assert, adapterResult.data, step.id)
            }

            // Handle captures from step result
            // Note: For HTTP adapter, captures are handled directly by the adapter
            // (passed via interpolatedParams.capture), so we skip redundant capture here.
            // For other adapters that don't handle captures internally, we extract from result data.
            // We check if capture was NOT passed to the adapter by checking if it exists in result data structure.
            // The HTTP adapter returns {request, response} which doesn't match capture paths like $.field
            // so we only run this for adapters that return the actual result data directly.

            const duration = Date.now() - startTime
            this.logger.debug(`Step ${step.id} completed in ${duration}ms`)

            return this.createStepResult(step, 'passed', duration, adapterResult.data, retryCount)
        } catch (error) {
            const duration = Date.now() - startTime
            const errorObj = error instanceof Error ? error : new Error(String(error))

            // Check if we should continue on error
            if (step.continueOnError) {
                this.logger.warn(
                    `Step ${step.id} failed but continueOnError=true: ${errorObj.message}`,
                )
                return this.createStepResult(
                    step,
                    'passed',
                    duration,
                    undefined,
                    retryCount,
                    errorObj,
                )
            }

            this.logger.error(`Step ${step.id} failed: ${errorObj.message}`)
            return this.createStepResult(step, 'failed', duration, undefined, retryCount, errorObj)
        }
    }

    /**
     * Execute a step once without retry logic
     */
    private async executeStepOnce(
        step: UnifiedStep,
        context: AdapterContext,
        interpolationContext: InterpolationContext,
    ): Promise<AdapterStepResult> {
        // Check for TypeScript function step
        if (step.action === TYPESCRIPT_FUNCTION_ACTION) {
            return this.executeTypeScriptFunction(step, context)
        }

        // Interpolate variables in params
        const interpolatedParams = interpolateObject(step.params, interpolationContext)

        // Pass step-level assertions to adapter for validation
        // This fixes a bug where assertions defined in YAML were separated from params
        // by the loader, causing the HTTP adapter to never validate them
        // Assertions must also be interpolated to resolve variables like {{test_name}}
        if (step.assert) {
            interpolatedParams.assert = interpolateObject(step.assert, interpolationContext)
        }

        // Pass step-level captures to adapter for extraction
        // Same fix as assertions - captures were separated from params by the loader
        // Captures must also be interpolated to resolve variables in capture paths
        if (step.capture) {
            interpolatedParams.capture = interpolateObject(step.capture, interpolationContext)
        }

        // Get the appropriate adapter
        const adapter = this.adapters.get(step.adapter)
        if (!adapter) {
            throw new AdapterError(
                step.adapter,
                step.action,
                `Adapter "${step.adapter}" is not configured`,
            )
        }

        // Execute the action on the adapter
        return adapter.execute(step.action, interpolatedParams, context)
    }

    /**
     * Execute a TypeScript function step
     */
    private async executeTypeScriptFunction(
        step: UnifiedStep,
        context: AdapterContext,
    ): Promise<AdapterStepResult> {
        const params = step.params as TypeScriptFunctionParams
        const fn = params.__function

        if (typeof fn !== 'function') {
            throw new ExecutionError(
                `Step ${step.id} has action ${TYPESCRIPT_FUNCTION_ACTION} but params.__function is not a function`,
                { stepId: step.id },
            )
        }

        const startTime = Date.now()

        try {
            const result = await fn(context)
            return {
                success: true,
                data: result,
                duration: Date.now() - startTime,
            }
        } catch (error) {
            return {
                success: false,
                error: error instanceof Error ? error : new Error(String(error)),
                duration: Date.now() - startTime,
            }
        }
    }

    /**
     * Validate assertions against step result data
     * Note: Full assertion engine will be integrated in Phase 5
     */
    private validateAssertions(assertions: unknown, data: unknown, stepId: string): void {
        // Basic assertion validation - full assertion engine is in Phase 5
        // For now, just log that assertions are present
        this.logger.debug(`Step ${stepId} has assertions to validate:`, assertions)

        // Assertions will be validated by the assertion module in Phase 5
        // For now, we just check if assertions is truthy to indicate it exists
        if (assertions && typeof assertions === 'object') {
            this.logger.debug(`Assertions pending validation for step ${stepId}`)
        }
    }

    /**
     * Capture values from step result
     */
    private captureValues(
        capture: Record<string, string>,
        data: unknown,
        context: AdapterContext,
    ): void {
        for (const [varName, path] of Object.entries(capture)) {
            const value = this.extractValue(data, path)
            context.capture(varName, value)
        }
    }

    /**
     * Extract a value from data using a path expression
     * Supports dot notation and array indexing
     */
    private extractValue(data: unknown, path: string): unknown {
        if (!path || path === '.') {
            return data
        }

        const parts = path.split(/\.|\[(\d+)\]/).filter(Boolean)
        let current: unknown = data

        for (const part of parts) {
            if (current === null || current === undefined) {
                return undefined
            }

            if (Array.isArray(current)) {
                const index = Number.parseInt(part, 10)
                if (!Number.isNaN(index)) {
                    current = current[index]
                    continue
                }
            }

            if (typeof current === 'object') {
                current = (current as Record<string, unknown>)[part]
            } else {
                return undefined
            }
        }

        return current
    }

    /**
     * Create a step result object
     */
    private createStepResult(
        step: UnifiedStep,
        status: StepStatus,
        duration: number,
        data?: unknown,
        retryCount: number = 0,
        error?: Error,
    ): StepResult {
        return {
            stepId: step.id,
            adapter: step.adapter,
            action: step.action,
            description: step.description,
            status,
            duration,
            data,
            error,
            retryCount,
        }
    }
}

// ============================================================================
// Factory Functions
// ============================================================================

/**
 * Create a step executor instance
 *
 * @param adapters - Adapter registry
 * @param options - Executor options
 * @returns A new StepExecutor instance
 */
export function createStepExecutor(
    adapters: AdapterRegistry,
    options: StepExecutorOptions,
): StepExecutor {
    return new StepExecutor(adapters, options)
}

// ============================================================================
// Step Utilities
// ============================================================================

/**
 * Check if a step is a TypeScript function step
 *
 * @param step - The step to check
 * @returns True if the step is a TypeScript function
 */
export function isTypeScriptFunctionStep(step: UnifiedStep): boolean {
    return step.action === TYPESCRIPT_FUNCTION_ACTION
}

/**
 * Create a TypeScript function step
 *
 * @param id - Step ID
 * @param fn - The async function to execute
 * @param options - Additional step options
 * @returns A UnifiedStep for the function
 */
export function createFunctionStep(
    id: string,
    fn: (ctx: AdapterContext) => Promise<unknown>,
    options: Partial<Omit<UnifiedStep, 'id' | 'adapter' | 'action' | 'params'>> = {},
): UnifiedStep {
    return {
        id,
        adapter: 'http' as AdapterType, // Default adapter for function steps
        action: TYPESCRIPT_FUNCTION_ACTION,
        params: { __function: fn },
        ...options,
    }
}

/**
 * Check if all steps in a list passed
 *
 * @param results - Array of step results
 * @returns True if all steps passed
 */
export function allStepsPassed(results: StepResult[]): boolean {
    return results.every((r) => r.status === 'passed')
}

/**
 * Get the first failed step from results
 *
 * @param results - Array of step results
 * @returns The first failed step or undefined
 */
export function getFirstFailedStep(results: StepResult[]): StepResult | undefined {
    return results.find((r) => r.status === 'failed')
}

/**
 * Calculate total duration from step results
 *
 * @param results - Array of step results
 * @returns Total duration in milliseconds
 */
export function calculateTotalDuration(results: StepResult[]): number {
    return results.reduce((sum, r) => sum + r.duration, 0)
}
