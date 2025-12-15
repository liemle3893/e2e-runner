# E2E Test Runner

A powerful, flexible end-to-end testing framework for API and database testing. Write tests in YAML or TypeScript, validate against multiple data stores, and generate comprehensive reports.

## Features

- **Multi-format tests**: Write tests in YAML for simplicity or TypeScript for full control
- **Database adapters**: Built-in support for PostgreSQL, MongoDB, Redis, and Azure EventHub
- **HTTP testing**: Native HTTP adapter for REST API testing with JSONPath assertions
- **Parallel execution**: Run tests concurrently with configurable parallelism
- **Multiple reporters**: Console, JUnit XML, HTML, and JSON output formats
- **Variable interpolation**: Dynamic values with built-in functions (`${uuid()}`, `${now()}`, etc.)
- **Flexible filtering**: Filter tests by tags, priority, or name patterns

## Installation

### Global Installation (recommended for CLI usage)

```bash
npm install -g @liemle3893/e2e-runner
```

### Local Installation (for project integration)

```bash
npm install --save-dev @liemle3893/e2e-runner
```

### Using npx (no installation)

```bash
npx @liemle3893/e2e-runner --help
npx @liemle3893/e2e-runner run --config ./e2e.config.yaml
```

### Optional Adapters

Install database adapters as needed:

```bash
# PostgreSQL
npm install pg

# MongoDB
npm install mongodb

# Redis
npm install ioredis

# Azure EventHub
npm install @azure/event-hubs
```

## Quick Start

### 1. Initialize Configuration

```bash
e2e init
```

This creates `e2e.config.yaml` in your current directory.

### 2. Create a Test

Create `my-api.test.yaml`:

```yaml
name: Get Users API
description: Test the users endpoint
tags: [smoke, api]
priority: P1

execute:
  - id: get-users
    adapter: http
    action: GET
    params:
      url: /api/users
    assertions:
      - path: $.status
        operator: equals
        expected: 200
      - path: $.body.data
        operator: isArray
```

### 3. Run Tests

```bash
e2e run
```

## CLI Commands

### `e2e run`

Execute E2E tests.

```bash
# Run all tests
e2e run

# Run with specific config
e2e run -c ./config/e2e.config.yaml

# Run tests from a specific directory
e2e run -d ./tests/integration

# Filter by tags
e2e run --tag smoke --tag regression

# Filter by priority
e2e run --priority P0 --priority P1

# Filter by name pattern
e2e run -g "user*"

# Parallel execution
e2e run -p 4

# Stop on first failure
e2e run --bail

# Dry run (show what would run)
e2e run --dry-run
```

### `e2e validate`

Validate test file syntax without running.

```bash
e2e validate
e2e validate -d ./tests
```

### `e2e list`

List discovered tests.

```bash
e2e list
e2e list --tag smoke
```

### `e2e health`

Check adapter connectivity.

```bash
e2e health
e2e health --adapter postgresql
```

### `e2e init`

Initialize configuration file.

```bash
e2e init
```

## CLI Options

| Option | Short | Description | Default |
|--------|-------|-------------|---------|
| `--config` | `-c` | Config file path | `e2e.config.yaml` |
| `--env` | `-e` | Environment name | `local` |
| `--test-dir` | `-d` | Test directory | `.` (current) |
| `--report-dir` | | Report output directory | `./reports` |
| `--verbose` | `-v` | Verbose output | `false` |
| `--quiet` | `-q` | Errors only | `false` |
| `--parallel` | `-p` | Parallel test count | `1` |
| `--timeout` | `-t` | Timeout in ms | `30000` |
| `--retries` | `-r` | Retry count | `0` |
| `--bail` | | Stop on first failure | `false` |
| `--grep` | `-g` | Filter by name pattern | |
| `--tag` | | Filter by tag (repeatable) | |
| `--priority` | | Filter by P0/P1/P2/P3 | |
| `--reporter` | | Reporter type (repeatable) | `console` |
| `--output` | `-o` | Report output path | |
| `--dry-run` | | Show tests without running | `false` |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `E2E_CONFIG` | Config file path |
| `E2E_ENV` | Environment name |
| `E2E_TEST_DIR` | Test directory |
| `E2E_REPORT_DIR` | Report output directory |
| `E2E_VERBOSE` | Enable verbose output (`1` or `true`) |
| `NO_COLOR` | Disable colored output (`1` or `true`) |

## Configuration File

```yaml
version: "1.0"

environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      postgresql:
        connectionString: "${POSTGRESQL_CONNECTION_STRING}"
      mongodb:
        connectionString: "${MONGODB_CONNECTION_STRING}"
      redis:
        connectionString: "${REDIS_CONNECTION_STRING}"

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
```

## Writing Tests

### YAML Format

```yaml
name: Create User Flow
description: Test user creation and retrieval
tags: [integration, users]
priority: P1
timeout: 60000

variables:
  testEmail: "test-${uuid()}@example.com"

setup:
  - id: cleanup-existing
    adapter: postgresql
    action: exec
    params:
      query: "DELETE FROM users WHERE email LIKE 'test-%@example.com'"

execute:
  - id: create-user
    adapter: http
    action: POST
    params:
      url: /api/users
      body:
        email: "${testEmail}"
        name: "Test User"
    capture:
      userId: $.body.id
    assertions:
      - path: $.status
        operator: equals
        expected: 201

  - id: get-user
    adapter: http
    action: GET
    params:
      url: "/api/users/${captured.userId}"
    assertions:
      - path: $.body.email
        operator: equals
        expected: "${testEmail}"

verify:
  - id: check-database
    adapter: postgresql
    action: query
    params:
      query: "SELECT * FROM users WHERE id = $1"
      params: ["${captured.userId}"]
    assertions:
      - path: $.rows[0].email
        operator: equals
        expected: "${testEmail}"

teardown:
  - id: delete-user
    adapter: postgresql
    action: exec
    params:
      query: "DELETE FROM users WHERE id = $1"
      params: ["${captured.userId}"]
```

### TypeScript Format

```typescript
// users.test.ts
import type { TestDefinition } from '@liemle3893/e2e-runner';

export default {
  name: 'Create User Flow',
  tags: ['integration', 'users'],
  priority: 'P1',

  async execute(ctx) {
    // Create user via API
    const createRes = await ctx.http.post('/api/users', {
      email: `test-${ctx.uuid()}@example.com`,
      name: 'Test User',
    });

    ctx.expect(createRes.status).toBe(201);
    ctx.capture('userId', createRes.body.id);

    // Verify in database
    const dbResult = await ctx.postgresql.query(
      'SELECT * FROM users WHERE id = $1',
      [ctx.captured.userId]
    );

    ctx.expect(dbResult.rows).toHaveLength(1);
    ctx.expect(dbResult.rows[0].email).toContain('test-');
  },

  async teardown(ctx) {
    await ctx.postgresql.exec(
      'DELETE FROM users WHERE id = $1',
      [ctx.captured.userId]
    );
  },
} as TestDefinition;
```

## Variable Interpolation

### Built-in Functions

| Function | Description | Example |
|----------|-------------|---------|
| `${uuid()}` | Generate UUID v4 | `550e8400-e29b-41d4-a716-446655440000` |
| `${now()}` | Current timestamp (ISO) | `2024-01-15T10:30:00.000Z` |
| `${timestamp()}` | Unix timestamp (ms) | `1705315800000` |
| `${random()}` | Random number 0-1 | `0.7234` |
| `${randomInt(min, max)}` | Random integer | `42` |

### Environment Variables

Access environment variables with `${env.VAR_NAME}`:

```yaml
params:
  apiKey: "${env.API_KEY}"
```

### Captured Values

Access values captured from previous steps:

```yaml
params:
  url: "/api/users/${captured.userId}"
```

## Adapters

### HTTP Adapter

```yaml
- adapter: http
  action: POST  # GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS
  params:
    url: /api/endpoint
    headers:
      Authorization: "Bearer ${token}"
    body:
      key: value
    timeout: 5000
```

### PostgreSQL Adapter

```yaml
- adapter: postgresql
  action: query  # or 'exec'
  params:
    query: "SELECT * FROM users WHERE id = $1"
    params: [123]
```

### MongoDB Adapter

```yaml
- adapter: mongodb
  action: find  # insertOne, updateOne, deleteOne, find, etc.
  params:
    collection: users
    filter:
      email: "test@example.com"
```

### Redis Adapter

```yaml
- adapter: redis
  action: get  # set, del, hgetall, etc.
  params:
    key: "user:123"
```

## Assertions

| Operator | Description |
|----------|-------------|
| `equals` | Strict equality |
| `notEquals` | Not equal |
| `contains` | String/array contains |
| `notContains` | Does not contain |
| `greaterThan` | Numeric > |
| `lessThan` | Numeric < |
| `matches` | Regex match |
| `isArray` | Is array type |
| `isObject` | Is object type |
| `hasLength` | Array/string length |
| `hasProperty` | Object has property |

## Reporters

### Console Reporter

Default output to terminal with colors and progress.

### JUnit Reporter

XML output for CI/CD integration:

```yaml
reporters:
  - type: junit
    output: "./reports/junit.xml"
```

### HTML Reporter

Interactive HTML report:

```yaml
reporters:
  - type: html
    output: "./reports/report.html"
```

### JSON Reporter

Machine-readable JSON:

```yaml
reporters:
  - type: json
    output: "./reports/results.json"
```

## Programmatic API

```typescript
import { runTests, validateTests, listTests, checkHealth } from '@liemle3893/e2e-runner';

// Run tests
const result = await runTests({
  config: './e2e.config.yaml',
  testDir: './tests',
  tag: ['smoke'],
  parallel: 4,
});

console.log(`Passed: ${result.passed}, Failed: ${result.failed}`);

// Validate test syntax
const validation = await validateTests({ testDir: './tests' });
if (!validation.valid) {
  console.error('Validation errors:', validation.errors);
}

// List available tests
const tests = await listTests({ tag: ['integration'] });
tests.forEach(t => console.log(`- ${t.name} (${t.tags.join(', ')})`));

// Check adapter health
const health = await checkHealth();
console.log('All adapters healthy:', health.healthy);
```

## License

MIT
