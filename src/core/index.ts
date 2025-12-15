/**
 * E2E Test Runner - Core Module Index
 *
 * Re-exports all core functionality
 */

// Config Loader
export {
    createDefaultConfig,
    initConfig,
    loadConfig,
    mergeConfigWithOptions,
    validateAdapterConnectionStrings,
} from './config-loader'

// Test Discovery
export {
    categorizeTestFile,
    discoverTests,
    filterTestsByGrep,
    filterTestsByPatterns,
    filterTestsByPriority,
    filterTestsByTags,
    getTestNameFromPath,
    groupTestsByPriority,
    groupTestsByTags,
    sortTestsByDependencies,
} from './test-discovery'

// Variable Interpolation
export {
    BUILT_IN_FUNCTIONS,
    createInterpolationContext,
    extractVariableNames,
    getNestedValue,
    hasInterpolation,
    interpolate,
    interpolateObject,
    setNestedValue,
} from './variable-interpolator'

// YAML Loader
export {
    getYAMLTestMetadata,
    loadYAMLTest,
    loadYAMLTests,
    validateYAMLWithSchema,
} from './yaml-loader'

// TypeScript Loader
export {
    createE2EFunction,
    getTSTestMetadata,
    isValidTSTestFile,
    loadTSTest,
    loadTSTests,
} from './ts-loader'

// Context Factory
export type { TestContext } from './context-factory'
export {
    cloneCapturedValues,
    ContextFactory,
    createContextFactory,
    createMinimalContext,
    createStandaloneContext,
    getCapturedValue,
    hasCapturedValue,
    mergeCapturedValues,
} from './context-factory'

// Step Executor
export type { StepExecutorOptions } from './step-executor'
export {
    allStepsPassed,
    calculateTotalDuration,
    createFunctionStep,
    createStepExecutor,
    getFirstFailedStep,
    isTypeScriptFunctionStep,
    StepExecutor,
    TYPESCRIPT_FUNCTION_ACTION,
} from './step-executor'

// Test Orchestrator
export type {
    OrchestratorEventListener,
    OrchestratorEventType,
    OrchestratorOptions,
} from './test-orchestrator'
export {
    createAndInitializeRunner,
    createOrchestrator,
    TestOrchestrator,
} from './test-orchestrator'
