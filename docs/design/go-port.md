# Tryve — Design Spec

> **Historical record.** This is the design that produced the current Go
> implementation, kept for the reasoning behind its decisions. It describes the
> port as it was planned in April 2026; where it and the code disagree, the code
> is right. Notably the plan to keep the TypeScript source under `ts/` was not
> carried out — the TypeScript implementation was removed entirely. For how the
> code is organised today, see `CLAUDE.md`.

**Date:** 2026-04-10
**Status:** Approved
**Approach:** Go-native architecture, full feature parity with TypeScript e2e-runner
**Binary name:** `tryve` (try + verify)

## Summary

Port the entire e2e-runner project (~16K LOC TypeScript) to Go as a single static binary called `tryve`. Users get one download — no Node.js, no npm, no peer dependency installation. The Go version runs the exact same YAML test files and `e2e.config.yaml` format. The TypeScript DSL is dropped; hooks become shell commands.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Scope | Full port — all 7 adapters, all 4 reporters, all features | User wants complete replacement |
| YAML format | Identical to TypeScript version | Zero migration for existing users |
| Config format | Identical, hooks become shell commands | Single binary can't run .ts files |
| TypeScript DSL | Dropped | Go binary can't execute TypeScript |
| Distribution | Cross-compiled binaries + `go install` | GitHub releases for all platforms |
| CGo | None — pure Go only | Clean cross-compilation, single static binary |
| Repo strategy | Same repo, Go at root, TypeScript moved to `ts/` | Single source of truth |
| Binary name | `tryve` (try + verify) | Unique, 5 chars, not taken by any existing tool |
| Architecture | Go-native (interfaces, context, goroutines) | Idiomatic, maintainable, performant |

## Project Layout

```
e2e-runner/
├── ts/                        # TypeScript source (moved from root)
│   ├── src/
│   ├── package.json
│   └── ...
├── cmd/
│   └── tryve/
│       └── main.go            # CLI entrypoint (minimal — wires cobra to internal)
├── internal/
│   ├── adapter/               # Adapter interface + 7 implementations
│   │   ├── adapter.go         # Interface + StepResult + helpers
│   │   ├── registry.go        # Lazy adapter registry
│   │   ├── http.go
│   │   ├── postgresql.go
│   │   ├── mongodb.go
│   │   ├── redis.go
│   │   ├── kafka.go
│   │   ├── eventhub.go
│   │   └── shell.go
│   ├── assertion/             # Assertion engine
│   │   ├── assertion.go       # runAssertion() entrypoint
│   │   ├── matchers.go        # All assertion operators
│   │   └── jsonpath.go        # JSONPath evaluation wrapper
│   ├── config/                # Configuration
│   │   ├── config.go          # Load + validate e2e.config.yaml
│   │   └── types.go           # Config struct definitions
│   ├── executor/              # Test execution
│   │   ├── orchestrator.go    # Suite-level: discovery, filtering, ordering, worker pool
│   │   ├── runner.go          # Test-level: phases, retries
│   │   ├── step.go            # Step-level: interpolate → execute → capture → assert
│   │   └── hooks.go           # Shell hook execution
│   ├── interpolate/           # Variable interpolation
│   │   ├── interpolate.go     # Parser + resolver
│   │   └── builtins.go        # 16 built-in functions
│   ├── loader/                # Test loading
│   │   ├── discovery.go       # Glob-based file discovery
│   │   ├── parser.go          # YAML → TestDefinition
│   │   └── validator.go       # Schema + semantic validation
│   ├── reporter/              # Reporting
│   │   ├── reporter.go        # Interface + multi-reporter
│   │   ├── console.go         # Colored terminal output
│   │   ├── junit.go           # JUnit XML
│   │   ├── html.go            # HTML report (html/template)
│   │   └── json.go            # JSON report
│   └── watcher/               # Watch mode
│       └── watcher.go         # fsnotify-based file watcher
├── pkg/
│   └── runner/                # Public Go API
│       └── runner.go          # RunTests, ValidateTests, ListTests, CheckHealth
├── go.mod
├── go.sum
├── Makefile
├── .goreleaser.yaml           # Cross-compilation release config
├── docs/
├── skills/
├── tests/                     # Shared YAML test files (work for both TS and Go)
└── CLAUDE.md
```

## Core Interfaces

### Adapter

```go
package adapter

import "context"

// Adapter is the single interface all 7 adapters implement.
type Adapter interface {
    // Name returns the adapter identifier (e.g., "http", "postgresql").
    Name() string

    // Connect establishes the connection. Called lazily when first needed.
    Connect(ctx context.Context) error

    // Close releases resources.
    Close(ctx context.Context) error

    // Health checks connectivity. Used by `e2e health`.
    Health(ctx context.Context) error

    // Execute runs an action (e.g., "request", "query", "produce").
    // params comes directly from the YAML step's `params` field after interpolation.
    // Returns structured result for capture and assertion.
    Execute(ctx context.Context, action string, params map[string]any) (*StepResult, error)
}

// StepResult is the unified return type from all adapters.
type StepResult struct {
    Data     map[string]any // Response data (body, rows, value, etc.)
    Duration time.Duration  // Execution time
    Metadata map[string]any // Adapter-specific metadata (status code, headers, etc.)
}
```

No base struct embedding. Shared logic (timing, result construction) lives as package-level helper functions that adapters call.

### Reporter

```go
package reporter

import "context"

// Reporter receives lifecycle events during test execution.
type Reporter interface {
    OnSuiteStart(ctx context.Context, suite *Suite) error
    OnTestStart(ctx context.Context, test *Test) error
    OnStepComplete(ctx context.Context, step *Step, result *StepResult) error
    OnTestComplete(ctx context.Context, test *Test, result *TestResult) error
    OnSuiteComplete(ctx context.Context, suite *Suite, result *SuiteResult) error
    Flush() error
}

// Multi wraps multiple reporters into one.
type Multi struct {
    reporters []Reporter
}
```

The orchestrator holds a single `reporter.Multi` and calls it at each lifecycle point. Each reporter writes to its own `io.Writer`.

### Built-in Functions

```go
package interpolate

type BuiltinFunc func(args ...string) (string, error)

var builtins = map[string]BuiltinFunc{
    "uuid":         builtinUUID,
    "timestamp":    builtinTimestamp,
    "isoDate":      builtinISODate,
    "random":       builtinRandom,
    "randomString": builtinRandomString,
    "env":          builtinEnv,
    "file":         builtinFile,
    "base64":       builtinBase64,
    "base64Decode": builtinBase64Decode,
    "md5":          builtinMD5,
    "sha256":       builtinSHA256,
    "now":          builtinNow,
    "dateAdd":      builtinDateAdd,
    "dateSub":      builtinDateSub,
    "totp":         builtinTOTP,
}
```

## Dependencies

All pure Go. No CGo.

| Purpose | Package | Why this one |
|---------|---------|-------------|
| CLI framework | `github.com/spf13/cobra` | Industry standard, subcommand support |
| YAML parsing | `gopkg.in/yaml.v3` | Stable, handles anchors/aliases |
| PostgreSQL | `github.com/jackc/pgx/v5` | Pure Go, connection pooling, best perf |
| MongoDB | `go.mongodb.org/mongo-driver/v2` | Official driver, pure Go |
| Redis | `github.com/redis/go-redis/v9` | Official, full feature set |
| Kafka | `github.com/segmentio/kafka-go` | Pure Go, no CGo (unlike confluent) |
| EventHub | `github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs` | Official Azure SDK |
| JSONPath | `github.com/ohler55/ojg` | Fast, supports full JSONPath spec |
| File watching | `github.com/fsnotify/fsnotify` | Standard, cross-platform |
| TOTP | `github.com/pquerna/otp` | RFC 6238 compliant |
| UUID | `github.com/google/uuid` | Google-maintained, v4 support |
| Hashing | stdlib `crypto/md5`, `crypto/sha256` | No external dep needed |
| Base64 | stdlib `encoding/base64` | No external dep needed |
| Console color | ANSI escape codes | No external dep needed |
| XML (JUnit) | stdlib `encoding/xml` | No external dep needed |
| HTML report | stdlib `html/template` | No external dep needed |
| JSON report | stdlib `encoding/json` | No external dep needed |
| Concurrency | `golang.org/x/sync/errgroup` | Bounded worker pool |

## Concurrency Model

### Parallel Test Execution

```go
g, ctx := errgroup.WithContext(suiteCtx)
g.SetLimit(config.Parallel) // e.g., 4

for _, test := range sortedTests {
    t := test
    g.Go(func() error {
        return runner.RunTest(ctx, t, adapters, reporters)
    })
}

err := g.Wait()
```

### Test Dependency Ordering

Tests with `depends` are topologically sorted. Independent tests are dispatched to the worker pool concurrently. A test with `depends: [other-test]` blocks until `other-test` completes successfully (coordinated via channels or sync primitives).

### Timeouts

Every operation gets a `context.WithTimeout`:
- Suite-level: optional global timeout
- Test-level: `timeout` field from YAML or default
- Step-level: adapter-specific timeouts
- Hook-level: configurable hook timeout

Cancellation propagates down: cancelling the suite context cancels all running tests.

## Variable Interpolation

### Syntax

Both `${varName}` and `{{varName}}` are supported (identical to TypeScript version).

### Resolution Order

1. Captured values from prior steps (`${captured.userId}`)
2. Test-level variables
3. Config-level variables
4. Environment variables (`${env.VAR_NAME}`)
5. Built-in functions (`${$uuid()}`, `${$random(1, 100)}`)

### Implementation

Regex-based parser: `\$\{([^}]+)\}` and `\{\{([^}]+)\}\}`. The matched expression is resolved against an `InterpolationContext` struct that holds all variable sources. Recursive resolution with cycle detection (max depth 10).

## Assertion Engine

### Supported Operators

Identical to TypeScript version:

- **Equality:** `equals`, `notEquals`
- **Comparison:** `greaterThan`, `lessThan`, `greaterThanOrEqual`, `lessThanOrEqual`
- **String:** `contains`, `notContains`, `matches` (regex)
- **Type:** `type` (string, number, object, array, null, boolean)
- **Existence:** `exists`, `notExists`, `isNull`, `isNotNull`
- **Collection:** `length`, `notEmpty`, `isEmpty`
- **Object:** `hasProperty`, `notHasProperty`

### HTTP-Specific Assertions

`status`, `statusRange`, `headers`, `json` (array of JSONPath assertions), `duration` — all parsed from the YAML `assert` block and dispatched to the appropriate matcher.

### JSONPath

Uses `github.com/ohler55/ojg` for JSONPath evaluation against `map[string]any` data. Supports property access, array indexing, wildcards, recursive descent.

## Error Handling

```go
package errors

// TryveError is the base error type.
type TryveError struct {
    Code    string // Machine-readable code (e.g., "CONFIG_INVALID")
    Message string // Human-readable message
    Hint    string // Suggested fix
    Cause   error  // Wrapped underlying error
}

func (e *TryveError) Error() string { return e.Message }
func (e *TryveError) Unwrap() error { return e.Cause }
```

Constructor functions for each category:

```go
func ConfigError(msg, hint string, cause error) *TryveError
func ValidationError(msg, hint string, cause error) *TryveError
func ConnectionError(adapter, msg string, cause error) *TryveError
func ExecutionError(step, msg string, cause error) *TryveError
func AssertionError(path, operator string, expected, actual any) *TryveError
func TimeoutError(operation string, duration time.Duration) *TryveError
func InterpolationError(expr, msg string) *TryveError
func AdapterError(adapter, action, msg string, cause error) *TryveError
```

All support `errors.Is()` and `errors.As()` for type checking in callers.

## Test Execution Flow

```
1. cmd/tryve/main.go
   └─ cobra parses CLI flags
   └─ calls pkg/runner.RunTests(opts)

2. pkg/runner.RunTests
   └─ config.Load(path)                    → Config
   └─ loader.Discover(testDir, filters)    → []TestDefinition
   └─ loader.Validate(tests)               → []error or nil
   └─ adapter.NewRegistry(config)          → Registry (lazy init)
   └─ executor.NewOrchestrator(registry, reporters)
   └─ orchestrator.Run(ctx, tests)

3. executor.Orchestrator.Run
   └─ hooks.RunAll(beforeAll)
   └─ topologicalSort(tests)
   └─ errgroup worker pool:
       └─ for each test: runner.RunTest(ctx, test)
           └─ hooks.Run(beforeEach)
           └─ for phase in [setup, execute, verify, teardown]:
               └─ for step in phase:
                   └─ interpolate.Resolve(step.params, context)
                   └─ adapter.Execute(ctx, action, params)
                   └─ capture values → context
                   └─ assertion.Run(result, step.assert)
                   └─ reporter.OnStepComplete()
               └─ retry if failed (exponential backoff + jitter)
           └─ hooks.Run(afterEach)
           └─ reporter.OnTestComplete()
   └─ hooks.RunAll(afterAll)
   └─ reporter.OnSuiteComplete()
   └─ reporter.Flush()
   └─ registry.CloseAll()
```

## Retry Logic

```go
type RetryConfig struct {
    MaxRetries int
    BaseDelay  time.Duration
    MaxDelay   time.Duration // Cap at 30s
    Jitter     float64       // 0.0–0.3 (30% randomization)
}
```

Exponential backoff: `delay = min(baseDelay * 2^attempt * (1 + jitter), maxDelay)`. Context-aware — if the context is cancelled during a backoff sleep, the retry loop exits immediately.

## Hooks

Hooks are shell commands executed via `os/exec.CommandContext`:

```yaml
hooks:
  beforeAll: "./scripts/setup.sh"
  afterAll: "./scripts/teardown.sh"
  beforeEach: "./scripts/before-test.sh"
  afterEach: "./scripts/after-test.sh"
```

- Working directory: project root (where `e2e.config.yaml` lives)
- Environment: inherits process env + test variables
- Stdout/stderr: captured and logged at INFO level
- Non-zero exit: treated as failure (beforeAll failure aborts suite, beforeEach failure skips test)
- Timeout: governed by context deadline

## Reporters

### Console Reporter
- ANSI-colored output (respects `NO_COLOR` env var)
- Progress: test name + pass/fail + duration
- Summary: total/passed/failed/skipped counts, total duration
- Verbose mode: shows step-level detail

### JUnit Reporter
- Standard JUnit XML via `encoding/xml`
- `<testsuite>` → `<testcase>` structure
- Failures include assertion details and step context
- Compatible with CI systems (GitHub Actions, Jenkins, etc.)

### HTML Reporter
- Self-contained HTML file via `html/template`
- Embedded CSS (no external assets)
- Collapsible test/step details
- Pass/fail coloring, duration display

### JSON Reporter
- Machine-readable JSON via `encoding/json`
- Full result tree: suite → tests → steps → assertions
- Includes timing, captured values, error details

## CLI Commands

Identical flags and behavior to TypeScript version:

| Command | Description |
|---------|-------------|
| `tryve run` | Execute tests (default command) |
| `tryve validate` | Validate test file syntax |
| `tryve list` | List discovered tests |
| `tryve health` | Check adapter connectivity |
| `tryve init` | Initialize e2e.config.yaml |
| `tryve test create <name>` | Create test from template |
| `tryve test list-templates` | List available templates |
| `tryve doc [section]` | Show documentation |
| `tryve install --skills` | Install Claude Code skills (preserved from TS) |
| `tryve version` | Print version |

All flags from the TypeScript version are preserved: `--config`, `--env`, `--test-dir`, `--parallel`, `--timeout`, `--retries`, `--bail`, `--grep`, `--tag`, `--priority`, `--dry-run`, `--watch`, `--skip-setup`, `--skip-teardown`, `--reporter`, `--output`.

Environment variables: `TRYVE_CONFIG`, `TRYVE_ENV`, `TRYVE_TEST_DIR`, `TRYVE_REPORT_DIR`, `TRYVE_VERBOSE`, `NO_COLOR`.

### Embedded Documentation

The `tryve doc` command embeds documentation files at compile time using Go's `//go:embed` directive. Docs from `docs/sections/` are embedded into the binary so they're available without external files. The `install --skills` command similarly embeds skill templates from `skills/`.

## Build & Distribution

### Makefile

```makefile
VERSION ?= $(shell git describe --tags --always)

build:
    go build -ldflags "-s -w -X main.version=$(VERSION)" -o bin/tryve ./cmd/tryve

test:
    go test ./...

lint:
    golangci-lint run

release:
    goreleaser release --clean
```

### Cross-Compilation Matrix

| OS | Arch | Binary |
|----|------|--------|
| linux | amd64 | `tryve-linux-amd64` |
| linux | arm64 | `tryve-linux-arm64` |
| darwin | amd64 | `tryve-darwin-amd64` |
| darwin | arm64 | `tryve-darwin-arm64` |
| windows | amd64 | `tryve-windows-amd64.exe` |
| windows | arm64 | `tryve-windows-arm64.exe` |

### GitHub Actions Release Workflow

- Triggered on version tags (`v*`)
- Uses GoReleaser for building + checksums + release notes
- Publishes to GitHub Releases
- Users can also `go install github.com/liemle3893/tryve/cmd/tryve@latest`

## Adapter Implementation Notes

### HTTP Adapter
- Uses stdlib `net/http` client
- Cookie jar via `net/http/cookiejar`
- Multipart file upload via `mime/multipart`
- Configurable base URL from config
- Response body parsed as JSON into `map[string]any`

### PostgreSQL Adapter
- `pgx/v5` with connection pooling (`pgxpool`)
- Parameterized queries (prevents SQL injection)
- `execute` returns affected row count, `query`/`queryOne` return row data
- Connection string from config with env var interpolation

### MongoDB Adapter
- Official Go driver with connection pooling
- Actions: `insertOne`, `updateOne`, `deleteOne`, `find`, `findOne`, `countDocuments`, `aggregate`
- Collection-based operations with BSON filter support
- Database name from config

### Redis Adapter
- `go-redis/v9` client
- Actions: `get`, `set`, `del`, `hgetall`, `incr`, `lpush`, etc.
- Key prefix support
- Database selection from config

### Kafka Adapter
- `segmentio/kafka-go` for pure Go implementation
- Actions: `produce`, `consume`, `waitFor`, `clear`
- SASL auth (plain, SCRAM-SHA-256, SCRAM-SHA-512)
- TLS support
- Configurable consumer group, client ID

### EventHub Adapter
- Azure SDK for Go
- Consumer group support
- Event publishing and consumption
- Checkpoint store configuration

### Shell Adapter
- `os/exec.CommandContext` for command execution
- Configurable timeout, working directory, environment
- Stdout/stderr capture
- Exit code in result metadata

## Verification Strategy

The existing YAML test files in `tests/` are the behavior contract. Both the TypeScript and Go versions must produce the same pass/fail results on the same test files. Verification approach:

1. Run existing E2E tests against Go binary
2. Compare pass/fail results with TypeScript version
3. Unit tests for each Go package (assertions, interpolation, config loading, etc.)
4. Integration tests per adapter
