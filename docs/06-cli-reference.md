# CLI Reference

Complete reference for all CLI commands and options.

## Installation

```bash
# Local installation
npm install @liemle3893/e2e-runner

# Global installation
npm install -g @liemle3893/e2e-runner
```

## Commands Overview

| Command | Description |
|---------|-------------|
| `e2e run` | Execute E2E tests (default) |
| `e2e validate` | Validate test files without execution |
| `e2e list` | List discovered tests |
| `e2e health` | Check adapter connectivity |
| `e2e init` | Initialize configuration file |

---

## `e2e run`

Execute E2E tests with filtering and execution options.

```bash
e2e run [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name | `local` |
| `-d, --test-dir <path>` | Test directory | Current directory |
| `-p, --parallel <n>` | Parallel test count | `1` |
| `-t, --timeout <ms>` | Test timeout | `30000` |
| `-r, --retries <n>` | Retry failed tests | `0` |
| `--bail` | Stop on first failure | `false` |
| `-g, --grep <pattern>` | Filter by name regex | |
| `--tag <tag>` | Filter by tag (repeatable) | |
| `--priority <level>` | Filter by priority (repeatable) | |
| `--skip-setup` | Skip setup phase | `false` |
| `--skip-teardown` | Skip teardown phase | `false` |
| `--dry-run` | List tests without execution | `false` |
| `--reporter <type>` | Reporter type (repeatable) | `console` |
| `-o, --output <path>` | Report output path | |
| `--debug` | Enable debug logging | `false` |
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
e2e validate [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name | `local` |
| `-d, --test-dir <path>` | Test directory | Current directory |
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
- YAML test file syntax
- Required fields (name, execute)
- Adapter action validity
- Variable references
- JSONPath syntax

---

## `e2e list`

List discovered tests with metadata.

```bash
e2e list [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name | `local` |
| `-d, --test-dir <path>` | Test directory | Current directory |
| `-g, --grep <pattern>` | Filter by name regex | |
| `--tag <tag>` | Filter by tag (repeatable) | |
| `--priority <level>` | Filter by priority (repeatable) | |
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

# Verbose output (shows tags, priority, file path)
e2e list --verbose
```

### Output

```
Tests found: 5

  TC-USER-001       User CRUD operations           [P0] [user, crud]
  TC-ORDER-001      Order creation flow            [P1] [order, e2e]
  TC-CACHE-001      Redis cache test               [P0] [redis, cache]
  TC-HEALTH-001     Health check                   [P0] [smoke]
  TC-INT-001        Integration test               [P1] [integration]
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
Checking adapter health for environment: local

  ✓ HTTP          http://localhost:3000           12ms
  ✓ PostgreSQL    localhost:5432/mydb             45ms
  ✓ Redis         localhost:6379                   8ms
  ✓ MongoDB       localhost:27017/mydb            23ms
  ✗ EventHub      localhost (emulator)            TIMEOUT

4/5 adapters healthy
```

---

## `e2e init`

Initialize configuration file template.

```bash
e2e init [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `--force` | Overwrite existing config | `false` |

### Examples

```bash
# Create e2e.config.yaml
e2e init

# Overwrite existing
e2e init --force
```

### Generated Template

```yaml
version: "1.0"

environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      # postgresql:
      #   connectionString: "postgresql://user:pass@localhost:5432/db"
      # redis:
      #   connectionString: "redis://localhost:6379"
      # mongodb:
      #   connectionString: "mongodb://localhost:27017"
      #   database: "mydb"

defaults:
  timeout: 30000
  retries: 2
  retryDelay: 1000
  parallel: 4

variables:
  # Define global variables here
  # apiKey: "${API_KEY}"

reporters:
  - type: console
    verbose: true
  - type: junit
    output: "./reports/junit.xml"
  - type: html
    output: "./reports/report.html"
```

---

## Exit Codes

| Code | Name | Description |
|------|------|-------------|
| `0` | SUCCESS | All tests passed |
| `1` | TEST_FAILURE | One or more tests failed |
| `2` | VALIDATION_ERROR | Configuration or test validation failed |
| `3` | CONNECTION_ERROR | Adapter connection failed |
| `4` | EXECUTION_ERROR | Test execution error |
| `5` | FATAL | Unexpected fatal error |

### Usage in CI/CD

```bash
# Exit with appropriate code
e2e run --env staging || exit 1

# Check specific exit code
e2e run --env staging
case $? in
  0) echo "All tests passed" ;;
  1) echo "Tests failed" ;;
  2) echo "Validation error" ;;
  3) echo "Connection error" ;;
  *) echo "Execution error" ;;
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
| `E2E_DEBUG` | Enable debug mode (`true`/`false`) |
| `NO_COLOR` | Disable colored output |

```bash
# Set defaults
export E2E_CONFIG=./config/e2e.yaml
export E2E_ENV=staging

# Now runs with these defaults
e2e run
```
