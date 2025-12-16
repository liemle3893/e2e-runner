/**
 * E2E Test Runner - Test Command
 *
 * Create new test files from templates
 */

import * as fs from 'node:fs'
import * as path from 'node:path'

import { loadConfig } from '../core/config-loader'
import { ConfigurationError } from '../errors'
import type { CLIOptions } from '../types'
import { EXIT_CODES } from '../utils/exit-codes'
import { createLogger, type LogLevel } from '../utils/logger'
import { printError, printSuccess } from './index'

// ============================================================================
// Types
// ============================================================================

export type TemplateType = 'api' | 'crud' | 'integration' | 'event-driven' | 'db-verification'

export interface TestCreateOptions {
    name: string
    template: TemplateType
    description?: string
    output?: string
    priority?: string
    tags?: string
    config?: string
    env?: string
    verbose?: boolean
    quiet?: boolean
    noColor?: boolean
}

export interface TestCommandResult {
    exitCode: number
    filePath?: string
}

// ============================================================================
// Templates
// ============================================================================

const TEMPLATES: Record<TemplateType, string> = {
    api: `name: "{{name}}"
description: "{{description}}"
priority: {{priority}}
tags: [{{tags}}]

variables:
  unique_id: "{{$uuid()}}"

execute:
  - adapter: http
    action: request
    method: GET
    url: "{{baseUrl}}/endpoint"
    headers:
      Content-Type: "application/json"
    capture:
      response_id: "$.id"
    assert:
      status: 200
      json:
        - path: "$.id"
          exists: true
`,

    crud: `name: "{{name}}"
description: "{{description}}"
priority: {{priority}}
tags: [{{tags}}]

variables:
  unique_id: "{{$uuid()}}"

setup:
  - adapter: postgresql
    action: execute
    sql: "DELETE FROM resources WHERE name LIKE 'test-%'"
    continueOnError: true

execute:
  # Create
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/resources"
    headers:
      Content-Type: "application/json"
    body:
      name: "test-{{$uuid()}}"
    capture:
      resource_id: "$.id"
    assert:
      status: 201
      json:
        - path: "$.id"
          exists: true

  # Read
  - adapter: http
    action: request
    method: GET
    url: "{{baseUrl}}/resources/{{captured.resource_id}}"
    assert:
      status: 200

  # Update
  - adapter: http
    action: request
    method: PUT
    url: "{{baseUrl}}/resources/{{captured.resource_id}}"
    body:
      name: "updated-{{$uuid()}}"
    assert:
      status: 200

  # Delete
  - adapter: http
    action: request
    method: DELETE
    url: "{{baseUrl}}/resources/{{captured.resource_id}}"
    assert:
      status: 204

verify:
  - adapter: postgresql
    action: queryOne
    sql: "SELECT COUNT(*) as count FROM resources WHERE id = $1"
    params: ["{{captured.resource_id}}"]
    assert:
      - column: count
        equals: 0

teardown:
  - adapter: http
    action: request
    method: DELETE
    url: "{{baseUrl}}/resources/{{captured.resource_id}}"
    continueOnError: true
`,

    integration: `name: "{{name}}"
description: "{{description}}"
priority: {{priority}}
tags: [{{tags}}]

variables:
  unique_id: "{{$uuid()}}"
  test_email: "test-{{$uuid()}}@example.com"

setup:
  # Clean PostgreSQL
  - adapter: postgresql
    action: execute
    sql: "DELETE FROM users WHERE email LIKE 'test-%@example.com'"
    continueOnError: true

  # Clean Redis cache
  - adapter: redis
    action: flushPattern
    pattern: "user:test-*"
    continueOnError: true

  # Clean MongoDB
  - adapter: mongodb
    action: deleteMany
    collection: "user_logs"
    filter:
      email:
        $regex: "^test-"
    continueOnError: true

execute:
  # Create user via HTTP (stored in PostgreSQL)
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/users"
    headers:
      Content-Type: "application/json"
    body:
      email: "{{test_email}}"
      name: "Test User"
    capture:
      user_id: "$.id"
    assert:
      status: 201

  # Cache user in Redis via HTTP
  - adapter: http
    action: request
    method: PUT
    url: "{{baseUrl}}/cache/user:{{captured.user_id}}"
    body:
      email: "{{test_email}}"
      name: "Test User"
    assert:
      status: 200

  # Log action to MongoDB via HTTP
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/logs"
    body:
      userId: "{{captured.user_id}}"
      action: "created"
      email: "{{test_email}}"
    assert:
      status: 201

verify:
  # Verify PostgreSQL
  - adapter: postgresql
    action: queryOne
    sql: "SELECT * FROM users WHERE id = $1"
    params: ["{{captured.user_id}}"]
    assert:
      - column: email
        equals: "{{test_email}}"

  # Verify Redis cache
  - adapter: redis
    action: get
    key: "user:{{captured.user_id}}"
    assert:
      isNotNull: true
      contains: "{{test_email}}"

  # Verify MongoDB log
  - adapter: mongodb
    action: findOne
    collection: "user_logs"
    filter:
      userId: "{{captured.user_id}}"
    assert:
      - path: "action"
        equals: "created"

teardown:
  - adapter: postgresql
    action: execute
    sql: "DELETE FROM users WHERE id = $1"
    params: ["{{captured.user_id}}"]
    continueOnError: true

  - adapter: redis
    action: del
    key: "user:{{captured.user_id}}"
    continueOnError: true

  - adapter: mongodb
    action: deleteOne
    collection: "user_logs"
    filter:
      userId: "{{captured.user_id}}"
    continueOnError: true
`,

    'event-driven': `name: "{{name}}"
description: "{{description}}"
priority: {{priority}}
tags: [{{tags}}]

variables:
  unique_id: "{{$uuid()}}"
  event_type: "user.created"

setup:
  # Clear any pending events from previous runs
  - adapter: eventhub
    action: clear
    topic: "events"

execute:
  # Trigger an action that publishes an event
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/users"
    headers:
      Content-Type: "application/json"
    body:
      email: "test-{{$uuid()}}@example.com"
      name: "Test User"
    capture:
      user_id: "$.id"
    assert:
      status: 201

verify:
  # Wait for and verify the event was published
  - adapter: eventhub
    action: waitFor
    topic: "events"
    timeout: 10000
    filter:
      type: "{{event_type}}"
      data.userId: "{{captured.user_id}}"
    capture:
      event_data: "data"
    assert:
      - path: "type"
        equals: "{{event_type}}"
      - path: "data.userId"
        equals: "{{captured.user_id}}"

teardown:
  - adapter: http
    action: request
    method: DELETE
    url: "{{baseUrl}}/users/{{captured.user_id}}"
    continueOnError: true

  - adapter: eventhub
    action: clear
    topic: "events"
`,

    'db-verification': `name: "{{name}}"
description: "{{description}}"
priority: {{priority}}
tags: [{{tags}}]

variables:
  unique_id: "{{$uuid()}}"
  test_email: "test-{{$uuid()}}@example.com"

setup:
  - adapter: postgresql
    action: execute
    sql: "DELETE FROM users WHERE email LIKE 'test-%@example.com'"
    continueOnError: true

execute:
  # Create user via HTTP
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/users"
    headers:
      Content-Type: "application/json"
    body:
      email: "{{test_email}}"
      name: "Test User"
      status: "active"
    capture:
      user_id: "$.id"
    assert:
      status: 201

verify:
  # Direct database verification - single record
  - adapter: postgresql
    action: queryOne
    sql: "SELECT * FROM users WHERE id = $1"
    params: ["{{captured.user_id}}"]
    assert:
      - column: email
        equals: "{{test_email}}"
      - column: name
        equals: "Test User"
      - column: status
        equals: "active"
      - column: deleted_at
        isNull: true
      - column: created_at
        isNotNull: true

  # Count verification
  - adapter: postgresql
    action: count
    sql: "SELECT COUNT(*) as count FROM users WHERE email = $1"
    params: ["{{test_email}}"]
    assert:
      - column: count
        equals: 1

  # Query multiple records (if applicable)
  - adapter: postgresql
    action: query
    sql: "SELECT id, email, status FROM users WHERE email LIKE $1 ORDER BY created_at DESC LIMIT 5"
    params: ["test-%@example.com"]
    assert:
      - row: 0
        column: email
        equals: "{{test_email}}"

teardown:
  - adapter: postgresql
    action: execute
    sql: "DELETE FROM users WHERE id = $1"
    params: ["{{captured.user_id}}"]
    continueOnError: true
`,
}

// ============================================================================
// Test Create Command
// ============================================================================

/**
 * Execute the test create command
 */
export async function testCreateCommand(
    options: TestCreateOptions
): Promise<TestCommandResult> {
    const logLevel: LogLevel = options.quiet ? 'error' : options.verbose ? 'debug' : 'info'
    const logger = createLogger({
        level: logLevel,
        useColors: !options.noColor,
    })

    const { name, template, description, priority = 'P0', tags = 'e2e' } = options

    // Validate template type
    if (!TEMPLATES[template]) {
        const availableTemplates = Object.keys(TEMPLATES).join(', ')
        printError(
            `Unknown template type: ${template}`,
            `Available templates: ${availableTemplates}`
        )
        return { exitCode: EXIT_CODES.VALIDATION_ERROR }
    }

    // Determine output directory
    let outputDir = options.output
    if (!outputDir) {
        // Try to get testDir from config
        try {
            const config = await loadConfig({
                configPath: options.config || 'e2e.config.yaml',
                environment: options.env || 'local',
            })
            outputDir = config.testDir
        } catch {
            // Config not found, use current directory
            outputDir = '.'
        }
    }

    // Create output directory if it doesn't exist
    const absoluteOutputDir = path.resolve(process.cwd(), outputDir)
    if (!fs.existsSync(absoluteOutputDir)) {
        fs.mkdirSync(absoluteOutputDir, { recursive: true })
        logger.debug(`Created directory: ${absoluteOutputDir}`)
    }

    // Generate filename from test name
    const fileName = `${name}.test.yaml`
    const filePath = path.join(absoluteOutputDir, fileName)

    // Check if file already exists
    if (fs.existsSync(filePath)) {
        printError(
            `Test file already exists: ${filePath}`,
            'Choose a different name or delete the existing file'
        )
        return { exitCode: EXIT_CODES.VALIDATION_ERROR }
    }

    // Generate test content from template
    const testDescription = description || `E2E test for ${name}`
    const content = TEMPLATES[template]
        .replace(/\{\{name\}\}/g, name)
        .replace(/\{\{description\}\}/g, testDescription)
        .replace(/\{\{priority\}\}/g, priority)
        .replace(/\{\{tags\}\}/g, tags)

    // Write the file
    try {
        fs.writeFileSync(filePath, content, 'utf8')
        logger.info(`Created test file: ${filePath}`)
        printSuccess(`Created ${template} test: ${path.relative(process.cwd(), filePath)}`)
        return { exitCode: EXIT_CODES.SUCCESS, filePath }
    } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        printError(`Failed to create test file: ${message}`)
        return { exitCode: EXIT_CODES.FATAL }
    }
}

/**
 * List available templates
 */
export function listTemplates(): void {
    console.log('\nAvailable templates:\n')
    console.log('  api             Simple API test (GET/POST with assertions)')
    console.log('  crud            Full CRUD operations with DB verification')
    console.log('  integration     Multi-adapter test (HTTP + PostgreSQL + Redis + MongoDB)')
    console.log('  event-driven    EventHub publish/consume pattern')
    console.log('  db-verification Direct database assertion patterns')
    console.log()
}
