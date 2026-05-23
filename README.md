# Autoflow

A YAML-driven, multi-protocol end-to-end test runner. Write tests in YAML, run them against HTTP APIs and databases, and get results in console, JUnit, HTML, or JSON.

Single binary. Zero runtime dependencies. Cross-platform.

## Installation

### Shell (Linux / macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/liemle3893/go-autoflow/main/install.sh | sh
```

Options: `--dir /custom/path` to change install location, `--version v2.0.0` to pin a version.

### PowerShell (Windows)

```powershell
irm https://raw.githubusercontent.com/liemle3893/go-autoflow/main/install.ps1 | iex
```

Options: `-Dir C:\custom\path` to change install location, `-Version v2.0.0` to pin a version.

### Go Install

```bash
go install github.com/liemle3893/go-autoflow/cmd/autoflow@latest
```

### From Source

```bash
git clone https://github.com/liemle3893/go-autoflow.git
cd go-autoflow
make build    # binary at ./bin/autoflow
```

## Quick Start

```bash
# Create a config file
autoflow e2e init

# Create a test
autoflow e2e test create my-api-test

# Run all tests
autoflow e2e run

# Run with filters
autoflow e2e run --tag smoke --bail
```

## Writing Tests

Tests are YAML files with four optional phases: `setup`, `execute`, `verify`, and `teardown`.

```yaml
name: Create and verify user
description: Full user lifecycle test
tags: [smoke, users]
priority: P1

variables:
  email: "test-${uuid()}@example.com"

setup:
  - id: clean-slate
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
        email: "${email}"
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
        expected: "${email}"

verify:
  - id: check-db
    adapter: postgresql
    action: query
    params:
      query: "SELECT * FROM users WHERE id = $1"
      params: ["${captured.userId}"]
    assertions:
      - path: $.rows[0].email
        operator: equals
        expected: "${email}"

teardown:
  - id: cleanup
    adapter: postgresql
    action: exec
    params:
      query: "DELETE FROM users WHERE id = $1"
      params: ["${captured.userId}"]
```

## Configuration

`e2e.config.yaml` defines environments, adapters, defaults, and variables.

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

## Adapters

| Adapter | Actions | Config Key |
|---------|---------|------------|
| **HTTP** | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS` | `baseUrl` |
| **PostgreSQL** | `query`, `exec` | `connectionString` |
| **MongoDB** | `find`, `findOne`, `insertOne`, `updateOne`, `deleteOne`, `deleteMany`, `countDocuments`, `aggregate` | `connectionString` |
| **Redis** | `get`, `set`, `del`, `exists`, `hget`, `hset`, `hgetall`, `lpush`, `rpush`, `lrange`, `sadd`, `smembers`, `publish` | `connectionString` |
| **Kafka** | `produce`, `consume` | `brokers` |
| **Azure EventHub** | `send`, `receive` | `connectionString`, `eventHubName` |
| **Shell** | `exec` | _(none)_ |

## Assertions

Use JSONPath expressions in `path` to extract values, then assert with an operator.

| Operator | Description |
|----------|-------------|
| `equals` | Strict equality |
| `notEquals` | Not equal |
| `contains` | String/array contains |
| `notContains` | Does not contain |
| `matches` | Regex match |
| `type` | Type check (`string`, `number`, `boolean`, `array`, `object`, `null`) |
| `exists` | Value is present |
| `notExists` | Value is absent |
| `isNull` | Value is null |
| `isNotNull` | Value is not null |
| `greaterThan` | Numeric `>` |
| `lessThan` | Numeric `<` |
| `greaterThanOrEqual` | Numeric `>=` |
| `lessThanOrEqual` | Numeric `<=` |
| `length` | Array/string length equals |
| `isEmpty` | Array/string is empty |
| `notEmpty` | Array/string is not empty |
| `hasProperty` | Object has key |
| `notHasProperty` | Object lacks key |

## Built-in Functions

Use `${functionName(args)}` in any string value.

| Function | Description | Example Output |
|----------|-------------|----------------|
| `uuid()` | UUID v4 | `550e8400-e29b-41d4-a716-446655440000` |
| `now()` | Current time (ISO 8601) | `2026-04-12T10:30:00Z` |
| `timestamp()` | Unix timestamp (seconds) | `1744454400` |
| `isoDate()` | Current date (ISO) | `2026-04-12` |
| `random()` | Random float 0-1 | `0.7234` |
| `randomString(len)` | Random alphanumeric string | `aB3kZ9mQ` |
| `env(name)` | Environment variable | _(value of $name)_ |
| `file(path)` | Read file contents | _(file contents)_ |
| `base64(value)` | Base64 encode | `aGVsbG8=` |
| `base64Decode(value)` | Base64 decode | `hello` |
| `md5(value)` | MD5 hash | `5d41402abc4b2a76...` |
| `sha256(value)` | SHA-256 hash | `2cf24dba5fb0a30e...` |
| `dateAdd(duration)` | Current time + duration | `2026-04-13T10:30:00Z` |
| `dateSub(duration)` | Current time - duration | `2026-04-11T10:30:00Z` |
| `totp(secret)` | TOTP code | `483920` |
| `jsonStringify(value)` | JSON encode | `"{\"key\":\"val\"}"` |
| `lower(value)` | Lowercase | `hello` |
| `upper(value)` | Uppercase | `HELLO` |
| `trim(value)` | Trim whitespace | `hello` |

## CLI Reference

### Commands

```
autoflow e2e run          Run tests
autoflow e2e validate     Validate test file syntax
autoflow e2e list         List discovered tests
autoflow e2e health       Check adapter connectivity
autoflow e2e init         Create e2e.config.yaml
autoflow e2e test create  Create a test from template
autoflow e2e doc          Browse built-in documentation
autoflow install      Install all Claude Code skills + agents (default; narrow with --skills or --autoflow)
autoflow     Jira-to-PR autoflow helpers (see below)
autoflow version      Print version
```

### `autoflow autoflow`

A Go port of [`winx-autoflow`](https://github.com/the-winx-corp/winx-ai-autoflow).
Adds a Jira-to-PR delivery workflow to the autoflow binary with no external
runtime dependencies (`git` and `gh` only).

```
autoflow deliver jira config {set,get,del,show}                    Manage .autoflow/jira-config.json
autoflow deliver jira upload <KEY> <file>...                       Upload attachments to a Jira issue
autoflow deliver jira download <KEY> <dir>                         Download attachments from a Jira issue
autoflow deliver worktree bootstrap <path>                         Copy .claude + config files into a worktree
autoflow deliver {init,next,complete}                              13-step delivery state machine
autoflow deliver timings [--ticket KEY]                            Per-step duration report (or aggregate across tickets)
autoflow deliver loop-state {init,append,read,round-count}         Generic agentic-loop state manager
autoflow deliver sandbox {bootstrap,status}                        Manage the per-repo sbx sandbox used by coding agents
autoflow deliver config {set,get,del,show}                         Manage .autoflow/config.json (coding agent + sandbox)
autoflow deliver doctor                                            Preflight checklist (git, gh, Jira, install layout)
autoflow e2e scaffold --ticket KEY --area A --count N              Generate E2E test stubs
```

Install everything with `autoflow install` (or scope down with
`--skills` / `--autoflow`). This drops `.claude/skills/autoflow-*/` and
`.claude/agents/autoflow-*.md` into the current project and removes any
legacy `.claude/scripts/autoflow/` dir left by the old bash installer.

### `autoflow e2e run` Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--config` | `-c` | Config file path | `e2e.config.yaml` |
| `--env` | `-e` | Environment name | `local` |
| `--test-dir` | `-d` | Test directory | `tests` |
| `--parallel` | `-p` | Parallel test count | config default |
| `--timeout` | `-t` | Per-test timeout (ms) | config default |
| `--retries` | `-r` | Retry count on failure | config default |
| `--bail` | | Stop on first failure | `false` |
| `--grep` | `-g` | Filter by name (regex) | |
| `--tag` | | Filter by tag (repeatable) | |
| `--priority` | | Filter by priority (P0-P3) | |
| `--dry-run` | | Show matching tests without running | `false` |
| `--skip-setup` | | Skip setup phase | `false` |
| `--skip-teardown` | | Skip teardown phase | `false` |
| `--reporter` | | Reporter type (repeatable) | `console` |
| `--output` | `-o` | Output path for file reporters | |
| `--verbose` | | Show per-step detail | `false` |
| `--debug` | | Show full request/response data | `false` |
| `--watch` | | Re-run on file changes | `false` |

### Reporters

| Type | Output | Use Case |
|------|--------|----------|
| `console` | Terminal | Local development |
| `junit` | XML file | CI/CD integration |
| `html` | HTML file | Shareable reports |
| `json` | JSON file | Programmatic consumption |

```bash
# Multiple reporters
autoflow e2e run --reporter console --reporter junit -o reports/results.xml
```

## Programmatic Usage (Go)

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/liemle3893/go-autoflow/pkg/runner"
)

func main() {
    r, err := runner.New(
        runner.WithConfig("e2e.config.yaml"),
        runner.WithEnv("local"),
        runner.WithTags("smoke"),
        runner.WithParallel(4),
    )
    if err != nil {
        log.Fatal(err)
    }

    result, err := r.Run(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Passed: %d, Failed: %d\n", result.Passed, result.Failed)
}
```

## Development

```bash
make build       # Build binary to bin/autoflow
make test        # Run all tests
make test-v      # Run tests with verbose output
make lint        # Run golangci-lint
make clean       # Remove build artifacts
```

## License

MIT
