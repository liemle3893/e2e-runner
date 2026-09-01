---
name: e2e-runner
description: This skill should be used when writing E2E tests for APIs and databases using the tryve test runner. Use when creating YAML test files, configuring adapters (HTTP, Shell, PostgreSQL, MongoDB, Redis, EventHub, Kafka), writing assertions, or running tests. Provides complete syntax reference for YAML tests, assertion operators, variable interpolation, and built-in functions.
---

# Tryve — YAML-Driven E2E Test Runner

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

Run with: `tryve run --env local`

## CLI Commands

| Command | Description |
|---------|-------------|
| `tryve run [path...]` | Execute E2E tests. Each path is a file, directory, or glob; omit to use the configured `testDir` |
| `tryve validate` | Validate test files without execution |
| `tryve list` | List discovered tests |
| `tryve health` | Check adapter connectivity |
| `tryve init` | Initialize `e2e.config.yaml` |
| `tryve migrate` | Move a suite to a new apiVersion (see below) |
| `tryve test create <name>` | Create test from template (`--template http\|shell`) |
| `tryve test list-templates` | List available templates |
| `tryve doc [section]` | Show documentation for a section |
| `tryve install --skills` | Install Claude Code skills to `.claude/skills/e2e-runner/` |
| `tryve version` | Print version |

### `tryve run` Options

| Flag | Description |
|------|-------------|
| *(positional)* | One or more test files, directories, or globs to run. A path matching nothing is an error. |
| `-d, --test-dir` | Directory to search for test files (default: `tests`) |
| `-g, --grep` | Filter tests by name (regex or substring) |
| `--tag` | Filter by tag (repeatable) |
| `--priority` | Filter by priority (P0, P1, P2, P3) |
| `-p, --parallel` | Concurrent test count (0 = config default) |
| `-t, --timeout` | Per-test timeout in ms (0 = config default) |
| `-r, --retries` | Retry count on failure (-1 = config default) |
| `--bail` | Stop after first failure |
| `--failed-only` | Rerun only previously failed tests |
| `--dry-run` | List matching tests without execution |
| `--skip-setup` | Skip setup phase |
| `--skip-teardown` | Skip teardown phase |
| `--reporter` | Additional reporter: `junit`, `html`, `json` (repeatable) |
| `-o, --output` | Output file for file-based reporters |
| `--verbose` | Show per-step output |
| `--debug` | Show full request/response data |
| `--watch` | Re-run tests on file changes |
| `--strict` | Fail a step when a `{{expression}}` cannot be resolved |
| `--api-version` | Behaviour level for this run: `tryve/v1` or `tryve/v2` |

Global flags: `--config, -c` (config file path), `--env, -e` (environment name)

**Run a single test file** — the fastest way to iterate; it skips parsing the
rest of the suite:

```bash
tryve run tests/e2e/users/TC-USER-001.test.yaml
tryve run tests/e2e/users tests/e2e/auth        # several directories
tryve run 'tests/e2e/**/TC-AUTH-*.test.yaml'    # glob (quote it)
```

### `tryve test create` Options

| Flag | Description |
|------|-------------|
| `-t, --template` | Template to use: `http`, `shell` (default: `http`) |
| `-o, --output` | Output file path (default: `<name>.test.yaml`) |

## API Version

`apiVersion` selects the behaviour level. **Absent means `tryve/v1`**, so
pointing a new binary at an existing suite never changes it; `tryve init` writes
`tryve/v2` for new projects.

```yaml
# e2e.config.yaml
apiVersion: tryve/v2
```

A single test file may declare its own, overriding the suite:

```yaml
# tests/e2e/users/TC-USER-001.test.yaml
apiVersion: tryve/v1        # not migrated yet

name: TC-USER-001
```

A `compatibility` map refines it per area, for adopting one at a time:

```yaml
apiVersion: tryve/v1
compatibility:
  assertions: tryve/v2
```

| Area | `tryve/v1` | `tryve/v2` |
|---|---|---|
| `assertions` | Only the HTTP keys and the 19 original operators are evaluated; anything else is dropped silently | Field assertions (`exitCode`, `stdout`, `rowCount`), `row`/`column`, aliases and added operators work; an unknown operator fails; an array body is the JSONPath root |
| `interpolation` | Everything renders to text; objects use `%v`; output is re-scanned; builtin args keep quotes | A lone `{{expr}}` keeps its type; objects render as JSON; single-pass; args are unquoted |
| `execution` | Step `timeout` and `skip` ignored | Both take effect |
| `adapters` | Driver structs reach assertions, `date` keeps its time, `count` = rows, `findOne` = `{document: …}`, shell unbounded, HTTP capped at 30s | JSON-friendly SQL values, `date` is `YYYY-MM-DD`, `count` is the aggregate, `findOne` fields at top level, 60s shell fallback, step-driven HTTP deadline |

`tryve migrate` manages the move:

```bash
tryve migrate                    # what would change at tryve/v2
tryve migrate --apply            # raise the suite, pin what would break to tryve/v1
tryve migrate --status           # how many files remain pinned
tryve migrate --explain <file>   # what to fix in one file
tryve migrate --unpin <file>     # once it passes at the new version
```

**When writing tests for an existing project, check its `apiVersion` first** —
both the config and the file's own declaration. The assertion forms and result
shapes below describe `tryve/v2`.

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
  timeout: 10000                    # Fail this step after this many ms
  skip: false                       # Skip this step
  skipReason: ""                    # Required alongside skip

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

Both `{{expression}}` and `${expression}` syntaxes are supported.

**Types are preserved.** When an expression is the entire value, the resolved
value keeps its type — so `equals: "{{captured.total}}"` compares a number
against a numeric column. Mixing an expression with literal text produces a
string.

**Paths reach into captured data**, including array indices and JSON held as a
string, so a captured stdout can be addressed directly instead of shelling out
to `node -e` or `jq`:

```yaml
capture:
  setup_result: "$.stdout"                       # '{"promotionId":"p-123","games":[…]}'
# then
"{{captured.setup_result.promotionId}}"          # → "p-123"
"{{captured.setup_result.games[0].gameId}}"      # → "g-1"
```

Path forms: `a.b`, `a.b[0].c`, `a.b.0.c`.

**Captured data is never re-interpolated** — a response containing `{{` is
inserted literally. Only a variable whose own value is a template expands
further.

**Unresolved expressions** pass through as literal text by default. With
`--strict` (or `defaults.strictResolve: true`) they fail the step instead;
`${…}` is never affected, so shell and SQL keep their own syntax. Use
`{{$default(value, fallback)}}` for genuinely optional values.

```yaml
# Test variables
"{{my_variable}}"

# Captured values from prior steps
"{{user_id}}"

# Config variables (from e2e.config.yaml)
"{{baseUrl}}"

# Environment variables
"{{$env(API_KEY)}}"

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

# JSON — replaces `jq` and `node -e` shell steps
"{{$json(captured.raw)}}"                          # Parse a JSON string
"{{$jsonPath(captured.result, data.items[0].id)}}" # Read a path out of a value
"{{$jsonFile(local.settings.json, Values.KEY)}}"   # Read a path out of a JSON file

# Types — explicit coercion for typed SQL params and comparisons
"{{$int(captured.n)}}"             # Integer
"{{$number(captured.amount)}}"     # Float
"{{$bool(captured.flag)}}"         # Boolean
"{{$default(captured.token, anon)}}" # Value, or fallback when unset

# Auth — replaces token-minting shell scripts
'{{$jwt(HS256, {{$env(JWT_SECRET)}}, {"sub":"84987654321"}, 1h)}}'
'{{$jwt(RS256, {{$jsonFile(keys.json, private)}}, {"sub":"1"}, 30m, key-1)}}'
"{{$hmac(payload, secret)}}"       # Hex HMAC-SHA256
"{{$base64url(value)}}"            # Unpadded base64url

# Arguments may be literals, context references, or nested {{…}} expressions.
# Commas inside quotes, parens, and brackets are not treated as separators.

# Variable cross-references (resolved in dependency order)
base_id: "TEST"
run_id: "{{base_id}}_RUN"          # → "TEST_RUN"
full_id: "{{run_id}}_{{$uuid()}}"  # → "TEST_RUN_<uuid>"
# Circular references are detected and throw errors
# {{baseUrl}} and captured refs are deferred to step time

# Built-in functions in the variables block are evaluated ONCE at test
# initialization — every phase (setup/execute/verify/teardown) sees the
# same resolved value. Use this to keep IDs consistent across phases.
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
| `startsWith` | string | String prefix |
| `endsWith` | string | String suffix |
| `oneOf` | array | Value is a member of the list |
| `notOneOf` | array | Value is not a member of the list |
| `minLength` | number | Length >= n |
| `maxLength` | number | Length <= n |

Aliases: `eq`, `ne`/`neq`, `gt`, `gte`, `lt`, `lte`, `in` (→ `oneOf`),
`notIn`, `lengthEquals` (→ `length`), `empty` (→ `isEmpty`).

**An unrecognised operator fails the assertion** and names the valid ones, so a
misspelling is never dropped silently.

### Assertion Shapes

```yaml
# 1. HTTP keys
assert:
  status: 200
  statusRange: [200, 299]
  headers: { Content-Type: "application/json" }
  json:
    - path: "$.data.id"
      exists: true
  duration: { lessThan: 500 }

# 2. Field of the result — any other top-level key names a result field
assert:
  exitCode: 0                    # shell
  stdout:
    contains: "BOTH_OK"          # shell
  rowCount:
    gte: 1                       # SQL

# 3. Path list
assert:
  - path: "$.rows[0].email"
    equals: "test@example.com"

# 4. Row/column, for SQL results (row defaults to 0)
assert:
  - row: 0
    column: reward_key
    equals: "only_reward"
```

## HTTP Adapter

**Action:** `request`

**Params:** `url` (required), `method` (default GET), `headers`, `query`, `body`,
`multipart`, `timeout`, `followRedirects`

Content-Type auto-set to `application/json` when body is present. Cookie jar persists across steps.

**File uploads** use `multipart` (mutually exclusive with `body`); each entry has
`name` plus either `file` or `value`, and may override `filename`/`contentType`:

```yaml
- adapter: http
  action: request
  method: POST
  url: "{{baseUrl}}/ops/upload"
  headers:
    Authorization: "Bearer {{captured.ops_token}}"
  multipart:
    - name: file
      file: "./tests/e2e/fixtures/members.csv"
      contentType: "text/csv"
    - name: created_by
      value: "ops-admin"
  assert:
    status: 200
```

Set `followRedirects: false` to assert on a 3xx and its `Location` header.

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

**Params:** `command` (required), `cwd`, `env` (map), `timeout` (ms)

A non-zero exit fails the step automatically. Adding an `exitCode` assertion
takes over that decision, which is how you assert on a command expected to fail.

```yaml
- adapter: shell
  action: exec
  command: "echo 'hello world'"
  cwd: "/app"
  timeout: 10000
  env:
    NODE_ENV: "test"
  assert:
    exitCode: 0
    stdout:
      contains: "hello"
  capture:
    version: "$.stdout"
```

**Result data:** `stdout` (string), `stderr` (string), `exitCode` (number)

**Every command is bounded by a timeout** — the step's `timeout`, else the
adapter's `defaultTimeout`, else 60 s. On expiry the whole process group is
killed, so anything the command backgrounded dies with it.

**Command policy.** `adapters.shell` in the config may carry `allow` (regexes a
command must match — deny-by-default once present), `deny`, `env` (the only
variables commands inherit), and `cwd`. A refused command fails before it runs.

**Prefer a built-in over a shell step** where one exists — it runs in-process,
needs no policy exception, and fails more clearly:

| Shell command | Use instead |
|---|---|
| `node -e "process.stdout.write(String(Date.now()))"` | `{{$now(unixMs)}}` |
| `node -e "…JSON.parse(argv[1]).someId"` | `{{captured.result.someId}}` |
| `cat f.json \| jq -r .a.b` | `{{$jsonFile(f.json, a.b)}}` |
| `echo "$JSON" \| jq .field` | `{{$jsonPath(captured.json, field)}}` |
| a script that mints a test JWT | `{{$jwt(HS256, secret, {"sub":"…"}, 1h)}}` |
| `curl -F file=@x.csv …` | the HTTP adapter's `multipart` |
| `psql -c "…"` | the postgresql adapter |
| `sleep 2` | `delay: 2000` on the next step |

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

# Query rows (returns {rows: [...], rowCount: N})
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

# Get single row (row fields at top level, plus found: true; errors if 0 rows)
- adapter: postgresql
  action: queryOne
  sql: "SELECT * FROM users WHERE id = $1"
  params: ["{{user_id}}"]
  assert:
    - path: "$.email"
      equals: "{{email}}"
    - path: "$.deleted_at"
      isNull: true
    # or address cells by name (row defaults to 0):
    - column: email
      equals: "{{email}}"

# Assert that NO row matches — allowEmpty makes an empty result a fact, not an error
- adapter: postgresql
  action: queryOne
  allowEmpty: true
  sql: "SELECT id FROM sessions WHERE user_id = $1"
  params: ["{{user_id}}"]
  assert:
    found: false

# Count — works with a COUNT(*) aggregate or a plain SELECT
- adapter: postgresql
  action: count
  sql: "SELECT COUNT(*) FROM users WHERE active = true"
  assert:
    count:
      gte: 1
```

**Column types.** Values arrive as their natural JSON type, so no casts are
needed to compare them:

| PostgreSQL type | Becomes |
|---|---|
| `int2`/`int4`/`int8`, `float4`/`float8` | number |
| `numeric`/`decimal` | number (exact decimal text when too large for a float64) |
| `uuid` | canonical string |
| `date` | `2026-08-31` (no time component) |
| `timestamp`/`timestamptz` | RFC 3339 string |
| `interval` | ISO 8601 duration, e.g. `P2DT3600S` |
| `json`/`jsonb` | parsed object or array |
| arrays | array, elements converted the same way |
| `NULL` | null |

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

# Find one — the document's fields are at the top level, plus found: true
- adapter: mongodb
  action: findOne
  collection: "users"
  filter:
    email: "{{email}}"
  capture:
    user_id: "_id"
  assert:
    - path: "$.roles"
      type: "array"
      length: 1

# Assert a document does NOT exist
- adapter: mongodb
  action: findOne
  collection: "sessions"
  allowEmpty: true
  filter:
    userId: "{{captured.user_id}}"
  assert:
    found: false

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
  message:
    key: "user-123"
    value:
      type: "user.created"
      userId: "{{user_id}}"
    headers:
      source: "test"

# Produce a batch
- adapter: kafka
  action: produce
  topic: "user-events"
  messages:
    - key: "k1"
      value: { type: "one" }
    - key: "k2"
      value: { type: "two" }

# Wait for a specific message. Filter keys address the payload with
# dot-notation; envelope fields (key, topic, partition, offset) also match.
- adapter: kafka
  action: waitFor
  topic: "user-events"
  timeout: 30000
  filter:
    type: "user.created"
    userId: "{{user_id}}"
  assert:
    - path: "type"
      equals: "user.created"
  capture:
    event_type: "type"

# Consume one message
- adapter: kafka
  action: consume
  topic: "events"
  timeout: 10000
```

**consume result data:** `key` (string), `value` (parsed JSON or string), `headers` (map), `topic` (string), `partition` (number), `offset` (number)

**waitFor result data:** the matched payload's fields at the top level, plus the
envelope fields (`key`, `value`, `headers`, `topic`, `partition`, `offset`)
alongside them; the untouched envelope is also under `message`.

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
apiVersion: tryve/v2               # Behaviour level; absent means tryve/v1
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
      shell:
        defaultTimeout: 60000      # Bound for steps that set no timeout
        cwd: "."                   # Relative to this file
        allow:                     # Optional: deny-by-default once present
          - "^node scripts/e2e/"
        deny: []
        env: []                    # Optional: the only variables commands inherit

defaults:
  timeout: 30000                   # Per-test timeout (ms)
  retries: 0                       # Default retries
  retryDelay: 1000                 # Backoff base (ms)
  parallel: 1                      # Concurrent tests
  strictResolve: false             # Fail a step on an unresolvable {{expression}}

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

* **Getting Started** [references/getting-started.md](references/getting-started.md)
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
