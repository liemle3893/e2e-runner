# CLI Reference

Complete reference for all CLI commands and options.

## Installation

Tryve is a single binary with no runtime dependencies.

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/liemle3893/go-tryve/main/install.sh | sh

# Windows (PowerShell)
irm https://raw.githubusercontent.com/liemle3893/go-tryve/main/install.ps1 | iex

# Go toolchain
go install github.com/liemle3893/go-tryve/cmd/tryve@latest

# From source
git clone https://github.com/liemle3893/go-tryve.git && cd go-tryve && make build
```

## Commands Overview

| Command | Description |
|---------|-------------|
| `tryve run` | Execute E2E tests (default) |
| `tryve validate` | Validate test files without execution |
| `tryve list` | List discovered tests |
| `tryve health` | Check adapter connectivity |
| `tryve init` | Initialize project structure and config |
| `tryve migrate` | Move a suite to a new compatibility level |
| `tryve test create <name>` | Create test from template |
| `tryve test list-templates` | List available test templates |
| `tryve doc [section]` | Show bundled documentation |
| `tryve install --skills` | Install Claude Code skill bundle |

---

## `tryve run`

Execute E2E tests with filtering and execution options.

```bash
tryve run [options] [path...]
```

Each path may be a **test file**, a **directory** to search recursively, or a
**glob** (including `**`). With no path, the configured `testDir` is searched.
A path that matches no test is an error, so a typo is reported rather than
silently running a different set of tests.

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path (global flag) | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name (global flag) | `local` |
| `-d, --test-dir <path>` | Test directory | Config `testDir` or `tests` |
| `-p, --parallel <n>` | Parallel test count (0 = config default) | `0` |
| `-t, --timeout <ms>` | Per-test timeout (0 = config default) | `0` |
| `-r, --retries <n>` | Retry failed tests (-1 = config default) | `-1` |
| `--bail` | Stop on first failure | `false` |
| `--watch` | Re-run tests on file changes | `false` |
| `-g, --grep <pattern>` | Filter by name (regex or substring) | |
| `--tag <tag>` | Filter by tag (repeatable) | |
| `--priority <level>` | Filter by priority (`P0`–`P3`; single value) | |
| `--skip-setup` | Skip setup phase | `false` |
| `--skip-teardown` | Skip teardown phase | `false` |
| `--dry-run` | List tests without execution | `false` |
| `--failed-only` | Rerun only previously failed tests | `false` |
| `--strict` | Fail a step when a `{{expression}}` cannot be resolved | Config `defaults.strictResolve` |
| `--api-version <v>` | Behaviour level: `tryve/v1` (previous) or `tryve/v2` (current) | Config `apiVersion`, else `tryve/v1` |
| `--reporter <type>` | Additional reporter: `junit`, `html`, `json` (repeatable) | `console` |
| `-o, --output <path>` | Output file for file-based reporters | |
| `--verbose` | Show per-step output | `false` |
| `--debug` | Show full request/response data for every step | `false` |

### Examples

**Basic run:**
```bash
# Run all tests in local environment
tryve run --env local

# Run with specific config
tryve run --config ./custom-config.yaml --env staging
```

**Filtering:**
```bash
# Filter by name pattern
tryve run --grep "user"
tryve run --grep "TC-USER-.*"

# Filter by tag (all tags must match)
tryve run --tag smoke
tryve run --tag e2e --tag user

# Filter by priority
tryve run --priority P0
tryve run --priority P0 --priority P1

# Combine filters
tryve run --grep "create" --tag user --priority P0
```

**Execution control:**
```bash
# Run 4 tests in parallel
tryve run --parallel 4

# Set timeout to 60 seconds
tryve run --timeout 60000

# Retry failed tests 3 times
tryve run --retries 3

# Stop on first failure
tryve run --bail
```

**Phase control:**
```bash
# Skip setup phase (use existing data)
tryve run --skip-setup

# Skip teardown (keep test data)
tryve run --skip-teardown
```

**Reporting:**
```bash
# Multiple reporters
tryve run --reporter junit --reporter html

# Specify output paths
tryve run --reporter junit -o ./reports/results.xml

# Debug mode
tryve run --debug --verbose
```

**Dry run:**
```bash
# List tests without running
tryve run --dry-run
tryve run --dry-run --grep "user"
```

**Rerun failures:**
```bash
# After a run with failures, rerun only those that failed
tryve run --failed-only

# Combine with other options
tryve run --failed-only --retries 2
```

**Choosing what to run:**
```bash
# Everything under the configured testDir
tryve run

# One file — the fastest way to iterate on a single test
tryve run tests/e2e/users/TC-USER-001.test.yaml

# Several directories
tryve run tests/e2e/users tests/e2e/auth

# A glob (quote it so the shell does not expand it first)
tryve run 'tests/e2e/**/TC-AUTH-*.test.yaml'

# The --test-dir flag still works and is equivalent to a single directory path
tryve run --test-dir ./tests/e2e
```

Naming files directly also skips parsing the rest of the suite, so a single-test
run starts in milliseconds rather than seconds.

**Behaviour level:**
```bash
# See what adopting the current behaviour would do, without editing the config
tryve run --api-version tryve/v2

# Hold a run to the previous behaviour
tryve run --api-version tryve/v1
```

An absent `apiVersion` in the config means `tryve/v1`, so pointing a new binary
at an existing suite never changes how it behaves. Run `tryve doc config` for
what differs between the versions.

**Strict resolution:**
```bash
# Fail the step when {{captured.typo}} resolves to nothing, instead of sending
# the literal text "{{captured.typo}}" to the system under test
tryve run --strict
```

---

## `tryve migrate`

Move a suite to a new compatibility level. A suite of any size cannot be
migrated by reviewing every file at once, so this raises the suite's level and
**pins the affected files to their current one** — the suite keeps passing
exactly as it does today, and each pinned file is then worked through
individually.

```bash
tryve migrate [path...]
```

Nothing is written without `--apply`.

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path (global flag) | `e2e.config.yaml` |
| `-d, --test-dir <path>` | Test directory | Config `testDir` |
| `--to <version>` | Target apiVersion | `tryve/v2` |
| `--area <name>` | Limit to `assertions`, `interpolation`, `execution`, or `adapters` (repeatable) | all |
| `--apply` | Write: raise the config, pin affected files | `false` |
| `--explain` | List every difference in the named files | `false` |
| `--status` | Report how many files are still pinned | `false` |
| `--unpin` | Remove the pin from the named files | `false` |
| `--only-certain` | Pin only files that provably change, not those that merely may | `false` |

### The migration

```bash
# 1. See what would change
tryve migrate

# 2. Raise the suite, pinning what would break
tryve migrate --apply

# 3. Confirm nothing moved
tryve run

# 4. Work through the pinned files one at a time
tryve migrate --status
tryve migrate --explain tests/e2e/users/TC-USER-001.test.yaml
#    …fix what it reports, then:
tryve migrate --unpin tests/e2e/users/TC-USER-001.test.yaml
tryve run tests/e2e/users/TC-USER-001.test.yaml
```

Take one area at a time on a large suite — `assertions` first, since that is
where checks that never ran are:

```bash
tryve migrate --area assertions --apply
```

### Certainty

Each difference is reported as **will change** — decidable from the file, such
as an assertion form that was discarded — or **may change**, which depends on
runtime values: whether a captured value is a number, whether a response body is
an array, how long a command takes.

`--apply` pins both by default, because a difference that only *may* materialise
still breaks the suite when it does. `--only-certain` narrows the pin set for a
tighter diff, at the cost of a suite that may not be green immediately.

### Pins

A pin is an `apiVersion` at the top of a test file, overriding the suite's level
for that file alone:

```yaml
# Pinned by `tryve migrate`: this file relies on behaviour that changed.
# Run `tryve migrate --explain <this file>` for what differs, fix what it
# reports, then delete these three lines to move it to the suite's level.
apiVersion: tryve/v1

name: TC-USER-001
```

With `--area`, the suite gains a `compatibility` refinement instead of a new
`apiVersion`, and files are pinned to the version they are on today.

Pinning is purely additive — comments, key order, and block scalars are left
byte-for-byte intact, so the diff is reviewable.

---

## `tryve validate`

Parse and validate test files without executing them.

```bash
tryve validate [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path (global flag) | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name (global flag) | `local` |
| `-d, --test-dir <path>` | Test directory | Config `testDir` or `tests` |

### Examples

```bash
# Validate all tests
tryve validate --env local

# Validate specific directory
tryve validate --test-dir tests/e2e/users

# Validate one file
tryve validate --test-dir tests/e2e/users/TC-USER-001.test.yaml
```

### What it validates:

- YAML test file syntax
- Required fields (`name`, at least one `execute` step)
- `priority`, `timeout`, and `retries` are within range
- Every step names a known adapter and an action that adapter supports
- Per-adapter required params (`url` for http, `command` for shell, `sql` for
  postgresql, and so on)

It does not connect to anything — use `tryve health` for that.

---

## `tryve list`

List discovered test files and their metadata.

```bash
tryve list [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path (global flag) | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name (global flag) | `local` |
| `-d, --test-dir <path>` | Test directory | Config `testDir` or `tests` |
| `-g, --grep <pattern>` | Filter by name (regex or substring) | |
| `--tag <tag>` | Filter by tag (repeatable) | |
| `--priority <level>` | Filter by priority (`P0`–`P3`; single value) | |

### Examples

```bash
# List all tests
tryve list

# List with filters
tryve list --tag smoke
tryve list --priority P0

# Filter, then list
tryve list --tag smoke --priority P0
```

### Output

```
  Discovered Tests
  ────────────────────────────────────────────────────────

   P0   TC-USER-001  #user #crud
        /repo/tests/e2e/users/TC-USER-001.test.yaml
   P1   TC-ORDER-001  #order #e2e
        /repo/tests/e2e/orders/TC-ORDER-001.test.yaml

  ────────────────────────────────────────────────────────
  2 test(s)  P0:1  P1:1
```

---

## `tryve health`

Check adapter connectivity and health.

```bash
tryve health [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path (global flag) | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name (global flag) | `local` |

`health` checks every adapter configured for the active environment.

### Examples

```bash
# Check all adapters
tryve health --env local

# Check specific adapter
tryve health --env staging
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

## `tryve init`

Initialize E2E test project structure with configuration, example tests, schemas, and environment template.

```bash
tryve init [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-c, --config <path>` | Config file path (global flag) | `e2e.config.yaml` |
| `-e, --env <name>` | Environment name (global flag) | `local` |

### Examples

```bash
# Initialize project structure
tryve init
```

### What it creates:

A single starter `e2e.config.yaml` in the current directory, with a `local`
environment and commented adapter blocks to fill in.

An existing `e2e.config.yaml` is never overwritten.

To create your first test afterwards, use `tryve test create <name>`.
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

## `tryve test create <name>`

Create a new test file from a template.

```bash
tryve test create <name> [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-t, --template <type>` | Template to use: `http` or `shell` | `http` |
| `-o, --output <path>` | Output file path | `<name>.test.yaml` |

Run `tryve test list-templates` for the current list.

### Templates

| Template | Description |
|----------|-------------|
| `http` | HTTP request with status and JSON body assertions |
| `shell` | Shell command with exit-code and stdout assertions |

### Examples

```bash
# Create an HTTP test (default template) as ./user-crud.test.yaml
tryve test create user-crud

# Create a shell test
tryve test create check-migrations --template shell

# Write to a specific path
tryve test create TC-PAYMENT-001 -o ./tests/e2e/payments/TC-PAYMENT-001.test.yaml
```

The generated file is a starting point — fill in the URL, assertions, and tags.

---

## `tryve test list-templates`

List all available test templates.

```bash
tryve test list-templates
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

## `tryve doc`

Display bundled documentation sections. When invoked without arguments, lists all available sections. When a section name is provided, prints the full content of that section to stdout.

```bash
tryve doc [section]
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
tryve doc

# View a specific section
tryve doc assertions
tryve doc adapters.http
tryve doc yaml-test
```

---

## `tryve install`

Install bundled assets into the current project. Currently supports installing the Claude Code skill bundle.

```bash
tryve install --skills
```

### Options

| Option | Description |
|--------|-------------|
| `--skills` | Install Claude Code skills to `.claude/skills/e2e-runner/` |

When invoked without `--skills`, the command prints usage help.

### What it installs

Running `tryve install --skills` copies the following into your project:

- `.claude/skills/e2e-runner/SKILL.md` -- The main skill bundle file
- `.claude/skills/e2e-runner/references/` -- All documentation section files (mirrors `docs/sections/`)

The `references/` directory contains the same markdown files available via `tryve doc`, allowing Claude Code to reference them directly as skill context.

### Examples

```bash
# Install Claude Code skills to the current project
tryve install --skills
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
tryve run --env staging || exit 1

# Check specific exit code
tryve run --env staging
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
tryve run
```
