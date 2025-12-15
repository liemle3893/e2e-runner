/**
 * E2E Test Runner - YAML Test Loader
 *
 * Parses and validates YAML test files
 */

import * as fs from 'node:fs';
import * as path from 'node:path';

import { LoaderError, ValidationError } from '../errors';
import { TEST_SCHEMA } from '../cli/init-templates';
import type {
  AdapterType,
  DiscoveredTest,
  Logger,
  TestPriority,
  UnifiedStep,
  UnifiedTestDefinition,
} from '../types';

// ============================================================================
// Types
// ============================================================================

interface RawYAMLTest {
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
  setup?: RawYAMLStep[];
  execute: RawYAMLStep[];
  verify?: RawYAMLStep[];
  teardown?: RawYAMLStep[];
}

interface RawYAMLStep {
  adapter: AdapterType;
  action: string;
  description?: string;
  continueOnError?: boolean;
  retry?: number;
  delay?: number;
  [key: string]: unknown;
}

// ============================================================================
// Schema Definition (inline for validation)
// ============================================================================

const VALID_ADAPTERS: AdapterType[] = ['postgresql', 'redis', 'mongodb', 'eventhub', 'http'];
const VALID_PRIORITIES: TestPriority[] = ['P0', 'P1', 'P2', 'P3'];

// ============================================================================
// Loader Functions
// ============================================================================

/**
 * Load a single YAML test file
 */
export async function loadYAMLTest(
  filePath: string,
  logger?: Logger
): Promise<UnifiedTestDefinition> {
  logger?.debug(`Loading YAML test: ${filePath}`);

  if (!fs.existsSync(filePath)) {
    throw new LoaderError('yaml', filePath, `File not found: ${filePath}`);
  }

  let content: string;
  try {
    content = fs.readFileSync(filePath, 'utf8');
  } catch (error) {
    throw new LoaderError(
      'yaml',
      filePath,
      `Failed to read file: ${error instanceof Error ? error.message : String(error)}`
    );
  }

  let raw: RawYAMLTest;
  try {
    const yaml = await import('yaml');
    raw = yaml.parse(content) as RawYAMLTest;
  } catch (error) {
    throw new LoaderError(
      'yaml',
      filePath,
      `Failed to parse YAML: ${error instanceof Error ? error.message : String(error)}`
    );
  }

  // Validate the parsed YAML
  validateRawTest(raw, filePath);

  // Convert to unified format
  return convertToUnified(raw, filePath);
}

/**
 * Load multiple YAML test files
 */
export async function loadYAMLTests(
  tests: DiscoveredTest[],
  logger?: Logger
): Promise<UnifiedTestDefinition[]> {
  const results: UnifiedTestDefinition[] = [];
  const errors: string[] = [];

  for (const test of tests) {
    if (test.type !== 'yaml') {
      continue;
    }

    try {
      const definition = await loadYAMLTest(test.filePath, logger);
      results.push(definition);
    } catch (error) {
      errors.push(
        `${test.name}: ${error instanceof Error ? error.message : String(error)}`
      );
    }
  }

  if (errors.length > 0) {
    throw new LoaderError('yaml', 'multiple', `Failed to load ${errors.length} test(s):\n${errors.join('\n')}`);
  }

  return results;
}

/**
 * Validate a raw YAML test structure
 */
function validateRawTest(raw: RawYAMLTest, filePath: string): void {
  const errors: string[] = [];

  // Required fields
  if (!raw.name || typeof raw.name !== 'string') {
    errors.push('Missing or invalid "name" field');
  }

  if (!raw.execute || !Array.isArray(raw.execute) || raw.execute.length === 0) {
    errors.push('Missing or empty "execute" array');
  }

  // Validate priority if present
  if (raw.priority && !VALID_PRIORITIES.includes(raw.priority)) {
    errors.push(`Invalid priority "${raw.priority}". Must be one of: ${VALID_PRIORITIES.join(', ')}`);
  }

  // Validate tags if present
  if (raw.tags && !Array.isArray(raw.tags)) {
    errors.push('"tags" must be an array');
  }

  // Validate timeout if present
  if (raw.timeout !== undefined) {
    if (typeof raw.timeout !== 'number' || raw.timeout < 1000 || raw.timeout > 300000) {
      errors.push('"timeout" must be a number between 1000 and 300000');
    }
  }

  // Validate retries if present
  if (raw.retries !== undefined) {
    if (typeof raw.retries !== 'number' || raw.retries < 0 || raw.retries > 5) {
      errors.push('"retries" must be a number between 0 and 5');
    }
  }

  // Validate phases
  const phases = ['setup', 'execute', 'verify', 'teardown'] as const;
  for (const phase of phases) {
    const steps = raw[phase];
    if (steps) {
      if (!Array.isArray(steps)) {
        errors.push(`"${phase}" must be an array`);
        continue;
      }

      for (let i = 0; i < steps.length; i++) {
        const step = steps[i];
        const stepErrors = validateStep(step, `${phase}[${i}]`);
        errors.push(...stepErrors);
      }
    }
  }

  if (errors.length > 0) {
    throw new ValidationError(
      `Invalid YAML test file: ${filePath}\n${errors.map((e) => `  - ${e}`).join('\n')}`,
      errors.map((e) => ({ path: '/', message: e, keyword: 'custom' }))
    );
  }
}

/**
 * Validate a single step
 */
function validateStep(step: RawYAMLStep, location: string): string[] {
  const errors: string[] = [];

  if (!step.adapter) {
    errors.push(`${location}: Missing "adapter" field`);
  } else if (!VALID_ADAPTERS.includes(step.adapter)) {
    errors.push(
      `${location}: Invalid adapter "${step.adapter}". Must be one of: ${VALID_ADAPTERS.join(', ')}`
    );
  }

  if (!step.action) {
    errors.push(`${location}: Missing "action" field`);
  }

  // Adapter-specific validation
  if (step.adapter && step.action) {
    const adapterErrors = validateAdapterStep(step, location);
    errors.push(...adapterErrors);
  }

  return errors;
}

/**
 * Validate adapter-specific step fields
 */
function validateAdapterStep(step: RawYAMLStep, location: string): string[] {
  const errors: string[] = [];

  switch (step.adapter) {
    case 'postgresql':
      if (!['execute', 'query', 'queryOne', 'count'].includes(step.action)) {
        errors.push(
          `${location}: Invalid PostgreSQL action "${step.action}". Must be: execute, query, queryOne, count`
        );
      }
      if (!step.sql) {
        errors.push(`${location}: PostgreSQL steps require "sql" field`);
      }
      break;

    case 'redis':
      if (
        !['get', 'set', 'del', 'exists', 'incr', 'hget', 'hset', 'hgetall', 'keys', 'flushPattern'].includes(
          step.action
        )
      ) {
        errors.push(`${location}: Invalid Redis action "${step.action}"`);
      }
      if (['get', 'set', 'del', 'exists', 'incr', 'hget', 'hset', 'hgetall'].includes(step.action) && !step.key) {
        errors.push(`${location}: Redis action "${step.action}" requires "key" field`);
      }
      if (['keys', 'flushPattern'].includes(step.action) && !step.pattern) {
        errors.push(`${location}: Redis action "${step.action}" requires "pattern" field`);
      }
      break;

    case 'mongodb':
      if (
        !['insertOne', 'insertMany', 'findOne', 'find', 'updateOne', 'updateMany', 'deleteOne', 'deleteMany', 'count', 'aggregate'].includes(
          step.action
        )
      ) {
        errors.push(`${location}: Invalid MongoDB action "${step.action}"`);
      }
      if (!step.collection) {
        errors.push(`${location}: MongoDB steps require "collection" field`);
      }
      break;

    case 'eventhub':
      if (!['publish', 'waitFor', 'consume', 'clear'].includes(step.action)) {
        errors.push(`${location}: Invalid EventHub action "${step.action}"`);
      }
      if (['publish', 'waitFor', 'consume'].includes(step.action) && !step.topic) {
        errors.push(`${location}: EventHub action "${step.action}" requires "topic" field`);
      }
      break;

    case 'http':
      if (step.action !== 'request') {
        errors.push(`${location}: Invalid HTTP action "${step.action}". Must be "request"`);
      }
      if (!step.url) {
        errors.push(`${location}: HTTP steps require "url" field`);
      }
      break;
  }

  return errors;
}

/**
 * Convert raw YAML to unified test definition
 */
function convertToUnified(raw: RawYAMLTest, filePath: string): UnifiedTestDefinition {
  return {
    name: raw.name,
    description: raw.description,
    priority: raw.priority,
    tags: raw.tags,
    skip: raw.skip,
    skipReason: raw.skipReason,
    timeout: raw.timeout,
    retries: raw.retries,
    depends: raw.depends,
    variables: raw.variables,
    setup: raw.setup?.map((s, i) => convertStep(s, 'setup', i)),
    execute: raw.execute.map((s, i) => convertStep(s, 'execute', i)),
    verify: raw.verify?.map((s, i) => convertStep(s, 'verify', i)),
    teardown: raw.teardown?.map((s, i) => convertStep(s, 'teardown', i)),
    sourceFile: path.resolve(filePath),
    sourceType: 'yaml',
  };
}

/**
 * Convert a raw step to unified step format
 */
function convertStep(raw: RawYAMLStep, phase: string, index: number): UnifiedStep {
  const { adapter, action, description, continueOnError, retry, delay, ...rest } = raw;

  // Extract capture and assert from rest, everything else goes to params
  const { capture, assert, ...params } = rest;

  return {
    id: `${phase}-${index}`,
    adapter,
    action,
    description,
    params: params as Record<string, unknown>,
    capture: normalizeCapture(capture),
    assert,
    continueOnError,
    retry,
    delay,
  };
}

/**
 * Normalize capture field to consistent format
 */
function normalizeCapture(capture: unknown): Record<string, string> | undefined {
  if (!capture) {
    return undefined;
  }

  // If it's a string (for simple adapters like Redis), wrap it
  if (typeof capture === 'string') {
    return { value: capture };
  }

  // If it's an object, return as-is
  if (typeof capture === 'object') {
    return capture as Record<string, string>;
  }

  return undefined;
}

/**
 * Validate YAML test file against JSON schema (using embedded schema)
 */
export async function validateYAMLWithSchema(
  filePath: string
): Promise<{ valid: boolean; errors: string[] }> {
  try {
    const Ajv = (await import('ajv')).default;
    const yaml = await import('yaml');

    const ajv = new Ajv({ allErrors: true });
    const content = yaml.parse(fs.readFileSync(filePath, 'utf8'));

    // Use embedded schema from init-templates
    const validate = ajv.compile(TEST_SCHEMA);
    const valid = validate(content);

    if (!valid && validate.errors) {
      const errors = validate.errors.map((e) => {
        const errorPath = (e as { instancePath?: string }).instancePath || '/';
        return `${errorPath}: ${e.message}`;
      });
      return { valid: false, errors };
    }

    return { valid: true, errors: [] };
  } catch (error) {
    return {
      valid: false,
      errors: [`Schema validation failed: ${error instanceof Error ? error.message : String(error)}`],
    };
  }
}

/**
 * Get test metadata from YAML file without fully loading
 */
export async function getYAMLTestMetadata(
  filePath: string
): Promise<{ name: string; priority?: TestPriority; tags?: string[] }> {
  const content = fs.readFileSync(filePath, 'utf8');
  const yaml = await import('yaml');
  const raw = yaml.parse(content) as RawYAMLTest;

  return {
    name: raw.name,
    priority: raw.priority,
    tags: raw.tags,
  };
}
