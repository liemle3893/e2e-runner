# YAML Test Syntax

Complete reference for writing E2E tests in YAML format.

## Test File Structure

Test files must have `.test.yaml` extension and follow this structure:

```yaml
name: string                         # Required: unique test identifier
description: string                  # Optional: test description
priority: P0 | P1 | P2 | P3         # Optional: test priority
tags: [string]                       # Optional: tags for filtering
skip: boolean                        # Optional: skip this test
skipReason: string                   # Optional: reason for skipping
timeout: number                      # Optional: test timeout (ms)
retries: number                      # Optional: retry count
depends: [string]                    # Optional: test names this test depends on

variables:                           # Optional: test-scoped variables
  key: value

setup: Step[]                        # Optional: setup phase
execute: Step[]                      # Required: execution phase
verify: Step[]                       # Optional: verification phase
teardown: Step[]                     # Optional: cleanup phase
```

## Test Metadata

### Name

Unique identifier for the test. Use consistent naming convention:

```yaml
name: TC-USER-001              # Recommended: TC-{FEATURE}-{NUMBER}
name: test-user-creation       # Alternative: kebab-case
```

### Priority

Test priority for filtering and reporting:

```yaml
priority: P0    # Critical - must always pass
priority: P1    # High - important functionality
priority: P2    # Medium - standard tests
priority: P3    # Low - edge cases
```

### Tags

Array of tags for filtering:

```yaml
tags: [smoke, user, crud, e2e]
```

Run by tag:
```bash
npx e2e run --tag smoke --tag user
```

### Skip

Skip test with optional reason:

```yaml
skip: true
skipReason: "Feature not implemented yet"
```

### Depends

Declare dependencies on other tests. Dependent tests run after their dependencies:

```yaml
depends: [TC-USER-001, TC-AUTH-001]
```

This ensures that the current test only runs after `TC-USER-001` and `TC-AUTH-001` have completed successfully.

### Timeout

Test-level timeout override (in milliseconds):

```yaml
timeout: 60000    # 60 seconds
```

### Retries

Test-level retry count override:

```yaml
retries: 3        # Retry up to 3 times on failure
```

## Variables

Define test-scoped variables:

```yaml
variables:
  user_email: "test@example.com"
  user_name: "Test User"
  api_version: "v1"
```

Use in steps with `{{variableName}}`:

```yaml
execute:
  - adapter: http
    action: request
    url: "{{baseUrl}}/api/{{api_version}}/users"
    body:
      email: "{{user_email}}"
      name: "{{user_name}}"
```

### Dynamic Variables

Use built-in functions for dynamic values:

```yaml
variables:
  unique_email: "test-{{$uuid()}}@example.com"
  timestamp: "{{$timestamp()}}"
  random_id: "{{$random(1000, 9999)}}"
```

> **Evaluated once at test start:** Functions in the `variables` block are called exactly once when the test initialises. Every reference to `{{unique_email}}` — in `setup`, `execute`, `verify`, and `teardown` — resolves to the **same** generated value. Use this to keep IDs consistent across phases (e.g. INSERT in setup, GET in execute, DELETE in teardown all use the same UUID).
>
> Functions placed directly in step params (outside `variables`) are re-evaluated per step.

### Variable Cross-References

Variables can reference other variables. The runner resolves cross-references automatically using dependency order:

```yaml
variables:
  base_id: "TEST"
  run_id: "{{base_id}}_RUN"           # → "TEST_RUN"
  full_id: "{{run_id}}_{{$uuid()}}"   # → "TEST_RUN_<uuid>"
```

Multi-level chains are supported:

```yaml
variables:
  env: "staging"
  prefix: "{{env}}-api"               # → "staging-api"
  endpoint: "{{prefix}}/v1/users"     # → "staging-api/v1/users"
```

**Resolution order:** Test variables override global variables, then all cross-references are resolved before the first step executes.

**Circular reference detection:** The runner detects circular references and throws an error:

```yaml
# ERROR: Circular variable reference detected
variables:
  a: "{{b}}"
  b: "{{a}}"
```

**Depth limit:** Nested references are resolved up to 10 levels deep. Deeper nesting throws an error.

**Deferred references:** Variables that only reference `{{baseUrl}}` or `{{captured.*}}` are not resolved at definition time — they are resolved at step execution time when those values are available.

## Test Phases

### Setup Phase

Prepare test prerequisites:

```yaml
setup:
  # Clean up existing data
  - adapter: postgresql
    action: execute
    sql: "DELETE FROM users WHERE email LIKE 'test-%@example.com'"

  # Create prerequisite data
  - adapter: redis
    action: set
    key: "feature:enabled"
    value: "true"

  # Start a background process
  - name: api-server
    adapter: process
    action: start
    command: go run ./cmd/server
    env:
      PORT: "{{free_port}}"
    readiness:
      http: "http://localhost:{{free_port}}/health"
    capture:
      port: "port"
      pid: "pid"
```

Skip with `--skip-setup` flag.

### Step Name

Steps can have an optional `name` field. When set, captured values are stored in a nested namespace:

```yaml
- name: my-server
  adapter: process
  action: start
  command: "start-server"
  capture:
    port: "port"
    pid: "pid"
# Captured values accessible as: {{captured.my-server.port}}, {{captured.my-server.pid}}
```

Steps without a `name` continue to use flat capture (`{{captured.port}}`). Step names must be unique across all phases in a test.

### Execute Phase

Main test actions (required):

```yaml
execute:
  # Call API
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/users"
    body:
      email: "{{unique_email}}"
    capture:
      user_id: "$.id"
    assert:
      status: 201

  # Follow-up call using captured value
  - adapter: http
    action: request
    method: GET
    url: "{{baseUrl}}/users/{{captured.user_id}}"
    assert:
      status: 200
```

### Verify Phase

Validate results:

```yaml
verify:
  # Direct database verification
  - adapter: postgresql
    action: queryOne
    sql: "SELECT * FROM users WHERE id = $1"
    params: ["{{captured.user_id}}"]
    assert:
      - column: email
        equals: "{{unique_email}}"
```

### Teardown Phase

Cleanup test data:

```yaml
teardown:
  - adapter: http
    action: request
    method: DELETE
    url: "{{baseUrl}}/users/{{captured.user_id}}"
    continueOnError: true    # Don't fail test if cleanup fails
```

Skip with `--skip-teardown` flag.

## Step Definition

Each step has common fields plus adapter-specific parameters:

```yaml
- id: step_identifier            # Optional: step ID for logging
  adapter: http                  # Required: adapter name
  action: request                # Required: action name
  description: "Create user"     # Optional: step description
  continueOnError: false         # Optional: continue on failure
  retry: 3                       # Optional: step retry count
  delay: 1000                    # Optional: delay before execution (ms)

  # Adapter-specific parameters (HTTP example)
  method: POST
  url: "{{baseUrl}}/users"
  body: { ... }                    # JSON body (mutually exclusive with multipart)
  multipart: [...]                 # Multipart/form-data file uploads (see HTTP adapter docs)

  capture:                       # Optional: capture values from result
    key: "$.path"

  assert:                        # Optional: assertions on result
    status: 200
```

### Continue on Error

Allow step to fail without failing the test:

```yaml
teardown:
  - adapter: http
    action: request
    method: DELETE
    url: "{{baseUrl}}/temp-resource"
    continueOnError: true        # Test passes even if DELETE fails
```

### Step Retry

Override retry count for specific step:

```yaml
execute:
  - adapter: http
    action: request
    url: "{{baseUrl}}/flaky-endpoint"
    retry: 5                     # Retry this step up to 5 times
```

### Delay

Wait before executing step:

```yaml
execute:
  - adapter: http
    action: request
    url: "{{baseUrl}}/trigger-job"

  - adapter: http
    action: request
    delay: 2000                  # Wait 2 seconds
    url: "{{baseUrl}}/job-status"
```

## Value Capture

Extract values from step results for use in subsequent steps:

### HTTP Capture (JSONPath)

```yaml
- adapter: http
  action: request
  url: "{{baseUrl}}/users"
  capture:
    user_id: "$.id"              # Simple path
    user_email: "$.email"
    first_item: "$.items[0].id"  # Array access
    all_ids: "$.items[*].id"     # Wildcard
```

### PostgreSQL Capture

```yaml
- adapter: postgresql
  action: queryOne
  sql: "SELECT id, email FROM users WHERE id = $1"
  capture:
    db_id: "id"                  # Column name
    db_email: "email"
```

### Using Captured Values

Access captured values with `{{captured.keyName}}`:

```yaml
execute:
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/users"
    capture:
      user_id: "$.id"

  # Use captured value in next step
  - adapter: http
    action: request
    method: GET
    url: "{{baseUrl}}/users/{{captured.user_id}}"
```

## Variable Interpolation

The runner supports multiple interpolation patterns:

### Test Variables

```yaml
variables:
  my_var: "value"

execute:
  - adapter: http
    url: "{{baseUrl}}/{{my_var}}"     # → http://localhost/value
```

### Captured Values

```yaml
execute:
  - adapter: http
    capture:
      id: "$.id"
  - adapter: http
    url: "{{baseUrl}}/items/{{captured.id}}"
```

### Global Config Variables

Variables from `e2e.config.yaml`:

```yaml
# e2e.config.yaml
variables:
  apiKey: "secret-key"

# test.yaml
execute:
  - adapter: http
    headers:
      X-API-Key: "{{apiKey}}"
```

### Environment Variables

```yaml
execute:
  - adapter: http
    headers:
      Authorization: "Bearer {{$env(JWT_TOKEN)}}"
```

### Built-in Functions

```yaml
variables:
  unique_id: "{{$uuid()}}"
  now: "{{$isoDate()}}"
  random: "{{$random(1, 100)}}"
  otp_code: "{{$totp(JBSWY3DPEHPK3PXP)}}"
```

See [Built-in Functions Reference](./built-in-functions.md) for the full list of available functions.

## Complete Example

```yaml
name: TC-ORDER-001
description: Test complete order flow - create, update, verify, delete
priority: P0
tags: [order, crud, e2e]
timeout: 60000
retries: 2

variables:
  order_amount: 99.99
  customer_email: "customer-{{$uuid()}}@test.com"

setup:
  # Ensure clean state
  - adapter: postgresql
    action: execute
    sql: "DELETE FROM orders WHERE customer_email LIKE 'customer-%@test.com'"

execute:
  # Step 1: Create order
  - id: create_order
    adapter: http
    action: request
    description: "Create new order"
    method: POST
    url: "{{baseUrl}}/orders"
    headers:
      Content-Type: "application/json"
    body:
      customer_email: "{{customer_email}}"
      amount: "{{order_amount}}"
      items:
        - sku: "ITEM-001"
          quantity: 2
    capture:
      order_id: "$.id"
      order_status: "$.status"
    assert:
      status: 201
      json:
        - path: "$.status"
          equals: "pending"

  # Step 2: Update order status
  - id: update_order
    adapter: http
    action: request
    description: "Confirm order"
    method: PATCH
    url: "{{baseUrl}}/orders/{{captured.order_id}}"
    body:
      status: "confirmed"
    assert:
      status: 200
      json:
        - path: "$.status"
          equals: "confirmed"

verify:
  # Verify in database
  - adapter: postgresql
    action: queryOne
    sql: "SELECT * FROM orders WHERE id = $1"
    params: ["{{captured.order_id}}"]
    assert:
      - column: status
        equals: "confirmed"
      - column: customer_email
        equals: "{{customer_email}}"

teardown:
  # Cleanup
  - adapter: http
    action: request
    method: DELETE
    url: "{{baseUrl}}/orders/{{captured.order_id}}"
    continueOnError: true
```
