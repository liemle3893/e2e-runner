/**
 * E2E Test Runner - Init Command Templates
 *
 * Template strings for init command file generation
 */

// ============================================================================
// Example YAML Test Template
// ============================================================================

export const EXAMPLE_YAML_TEST = `# Example E2E Test - Gift Rule Creation
name: "TC-EXAMPLE-001 - Create and verify gift rule"
description: "Test creating a gift rule and verifying it in the database"
priority: P1
tags:
  - smoke
  - example
  - gift-rule

variables:
  ruleName: "Example Rule {{$uuid}}"
  giftItemId: "GIFT-001"

setup:
  - adapter: postgresql
    action: execute
    description: "Clean up any existing test rules"
    sql: |
      DELETE FROM gift_rules
      WHERE name LIKE 'Example Rule%'
      AND created_at < NOW() - INTERVAL '1 hour'

execute:
  - adapter: http
    action: request
    description: "Create a new gift rule via API"
    method: POST
    url: "/api/v1/gift-rules"
    headers:
      Content-Type: application/json
      Authorization: "Bearer {{$env(TEST_AUTH_TOKEN)}}"
    body:
      name: "{{ruleName}}"
      giftItemId: "{{giftItemId}}"
      minPurchaseAmount: 100000
      isActive: true
    capture:
      ruleId: "$.data.id"
    assert:
      status: 201
      body:
        "$.success": true
        "$.data.name": "{{ruleName}}"

verify:
  - adapter: postgresql
    action: queryOne
    description: "Verify rule was created in database"
    sql: |
      SELECT * FROM gift_rules WHERE id = $1
    params:
      - "{{ruleId}}"
    assert:
      rowCount: 1
      "name": "{{ruleName}}"
      "is_active": true

teardown:
  - adapter: postgresql
    action: execute
    description: "Clean up test rule"
    sql: |
      DELETE FROM gift_rules WHERE id = $1
    params:
      - "{{ruleId}}"
    continueOnError: true
`

// ============================================================================
// Example TypeScript Test Template
// ============================================================================

export const EXAMPLE_TS_TEST = `/**
 * Example E2E Test - TypeScript DSL
 *
 * Demonstrates how to write E2E tests using TypeScript
 */

import type { TestContext } from '../../scripts/e2e-runner/core';

export const name = 'TC-EXAMPLE-002';

export default {
  priority: 'P1' as const,
  tags: ['smoke', 'example', 'typescript'],
  timeout: 30000,

  variables: {
    testId: \`ts-test-\${Date.now()}\`,
  },

  async setup(ctx: TestContext) {
    ctx.logger.info('Setting up TypeScript test');
    const result = await ctx.adapters.getPostgreSQL().execute({
      action: 'execute',
      sql: 'SELECT 1 as check',
      params: [],
    });
    ctx.capture('setupResult', result);
  },

  async execute(ctx: TestContext) {
    ctx.logger.info('Executing TypeScript test');
    const response = await ctx.adapters.getHTTP().execute({
      action: 'request',
      method: 'GET',
      url: '/api/health',
    });
    ctx.capture('apiResponse', response);
  },

  async verify(ctx: TestContext) {
    const response = ctx.captured.apiResponse as { status: number };
    if (response.status !== 200) {
      throw new Error(\`Expected status 200, got \${response.status}\`);
    }
    ctx.logger.info('Verification passed');
  },

  async teardown(ctx: TestContext) {
    ctx.logger.info('Cleaning up TypeScript test');
  },
};
`

// ============================================================================
// Environment Variables Template
// ============================================================================

export const ENV_EXAMPLE = `# E2E Test Environment Variables
# Copy this to .env.e2e and fill in your values

# PostgreSQL
POSTGRESQL_CONNECTION_STRING=postgresql://user:password@localhost:5432/database

# Redis
REDIS_CONNECTION_STRING=redis://localhost:6379

# MongoDB
MONGODB_CONNECTION_STRING=mongodb://localhost:27017

# Azure EventHub (if using)
EVENTHUB_CONNECTION_STRING=

# Test authentication token
TEST_AUTH_TOKEN=your-test-token-here

# Optional: Override default config path
# E2E_CONFIG=tests/e2e/e2e.config.yaml

# Optional: Default environment
# E2E_ENV=local
`

// ============================================================================
// JSON Schema Templates
// ============================================================================

export const CONFIG_SCHEMA = {
    $schema: 'http://json-schema.org/draft-07/schema#',
    title: 'E2E Config Schema',
    type: 'object',
    required: ['version', 'environments'],
    properties: {
        version: { type: 'string', const: '1.0' },
        environments: {
            type: 'object',
            additionalProperties: {
                type: 'object',
                required: ['baseUrl'],
                properties: {
                    baseUrl: { type: 'string', format: 'uri' },
                    adapters: {
                        type: 'object',
                        properties: {
                            postgresql: { $ref: '#/definitions/postgresqlConfig' },
                            redis: { $ref: '#/definitions/redisConfig' },
                            mongodb: { $ref: '#/definitions/mongodbConfig' },
                            eventhub: { $ref: '#/definitions/eventhubConfig' },
                        },
                    },
                },
            },
        },
        defaults: { $ref: '#/definitions/defaults' },
        variables: { type: 'object' },
        reporters: { type: 'array', items: { $ref: '#/definitions/reporter' } },
    },
    definitions: {
        postgresqlConfig: {
            type: 'object',
            required: ['connectionString'],
            properties: {
                connectionString: { type: 'string' },
                schema: { type: 'string' },
                poolSize: { type: 'number', minimum: 1, maximum: 20 },
            },
        },
        redisConfig: {
            type: 'object',
            required: ['connectionString'],
            properties: {
                connectionString: { type: 'string' },
                db: { type: 'number' },
                keyPrefix: { type: 'string' },
            },
        },
        mongodbConfig: {
            type: 'object',
            required: ['connectionString'],
            properties: { connectionString: { type: 'string' }, database: { type: 'string' } },
        },
        eventhubConfig: {
            type: 'object',
            required: ['connectionString'],
            properties: { connectionString: { type: 'string' }, consumerGroup: { type: 'string' } },
        },
        defaults: {
            type: 'object',
            properties: {
                timeout: { type: 'number', minimum: 1000 },
                retries: { type: 'number', minimum: 0 },
                retryDelay: { type: 'number', minimum: 100 },
                parallel: { type: 'number', minimum: 1 },
            },
        },
        reporter: {
            type: 'object',
            required: ['type'],
            properties: {
                type: { type: 'string', enum: ['console', 'junit', 'html', 'json'] },
                output: { type: 'string' },
                verbose: { type: 'boolean' },
            },
        },
    },
}

export const TEST_SCHEMA = {
    $schema: 'http://json-schema.org/draft-07/schema#',
    title: 'E2E Test Schema',
    type: 'object',
    required: ['name', 'execute'],
    properties: {
        name: { type: 'string', minLength: 1 },
        description: { type: 'string' },
        priority: { type: 'string', enum: ['P0', 'P1', 'P2', 'P3'] },
        tags: { type: 'array', items: { type: 'string' } },
        skip: { type: 'boolean' },
        skipReason: { type: 'string' },
        timeout: { type: 'number', minimum: 1000 },
        retries: { type: 'number', minimum: 0, maximum: 5 },
        depends: { type: 'array', items: { type: 'string' } },
        variables: { type: 'object' },
        setup: { type: 'array', items: { $ref: '#/definitions/step' } },
        execute: { type: 'array', items: { $ref: '#/definitions/step' }, minItems: 1 },
        verify: { type: 'array', items: { $ref: '#/definitions/step' } },
        teardown: { type: 'array', items: { $ref: '#/definitions/step' } },
    },
    definitions: {
        step: {
            type: 'object',
            required: ['adapter', 'action'],
            properties: {
                adapter: {
                    type: 'string',
                    enum: ['http', 'postgresql', 'redis', 'mongodb', 'eventhub'],
                },
                action: { type: 'string' },
                description: { type: 'string' },
                continueOnError: { type: 'boolean' },
                retry: { type: 'number' },
                delay: { type: 'number' },
            },
        },
    },
}
