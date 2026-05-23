# CLI Reference

Complete reference for all CLI commands and options.

## Installation

```bash
# Local installation
npm install @liemle3893/go-autoflow

# Global installation
npm install -g @liemle3893/go-autoflow
```

## Commands Overview

| Command | Description |
|---------|-------------|
| `e2e run` | Execute E2E tests (default) |
| `e2e validate` | Validate test files without execution |
| `e2e list` | List discovered tests |
| `e2e health` | Check adapter connectivity |
| `e2e init` | Initialize project structure and config |
| `e2e test create <name>` | Create test from template |
| `e2e test list-templates` | List available test templates |
| `e2e scaffold --ticket KEY --area AREA --count N` | Generate E2E test stubs for a ticket |
| `e2e doc [section]` | Show bundled documentation |
| `install --skills` | Install Claude Code skill bundle (top-level command) |

---

## `e2e run`

Execute E2E tests with filtering and execution options.

```bash
e2e run [options] [patterns...]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name | `local` |
| `-d, --test-dir <path>` | Test directory | Config `testDir` or `.` |
| `--report-dir <path>` | Report output directory | `./reports` |
| `-p, --parallel <n>` | Parallel test count | `1` |
| `-t, --timeout <ms>` | Test timeout | `30000` |
| `-r, --retries <n>` | Retry failed tests | `0` |
| `--bail` | Stop on first failure | `false` |
| `--watch` | Watch mode | `false` |
| `-g, --grep <pattern>` | Filter by name regex | |
| `--tag <tag>` | Filter by tag (repeatable) | |
| `--priority <level>` | Filter by priority (repeatable) | |
| `--skip-setup` | Skip setup phase | `false` |
| `--skip-teardown` | Skip teardown phase | `false` |
| `--dry-run` | List tests without execution | `false` |
| `--failed-only` | Rerun only previously failed tests | `false` |
| `--reporter <type>` | Reporter type (repeatable) | `console` |
| `-o, --output <path>` | Report output path | |
| `--debug` | Enable debug logging | `false` |
| `--step-by-step` | Step-by-step execution mode | `false` |
| `--capture-traffic` | Capture network traffic | `false` |
| `-v, --verbose` | Verbose output | `false` |
| `-q, --quiet` | Errors only | `false` |
| `--no-color` | Disable colors | `false` |

### Examples

**Basic run:**
```bash
# Run all tests in local environment
e2e run --env local

# Run with specific config
e2e run --config ./custom-config.yaml --env staging
```

**Filtering:**
```bash
# Filter by name pattern
e2e run --grep "user"
e2e run --grep "TC-USER-.*"

# Filter by tag (all tags must match)
e2e run --tag smoke
e2e run --tag e2e --tag user

# Filter by priority
e2e run --priority P0
e2e run --priority P0 --priority P1

# Combine filters
e2e run --grep "create" --tag user --priority P0
```

**Execution control:**
```bash
# Run 4 tests in parallel
e2e run --parallel 4

# Set timeout to 60 seconds
e2e run --timeout 60000

# Retry failed tests 3 times
e2e run --retries 3

# Stop on first failure
e2e run --bail
```

**Phase control:**
```bash
# Skip setup phase (use existing data)
e2e run --skip-setup

# Skip teardown (keep test data)
e2e run --skip-teardown
```

**Reporting:**
```bash
# Multiple reporters
e2e run --reporter console --reporter junit --reporter html

# Specify output paths
e2e run --reporter junit -o ./reports/results.xml

# Custom report directory
e2e run --report-dir ./test-reports

# Debug mode
e2e run --debug --verbose
```

**Dry run:**
```bash
# List tests without running
e2e run --dry-run
e2e run --dry-run --grep "user"
```

**Test directory:**
```bash
# Run tests from current directory (default)
e2e run

# Run tests from a specific directory
e2e run --test-dir ./tests/e2e
e2e run -d ./integration-tests

# Using environment variable
E2E_TEST_DIR=./tests/e2e e2e run
```

---

## `e2e validate`

Validate test files and configuration without executing tests.

```bash
e2e validate [options] [patterns...]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name | `local` |
| `-d, --test-dir <path>` | Test directory | Config `testDir` or `.` |
| `-v, --verbose` | Verbose output | `false` |
| `-q, --quiet` | Errors only | `false` |
| `--no-color` | Disable colors | `false` |

### Examples

```bash
# Validate all tests
e2e validate --env local

# Validate specific directory
e2e validate --test-dir tests/e2e/users

# Verbose validation
e2e validate --verbose
```

### What it validates:

- Configuration file syntax and structure
- YAML test file syntax and schema
- TypeScript test file syntax and exports
- Required fields (name, execute)
- Adapter action validity

---

## `e2e list`

List discovered tests with metadata.

```bash
e2e list [options] [patterns...]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name | `local` |
| `-d, --test-dir <path>` | Test directory | Config `testDir` or `.` |
| `-g, --grep <pattern>` | Filter by name regex | |
| `--tag <tag>` | Filter by tag (repeatable) | |
| `--priority <level>` | Filter by priority (repeatable) | |
| `-o, --output <format>` | Output format (`json` for JSON) | table |
| `-v, --verbose` | Verbose output | `false` |
| `-q, --quiet` | Errors only | `false` |
| `--no-color` | Disable colors | `false` |

### Examples

```bash
# List all tests
e2e list

# List with filters
e2e list --tag smoke
e2e list --priority P0

# Verbose output (shows file path, skip reasons)
e2e list --verbose

# JSON output (for scripting)
e2e list --output json
```

### Output

```
Discovered E2E Tests
================================================================================

  Name                                    Type        Priority  Tags
  ------------------------------------------------------------------------------
  TC-USER-001                             YAML        P0        user, crud
  TC-ORDER-001                            YAML        P1        order, e2e
  TC-CACHE-001                            YAML        P0        redis, cache
  TC-HEALTH-001                           TypeScript  P0        smoke
  TC-INT-001                              YAML        P1        integration

Summary
----------------------------------------
  Total tests:      5
  YAML tests:       4
  TypeScript tests: 1

  By Priority:
    P0 (Critical):  3
    P1 (High):      2
    P2 (Medium):    0
    P3 (Low):       0
```

---

## `e2e health`

Check adapter connectivity and health.

```bash
e2e health [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name | `local` |
| `--adapter <type>` | Check specific adapter | All |
| `-v, --verbose` | Verbose output | `false` |
| `-q, --quiet` | Errors only | `false` |
| `--no-color` | Disable colors | `false` |

### Examples

```bash
# Check all adapters
e2e health --env local

# Check specific adapter
e2e health --env local --adapter postgresql
e2e health --env staging --adapter redis
```

### Output

```
E2E Adapter Health Check
==================================================

Environment: local

Checking adapters...

  ✓ HTTP            HEALTHY (12ms)
  ✓ PostgreSQL      HEALTHY (45ms)
  ✓ Redis           HEALTHY (8ms)
  ✓ MongoDB         HEALTHY (23ms)
  ✗ EventHub        UNHEALTHY
    Error: Connection timed out

Summary
----------------------------------------
  Total adapters: 5
  Healthy:        4
  Unhealthy:      1
  Avg latency:    22ms
```

---

## `e2e init`

Initialize E2E test project structure with configuration, example tests, schemas, and environment template.

```bash
e2e init [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-v, --verbose` | Verbose output | `false` |
| `-q, --quiet` | Errors only | `false` |
| `--no-color` | Disable colors | `false` |

### Examples

```bash
# Initialize project structure
e2e init
```

### What it creates:

**Directories:**
- `tests/e2e/` — Main test directory
- `tests/e2e/schemas/` — JSON schema files for validation
- `tests/e2e/examples/` — Example test files
- `tests/e2e/reports/` — Report output directory
- `tests/e2e/fixtures/` — Test fixture data

**Files:**
- `tests/e2e/e2e.config.yaml` — Configuration file
- `tests/e2e/examples/TC-EXAMPLE-001.test.yaml` — Example YAML test
- `tests/e2e/examples/TC-EXAMPLE-002.test.ts` — Example TypeScript test
- `tests/e2e/schemas/e2e-config.schema.json` — Config JSON schema
- `tests/e2e/schemas/e2e-test.schema.json` — Test file JSON schema
- `.env.e2e.example` — Environment variable template

Existing files are not overwritten. The command will skip files that already exist.

### Generated Config Template

```yaml
# E2E Test Runner Configuration
version: "1.0"

environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      postgresql:
        connectionString: "${POSTGRESQL_CONNECTION_STRING}"
      redis:
        connectionString: "${REDIS_CONNECTION_STRING}"
      mongodb:
        connectionString: "${MONGODB_CONNECTION_STRING}"

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

---

## `e2e test create <name>`

Create a new test file from a template.

```bash
e2e test create <name> [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `--template <type>`, `-T` | Template type | `api` |
| `--description <text>`, `-D` | Test description | `E2E test for <name>` |
| `-o, --output <path>` | Output directory | Config `testDir` or `.` |
| `--test-priority <level>` | Priority: P0, P1, P2, P3 | `P0` |
| `--test-tags <tags>` | Comma-separated tags | `e2e` |

### Templates

| Template | Description |
|----------|-------------|
| `api` | Simple API test (GET/POST with assertions) |
| `crud` | Full CRUD operations with DB verification |
| `integration` | Multi-adapter test (HTTP + PostgreSQL + Redis + MongoDB) |
| `event-driven` | EventHub publish/consume pattern |
| `db-verification` | Direct database assertion patterns |

### Examples

```bash
# Create a basic API test (default template)
e2e test create user-crud

# Create with specific template
e2e test create order-flow --template crud
e2e test create user-sync -T integration

# Specify description and tags
e2e test create login-flow --template api --description "Login authentication flow" --test-tags "auth,smoke"

# Output to specific directory
e2e test create TC-PAYMENT-001 --template crud -o ./tests/e2e/payments

# Creates: ./tests/e2e/payments/TC-PAYMENT-001.test.yaml
```

---

## `e2e test list-templates`

List all available test templates.

```bash
e2e test list-templates
```

### Output

```
Available templates:

  api             Simple API test (GET/POST with assertions)
  crud            Full CRUD operations with DB verification
  integration     Multi-adapter test (HTTP + PostgreSQL + Redis + MongoDB)
  event-driven    EventHub publish/consume pattern
  db-verification Direct database assertion patterns
```

---

## `e2e doc`

Display bundled documentation sections. When invoked without arguments, lists all available sections. When a section name is provided, prints the full content of that section to stdout.

```bash
e2e doc [section]
```

### Available Sections

| Section | Description |
|---------|-------------|
| `yaml-test` | YAML test file syntax and structure |
| `assertions` | Assertion operators and JSONPath syntax |
| `built-in-functions` | Built-in functions (`$uuid`, `$timestamp`, etc.) |
| `config` | `e2e.config.yaml` configuration reference |
| `cli` | CLI commands and options |
| `examples` | Common test patterns and recipes |
| `adapters` | Adapter overview and comparison |
| `adapters.http` | HTTP adapter for REST API testing |
| `adapters.postgresql` | PostgreSQL adapter |
| `adapters.mongodb` | MongoDB adapter |
| `adapters.redis` | Redis adapter |
| `adapters.eventhub` | Azure EventHub adapter |
| `adapters.kafka` | Apache Kafka adapter |

### Examples

```bash
# List all available documentation sections
e2e doc

# View a specific section
e2e doc assertions
e2e doc adapters.http
e2e doc yaml-test
```

---

## `e2e install`

Install bundled assets into the current project. Currently supports installing the Claude Code skill bundle.

```bash
e2e install --skills
```

### Options

| Option | Description |
|--------|-------------|
| `--skills` | Install Claude Code skills to `.claude/skills/autoflow-cli/` |

When invoked without `--skills`, the command prints usage help.

### What it installs

Running `e2e install --skills` copies the following into your project:

- `.claude/skills/autoflow-cli/SKILL.md` -- The main skill bundle file
- `.claude/skills/autoflow-cli/references/` -- All documentation section files (mirrors `docs/sections/`)

The `references/` directory contains the same markdown files available via `e2e doc`, allowing Claude Code to reference them directly as skill context.

### Examples

```bash
# Install Claude Code skills to the current project
e2e install --skills
```

---

## Exit Codes

| Code | Name | Description |
|------|------|-------------|
| `0` | SUCCESS | All tests passed |
| `1` | TEST_FAILURE | One or more tests failed |
| `2` | CONFIG_ERROR | Configuration file error (missing, invalid, or parse error) |
| `3` | CONNECTION_ERROR | Adapter connection failed |
| `4` | VALIDATION_ERROR | Test file validation error |
| `5` | TIMEOUT | Test or operation timed out |
| `127` | FATAL | Unexpected fatal error or unknown command |

### Usage in CI/CD

```bash
# Exit with appropriate code
e2e run --env staging || exit 1

# Check specific exit code
e2e run --env staging
case $? in
  0) echo "All tests passed" ;;
  1) echo "Tests failed" ;;
  2) echo "Configuration error" ;;
  3) echo "Connection error" ;;
  4) echo "Validation error" ;;
  5) echo "Timeout" ;;
  *) echo "Fatal error" ;;
esac
```

---

## Environment Variables

The CLI respects these environment variables:

| Variable | Description |
|----------|-------------|
| `E2E_CONFIG` | Default config file path |
| `E2E_ENV` | Default environment name |
| `E2E_TEST_DIR` | Default test directory |
| `E2E_REPORT_DIR` | Default report output directory |
| `E2E_VERBOSE` | Enable verbose output (`true` or `1`) |
| `NO_COLOR` | Disable colored output (`true` or `1`) |

```bash
# Set defaults
export E2E_CONFIG=./config/e2e.yaml
export E2E_ENV=staging
export E2E_REPORT_DIR=./test-reports

# Now runs with these defaults
e2e run
```
