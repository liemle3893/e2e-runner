# E2E Test Runner Documentation

A powerful, YAML-based end-to-end testing framework with multi-adapter support for testing APIs and databases.

## Table of Contents

1. [Getting Started](./01-getting-started.md) - Installation and first test
2. [Configuration](./02-configuration.md) - Config file reference
3. [YAML Tests](./03-yaml-tests.md) - Test file syntax
4. [Adapters](./04-adapters.md) - HTTP, PostgreSQL, Redis, MongoDB, EventHub
5. [Assertions](./05-assertions.md) - All assertion operators
6. [CLI Reference](./06-cli-reference.md) - Command line options
7. [Built-in Functions](./07-built-in-functions.md) - Dynamic value generation

## Quick Start

### Installation

```bash
npm install @liemle3893/e2e-runner
```

### Initialize Configuration

```bash
npx e2e init
```

### Create Your First Test

Create `tests/e2e/hello.test.yaml`:

```yaml
name: TC-HELLO-001
description: Simple HTTP health check
priority: P0
tags: [smoke, http]

execute:
  - adapter: http
    action: request
    method: GET
    url: "{{baseUrl}}/health"
    assert:
      status: 200
```

### Run Tests

```bash
npx e2e run --env local
```

## Key Features

- **YAML-based tests** - Write tests in declarative YAML format
- **Multi-adapter support** - HTTP, PostgreSQL, Redis, MongoDB, EventHub
- **4-phase lifecycle** - setup → execute → verify → teardown
- **Variable interpolation** - Dynamic values with `{{variable}}` syntax
- **Built-in functions** - UUID, timestamps, random data, file reading
- **Multiple reporters** - Console, JUnit XML, HTML, JSON
- **Parallel execution** - Run tests concurrently
- **Retry logic** - Automatic retry with exponential backoff

## Test Lifecycle

Each test follows a 4-phase execution model:

```
┌─────────────────────────────────────────────────┐
│                    TEST                         │
├─────────────────────────────────────────────────┤
│  1. SETUP     │ Initialize test data            │
│  2. EXECUTE   │ Main test actions (required)    │
│  3. VERIFY    │ Validate results                │
│  4. TEARDOWN  │ Cleanup test data               │
└─────────────────────────────────────────────────┘
```

## Supported Adapters

| Adapter | Purpose | Actions |
|---------|---------|---------|
| `http` | REST APIs | request |
| `postgresql` | PostgreSQL DB | execute, query, queryOne, count |
| `redis` | Redis cache | get, set, del, exists, incr, hget, hset, hgetall, keys, flushPattern |
| `mongodb` | MongoDB | insertOne, insertMany, findOne, find, updateOne, updateMany, deleteOne, deleteMany, count, aggregate |
| `eventhub` | Azure EventHub | publish, waitFor, consume, clear |

## Example Test

```yaml
name: TC-USER-CRUD-001
description: Test user creation and retrieval
priority: P0
tags: [user, crud, e2e]
timeout: 30000

variables:
  test_email: "test-{{$uuid()}}@example.com"
  test_name: "Test User"

setup:
  - adapter: postgresql
    action: execute
    sql: "DELETE FROM users WHERE email LIKE 'test-%@example.com'"

execute:
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/users"
    headers:
      Content-Type: "application/json"
    body:
      email: "{{test_email}}"
      name: "{{test_name}}"
    capture:
      user_id: "$.id"
    assert:
      status: 201
      json:
        - path: "$.name"
          equals: "{{test_name}}"

verify:
  - adapter: http
    action: request
    method: GET
    url: "{{baseUrl}}/users/{{captured.user_id}}"
    assert:
      status: 200
      json:
        - path: "$.email"
          equals: "{{test_email}}"

teardown:
  - adapter: http
    action: request
    method: DELETE
    url: "{{baseUrl}}/users/{{captured.user_id}}"
    continueOnError: true
```

## License

MIT
