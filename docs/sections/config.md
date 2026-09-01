# Configuration Reference

The E2E Test Runner is configured via `e2e.config.yaml` in your project root.

## Complete Configuration Schema

```yaml
version: "1.0"                    # Required: config version

apiVersion: tryve/v2              # Optional: behaviour level (default: tryve/v1)

testDir: "tests/e2e"              # Optional: test directory (default: ".")

environments:                     # Required: at least one environment
  local:                          # Environment name
    baseUrl: "http://localhost:3000"  # Required: base URL for HTTP adapter
    adapters:                     # Optional: adapter configurations
      postgresql:
        connectionString: "postgresql://user:pass@localhost:5432/db"
        schema: "public"          # Default schema
        poolSize: 10              # Connection pool size

      redis:
        connectionString: "redis://localhost:6379"
        db: 0                     # Redis database number
        keyPrefix: ""             # Key prefix for all operations

      mongodb:
        connectionString: "mongodb://user:pass@localhost:27017"
        database: "mydb"          # Database name

      eventhub:
        connectionString: "Endpoint=sb://...;EntityPath=events"
        consumerGroup: "$Default" # Consumer group name
        checkpointStore: ""       # Optional: checkpoint store connection

      shell:
        defaultTimeout: 60000     # Timeout for steps that set none (default: 60000)
        cwd: "."                  # Working directory, relative to this file
        allow:                    # Optional: only these commands may run
          - "^node scripts/e2e/"
        deny: []                  # Optional: refused before allow is consulted
        env:                      # Optional: the only variables commands inherit
          - PATH
          - HOME

  staging:
    baseUrl: "https://staging.example.com"
    adapters:
      # Staging adapter configs...

  production:
    baseUrl: "https://api.example.com"
    adapters:
      # Production adapter configs...

defaults:                         # Optional: default settings
  timeout: 30000                  # Default test timeout (ms)
  retries: 0                      # Default retry count
  retryDelay: 1000                # Delay between retries (ms)
  parallel: 1                     # Parallel test count
  strictResolve: false            # Fail a step on an unresolvable {{expression}}

variables:                        # Optional: global variables
  testPrefix: "e2e_"
  defaultUserId: "test-user"
  apiVersion: "v1"

hooks:                            # Optional: lifecycle hooks
  beforeAll: "./scripts/seed.sh"  # Shell command before all tests
  afterAll: "./scripts/cleanup.sh" # Shell command after all tests
  beforeEach: ""                  # Shell command before each test
  afterEach: ""                   # Shell command after each test

reporters:                        # Optional: report configuration
  - type: console
    verbose: true

  - type: junit
    output: "./reports/junit.xml"

  - type: html
    output: "./reports/report.html"

  - type: json
    output: "./reports/results.json"
```

## Top-Level Options

| Option        | Type                          | Default | Description                              |
|---------------|-------------------------------|---------|------------------------------------------|
| `version`     | `"1.0"`                       | —       | Required. Config schema version.         |
| `apiVersion`  | `string`                      | `tryve/v1` | Behaviour level. See below.           |
| `compatibility` | map                         | —       | Per-area refinement of `apiVersion`.     |
| `testDir`     | `string`                      | `"."`   | Directory to discover test files in.     |
| `environments`| `Record<string, Environment>` | —       | Required. At least one environment.      |
| `defaults`    | `DefaultsConfig`              | —       | Default timeout, retries, parallelism.   |
| `variables`   | `Record<string, string\|number\|boolean>` | — | Global variables for all tests. |
| `hooks`       | `HooksConfig`                 | —       | Lifecycle shell commands.                |
| `reporters`   | `ReporterConfig[]`            | `[{type:"console"}]` | Output format configuration. |

## Environment Configuration

### Base URL

The `baseUrl` is required and used as the base for all HTTP requests:

```yaml
environments:
  local:
    baseUrl: "http://localhost:3000"
```

Access in tests via `{{baseUrl}}`:

```yaml
- adapter: http
  action: request
  url: "{{baseUrl}}/api/users"
```

### Adapter Configuration

Each adapter has specific configuration options:

#### PostgreSQL

| Option             | Type     | Default    | Description                   |
|--------------------|----------|------------|-------------------------------|
| `connectionString` | `string` | —          | Required. PostgreSQL URI.     |
| `schema`           | `string` | `"public"` | Default schema for queries.   |
| `poolSize`         | `number` | —          | Connection pool size.         |

```yaml
postgresql:
  connectionString: "postgresql://user:password@host:port/database"
  schema: "public"      # Default schema for queries
  poolSize: 10          # Connection pool size
```

#### Redis

| Option             | Type     | Default | Description                   |
|--------------------|----------|---------|-------------------------------|
| `connectionString` | `string` | —       | Required. Redis URI.          |
| `db`               | `number` | `0`     | Database number (0-15).       |
| `keyPrefix`        | `string` | `""`    | Prefix added to all keys.     |

```yaml
redis:
  connectionString: "redis://user:password@host:port"
  db: 0                 # Database number (0-15)
  keyPrefix: "test:"    # Prefix added to all keys
```

#### MongoDB

| Option             | Type     | Default | Description                   |
|--------------------|----------|---------|-------------------------------|
| `connectionString` | `string` | —       | Required. MongoDB URI.        |
| `database`         | `string` | —       | Database name.                |

```yaml
mongodb:
  connectionString: "mongodb://user:password@host:port"
  database: "mydb"      # Database name
```

#### EventHub

| Option             | Type     | Default      | Description                        |
|--------------------|----------|--------------|------------------------------------|
| `connectionString` | `string` | —            | Required. EventHub connection.     |
| `consumerGroup`    | `string` | `"$Default"` | Consumer group name.               |
| `checkpointStore`  | `string` | —            | Checkpoint store connection string.|

```yaml
eventhub:
  connectionString: "Endpoint=sb://namespace.servicebus.windows.net/;SharedAccessKeyName=...;SharedAccessKey=...;EntityPath=hub-name"
  consumerGroup: "$Default"
```

For local development with EventHub emulator:

```yaml
eventhub:
  connectionString: "Endpoint=sb://localhost;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=SAS_KEY_VALUE;UseDevelopmentEmulator=true;EntityPath=events"
  consumerGroup: "$Default"
```

## Environment Variables

Use `${env.VAR_NAME}` or `${VAR_NAME}` in configuration to reference environment variables:

```yaml
environments:
  staging:
    baseUrl: "${STAGING_URL}"
    adapters:
      postgresql:
        connectionString: "${STAGING_PG_CONNECTION_STRING}"
      redis:
        connectionString: "${STAGING_REDIS_URL}"
```

Set variables before running:

```bash
export STAGING_URL="https://staging.example.com"
export STAGING_PG_CONNECTION_STRING="postgresql://..."
tryve run --env staging
```

## Global Variables

Define variables accessible in all tests:

```yaml
variables:
  testPrefix: "e2e_test_"
  defaultUserId: "test-user-001"
  apiKey: "${API_KEY}"
```

Access in tests:

```yaml
execute:
  - adapter: http
    action: request
    url: "{{baseUrl}}/users/{{defaultUserId}}"
    headers:
      X-API-Key: "{{apiKey}}"
```

## Default Settings

Configure defaults applied to all tests:

| Option       | Type     | Default | Description                     |
|--------------|----------|---------|---------------------------------|
| `timeout`    | `number` | `30000` | Test timeout in ms.             |
| `retries`    | `number` | `0`     | Retry count for failed tests.   |
| `retryDelay` | `number` | `1000`  | Delay between retries in ms.    |
| `parallel`   | `number` | `1`     | Number of tests to run concurrently. |

```yaml
defaults:
  timeout: 30000       # 30 second timeout
  retries: 0           # No retries by default
  retryDelay: 1000     # 1 second between retries
  parallel: 1          # Sequential execution by default
  strictResolve: false # Pass unresolved {{…}} through as literal text
```

### Shell command policy

A shell step runs a command on whatever machine the suite runs on, with whatever
that machine's environment holds. `adapters.shell` narrows that:

| Key | Effect |
|---|---|
| `allow` | Regular expressions a command must match. When present the adapter is deny-by-default and anything unmatched is refused before it runs. |
| `deny` | Regular expressions refused outright, evaluated before `allow`. |
| `env` | The only environment variables a command inherits. When absent, the full process environment is passed through. |
| `cwd` | Working directory, resolved relative to the config file. |
| `defaultTimeout` | Bound applied to steps that name no `timeout` (default 60 000 ms). |

Omitting `allow` keeps every command runnable, which is what an existing suite
expects. Adding one covering the scripts you genuinely invoke makes anything
else fail loudly instead of executing.

See `tryve doc adapters.shell`.

### `apiVersion`

Selects the behaviour level. **An absent value means `tryve/v1`**, so pointing a
new binary at an existing project never changes how it behaves. `tryve init`
writes `tryve/v2` into new projects.

```yaml
apiVersion: tryve/v2       # current behaviour
apiVersion: tryve/v1       # previous behaviour (the default)
```

`tryve/v2` and the bare `v2` mean the same thing; `tryve.dev/v2` is accepted too.
This is distinct from `version`, which is the config file's own schema version.

`--api-version tryve/v2` on `tryve run` overrides the config for a single run,
which is the quickest way to see what adopting a version would do.

An individual test file may declare its own `apiVersion`, overriding the suite.
That is what makes a large suite migratable: raise the suite, leave the files
that are not ready on `tryve/v1`, and move them one at a time. `tryve migrate`
does this bookkeeping — see `tryve doc cli`.

```yaml
# tests/e2e/users/TC-USER-001.test.yaml
apiVersion: tryve/v1

name: TC-USER-001
```

| Area | `tryve/v1` | `tryve/v2` |
|---|---|---|
| `assertions` | Only `status`, `statusRange`, `headers`, `json`, `body`, `duration` and the 19 original operators are evaluated; every other key is dropped in silence | Field assertions (`exitCode`, `stdout`, `rowCount`, …), `row`/`column` for SQL results, operator aliases (`gte`, `in`, …) and the added operators all work, an unrecognised operator **fails**, and an array response body is the JSONPath root |
| `interpolation` | Every resolved value renders to text; objects render with Go's `%v`; substituted text is re-scanned for further expressions; builtin arguments keep their quotes | A lone `{{expr}}` keeps its resolved type; objects render as JSON; substitution is single-pass so captured data is never re-expanded; quoted arguments are unquoted |
| `execution` | A step's `timeout` and `skip` are parsed and ignored | Both take effect |
| `adapters` | `numeric`/`interval` reach assertions as driver structs, `date` keeps its time component, `count` reports rows returned, `findOne` returns only `{document: …}`, shell commands are unbounded, HTTP requests are capped at 30s | SQL values convert to JSON-friendly types, `date` is `YYYY-MM-DD`, `count` returns a `COUNT(*)` scalar, `findOne` exposes fields at the top level, shell commands get a 60s fallback timeout, HTTP requests follow the step deadline |

Adopting `assertions` is the one that matters: under `tryve/v1` an assertion the
runner does not recognise is discarded, so the step passes whatever the result
was. Expect previously-green tests to fail when you move it — those are checks
that were never running.

### `compatibility`

Refines `apiVersion` per area, for adopting one at a time across a suite too
large to move at once:

```yaml
apiVersion: tryve/v1
compatibility:
  assertions: tryve/v2      # take the assertion fixes, leave the rest
```

It can also hold an area back from a newer baseline:

```yaml
apiVersion: tryve/v2
compatibility:
  adapters: tryve/v1
```

It is a map of areas only — `assertions`, `interpolation`, `execution`,
`adapters`. To set the level for a whole file or suite, use `apiVersion`.

### `strictResolve`

By default an expression that names nothing — a misspelled variable, a value a
previous step failed to capture — is left in place as the literal text
`{{captured.typo}}` and sent to the system under test. The test then fails
somewhere far from the cause, or worse, passes.

With `strictResolve: true` the step fails immediately, naming the expression
that could not be resolved. `${…}` is never affected, so shell and SQL keep
their own variable syntax. Use `{{$default(value, fallback)}}` for values that
are genuinely optional.

The `--strict` CLI flag overrides this per run.

Override in individual tests:

```yaml
name: TC-SLOW-001
timeout: 120000     # Override: 2 minute timeout
retries: 5          # Override: 5 retries

execute:
  - adapter: http
    action: request
    url: "{{baseUrl}}/slow-endpoint"
```

## Hooks

Run shell commands at specific points in the test lifecycle:

| Hook         | Type     | Description                          |
|--------------|----------|--------------------------------------|
| `beforeAll`  | `string` | Runs once before all tests start.    |
| `afterAll`   | `string` | Runs once after all tests complete.  |
| `beforeEach` | `string` | Runs before each individual test.    |
| `afterEach`  | `string` | Runs after each individual test.     |

```yaml
hooks:
  beforeAll: "./scripts/db-seed.sh"
  afterAll: "./scripts/db-cleanup.sh"
  beforeEach: "echo 'Starting test...'"
  afterEach: "echo 'Test complete.'"
```

## Reporters

Configure multiple reporters for different output formats.

Each reporter has these fields:

| Field     | Type                                    | Default | Description                          |
|-----------|-----------------------------------------|---------|--------------------------------------|
| `type`    | `"console" \| "junit" \| "html" \| "json"` | —   | Required. Reporter type.             |
| `output`  | `string`                                | —       | Output file path (required for non-console). |
| `verbose` | `boolean`                               | —       | Show detailed step output.           |

### Console Reporter

Real-time terminal output:

```yaml
reporters:
  - type: console
    verbose: true     # Show detailed step output
```

### JUnit Reporter

XML format for CI/CD integration:

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

## Multiple Environments

Define multiple environments for different stages:

```yaml
environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      postgresql:
        connectionString: "postgresql://postgres:postgres@localhost:5432/dev"

  staging:
    baseUrl: "https://staging-api.example.com"
    adapters:
      postgresql:
        connectionString: "${STAGING_DB_URL}"

  production:
    baseUrl: "https://api.example.com"
    adapters:
      postgresql:
        connectionString: "${PROD_DB_URL}"
```

Run against specific environment:

```bash
tryve run --env staging
tryve run --env production
```
