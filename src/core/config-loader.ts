/**
 * E2E Test Runner - Configuration Loader
 */

import * as fs from 'node:fs';
import * as path from 'node:path';

import {
  ConfigurationError,
  ValidationError,
  type SchemaError,
} from '../errors';
import { CONFIG_SCHEMA } from '../cli/init-templates';
import type {
  E2EConfig,
  EnvironmentConfig,
  LoadedConfig,
  ReporterConfig,
  HooksConfig,
  DEFAULT_CONFIG,
} from '../types';

export interface LoadConfigOptions {
  configPath?: string;
  environment?: string;
}

/**
 * Load and validate the E2E configuration file
 */
export async function loadConfig(
  options: LoadConfigOptions = {}
): Promise<LoadedConfig> {
  const configPath = options.configPath || 'e2e.config.yaml';
  const environment = options.environment || 'local';

  // Resolve absolute path
  const absolutePath = path.resolve(process.cwd(), configPath);

  // Check if file exists
  if (!fs.existsSync(absolutePath)) {
    throw new ConfigurationError(
      `Configuration file not found: ${absolutePath}`,
      'Run "e2e init" to create a template configuration'
    );
  }

  // Read and parse YAML
  const content = fs.readFileSync(absolutePath, 'utf8');
  let raw: E2EConfig;

  try {
    // Dynamic import for yaml package
    const yaml = await import('yaml');
    raw = yaml.parse(content) as E2EConfig;
  } catch (error) {
    throw new ConfigurationError(
      `Failed to parse configuration file: ${
        error instanceof Error ? error.message : String(error)
      }`,
      'Check YAML syntax in the configuration file'
    );
  }

  // Validate version
  if (raw.version !== '1.0') {
    throw new ConfigurationError(
      `Unsupported configuration version: ${raw.version}`,
      'Use version "1.0"'
    );
  }

  // Validate schema (optional - requires ajv)
  await validateConfigSchema(raw);

  // Get environment config
  const envConfig = raw.environments?.[environment];
  if (!envConfig) {
    const availableEnvs = Object.keys(raw.environments || {}).join(', ');
    throw new ConfigurationError(
      `Environment not found: ${environment}`,
      `Available environments: ${availableEnvs || 'none'}`
    );
  }

  // Resolve environment variables in config
  // - baseUrl is required, resolve strictly
  // - adapters are optional, resolve non-strictly (will fail at connection time if needed)
  const resolvedEnvConfig: EnvironmentConfig = {
    baseUrl: resolveEnvironmentVariables(envConfig.baseUrl, true),
    adapters: envConfig.adapters
      ? resolveEnvironmentVariables(envConfig.adapters, false)
      : {},
  };

  // Build defaults
  const defaults = {
    timeout: raw.defaults?.timeout ?? 30000,
    retries: raw.defaults?.retries ?? 0,
    retryDelay: raw.defaults?.retryDelay ?? 1000,
    parallel: raw.defaults?.parallel ?? 1,
  };

  // Resolve environment variables in global variables (e.g., access_token: "${JWT}")
  const resolvedVariables = raw.variables
    ? resolveEnvironmentVariables(raw.variables, false)
    : {};

  return {
    raw,
    environment: resolvedEnvConfig,
    environmentName: environment,
    defaults,
    variables: resolvedVariables,
    reporters: raw.reporters || [{ type: 'console' }],
    hooks: raw.hooks || {},
  };
}

/**
 * Validate configuration against JSON schema (using embedded schema)
 */
async function validateConfigSchema(config: E2EConfig): Promise<void> {
  try {
    // Dynamic import for ajv
    const Ajv = (await import('ajv')).default;
    const ajv = new Ajv({ allErrors: true });

    // Use embedded schema from init-templates
    const validate = ajv.compile(CONFIG_SCHEMA);
    const valid = validate(config);

    if (!valid && validate.errors) {
      const errors: SchemaError[] = validate.errors.map((err) => ({
        path: (err as { instancePath?: string }).instancePath || '/',
        message: err.message || 'Unknown validation error',
        keyword: err.keyword,
      }));

      throw new ValidationError(
        'Configuration file validation failed',
        errors
      );
    }
  } catch (error) {
    if (error instanceof ValidationError) {
      throw error;
    }
    // If ajv is not installed, skip validation
    // This allows the runner to work without optional dependencies
  }
}

/**
 * Validate that adapter connection strings don't contain unresolved env vars.
 * This is called after non-strict resolution to provide clear error messages
 * when required adapters have missing env vars.
 */
export function validateAdapterConnectionStrings(
  config: EnvironmentConfig,
  requiredAdapters?: string[]
): void {
  const unresolvedPattern = /\$\{(\w+)\}/;

  if (!config.adapters) return;

  for (const [adapterName, adapterConfig] of Object.entries(config.adapters)) {
    // Skip validation for adapters that aren't required (if filter provided)
    if (requiredAdapters && !requiredAdapters.includes(adapterName)) {
      continue;
    }

    // Access connectionString safely using type assertion through unknown
    const config = adapterConfig as unknown as { connectionString?: string };
    const connectionString = config?.connectionString;
    if (typeof connectionString === 'string') {
      const match = connectionString.match(unresolvedPattern);
      if (match) {
        throw new ConfigurationError(
          `Missing environment variable: ${match[1]}`,
          `Set the ${match[1]} environment variable for ${adapterName} adapter`
        );
      }
    }
  }
}

/**
 * Resolve ${VAR} patterns in configuration values
 *
 * @param obj - The object to resolve environment variables in
 * @param strict - If true, throw on missing env vars. If false, leave them unresolved.
 */
function resolveEnvironmentVariables<T>(obj: T, strict: boolean = true): T {
  if (typeof obj === 'string') {
    return obj.replace(/\$\{(\w+)\}/g, (match, varName) => {
      const value = process.env[varName];
      if (value === undefined) {
        if (strict) {
          throw new ConfigurationError(
            `Missing environment variable: ${varName}`,
            `Set the ${varName} environment variable before running tests`
          );
        }
        // In non-strict mode, return the original placeholder
        return match;
      }
      return value;
    }) as T;
  }

  if (Array.isArray(obj)) {
    return obj.map((item) => resolveEnvironmentVariables(item, strict)) as T;
  }

  if (obj && typeof obj === 'object') {
    const result: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj)) {
      result[key] = resolveEnvironmentVariables(value, strict);
    }
    return result as T;
  }

  return obj;
}

/**
 * Create a default configuration template
 */
export function createDefaultConfig(): string {
  return `# E2E Test Runner Configuration
version: "1.0"

environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      postgresql:
        connectionString: "\${POSTGRESQL_CONNECTION_STRING}"
      redis:
        connectionString: "\${REDIS_CONNECTION_STRING}"
      mongodb:
        connectionString: "\${MONGODB_CONNECTION_STRING}"

defaults:
  timeout: 30000
  retries: 1
  retryDelay: 1000
  parallel: 4

variables:
  testPrefix: "e2e_test_"

reporters:
  - type: console
    verbose: true
  - type: junit
    output: "./reports/junit.xml"
`;
}

/**
 * Write default configuration to file
 */
export async function initConfig(configPath: string): Promise<void> {
  const absolutePath = path.resolve(process.cwd(), configPath);
  const dir = path.dirname(absolutePath);

  // Create directory if it doesn't exist
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }

  // Check if file already exists
  if (fs.existsSync(absolutePath)) {
    throw new ConfigurationError(
      `Configuration file already exists: ${absolutePath}`,
      'Delete the existing file or use a different path'
    );
  }

  // Write default config
  const content = createDefaultConfig();
  fs.writeFileSync(absolutePath, content, 'utf8');
}

/**
 * Merge CLI options with loaded config
 */
export function mergeConfigWithOptions(
  config: LoadedConfig,
  options: {
    timeout?: number;
    retries?: number;
    parallel?: number;
    reporter?: string[];
  }
): LoadedConfig {
  return {
    ...config,
    defaults: {
      ...config.defaults,
      timeout: options.timeout ?? config.defaults.timeout,
      retries: options.retries ?? config.defaults.retries,
      parallel: options.parallel ?? config.defaults.parallel,
    },
    reporters:
      options.reporter && options.reporter.length > 0
        ? options.reporter.map((type) => ({
            type: type as ReporterConfig['type'],
          }))
        : config.reporters,
  };
}
