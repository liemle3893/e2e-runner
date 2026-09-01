# Getting Started

Install Tryve and run your first test.

## Prerequisites

None beyond the binary. Tryve is a single statically-linked executable with no
runtime dependencies — no Node, no Python, no per-adapter packages to install.

Docker is useful for running the databases and queues you want to test against,
but Tryve itself does not need it.

## Installation

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/liemle3893/go-tryve/main/install.sh | sh

# Windows (PowerShell)
irm https://raw.githubusercontent.com/liemle3893/go-tryve/main/install.ps1 | iex

# Go toolchain
go install github.com/liemle3893/go-tryve/cmd/tryve@latest

# From source
git clone https://github.com/liemle3893/go-tryve.git
cd go-tryve && make build      # binary at ./bin/tryve
```

The install scripts take `--dir /custom/path` to change the install location and
`--version v2.0.0` to pin a release (`-Dir` / `-Version` in PowerShell).

Verify:

```bash
tryve version
```

## Quick Setup

### 1. Initialize configuration

```bash
tryve init
```

This writes `e2e.config.yaml` in the current directory. An existing file is never
overwritten.

```yaml
version: "1.0"
testDir: "tests"

environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      # Add a block per adapter you need — see `tryve doc config`

defaults:
  timeout: 30000
  retries: 0
  retryDelay: 1000
  parallel: 1

reporters:
  - type: console
    verbose: true
```

### 2. Create your first test

```bash
mkdir -p tests/e2e
tryve test create health -o tests/e2e/TC-HEALTH-001.test.yaml
```

Or write it by hand — `tests/e2e/TC-HEALTH-001.test.yaml`:

```yaml
name: TC-HEALTH-001
description: API health check
priority: P0
tags: [smoke, health]

execute:
  - adapter: http
    action: request
    method: GET
    url: "{{baseUrl}}/health"
    assert:
      status: 200
```

### 3. Run it

```bash
tryve run tests/e2e/TC-HEALTH-001.test.yaml
```

Naming the file directly is the fastest way to iterate — Tryve skips parsing the
rest of the suite.

## Project Structure

A layout that scales:

```
your-project/
├── e2e.config.yaml           # Configuration
├── .env                      # Secrets for ${VAR} substitution (git-ignored)
├── tests/
│   └── e2e/
│       ├── smoke/
│       │   └── TC-SMOKE-001.test.yaml
│       ├── users/
│       │   ├── TC-USER-001.test.yaml
│       │   └── TC-USER-002.test.yaml
│       └── integration/
│           └── TC-INT-001.test.yaml
└── reports/                  # Generated reports
    ├── junit.xml
    ├── report.html
    └── results.json
```

Tryve discovers any file ending in `.test.yaml` or `.test.yml` under the
configured `testDir`, skipping hidden directories and `node_modules`.

## Running Tests

### Choosing what to run

```bash
# Everything under the configured testDir
tryve run

# One file
tryve run tests/e2e/users/TC-USER-001.test.yaml

# Several directories
tryve run tests/e2e/users tests/e2e/smoke

# A glob — quote it so your shell does not expand it first
tryve run 'tests/e2e/**/TC-AUTH-*.test.yaml'
```

A path that matches no test is an error, so a typo is reported rather than
silently running a different set of tests.

### Filtering

```bash
tryve run --grep "user"          # by name (regex or substring)
tryve run --tag smoke            # by tag (repeatable)
tryve run --priority P0          # by priority
tryve run --failed-only          # only what failed last run
```

### Execution control

```bash
tryve run --parallel 4           # run 4 tests concurrently
tryve run --bail                 # stop at the first failure
tryve run --timeout 60000        # per-test timeout override
tryve run --watch                # re-run on file changes
```

### Seeing what happened

```bash
tryve run --verbose              # per-step output
tryve run --debug                # full request/response for every step
tryve run --dry-run              # list matching tests without running them
```

### Reports

```bash
tryve run --reporter junit -o ./reports/junit.xml
tryve run --reporter html --reporter json
```

## Validating Before You Run

```bash
tryve validate
```

Checks YAML syntax, required fields, and that every step names a known adapter,
a supported action, and the params that action requires. It connects to nothing.

## Checking Connectivity

```bash
tryve health
```

Connects to every adapter configured for the active environment and reports which
are reachable. Run this first when a suite fails everywhere at once.

## Environments and Secrets

Select an environment with `-e/--env` (default `local`):

```bash
tryve run --env staging
```

Config values may reference environment variables with `${VAR}`, and a `.env`
file beside the config is loaded automatically:

```yaml
environments:
  staging:
    baseUrl: "${STAGING_URL}"
    adapters:
      postgresql:
        connectionString: "${STAGING_DB_DSN}"
```

An unset variable is reported by name when the adapter connects, rather than
failing with an unparseable connection string.

## Working With an Agent

```bash
tryve install --skills
```

Installs a Claude Code skill into `.claude/skills/e2e-runner/`, with the full
reference set under `references/`. An agent then writes tests against the real
syntax instead of guessing.

The same reference is available in the terminal:

```bash
tryve doc                    # list sections
tryve doc yaml-test          # test file syntax
tryve doc assertions         # assertion operators
tryve doc adapters.postgresql
```

## Next Steps

- `tryve doc config` — full configuration reference
- `tryve doc yaml-test` — complete test file syntax
- `tryve doc adapters` — every adapter and its actions
- `tryve doc assertions` — assertion operators
- `tryve doc built-in-functions` — `$uuid`, `$now`, `$jwt`, and the rest
- `tryve doc examples` — common patterns
