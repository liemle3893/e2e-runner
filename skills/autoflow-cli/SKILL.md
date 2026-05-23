---
name: autoflow-cli
description: This skill should be used when writing E2E tests for APIs and databases using the autoflow e2e test runner. Use when creating YAML test files, configuring adapters (HTTP, Shell, PostgreSQL, MongoDB, Redis, EventHub, Kafka), writing assertions, or running tests. Provides complete syntax reference for YAML tests, assertion operators, variable interpolation, and built-in functions.
---

# Autoflow — YAML-Driven E2E Test Runner

A flexible end-to-end testing framework for API and database testing. Tests are written declaratively in YAML.

## Quick Start

```yaml
name: TC-HEALTH-001
description: Verify API health endpoint
tags: [smoke]

execute:
  - adapter: http
    action: request
    method: GET
    url: "{{baseUrl}}/health"
    assert:
      status: 200
      json:
        - path: "$.status"
          equals: "ok"
```

Run with: `autoflow e2e run --env local`

## CLI Commands

| Command | Description |
|---------|-------------|
| `autoflow e2e run` | Execute E2E tests |
| `autoflow e2e validate` | Validate test files without execution |
| `autoflow e2e list` | List discovered tests |
| `autoflow e2e health` | Check adapter connectivity |
| `autoflow e2e init` | Initialize `e2e.config.yaml` |
| `autoflow e2e test create <name>` | Create test from template (`--template http\|shell`) |
| `autoflow e2e test list-templates` | List available templates |
| `autoflow e2e scaffold --ticket KEY --area AREA --count N` | Generate E2E test stubs for a ticket |
| `autoflow e2e doc [section]` | Show documentation for a section |
| `autoflow install` | Install all bundled skills + agents (use `--skills` for autoflow-cli only) |
| `autoflow version` | Print version |

### `autoflow e2e run` Options

| Flag | Description |
|------|-------------|
| `-d, --test-dir` | Directory to search for test files (default: `tests`) |
| `-g, --grep` | Filter tests by name (regex or substring) |
| `--tag` | Filter by tag (repeatable) |
| `--priority` | Filter by priority (P0, P1, P2, P3) |
| `-p, --parallel` | Concurrent test count (0 = config default) |
| `-t, --timeout` | Per-test timeout in ms (0 = config default) |
| `-r, --retries` | Retry count on failure (-1 = config default) |
| `--bail` | Stop after first failure |
| `--dry-run` | List matching tests without execution |
| `--skip-setup` | Skip setup phase |
| `--skip-teardown` | Skip teardown phase |
| `--reporter` | Additional reporter: `junit`, `html`, `json` (repeatable) |
| `-o, --output` | Output file for file-based reporters |
| `--verbose` | Show per-step output |
| `--debug` | Show full request/response data |
| `--watch` | Re-run tests on file changes |
| `--failed-only` | Rerun only previously failed tests |

Global flags: `--config, -c` (config file path), `--env, -e` (environment name)

### `autoflow e2e test create` Options

| Flag | Description |
|------|-------------|
| `-t, --template` | Template to use: `http`, `shell` (default: `http`) |
| `-o, --output` | Output file path (default: `<name>.test.yaml`) |

## Test File Structure

Files must have `.test.yaml` extension. Four phases run sequentially:

```yaml
name: TC-FEATURE-001               # Required: unique identifier
description: What the test verifies # Optional
priority: P0                        # P0|P1|P2|P3
tags: [smoke, crud]                 # For filtering
timeout: 30000                      # Test timeout (ms), max 300000
retries: 2                          # Retry count (0-5, -1 = use default)
skip: false                         # Skip this test
skipReason: "Blocked by JIRA-123"   # Reason for skip
depends: ["TC-AUTH-001"]            # Wait for named tests to pass first

variables:                          # Test-scoped variables
  email: "test-{{$uuid()}}@example.com"

setup: []                           # Prepare prerequisites
execute: []                         # Required: main test actions
verify: []                          # Assert expected outcomes
teardown: []                        # Cleanup (always runs, even on failure)
```

## Step Definition

Each phase contains an array of steps:

```yaml
- id: create_user                   # Step identifier (auto: "{phase}-{index}")
  adapter: http                     # http|postgresql|mongodb|redis|eventhub|kafka|shell
  action: request                   # Adapter-specific action
  description: "Create a user"      # Optional
  continueOnError: false            # Convert failure to warning, keep running
  retry: 3                          # Step-level retry count
  delay: 1000                       # Delay before execution (ms)

  # Adapter-specific params (e.g. HTTP)
  method: POST
  url: "{{baseUrl}}/users"
  body:
    email: "{{email}}"

  capture:                          # Capture values for later steps
    user_id: "$.id"

  assert:                           # Inline assertions
    status: 201
```

## Variable Interpolation

Both `{{expression}}` and `${expression}` syntaxes are supported. Max nesting depth: 10 passes.

```yaml
# Test variables
"{{my_variable}}"

# Captured values from prior steps
"{{user_id}}"

# Config variables (from e2e.config.yaml)
"{{baseUrl}}"

# Environment variables
"{{$env(API_KEY)}}"                 # Required (errors if not set)
"{{$env(API_KEY, default_value)}}"  # With default (returns default if not set)

# Built-in functions
"{{$uuid()}}"                       # UUID v4
"{{$timestamp()}}"                  # Unix ms
"{{$isoDate()}}"                    # ISO 8601 / RFC3339 date
"{{$random(1, 100)}}"              # Random integer in [min, max]
"{{$randomString(32)}}"            # Random alphanumeric (default length: 8)
"{{$now(date)}}"                   # Formatted date (iso|date|time|datetime|unix|unixMs|Go layout)
"{{$dateAdd(1, day)}}"             # Future date (units: s|m|h|d|w|month|y)
"{{$dateSub(1, hour)}}"            # Past date
"{{$totp(BASE32SECRET)}}"          # 6-digit TOTP code (RFC 6238, 30s window)
"{{$base64(value)}}"               # Base64 encode
"{{$base64Decode(encoded)}}"       # Base64 decode
"{{$md5(value)}}"                  # MD5 hex digest
"{{$sha256(value)}}"               # SHA-256 hex digest
"{{$jsonStringify(value)}}"        # Escape for JSON embedding
"{{$file(./fixtures/data.json)}}"  # File contents as string
"{{$lower(value)}}"                # Lowercase
"{{$upper(value)}}"                # Uppercase
"{{$trim(value)}}"                 # Trim whitespace

# Variable cross-references (resolved in dependency order)
base_id: "TEST"
run_id: "{{base_id}}_RUN"          # → "TEST_RUN"
full_id: "{{run_id}}_{{$uuid()}}"  # → "TEST_RUN_<uuid>"
# Circular references are detected and throw errors
# {{baseUrl}} and captured refs are deferred to step time
```

## Assertion Operators

All operators work across every adapter:

| Operator | Type | Description |
|----------|------|-------------|
| `equals` | any | Deep equality (numerics normalized) |
| `notEquals` | any | Inverse of equals |
| `contains` | string/array | Substring or array element match |
| `notContains` | string/array | Inverse of contains |
| `matches` | string | Regex pattern match |
| `type` | string | Type check (`string`, `number`, `boolean`, `array`, `object`, `null`) |
| `exists` | boolean | Path exists (true) or not (false) |
| `notExists` | — | Path must not exist |
| `isNull` | — | Value is null/nil |
| `isNotNull` | — | Value is not null/nil |
| `greaterThan` | number | Numeric > |
| `lessThan` | number | Numeric < |
| `greaterThanOrEqual` | number | Numeric >= |
| `lessThanOrEqual` | number | Numeric <= |
| `length` | number | Exact length (strings, arrays, objects) |
| `isEmpty` | — | Zero length or nil |
| `notEmpty` | — | Has content |
| `hasProperty` | string | Object has key |
| `notHasProperty` | string | Object lacks key |

## HTTP Adapter

**Action:** `request`

**Params:** `url` (required), `method` (default GET), `headers`, `query`, `body`

Content-Type auto-set to `application/json` when body is present. Cookie jar persists across steps.

```yaml
- adapter: http
  action: request
  method: POST
  url: "{{baseUrl}}/users"
  headers:
    Authorization: "Bearer {{token}}"
  body:
    email: "{{email}}"
  assert:
    status: 201                       # or [200, 201] for oneOf
    statusRange: [200, 299]           # inclusive range
    headers:
      Content-Type: "application/json"
    json:
      - path: "$.id"
        exists: true
        type: "string"
      - path: "$.errors[0].code"
        equals: 8006
    body:
      contains: "success"             # Raw body string assertions
    duration:
      lessThan: 1000                  # Response time (ms)
  capture:
    user_id: "$.id"
```

**Result data:** `status` (number), `statusText` (string), `headers` (map), `body` (parsed JSON or string), `duration` (ms)

## Shell Adapter

**Action:** `exec`

**Params:** `command` (required), `cwd`, `env` (map)

Non-zero exit code is NOT an automatic failure — assert on `exitCode` explicitly.

```yaml
- adapter: shell
  action: exec
  command: "echo 'hello world'"
  cwd: "/app"
  env:
    NODE_ENV: "test"
  assert:
    - path: "$.exitCode"
      equals: 0
    - path: "$.stdout"
      contains: "hello"
  capture:
    version: "$.stdout"
```

**Result data:** `stdout` (string), `stderr` (string), `exitCode` (number)

## PostgreSQL Adapter

**Config:** `connectionString` (required), `schema`, `poolSize` (default 5)

**Actions:** `execute`, `query`, `queryOne`, `count`

```yaml
# Insert a row
- adapter: postgresql
  action: execute
  sql: "INSERT INTO users (email) VALUES ($1)"
  params: ["{{email}}"]
  assert:
    - path: "$.rowsAffected"
      equals: 1

# Query rows
- adapter: postgresql
  action: query
  sql: "SELECT * FROM users WHERE email = $1"
  params: ["{{email}}"]
  assert:
    - path: "$.rowCount"
      greaterThan: 0
    - path: "$.rows[0].email"
      equals: "{{email}}"
  capture:
    user_id: "$.rows[0].id"

# Get single row
- adapter: postgresql
  action: queryOne
  sql: "SELECT * FROM users WHERE id = $1"
  params: ["{{user_id}}"]
  assert:
    - path: "$.email"
      equals: "{{email}}"
    - path: "$.deleted_at"
      isNull: true

# Count rows
- adapter: postgresql
  action: count
  sql: "SELECT * FROM users WHERE active = true"
  assert:
    - path: "$.count"
      greaterThan: 0
```

## MongoDB Adapter

**Config:** `connectionString` (required), `database` (required)

**Actions:** `insertOne`, `insertMany`, `findOne`, `find`, `updateOne`, `updateMany`, `deleteOne`, `deleteMany`, `count`, `aggregate`

```yaml
# Insert
- adapter: mongodb
  action: insertOne
  collection: "users"
  document:
    email: "{{email}}"
    roles: ["user"]
  capture:
    mongo_id: "$.insertedId"

# Find
- adapter: mongodb
  action: findOne
  collection: "users"
  filter:
    email: "{{email}}"
  assert:
    - path: "$.roles"
      type: "array"
      length: 1

# Aggregate
- adapter: mongodb
  action: aggregate
  collection: "orders"
  pipeline:
    - $match: { status: "completed" }
    - $group: { _id: null, total: { $sum: "$amount" } }
  assert:
    - path: "$.documents[0].total"
      greaterThan: 0
```

## Redis Adapter

**Config:** `connectionString` (required), `db` (default 0), `keyPrefix`

**Actions:** `get`, `set`, `del`, `exists`, `incr`, `hget`, `hset`, `hgetall`, `keys`, `flushPattern`

```yaml
# Set and get
- adapter: redis
  action: set
  key: "user:{{user_id}}"
  value: '{"name": "test"}'
  ttl: 3600

- adapter: redis
  action: get
  key: "user:{{user_id}}"
  assert:
    - path: "$.value"
      isNotNull: true
      contains: "test"

# Hash operations
- adapter: redis
  action: hset
  key: "session:{{session_id}}"
  field: "token"
  value: "{{token}}"

- adapter: redis
  action: hgetall
  key: "session:{{session_id}}"
  assert:
    - path: "$.value.token"
      equals: "{{token}}"

# Cleanup by pattern
- adapter: redis
  action: flushPattern
  pattern: "user:*"
```

## Kafka Adapter

**Config:** `brokers` (required, array), `clientId`, `groupId`, `timeout` (ms, default 10000), `ssl`, `sasl` (`mechanism`, `username`, `password`)

**Actions:** `produce`, `consume`, `waitFor`, `clear`

```yaml
# Produce a message
- adapter: kafka
  action: produce
  topic: "user-events"
  key: "user-123"
  value:
    type: "user.created"
    userId: "{{user_id}}"
  headers:
    source: "test"

# Wait for a specific message
- adapter: kafka
  action: waitFor
  topic: "user-events"
  timeout: 30000
  match:
    key: "user-123"
  assert:
    - path: "$.value.type"
      equals: "user.created"
  capture:
    event_data: "$.value"

# Consume one message
- adapter: kafka
  action: consume
  topic: "events"
  timeout: 10000
```

**Message result data:** `key` (string), `value` (parsed JSON or string), `headers` (map), `topic` (string), `partition` (number), `offset` (number)

## EventHub Adapter

**Config:** `connectionString` (required), `eventHubName`, `consumerGroup` (default `$Default`)

**Actions:** `publish`, `consume`, `waitFor`, `clear`

```yaml
- adapter: eventhub
  action: publish
  topic: "partition-0"
  body:
    type: "order.placed"
    orderId: "{{order_id}}"

- adapter: eventhub
  action: waitFor
  topic: "partition-0"
  timeout: 15000
  match:
    type: "order.placed"
  assert:
    - path: "$.orderId"
      equals: "{{order_id}}"
```

## Configuration (e2e.config.yaml)

```yaml
version: "1.0"
testDir: "tests"                   # Relative to config file

environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      http: {}
      postgresql:
        connectionString: "${DB_URL}"
        schema: "public"
        poolSize: 5
      mongodb:
        connectionString: "${MONGO_URL}"
        database: "testdb"
      redis:
        connectionString: "${REDIS_URL}"
        db: 0
        keyPrefix: "test:"
      kafka:
        brokers: ["localhost:9092"]
        clientId: "test-runner"
        sasl:
          mechanism: "plain"
          username: "${KAFKA_USER}"
          password: "${KAFKA_PASS}"
      eventhub:
        connectionString: "${EVENTHUB_CONN_STR}"
        eventHubName: "my-hub"

defaults:
  timeout: 30000                   # Per-test timeout (ms)
  retries: 0                       # Default retries
  retryDelay: 1000                 # Backoff base (ms)
  parallel: 1                      # Concurrent tests

variables:
  apiVersion: "v1"

hooks:
  beforeAll: ""                    # Shell command
  afterAll: ""
  beforeEach: ""
  afterEach: ""

reporters:
  - type: console
  - type: junit
    output: "reports/junit.xml"
  - type: json
    output: "reports/results.json"
  - type: html
    output: "reports/index.html"
```

Environment variables are resolved via `${VAR_NAME}` in config. A `.env` file in the config directory is loaded automatically.

## Reference Files

* **YAML Test Syntax** [references/yaml-test.md](references/yaml-test.md)
* **Assertions** [references/assertions.md](references/assertions.md)
* **Built-in Functions** [references/built-in-functions.md](references/built-in-functions.md)
* **Configuration** [references/config.md](references/config.md)
* **CLI Reference** [references/cli.md](references/cli.md)
* **Examples** [references/examples.md](references/examples.md)
* **Adapters Overview** [references/adapters/index.md](references/adapters/index.md)
* **HTTP Adapter** [references/adapters/http.md](references/adapters/http.md)
* **PostgreSQL Adapter** [references/adapters/postgresql.md](references/adapters/postgresql.md)
* **MongoDB Adapter** [references/adapters/mongodb.md](references/adapters/mongodb.md)
* **Redis Adapter** [references/adapters/redis.md](references/adapters/redis.md)
* **EventHub Adapter** [references/adapters/eventhub.md](references/adapters/eventhub.md)
* **Kafka Adapter** [references/adapters/kafka.md](references/adapters/kafka.md)
* **Shell Adapter** [references/adapters/shell.md](references/adapters/shell.md)
