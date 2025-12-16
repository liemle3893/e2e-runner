# Getting Started

This guide walks you through installing and running your first E2E test.

## Prerequisites

- Node.js 18+
- npm or yarn
- (Optional) Docker for database adapters

## Installation

### As a Project Dependency

```bash
npm install @liemle3893/e2e-runner
```

### Global Installation

```bash
npm install -g @liemle3893/e2e-runner
```

## Quick Setup

### 1. Initialize Configuration

```bash
npx e2e init
```

This creates `e2e.config.yaml` with a basic template:

```yaml
version: "1.0"

environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      # Configure adapters as needed

defaults:
  timeout: 30000
  retries: 2
  retryDelay: 1000
  parallel: 4

reporters:
  - type: console
    verbose: true
```

### 2. Create Test Directory

```bash
mkdir -p tests/e2e
```

### 3. Write Your First Test

Create `tests/e2e/health.test.yaml`:

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

### 4. Run Tests

```bash
npx e2e run --env local
```

## Project Structure

Recommended directory structure:

```
your-project/
├── e2e.config.yaml          # Main configuration
├── tests/
│   └── e2e/
│       ├── smoke/           # Smoke tests
│       │   └── TC-SMOKE-001.test.yaml
│       ├── users/           # User feature tests
│       │   ├── TC-USER-001.test.yaml
│       │   └── TC-USER-002.test.yaml
│       └── integration/     # Integration tests
│           └── TC-INT-001.test.yaml
└── reports/                 # Generated reports
    ├── junit.xml
    ├── report.html
    └── results.json
```

## Running Tests

### Basic Run

```bash
# Run all tests in local environment
npx e2e run --env local

# Run tests in specific directory
npx e2e run --env local --test-dir tests/e2e/users
```

### Filtering Tests

```bash
# Filter by name pattern
npx e2e run --env local --grep "user"

# Filter by tag
npx e2e run --env local --tag smoke

# Filter by priority
npx e2e run --env local --priority P0

# Combine filters
npx e2e run --env local --tag e2e --priority P0 --priority P1
```

### Parallel Execution

```bash
# Run 4 tests in parallel
npx e2e run --env local --parallel 4
```

### Debug Mode

```bash
# Enable debug logging
npx e2e run --env local --debug --verbose
```

### Dry Run

```bash
# List tests without executing
npx e2e run --env local --dry-run
```

## Validating Tests

Before running, validate your test files:

```bash
npx e2e validate --env local
```

## Checking Adapter Health

Verify adapter connections:

```bash
# Check all adapters
npx e2e health --env local

# Check specific adapter
npx e2e health --env local --adapter postgresql
```

## Next Steps

- [Configuration Reference](./02-configuration.md) - Full config options
- [YAML Test Syntax](./03-yaml-tests.md) - Complete test syntax
- [Adapters Guide](./04-adapters.md) - All adapter actions
