# Assertions Reference

Complete reference for all assertion operators available in E2E tests.

## YAML Assertion Syntax

Assertions are specified in the `assert` block of each step. The structure varies by adapter.

### HTTP Assertions

```yaml
assert:
  status: number | number[]          # HTTP status code(s)
  statusRange: [min, max]            # Status code range
  headers: Record<string, string>    # Header assertions
  json: JSONAssertion[]              # JSON body assertions
  body: BodyAssertion                # Raw body assertions
  duration: DurationAssertion        # Response time assertions
```

### Database/Cache Assertions

```yaml
assert:
  - path: string                     # Path to value (JSONPath or dot notation)
    column: string                   # Column name (PostgreSQL)
    row: number                      # Row index (PostgreSQL, default: 0)

    # Operators (choose one or more):
    equals: any
    contains: string
    matches: string
    exists: boolean
    type: string
    length: number
    greaterThan: number
    lessThan: number
    isNull: boolean
    isNotNull: boolean
```

---

## Assertion Operators

### `equals`

Strict equality check. Works with strings, numbers, booleans, and objects.

```yaml
# Exact match
assert:
  json:
    - path: "$.status"
      equals: "active"
    - path: "$.count"
      equals: 42
    - path: "$.enabled"
      equals: true
```

### `contains`

String contains substring.

```yaml
assert:
  json:
    - path: "$.email"
      contains: "@example.com"
    - path: "$.message"
      contains: "success"
```

### `matches`

Regular expression match.

```yaml
assert:
  json:
    - path: "$.id"
      matches: "^[0-9a-f]{8}-[0-9a-f]{4}"    # UUID pattern
    - path: "$.phone"
      matches: "^\\+?[0-9]{10,15}$"          # Phone number
    - path: "$.status"
      matches: "^(pending|active|completed)$"
```

### `exists`

Check if path exists (or doesn't exist).

```yaml
assert:
  json:
    - path: "$.id"
      exists: true                   # Path must exist
    - path: "$.deletedAt"
      exists: false                  # Path must not exist
```

### `type`

Check value type.

Valid types: `string`, `number`, `boolean`, `object`, `array`, `null`, `undefined`

```yaml
assert:
  json:
    - path: "$.id"
      type: "string"
    - path: "$.count"
      type: "number"
    - path: "$.items"
      type: "array"
    - path: "$.metadata"
      type: "object"
```

### `length`

Check length of string or array.

```yaml
assert:
  json:
    - path: "$.items"
      length: 5                      # Array has 5 elements
    - path: "$.code"
      length: 6                      # String is 6 characters
```

### `greaterThan` / `lessThan`

Numeric comparisons.

```yaml
assert:
  json:
    - path: "$.age"
      greaterThan: 18
      lessThan: 100
    - path: "$.price"
      greaterThan: 0
    - path: "$.discount"
      lessThan: 50
```

### `isNull` / `isNotNull`

Check for null values.

```yaml
assert:
  json:
    - path: "$.deletedAt"
      isNull: true                   # Value is null or undefined
    - path: "$.id"
      isNotNull: true                # Value is not null
```

---

## HTTP-Specific Assertions

### Status Code

```yaml
assert:
  # Single status
  status: 200

  # Multiple acceptable statuses
  status: [200, 201, 204]

  # Status range
  statusRange: [200, 299]            # Any 2xx status
```

### Headers

```yaml
assert:
  headers:
    Content-Type: "application/json"
    Cache-Control: "no-cache"
    X-Request-Id: ".*"               # Regex pattern (in quotes)
```

### Body Assertions

For raw body (non-JSON):

```yaml
assert:
  body:
    contains: "success"
    matches: "<status>OK</status>"
    equals: "PONG"
```

### Duration

Response time assertions:

```yaml
assert:
  duration:
    lessThan: 1000                   # Under 1 second
    greaterThan: 100                 # At least 100ms
```

---

## PostgreSQL-Specific Assertions

### Column Assertions

```yaml
assert:
  # First row (default)
  - column: "email"
    equals: "test@example.com"

  # Specific row
  - row: 1
    column: "status"
    equals: "active"

  # Multiple columns
  - column: "first_name"
    equals: "John"
  - column: "last_name"
    equals: "Doe"
```

### Null Checks

```yaml
assert:
  - column: "deleted_at"
    isNull: true
  - column: "id"
    isNotNull: true
```

---

## Combining Assertions

Multiple operators can be combined on a single path:

```yaml
assert:
  json:
    - path: "$.age"
      type: "number"
      greaterThan: 0
      lessThan: 150

    - path: "$.email"
      type: "string"
      contains: "@"
      matches: "^[^@]+@[^@]+\\.[^@]+$"

    - path: "$.items"
      type: "array"
      exists: true
      length: 3
```

---

## Examples

### Complete HTTP Test

```yaml
execute:
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/users"
    body:
      email: "test@example.com"
      name: "Test User"
    assert:
      status: 201
      headers:
        Content-Type: "application/json"
      json:
        - path: "$.id"
          exists: true
          type: "string"
          matches: "^[0-9a-f-]{36}$"
        - path: "$.email"
          equals: "test@example.com"
        - path: "$.name"
          equals: "Test User"
        - path: "$.createdAt"
          exists: true
      duration:
        lessThan: 2000
```

### PostgreSQL Verification

```yaml
verify:
  - adapter: postgresql
    action: queryOne
    sql: "SELECT * FROM users WHERE id = $1"
    params: ["{{captured.user_id}}"]
    assert:
      - column: "email"
        equals: "test@example.com"
      - column: "name"
        equals: "Test User"
      - column: "status"
        equals: "active"
      - column: "deleted_at"
        isNull: true
```

### Redis Verification

```yaml
verify:
  - adapter: redis
    action: get
    key: "user:{{captured.user_id}}:cache"
    assert:
      isNotNull: true
      contains: "test@example.com"
```

### MongoDB Verification

```yaml
verify:
  - adapter: mongodb
    action: findOne
    collection: "users"
    filter:
      _id: "{{captured.user_id}}"
    assert:
      - path: "email"
        equals: "test@example.com"
      - path: "profile.verified"
        equals: true
      - path: "roles"
        type: "array"
        length: 2
```

---

## Programmatic Assertions (TypeScript)

When using TypeScript tests, the `expect()` API provides additional matchers:

```typescript
import { expect, assert, fail } from '@liemle3893/e2e-runner';

// Basic matchers
expect(value).toBe(expected);           // Strict equality (===)
expect(value).toEqual(expected);        // Deep equality
expect(value).toBeOneOf([a, b, c]);     // One of array values

// Truthiness
expect(value).toBeDefined();
expect(value).toBeUndefined();
expect(value).toBeNull();
expect(value).toBeNotNull();
expect(value).toBeTruthy();
expect(value).toBeFalsy();

// Collections
expect(array).toContain(item);
expect(value).toHaveLength(5);

// Strings/Patterns
expect(string).toMatch(/pattern/);

// Numbers
expect(num).toBeGreaterThan(5);
expect(num).toBeGreaterThanOrEqual(5);
expect(num).toBeLessThan(10);
expect(num).toBeLessThanOrEqual(10);

// Objects
expect(obj).toHaveProperty('key');
expect(obj).toHaveProperty('key', 'value');

// Types
expect(value).toBeType('string');

// Negation
expect(value).not.toBe(wrong);
expect(array).not.toContain(item);

// Direct assertions
assert(condition, 'Error message');
assertFalse(condition, 'Error message');
fail('Force failure');

// Async assertions
await assertThrowsAsync(async () => {
  await someAsyncFn();
}, 'Expected error');
```
