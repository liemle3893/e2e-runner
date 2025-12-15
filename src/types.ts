/**
 * E2E Test Runner - Core Type Definitions
 */

// ============================================================================
// Configuration Types
// ============================================================================

export interface E2EConfig {
  version: '1.0';
  environments: Record<string, EnvironmentConfig>;
  defaults?: DefaultsConfig;
  variables?: Record<string, string | number | boolean>;
  hooks?: HooksConfig;
  reporters?: ReporterConfig[];
}

export interface EnvironmentConfig {
  baseUrl: string;
  adapters: {
    postgresql?: PostgreSQLAdapterConfig;
    redis?: RedisAdapterConfig;
    mongodb?: MongoDBAdapterConfig;
    eventhub?: EventHubAdapterConfig;
  };
}

export interface PostgreSQLAdapterConfig {
  connectionString: string;
  schema?: string;
  poolSize?: number;
}

export interface RedisAdapterConfig {
  connectionString: string;
  db?: number;
  keyPrefix?: string;
}

export interface MongoDBAdapterConfig {
  connectionString: string;
  database?: string;
}

export interface EventHubAdapterConfig {
  connectionString: string;
  consumerGroup?: string;
  checkpointStore?: string;
}

export interface DefaultsConfig {
  timeout?: number;
  retries?: number;
  retryDelay?: number;
  parallel?: number;
}

export interface HooksConfig {
  beforeAll?: string;
  afterAll?: string;
  beforeEach?: string;
  afterEach?: string;
}

export interface ReporterConfig {
  type: 'console' | 'junit' | 'html' | 'json';
  output?: string;
  verbose?: boolean;
}

// ============================================================================
// Test Definition Types
// ============================================================================

export type TestPriority = 'P0' | 'P1' | 'P2' | 'P3';
export type AdapterType = 'postgresql' | 'redis' | 'mongodb' | 'eventhub' | 'http';
export type TestPhase = 'setup' | 'execute' | 'verify' | 'teardown';

export interface UnifiedTestDefinition {
  name: string;
  description?: string;
  priority?: TestPriority;
  tags?: string[];
  skip?: boolean;
  skipReason?: string;
  timeout?: number;
  retries?: number;
  depends?: string[];
  variables?: Record<string, unknown>;
  setup?: UnifiedStep[];
  execute: UnifiedStep[];
  verify?: UnifiedStep[];
  teardown?: UnifiedStep[];
  sourceFile: string;
  sourceType: 'yaml' | 'typescript';
}

export interface UnifiedStep {
  id: string;
  adapter: AdapterType;
  action: string;
  description?: string;
  params: Record<string, unknown>;
  capture?: Record<string, string>;
  assert?: unknown;
  continueOnError?: boolean;
  retry?: number;
  delay?: number;
}

// ============================================================================
// Execution Types
// ============================================================================

export type TestStatus = 'passed' | 'failed' | 'skipped' | 'error';
export type PhaseStatus = 'passed' | 'failed' | 'skipped';
export type StepStatus = 'passed' | 'failed' | 'skipped';

export interface TestExecutionResult {
  name: string;
  description?: string;
  status: TestStatus;
  phases: PhaseResult[];
  duration: number;
  error?: Error;
  retryCount: number;
  capturedValues: Record<string, unknown>;
}

export interface PhaseResult {
  phase: TestPhase;
  status: PhaseStatus;
  steps: StepResult[];
  duration: number;
  error?: Error;
}

export interface StepResult {
  stepId: string;
  adapter: AdapterType;
  action: string;
  description?: string;
  status: StepStatus;
  duration: number;
  data?: unknown;
  error?: Error;
  retryCount: number;
}

export interface TestSuiteResult {
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  duration: number;
  results: TestExecutionResult[];
  success: boolean;
}

// ============================================================================
// Adapter Types
// ============================================================================

export interface AdapterConfig {
  connectionString?: string;
  baseUrl?: string;
  [key: string]: unknown;
}

export interface AdapterContext {
  variables: Record<string, unknown>;
  captured: Record<string, unknown>;
  capture: (name: string, value: unknown) => void;
  logger: Logger;
  baseUrl: string;
}

export interface AdapterStepResult {
  success: boolean;
  data?: unknown;
  error?: Error;
  duration: number;
}

// ============================================================================
// Logger Types
// ============================================================================

export interface Logger {
  debug(message: string, ...args: unknown[]): void;
  info(message: string, ...args: unknown[]): void;
  warn(message: string, ...args: unknown[]): void;
  error(message: string, ...args: unknown[]): void;
}

// ============================================================================
// CLI Types
// ============================================================================

export type CLICommand = 'run' | 'validate' | 'list' | 'health' | 'init';

export interface CLIArgs {
  command: CLICommand;
  patterns: string[];
  options: CLIOptions;
}

export interface CLIOptions {
  // Common options
  config: string;
  env: string;
  verbose: boolean;
  quiet: boolean;
  noColor: boolean;

  // Path options (for standalone CLI usage)
  testDir: string;
  reportDir: string;

  // Run options
  parallel: number;
  timeout: number;
  retries: number;
  bail: boolean;
  watch: boolean;
  grep: string;
  tag: string[];
  priority: TestPriority[];
  skipSetup: boolean;
  skipTeardown: boolean;
  dryRun: boolean;

  // Report options
  reporter: string[];
  output: string;

  // Debug options
  debug: boolean;
  stepByStep: boolean;
  captureTraffic: boolean;

  // Health command options
  adapter: string;
}

// ============================================================================
// Reporter Types
// ============================================================================

export interface TestSuite {
  name: string;
  tests: UnifiedTestDefinition[];
  config: E2EConfig;
}

export interface ReporterEvent {
  type:
    | 'suite:start'
    | 'suite:end'
    | 'test:start'
    | 'test:end'
    | 'phase:start'
    | 'phase:end'
    | 'step:start'
    | 'step:end';
  timestamp: Date;
  data: unknown;
}

// ============================================================================
// Discovery Types
// ============================================================================

export interface DiscoveredTest {
  filePath: string;
  name: string;
  type: 'yaml' | 'typescript';
}

export interface DiscoveryOptions {
  basePath?: string;
  patterns?: string[];
  excludePatterns?: string[];
}

// ============================================================================
// Variable Interpolation Types
// ============================================================================

export interface InterpolationContext {
  variables: Record<string, unknown>;
  captured: Record<string, unknown>;
  baseUrl: string;
  env: Record<string, string>;
}

export type BuiltInFunction = (...args: string[]) => string | number;

// ============================================================================
// Loaded Config Type
// ============================================================================

export interface LoadedConfig {
  raw: E2EConfig;
  environment: EnvironmentConfig;
  environmentName: string;
  defaults: Required<DefaultsConfig>;
  variables: Record<string, unknown>;
  reporters: ReporterConfig[];
  hooks: HooksConfig;
}

// ============================================================================
// Default Values
// ============================================================================

export const DEFAULT_CONFIG: Required<DefaultsConfig> = {
  timeout: 30000,
  retries: 0,
  retryDelay: 1000,
  parallel: 1,
};

export const DEFAULT_CLI_OPTIONS: Partial<CLIOptions> = {
  config: 'e2e.config.yaml',
  env: 'local',
  verbose: false,
  quiet: false,
  noColor: false,
  testDir: '.',
  reportDir: './reports',
  parallel: 1,
  timeout: 30000,
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
};
