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
```

Skip with `--skip-setup` flag.

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

  # Adapter-specific parameters
  method: POST
  url: "{{baseUrl}}/users"
  body: { ... }

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
```

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
