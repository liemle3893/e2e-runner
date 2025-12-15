/**
 * E2E Test Runner - Test Orchestrator
 *
 * Manages test lifecycle: setup -> execute -> verify -> teardown
 */

import pLimit from 'p-limit'

import { AdapterRegistry, createAdapterRegistry } from '../adapters'
import { TimeoutError, wrapError } from '../errors'
import type {
    HooksConfig,
    LoadedConfig,
    Logger,
    PhaseResult,
    PhaseStatus,
    StepResult,
    TestExecutionResult,
    TestPhase,
    TestStatus,
    TestSuiteResult,
    UnifiedStep,
    UnifiedTestDefinition,
} from '../types'
import { withTimeout } from '../utils/retry'
import { ContextFactory, createContextFactory, TestContext } from './context-factory'
import { createStepExecutor, StepExecutor, StepExecutorOptions } from './step-executor'

// ============================================================================
// Types
// ============================================================================

/**
 * Options for the test orchestrator
 */
export interface OrchestratorOptions {
    /** Maximum parallel test executions */
    parallel: number
    /** Default test timeout in milliseconds */
    timeout: number
    /** Default retry count for steps */
    retries: number
    /** Base delay between retries */
    retryDelay: number
    /** Whether to skip setup phases */
    skipSetup: boolean
    /** Whether to skip teardown phases */
    skipTeardown: boolean
    /** Stop on first failure */
    bail: boolean
    /** Dry run mode - do not execute steps */
    dryRun: boolean
}

/**
 * Event types for progress reporting
 */
export type OrchestratorEventType =
    | 'suite:start'
    | 'suite:end'
    | 'test:start'
    | 'test:end'
    | 'phase:start'
    | 'phase:end'
    | 'step:start'
    | 'step:end'

/**
 * Event listener callback
 */
export type OrchestratorEventListener = (event: OrchestratorEventType, data: unknown) => void

// ============================================================================
// Test Orchestrator
// ============================================================================

/**
 * Orchestrates test execution lifecycle
 */
export class TestOrchestrator {
    private readonly config: LoadedConfig
    private readonly adapters: AdapterRegistry
    private readonly logger: Logger
    private readonly options: OrchestratorOptions
    private readonly contextFactory: ContextFactory
    private readonly stepExecutor: StepExecutor
    private readonly eventListeners: OrchestratorEventListener[] = []
    private shouldBail: boolean = false
    // Track current test context for event emission
    private currentTestIndex: number = 0
    private totalTests: number = 0
    private currentTest: UnifiedTestDefinition | null = null
    private currentPhase: TestPhase | null = null

    constructor(
        config: LoadedConfig,
        adapters: AdapterRegistry,
        logger: Logger,
        options: Partial<OrchestratorOptions> = {},
    ) {
        this.config = config
        this.adapters = adapters
        this.logger = logger
        this.options = this.mergeOptions(options)
        this.contextFactory = createContextFactory(config, adapters, logger)

        const stepOptions: StepExecutorOptions = {
            defaultRetries: this.options.retries,
            retryDelay: this.options.retryDelay,
            logger,
        }
        this.stepExecutor = createStepExecutor(adapters, stepOptions)
    }

    /**
     * Merge provided options with defaults
     */
    private mergeOptions(options: Partial<OrchestratorOptions>): OrchestratorOptions {
        return {
            parallel: options.parallel ?? this.config.defaults.parallel,
            timeout: options.timeout ?? this.config.defaults.timeout,
            retries: options.retries ?? this.config.defaults.retries,
            retryDelay: options.retryDelay ?? this.config.defaults.retryDelay,
            skipSetup: options.skipSetup ?? false,
            skipTeardown: options.skipTeardown ?? false,
            bail: options.bail ?? false,
            dryRun: options.dryRun ?? false,
        }
    }

    /**
     * Add an event listener for progress reporting
     */
    addEventListener(listener: OrchestratorEventListener): void {
        this.eventListeners.push(listener)
    }

    /**
     * Remove an event listener
     */
    removeEventListener(listener: OrchestratorEventListener): void {
        const index = this.eventListeners.indexOf(listener)
        if (index !== -1) {
            this.eventListeners.splice(index, 1)
        }
    }

    /**
     * Emit an event to all listeners
     */
    private emit(event: OrchestratorEventType, data: unknown): void {
        for (const listener of this.eventListeners) {
            try {
                listener(event, data)
            } catch (error) {
                this.logger.warn(`Event listener error: ${error}`)
            }
        }
    }

    /**
     * Run a test suite (collection of tests)
     */
    async runSuite(tests: UnifiedTestDefinition[]): Promise<TestSuiteResult> {
        const startTime = Date.now()
        this.shouldBail = false
        this.totalTests = tests.length
        this.currentTestIndex = 0

        // Note: suite:start is also emitted by run.command.ts, but we emit here
        // for any direct orchestrator listeners with proper format
        this.emit('suite:start', {
            suite: { name: 'E2E Tests', tests, config: this.config.raw },
            totalTests: tests.length,
            timestamp: new Date(),
        })
        this.logger.info(`Starting test suite with ${tests.length} test(s)`)

        // Run global beforeAll hook
        await this.runHook('beforeAll')

        // Filter out skipped tests
        const { runnableTests, skippedResults } = this.separateSkippedTests(tests)

        // Run tests with parallelism
        const limit = pLimit(this.options.parallel)
        const testPromises = runnableTests.map((test) =>
            limit(async () => {
                if (this.shouldBail) {
                    return this.createSkippedResult(test, 'Bailed due to previous failure')
                }
                return this.runTest(test)
            }),
        )

        const executedResults = await Promise.all(testPromises)
        const results = [...skippedResults, ...executedResults]

        // Run global afterAll hook
        await this.runHook('afterAll')

        const duration = Date.now() - startTime
        const suiteResult = this.createSuiteResult(results, duration)

        this.emit('suite:end', {
            result: suiteResult,
            timestamp: new Date(),
        })
        this.logger.info(
            `Test suite completed: ${suiteResult.passed}/${suiteResult.total} passed in ${duration}ms`,
        )

        return suiteResult
    }

    /**
     * Run a single test through all lifecycle phases
     */
    async runTest(test: UnifiedTestDefinition): Promise<TestExecutionResult> {
        const startTime = Date.now()
        const context = this.contextFactory.createTestContext(test)
        const phases: PhaseResult[] = []
        let testStatus: TestStatus
        let testError: Error | undefined

        // Track current test for event context
        this.currentTest = test
        const testIndex = this.currentTestIndex++

        this.emit('test:start', {
            test,
            index: testIndex,
            total: this.totalTests,
            timestamp: new Date(),
        })
        this.logger.info(`Running test: ${test.name}`)

        const timeout = test.timeout ?? this.options.timeout

        try {
            const result = await withTimeout(
                () => this.executeTestPhases(test, context, phases),
                timeout,
                `Test "${test.name}"`,
            )
            testStatus = result.status
            testError = result.error
        } catch (error) {
            testStatus = error instanceof TimeoutError ? 'error' : 'failed'
            testError = error instanceof Error ? error : new Error(String(error))
            this.logger.error(`Test ${test.name} failed: ${testError.message}`)
        } finally {
            await this.runTeardownPhase(test, context, phases)
            await this.runHook('afterEach')
        }

        if (testStatus === 'failed' && this.options.bail) {
            this.shouldBail = true
            this.logger.warn('Bail option enabled - stopping further tests')
        }

        const duration = Date.now() - startTime
        const result = this.buildTestResult(
            test.name,
            test.description,
            testStatus,
            phases,
            duration,
            testError,
            context,
        )

        this.emit('test:end', {
            test,
            result,
            index: testIndex,
            total: this.totalTests,
            timestamp: new Date(),
        })
        this.currentTest = null
        this.logger.info(`Test ${test.name}: ${testStatus} (${duration}ms)`)

        return result
    }

    /**
     * Execute main test phases (setup, execute, verify)
     */
    private async executeTestPhases(
        test: UnifiedTestDefinition,
        context: TestContext,
        phases: PhaseResult[],
    ): Promise<{ status: TestStatus; error?: Error }> {
        await this.runHook('beforeEach')

        const setupResult = await this.runPhase('setup', test.setup, context)
        phases.push(setupResult)

        if (setupResult.status === 'failed') {
            return { status: 'failed', error: setupResult.error }
        }

        const executeResult = await this.runPhase('execute', test.execute, context)
        phases.push(executeResult)

        if (executeResult.status === 'failed') {
            return { status: 'failed', error: executeResult.error }
        }

        const verifyResult = await this.runPhase('verify', test.verify, context)
        phases.push(verifyResult)

        if (verifyResult.status === 'failed') {
            return { status: 'failed', error: verifyResult.error }
        }

        return { status: 'passed' }
    }

    /**
     * Run teardown phase with error handling
     */
    private async runTeardownPhase(
        test: UnifiedTestDefinition,
        context: TestContext,
        phases: PhaseResult[],
    ): Promise<void> {
        if (this.options.skipTeardown || !test.teardown || test.teardown.length === 0) {
            return
        }

        try {
            const teardownResult = await this.runPhase('teardown', test.teardown, context)
            phases.push(teardownResult)
        } catch (teardownError) {
            const errorMessage =
                teardownError instanceof Error ? teardownError.message : String(teardownError)
            this.logger.error(`Teardown failed for ${test.name}: ${errorMessage}`)
            phases.push({
                phase: 'teardown',
                status: 'failed',
                steps: [],
                duration: 0,
                error:
                    teardownError instanceof Error
                        ? teardownError
                        : new Error(String(teardownError)),
            })
        }
    }

    /**
     * Build test execution result object
     */
    private buildTestResult(
        name: string,
        description: string | undefined,
        status: TestStatus,
        phases: PhaseResult[],
        duration: number,
        error: Error | undefined,
        context: TestContext,
    ): TestExecutionResult {
        return {
            name,
            description,
            status,
            phases,
            duration,
            error,
            retryCount: 0,
            capturedValues: context.captured,
        }
    }

    /**
     * Run a single phase (setup, execute, verify, or teardown)
     */
    private async runPhase(
        phaseName: TestPhase,
        steps: UnifiedStep[] | undefined,
        context: TestContext,
    ): Promise<PhaseResult> {
        const startTime = Date.now()

        // Check if phase should be skipped
        if (phaseName === 'setup' && this.options.skipSetup) {
            return { phase: phaseName, status: 'skipped', steps: [], duration: 0 }
        }

        if (!steps || steps.length === 0) {
            return { phase: phaseName, status: 'skipped', steps: [], duration: 0 }
        }

        // Track current phase for event context
        this.currentPhase = phaseName

        this.emit('phase:start', {
            testName: this.currentTest?.name ?? 'unknown',
            phase: phaseName,
            timestamp: new Date(),
        })
        this.logger.debug(`Starting ${phaseName} phase with ${steps.length} step(s)`)

        const stepResults: StepResult[] = []
        let phaseStatus: PhaseStatus = 'passed'
        let phaseError: Error | undefined

        // Execute steps sequentially
        for (const step of steps) {
            // Skip remaining steps if dry run
            if (this.options.dryRun) {
                stepResults.push({
                    stepId: step.id,
                    adapter: step.adapter,
                    action: step.action,
                    description: step.description,
                    status: 'skipped',
                    duration: 0,
                    retryCount: 0,
                })
                continue
            }

            this.emit('step:start', {
                testName: this.currentTest?.name ?? 'unknown',
                phase: phaseName,
                stepId: step.id,
                action: step.action,
                adapter: step.adapter,
                timestamp: new Date(),
            })

            const adapterContext = context.createAdapterContext()
            const interpolationContext = context.getInterpolationContext()

            const stepResult = await this.stepExecutor.executeStep(
                step,
                adapterContext,
                interpolationContext,
            )

            stepResults.push(stepResult)
            this.emit('step:end', {
                testName: this.currentTest?.name ?? 'unknown',
                phase: phaseName,
                stepId: step.id,
                result: stepResult,
                timestamp: new Date(),
            })

            // Check for failure
            if (stepResult.status === 'failed') {
                phaseStatus = 'failed'
                phaseError = stepResult.error

                // Stop phase execution unless continueOnError is set for all steps
                if (!step.continueOnError) {
                    this.logger.debug(`Phase ${phaseName} stopping due to step failure`)
                    break
                }
            }
        }

        const duration = Date.now() - startTime

        const phaseResult: PhaseResult = {
            phase: phaseName,
            status: phaseStatus,
            steps: stepResults,
            duration,
            error: phaseError,
        }

        this.emit('phase:end', {
            testName: this.currentTest?.name ?? 'unknown',
            phase: phaseName,
            result: phaseResult,
            timestamp: new Date(),
        })
        this.currentPhase = null
        this.logger.debug(`Phase ${phaseName}: ${phaseStatus} (${duration}ms)`)

        return {
            phase: phaseName,
            status: phaseStatus,
            steps: stepResults,
            duration,
            error: phaseError,
        }
    }

    /**
     * Run a hook from the configuration
     */
    private async runHook(hookName: keyof HooksConfig): Promise<void> {
        const hookPath = this.config.hooks[hookName]
        if (!hookPath) {
            return
        }

        this.logger.debug(`Running ${hookName} hook: ${hookPath}`)

        try {
            // Dynamic import of hook module
            const hookModule = await import(hookPath)
            if (typeof hookModule.default === 'function') {
                await hookModule.default(this.adapters, this.logger)
            } else if (typeof hookModule[hookName] === 'function') {
                await hookModule[hookName](this.adapters, this.logger)
            } else {
                this.logger.warn(`Hook ${hookName} at ${hookPath} does not export a valid function`)
            }
        } catch (error) {
            const wrappedError = wrapError(error, `Hook ${hookName} failed`)
            this.logger.error(wrappedError.message)
            throw wrappedError
        }
    }

    /**
     * Separate skipped tests from runnable tests
     */
    private separateSkippedTests(tests: UnifiedTestDefinition[]): {
        runnableTests: UnifiedTestDefinition[]
        skippedResults: TestExecutionResult[]
    } {
        const runnableTests: UnifiedTestDefinition[] = []
        const skippedResults: TestExecutionResult[] = []

        for (const test of tests) {
            if (test.skip) {
                skippedResults.push(
                    this.createSkippedResult(test, test.skipReason || 'Marked as skipped'),
                )
            } else {
                runnableTests.push(test)
            }
        }

        return { runnableTests, skippedResults }
    }

    /**
     * Create a skipped test result
     */
    private createSkippedResult(test: UnifiedTestDefinition, reason: string): TestExecutionResult {
        this.logger.info(`Skipping test: ${test.name} - ${reason}`)
        return {
            name: test.name,
            description: test.description,
            status: 'skipped',
            phases: [],
            duration: 0,
            retryCount: 0,
            capturedValues: {},
        }
    }

    /**
     * Create the final suite result
     */
    private createSuiteResult(results: TestExecutionResult[], duration: number): TestSuiteResult {
        const passed = results.filter((r) => r.status === 'passed').length
        const failed = results.filter((r) => r.status === 'failed' || r.status === 'error').length
        const skipped = results.filter((r) => r.status === 'skipped').length

        return {
            total: results.length,
            passed,
            failed,
            skipped,
            duration,
            results,
            success: failed === 0,
        }
    }
}

// ============================================================================
// Factory Functions
// ============================================================================

/**
 * Create a test orchestrator
 *
 * @param config - Loaded configuration
 * @param adapters - Adapter registry
 * @param logger - Logger instance
 * @param options - Orchestrator options
 * @returns A new TestOrchestrator instance
 */
export function createOrchestrator(
    config: LoadedConfig,
    adapters: AdapterRegistry,
    logger: Logger,
    options?: Partial<OrchestratorOptions>,
): TestOrchestrator {
    return new TestOrchestrator(config, adapters, logger, options)
}

/**
 * Create and initialize a complete test runner
 * Connects adapters and returns ready-to-use orchestrator
 *
 * @param config - Loaded configuration
 * @param logger - Logger instance
 * @param options - Orchestrator options
 * @returns Initialized TestOrchestrator
 */
export async function createAndInitializeRunner(
    config: LoadedConfig,
    logger: Logger,
    options?: Partial<OrchestratorOptions>,
): Promise<{ orchestrator: TestOrchestrator; cleanup: () => Promise<void> }> {
    // Create adapter registry
    const adapters = createAdapterRegistry(config.environment, logger)

    // Connect all adapters
    await adapters.connectAll()

    // Create orchestrator
    const orchestrator = createOrchestrator(config, adapters, logger, options)

    // Return orchestrator with cleanup function
    return {
        orchestrator,
        cleanup: async () => {
            await adapters.disconnectAll()
        },
    }
}
