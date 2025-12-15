/**
 * E2E Test Runner - Test Discovery
 *
 * Finds and resolves test files from glob patterns
 */

import * as fs from 'node:fs';
import * as path from 'node:path';

import type { DiscoveredTest, DiscoveryOptions, TestPriority } from '../types';

// ============================================================================
// Constants
// ============================================================================

const DEFAULT_BASE_PATH = '.';
const DEFAULT_PATTERNS = ['**/*.test.yaml', '**/*.test.ts'];
const DEFAULT_EXCLUDE = [
  '**/node_modules/**',
  '**/lib/**',
  '**/fixtures/**',
  '**/schemas/**',
  '**/reports/**',
  '**/__tests__/**',
];

// ============================================================================
// Discovery Functions
// ============================================================================

/**
 * Discover all E2E test files in the specified directory
 */
export async function discoverTests(
  options: DiscoveryOptions = {}
): Promise<DiscoveredTest[]> {
  const basePath = options.basePath || DEFAULT_BASE_PATH;
  const patterns = options.patterns || DEFAULT_PATTERNS;
  const excludePatterns = options.excludePatterns || DEFAULT_EXCLUDE;

  const absoluteBasePath = path.resolve(process.cwd(), basePath);

  if (!fs.existsSync(absoluteBasePath)) {
    return [];
  }

  const files: DiscoveredTest[] = [];

  // Try to use minimatch for glob matching, fall back to simple matching
  let minimatchFn: (path: string, pattern: string) => boolean;
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const minimatchModule = require('minimatch');
    // Handle both { minimatch } export and default export
    minimatchFn = typeof minimatchModule.minimatch === 'function'
      ? minimatchModule.minimatch
      : (typeof minimatchModule === 'function' ? minimatchModule : simpleMatch);
  } catch {
    // Fallback to simple pattern matching
    minimatchFn = simpleMatch;
  }

  walkDirectory(absoluteBasePath, (filePath) => {
    const relativePath = path.relative(absoluteBasePath, filePath);

    // Check exclusions first
    if (excludePatterns.some((pattern) => minimatchFn(relativePath, pattern))) {
      return;
    }

    // Check if matches any include pattern
    const matchesPattern = patterns.some((pattern) =>
      minimatchFn(relativePath, pattern)
    );
    if (!matchesPattern) {
      return;
    }

    const isYAML = filePath.endsWith('.test.yaml');
    const isTS = filePath.endsWith('.test.ts');

    if (isYAML || isTS) {
      files.push({
        filePath,
        name: path.basename(filePath, isYAML ? '.test.yaml' : '.test.ts'),
        type: isYAML ? 'yaml' : 'typescript',
      });
    }
  });

  // Sort by name for consistent ordering
  return files.sort((a, b) => a.name.localeCompare(b.name));
}

/**
 * Filter tests by patterns (glob or exact match)
 */
export async function filterTestsByPatterns(
  tests: DiscoveredTest[],
  patterns: string[]
): Promise<DiscoveredTest[]> {
  if (!patterns || patterns.length === 0) {
    return tests;
  }

  let minimatchFn: (path: string, pattern: string) => boolean;
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const minimatchModule = require('minimatch');
    minimatchFn = typeof minimatchModule.minimatch === 'function'
      ? minimatchModule.minimatch
      : (typeof minimatchModule === 'function' ? minimatchModule : simpleMatch);
  } catch {
    minimatchFn = simpleMatch;
  }

  return tests.filter((test) =>
    patterns.some((pattern) => {
      // Exact name match
      if (test.name === pattern) return true;
      // Glob pattern match on name
      if (minimatchFn(test.name, pattern)) return true;
      // Glob pattern match on full path
      if (minimatchFn(test.filePath, pattern)) return true;
      return false;
    })
  );
}

/**
 * Filter tests by tags
 */
export async function filterTestsByTags(
  tests: DiscoveredTest[],
  tags: string[],
  loadTest: (test: DiscoveredTest) => Promise<{ tags?: string[] }>
): Promise<DiscoveredTest[]> {
  if (!tags || tags.length === 0) {
    return tests;
  }

  const filtered: DiscoveredTest[] = [];

  for (const test of tests) {
    try {
      const definition = await loadTest(test);
      const testTags = definition.tags || [];

      if (tags.some((tag) => testTags.includes(tag))) {
        filtered.push(test);
      }
    } catch {
      // Skip tests that fail to load
    }
  }

  return filtered;
}

/**
 * Filter tests by priority
 */
export async function filterTestsByPriority(
  tests: DiscoveredTest[],
  priorities: TestPriority[],
  loadTest: (test: DiscoveredTest) => Promise<{ priority?: TestPriority }>
): Promise<DiscoveredTest[]> {
  if (!priorities || priorities.length === 0) {
    return tests;
  }

  const filtered: DiscoveredTest[] = [];

  for (const test of tests) {
    try {
      const definition = await loadTest(test);
      const testPriority = definition.priority;

      if (testPriority && priorities.includes(testPriority)) {
        filtered.push(test);
      }
    } catch {
      // Skip tests that fail to load
    }
  }

  return filtered;
}

/**
 * Filter tests by name pattern (grep)
 */
export function filterTestsByGrep(
  tests: DiscoveredTest[],
  pattern: string
): DiscoveredTest[] {
  if (!pattern) {
    return tests;
  }

  const regex = new RegExp(pattern, 'i');
  return tests.filter((test) => regex.test(test.name));
}

/**
 * Categorize a test file by its extension
 */
export function categorizeTestFile(
  filePath: string
): 'yaml' | 'typescript' | 'unknown' {
  if (filePath.endsWith('.test.yaml') || filePath.endsWith('.test.yml')) {
    return 'yaml';
  }
  if (filePath.endsWith('.test.ts')) {
    return 'typescript';
  }
  return 'unknown';
}

/**
 * Get test name from file path
 */
export function getTestNameFromPath(filePath: string): string {
  const basename = path.basename(filePath);
  return basename
    .replace(/\.test\.yaml$/, '')
    .replace(/\.test\.yml$/, '')
    .replace(/\.test\.ts$/, '');
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Walk a directory recursively and call callback for each file
 */
function walkDirectory(
  dir: string,
  callback: (filePath: string) => void
): void {
  if (!fs.existsSync(dir)) {
    return;
  }

  const entries = fs.readdirSync(dir, { withFileTypes: true });

  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);

    if (entry.isDirectory()) {
      walkDirectory(fullPath, callback);
    } else if (entry.isFile()) {
      callback(fullPath);
    }
  }
}

/**
 * Simple pattern matching fallback when minimatch is not available
 */
function simpleMatch(filepath: string, pattern: string): boolean {
  // Convert glob pattern to regex
  const regexPattern = pattern
    .replace(/\*\*/g, '{{GLOBSTAR}}')
    .replace(/\*/g, '[^/]*')
    .replace(/\?/g, '.')
    .replace(/{{GLOBSTAR}}/g, '.*');

  const regex = new RegExp(`^${regexPattern}$`);
  return regex.test(filepath);
}

/**
 * Sort tests by dependency order
 */
export function sortTestsByDependencies(
  tests: DiscoveredTest[],
  getDependencies: (test: DiscoveredTest) => string[]
): DiscoveredTest[] {
  const testMap = new Map(tests.map((t) => [t.name, t]));
  const sorted: DiscoveredTest[] = [];
  const visited = new Set<string>();
  const visiting = new Set<string>();

  function visit(test: DiscoveredTest): void {
    if (visited.has(test.name)) return;
    if (visiting.has(test.name)) {
      throw new Error(`Circular dependency detected involving: ${test.name}`);
    }

    visiting.add(test.name);

    const deps = getDependencies(test);
    for (const depName of deps) {
      const dep = testMap.get(depName);
      if (dep) {
        visit(dep);
      }
    }

    visiting.delete(test.name);
    visited.add(test.name);
    sorted.push(test);
  }

  for (const test of tests) {
    visit(test);
  }

  return sorted;
}

/**
 * Group tests by tags
 */
export function groupTestsByTags(
  tests: DiscoveredTest[],
  getTags: (test: DiscoveredTest) => string[]
): Map<string, DiscoveredTest[]> {
  const groups = new Map<string, DiscoveredTest[]>();

  for (const test of tests) {
    const tags = getTags(test);
    for (const tag of tags) {
      if (!groups.has(tag)) {
        groups.set(tag, []);
      }
      groups.get(tag)!.push(test);
    }
  }

  return groups;
}

/**
 * Group tests by priority
 */
export function groupTestsByPriority(
  tests: DiscoveredTest[],
  getPriority: (test: DiscoveredTest) => TestPriority | undefined
): Map<TestPriority, DiscoveredTest[]> {
  const groups = new Map<TestPriority, DiscoveredTest[]>();
  const priorities: TestPriority[] = ['P0', 'P1', 'P2', 'P3'];

  for (const p of priorities) {
    groups.set(p, []);
  }

  for (const test of tests) {
    const priority = getPriority(test) || 'P3';
    groups.get(priority)!.push(test);
  }

  return groups;
}
