/**
 * E2E Test Runner - TypeScript Test Loader
 *
 * Loads and converts TypeScript DSL test files to unified format
 */

import * as fs from 'node:fs';
import * as path from 'node:path';

import { LoaderError } from '../errors';
import type {
  DiscoveredTest,
  Logger,
  TestPriority,
  UnifiedTestDefinition,
} from '../types';

// ============================================================================
// Types (from tests/e2e/lib/types.ts)
// ============================================================================

interface TSTestMetadata {
  priority?: TestPriority;
  tags?: string[];
  timeout?: number;
  retries?: number;
  skip?: boolean;
  skipReason?: string;
  depends?: string[];
}

interface TSTestVariables {
  [key: string]: string | number | boolean | object | unknown[];
}

interface TSTestDefinition<V extends TSTestVariables = TSTestVariables> extends TSTestMetadata {
  variables?: V;
  setup?: (ctx: unknown) => Promise<void>;
  execute: (ctx: unknown) => Promise<void>;
  verify?: (ctx: unknown) => Promise<void>;
  teardown?: (ctx: unknown) => Promise<void>;
}

interface TSTestModule {
  default: TSTestDefinition;
  name?: string;
}

// ============================================================================
// Loader Functions
// ============================================================================

/**
 * Load a single TypeScript test file
 */
export async function loadTSTest(
  filePath: string,
  logger?: Logger
): Promise<UnifiedTestDefinition> {
  logger?.debug(`Loading TypeScript test: ${filePath}`);

  if (!fs.existsSync(filePath)) {
    throw new LoaderError('typescript', filePath, `File not found: ${filePath}`);
  }

  const absolutePath = path.resolve(filePath);

  try {
    // For TypeScript files, we need ts-node or similar to be registered
    // First, try to register ts-node if not already
    await ensureTSNodeRegistered();

    // Clear module cache to ensure fresh load
    clearModuleCache(absolutePath);

    // Dynamic import for TypeScript file
    const module = (await import(absolutePath)) as TSTestModule;

    if (!module.default) {
      throw new LoaderError(
        'typescript',
        filePath,
        'TypeScript test file must export a default test definition'
      );
    }

    const definition = module.default;

    // Extract test name from module or filename
    const testName = extractTestName(module, filePath);

    // Validate the definition
    validateTSDefinition(definition, filePath);

    // Convert to unified format
    return convertToUnified(testName, definition, filePath);
  } catch (error) {
    if (error instanceof LoaderError) {
      throw error;
    }

    throw new LoaderError(
      'typescript',
      filePath,
      `Failed to load TypeScript test: ${error instanceof Error ? error.message : String(error)}`
    );
  }
}

/**
 * Load multiple TypeScript test files
 */
export async function loadTSTests(
  tests: DiscoveredTest[],
  logger?: Logger
): Promise<UnifiedTestDefinition[]> {
  const results: UnifiedTestDefinition[] = [];
  const errors: string[] = [];

  for (const test of tests) {
    if (test.type !== 'typescript') {
      continue;
    }

    try {
      const definition = await loadTSTest(test.filePath, logger);
      results.push(definition);
    } catch (error) {
      errors.push(
        `${test.name}: ${error instanceof Error ? error.message : String(error)}`
      );
    }
  }

  if (errors.length > 0) {
    throw new LoaderError(
      'typescript',
      'multiple',
      `Failed to load ${errors.length} test(s):\n${errors.join('\n')}`
    );
  }

  return results;
}

/**
 * Ensure ts-node is registered for TypeScript imports
 */
async function ensureTSNodeRegistered(): Promise<void> {
  // Check if ts-node is already registered
  if (process.env.TS_NODE_REGISTERED === 'true') {
    return;
  }

  // Check for tsx or other TypeScript runtimes
  if (process.argv[0]?.includes('tsx') || process.env.TSX) {
    return;
  }

  try {
    // Try to dynamically register ts-node
    const tsNode = await import('ts-node');
    tsNode.register({
      transpileOnly: true,
      compilerOptions: {
        module: 'commonjs',
        esModuleInterop: true,
      },
    });
    process.env.TS_NODE_REGISTERED = 'true';
  } catch {
    // ts-node not available, might be running with tsx or esbuild-register
    // Just continue and let the import fail if TypeScript isn't supported
  }
}

/**
 * Clear module cache for a specific file
 */
function clearModuleCache(absolutePath: string): void {
  // Clear require cache if using CommonJS
  if (typeof require !== 'undefined' && require.cache) {
    delete require.cache[absolutePath];
  }
}

/**
 * Extract test name from module or filename
 */
function extractTestName(module: TSTestModule, filePath: string): string {
  // Check if module exports a name
  if (module.name) {
    return module.name;
  }

  // Check if the definition itself has metadata that includes name
  const def = module.default as unknown as { _name?: string };
  if (def._name) {
    return def._name;
  }

  // Fall back to filename
  const basename = path.basename(filePath);
  return basename.replace(/\.test\.ts$/, '');
}

/**
 * Validate TypeScript test definition
 */
function validateTSDefinition(definition: TSTestDefinition, filePath: string): void {
  const errors: string[] = [];

  if (!definition.execute || typeof definition.execute !== 'function') {
    errors.push('Test definition must have an "execute" function');
  }

  if (definition.setup && typeof definition.setup !== 'function') {
    errors.push('"setup" must be a function');
  }

  if (definition.verify && typeof definition.verify !== 'function') {
    errors.push('"verify" must be a function');
  }

  if (definition.teardown && typeof definition.teardown !== 'function') {
    errors.push('"teardown" must be a function');
  }

  if (definition.priority && !['P0', 'P1', 'P2', 'P3'].includes(definition.priority)) {
    errors.push(`Invalid priority "${definition.priority}". Must be: P0, P1, P2, P3`);
  }

  if (definition.tags && !Array.isArray(definition.tags)) {
    errors.push('"tags" must be an array');
  }

  if (definition.timeout !== undefined) {
    if (typeof definition.timeout !== 'number' || definition.timeout < 1000) {
      errors.push('"timeout" must be a number >= 1000');
    }
  }

  if (definition.retries !== undefined) {
    if (typeof definition.retries !== 'number' || definition.retries < 0) {
      errors.push('"retries" must be a non-negative number');
    }
  }

  if (errors.length > 0) {
    throw new LoaderError(
      'typescript',
      filePath,
      `Invalid test definition:\n${errors.map((e) => `  - ${e}`).join('\n')}`
    );
  }
}

/**
 * Convert TypeScript definition to unified format
 *
 * TypeScript tests use functions instead of declarative steps,
 * so we create a special unified definition that references the functions
 */
function convertToUnified(
  name: string,
  definition: TSTestDefinition,
  filePath: string
): UnifiedTestDefinition {
  // For TypeScript tests, we store the function references in special steps
  // The step executor will detect these and execute the functions directly

  const unified: UnifiedTestDefinition = {
    name,
    description: undefined, // TypeScript tests don't have a separate description
    priority: definition.priority,
    tags: definition.tags,
    skip: definition.skip,
    skipReason: definition.skipReason,
    timeout: definition.timeout,
    retries: definition.retries,
    depends: definition.depends,
    variables: definition.variables as Record<string, unknown>,
    sourceFile: path.resolve(filePath),
    sourceType: 'typescript',
    execute: [
      {
        id: 'execute-0',
        adapter: 'http', // Placeholder, TypeScript tests handle their own adapters
        action: '__typescript_function__',
        description: 'Execute TypeScript test function',
        params: {
          __function: definition.execute,
          __phase: 'execute',
        },
      },
    ],
  };

  // Add setup if present
  if (definition.setup) {
    unified.setup = [
      {
        id: 'setup-0',
        adapter: 'http',
        action: '__typescript_function__',
        description: 'Execute TypeScript setup function',
        params: {
          __function: definition.setup,
          __phase: 'setup',
        },
      },
    ];
  }

  // Add verify if present
  if (definition.verify) {
    unified.verify = [
      {
        id: 'verify-0',
        adapter: 'http',
        action: '__typescript_function__',
        description: 'Execute TypeScript verify function',
        params: {
          __function: definition.verify,
          __phase: 'verify',
        },
      },
    ];
  }

  // Add teardown if present
  if (definition.teardown) {
    unified.teardown = [
      {
        id: 'teardown-0',
        adapter: 'http',
        action: '__typescript_function__',
        description: 'Execute TypeScript teardown function',
        params: {
          __function: definition.teardown,
          __phase: 'teardown',
        },
      },
    ];
  }

  return unified;
}

/**
 * Get test metadata from TypeScript file without fully executing
 */
export async function getTSTestMetadata(
  filePath: string
): Promise<{ name: string; priority?: TestPriority; tags?: string[] }> {
  const definition = await loadTSTest(filePath);

  return {
    name: definition.name,
    priority: definition.priority,
    tags: definition.tags,
  };
}

/**
 * Create the e2e function that TypeScript tests use
 * This is a factory function that wraps test definitions with metadata
 */
export function createE2EFunction(): <V extends TSTestVariables>(
  name: string,
  definition: TSTestDefinition<V>
) => TSTestDefinition<V> & { _name: string } {
  return function e2e<V extends TSTestVariables>(
    name: string,
    definition: TSTestDefinition<V>
  ): TSTestDefinition<V> & { _name: string } {
    return {
      ...definition,
      _name: name,
    };
  };
}

/**
 * Check if a file is a valid TypeScript test file
 */
export function isValidTSTestFile(filePath: string): boolean {
  if (!filePath.endsWith('.test.ts')) {
    return false;
  }

  if (!fs.existsSync(filePath)) {
    return false;
  }

  // Quick check: file should contain 'export default' and 'execute'
  try {
    const content = fs.readFileSync(filePath, 'utf8');
    return content.includes('export default') && content.includes('execute');
  } catch {
    return false;
  }
}
