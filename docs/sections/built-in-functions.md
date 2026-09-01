# Built-in Functions Reference

Complete reference for all built-in functions available in variable interpolation.

## Argument Syntax

An argument may be a literal, a reference the interpolator understands
(`captured.foo`, a variable name, an environment key), or a nested `{{…}}`
expression:

```yaml
{{$upper(captured.region)}}
{{$jsonPath(captured.result, data.items[0].id)}}
{{$jwt(HS256, {{$env(JWT_SECRET)}}, {"sub":"123"}, 1h)}}
```

Arguments are separated by commas that sit outside quotes, parentheses, and
brackets, so a value containing a comma is safe when quoted:
`{{$hmac("a,b", key)}}`.

## Overview

Built-in functions are invoked using the `{{$functionName(args)}}` syntax in YAML tests.

```yaml
variables:
  unique_id: "{{$uuid()}}"
  timestamp: "{{$timestamp()}}"
  random_num: "{{$random(1, 100)}}"
```

> **Evaluated once per test:** Built-in functions in the `variables` block are called **exactly once** when the test starts. Every subsequent reference to `{{unique_id}}` returns the same resolved value for the entire test — across `setup`, `execute`, `verify`, and `teardown` phases. Functions used directly in step params (outside a `variables` block) are re-evaluated at each step.

---

## Identity Functions

### `$uuid()`

Generate a UUID v4.

```yaml
variables:
  user_id: "{{$uuid()}}"
  # → "550e8400-e29b-41d4-a716-446655440000"
```

### `$randomString(length)`

Generate random alphanumeric string.

```yaml
variables:
  token: "{{$randomString(32)}}"
  # → "aB3dF7gH2kL9mN5pR8sT0vW4xY6z"
```

### `$random(min, max)`

Generate random integer in range (inclusive).

```yaml
variables:
  random_id: "{{$random(1000, 9999)}}"
  # → 4523
```

---

## Date/Time Functions

### `$timestamp()`

Unix timestamp in milliseconds.

```yaml
variables:
  ts: "{{$timestamp()}}"
  # → 1703145600000
```

### `$isoDate()`

ISO 8601 date string.

```yaml
variables:
  date: "{{$isoDate()}}"
  # → "2024-12-21T10:30:00.000Z"
```

### `$now(format)`

Formatted current date/time.

| Format | Output |
|--------|--------|
| `iso` (default) | `2024-12-21T10:30:00.000Z` |
| `date` | `2024-12-21` |
| `time` | `10:30:00` |
| `datetime` | `2024-12-21 10:30:00` |
| `unix` | `1703145600` |
| `YYYY-MM-DD` | `2024-12-21` (alias for `date`) |
| `HH:mm:ss` | `10:30:00` (alias for `time`) |

Unrecognized format strings fall back to ISO 8601 output.

```yaml
variables:
  iso: "{{$now(iso)}}"           # 2024-12-21T10:30:00.000Z
  date: "{{$now(date)}}"         # 2024-12-21
  time: "{{$now(time)}}"         # 10:30:00
  datetime: "{{$now(datetime)}}" # 2024-12-21 10:30:00
  unix: "{{$now(unix)}}"         # 1703145600
```

### `$dateAdd(amount, unit)`

Add time to current date.

Units: `second`, `minute`, `hour`, `day`, `month`, `year`

```yaml
variables:
  tomorrow: "{{$dateAdd(1, day)}}"
  next_week: "{{$dateAdd(7, day)}}"
  next_month: "{{$dateAdd(1, month)}}"
  one_hour_later: "{{$dateAdd(1, hour)}}"
```

### `$dateSub(amount, unit)`

Subtract time from current date.

```yaml
variables:
  yesterday: "{{$dateSub(1, day)}}"
  last_week: "{{$dateSub(7, day)}}"
  one_hour_ago: "{{$dateSub(1, hour)}}"
```

---

## Environment Functions

### `$env(varName)`

Read environment variable.

```yaml
variables:
  api_key: "{{$env(API_KEY)}}"
  db_url: "{{$env(DATABASE_URL)}}"

execute:
  - adapter: http
    headers:
      Authorization: "Bearer {{$env(JWT_TOKEN)}}"
```

---

## File Functions

### `$file(path)`

Read file contents as string.

```yaml
variables:
  test_data: "{{$file(./fixtures/data.json)}}"
  template: "{{$file(./templates/request.xml)}}"
```

Useful for large request bodies:

```yaml
execute:
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/import"
    headers:
      Content-Type: "application/json"
    body: "{{$file(./fixtures/import-data.json)}}"
```

---

## Encoding Functions

### `$base64(value)`

Base64 encode string.

```yaml
variables:
  encoded: "{{$base64(Hello World)}}"
  # → "SGVsbG8gV29ybGQ="

execute:
  - adapter: http
    headers:
      Authorization: "Basic {{$base64(user:password)}}"
```

### `$base64Decode(value)`

Base64 decode string.

```yaml
variables:
  decoded: "{{$base64Decode(SGVsbG8gV29ybGQ=)}}"
  # → "Hello World"
```

### `$jsonStringify(value)`

JSON stringify value.

```yaml
variables:
  json_str: "{{$jsonStringify({\"key\": \"value\"})}}"
```

---

## Hash Functions

### `$md5(value)`

MD5 hash of string.

```yaml
variables:
  hash: "{{$md5(password123)}}"
  # → "482c811da5d5b4bc6d497ffa98491e38"
```

### `$sha256(value)`

SHA256 hash of string.

```yaml
variables:
  hash: "{{$sha256(password123)}}"
  # → "ef92b778bafe771e89245b89ecbc08a44a4e166c06659911881f383d4473e94f"
```

---

## String Functions

### `$lower(value)`

Convert to lowercase.

```yaml
variables:
  email: "{{$lower(User@Example.COM)}}"
  # → "user@example.com"
```

### `$upper(value)`

Convert to uppercase.

```yaml
variables:
  code: "{{$upper(abc123)}}"
  # → "ABC123"
```

### `$trim(value)`

Remove leading/trailing whitespace.

```yaml
variables:
  clean: "{{$trim(  hello world  )}}"
  # → "hello world"
```

---

## JSON Functions

These read structured data without shelling out to `jq` or `node -e`.

### `$json(value)`

Parse a JSON string into a value.

```yaml
variables:
  parsed: "{{$json(captured.raw_response)}}"
```

A value that is already structured is returned unchanged.

### `$jsonPath(value, path)`

Read a dotted path out of a value, parsing JSON strings along the way. The path
accepts `a.b`, `a.b[0]`, and `a.b.0`; a leading `$.` is optional.

```yaml
- adapter: postgresql
  action: execute
  sql: "UPDATE promotions SET mode = 'SOFT' WHERE id = $1"
  params: ["{{$jsonPath(captured.setup_result, promotionId)}}"]
```

Most of the time you do not need it: a captured JSON string can be addressed
directly, so `{{captured.setup_result.promotionId}}` says the same thing.

### `$jsonFile(path[, jsonPath])`

Read a JSON file, optionally returning one path within it. Replaces
`cat file.json | jq -r .some.key`.

```yaml
variables:
  api_key: "{{$jsonFile(local.settings.json, Values.INTERNAL_API_KEY)}}"
```

## Type Functions

An expression that is the whole value keeps its resolved type. These convert
explicitly when the source type is wrong — most often when a number arrives as
text from a shell step's stdout.

### `$int(value)` / `$number(value)` / `$bool(value)`

```yaml
- adapter: postgresql
  action: query
  sql: "SELECT * FROM orders WHERE quantity > $1"
  params: ["{{$int(captured.min_quantity)}}"]    # binds a number, not text
```

### `$default(value, fallback)`

Return `value` when it resolves to something non-empty, and `fallback`
otherwise. This is the escape hatch for genuinely optional values when strict
resolution is on.

```yaml
headers:
  Authorization: "{{$default(captured.token, anonymous)}}"
```

## Auth Functions

### `$jwt(algorithm, key, claims[, lifetime][, keyId])`

Sign a JSON Web Token. Supported algorithms: `HS256`, `RS256`. `claims` is a
JSON object; `lifetime` is a Go duration (default `1h`). `iat` and `exp` are
filled in from the lifetime unless the claims already set them.

```yaml
variables:
  # Symmetric key
  token: '{{$jwt(HS256, {{$env(JWT_SECRET)}}, {"sub":"84987654321"}, 1h)}}'

  # RSA key, PEM or base64-encoded PEM, with a key id
  internal_token: '{{$jwt(RS256, {{$jsonFile(local.settings.json, Values.PRIVATE_KEY)}}, {"sub":"84987654321","scope":"internal"}, 30m, key-1)}}'
```

This replaces per-test shell steps that call a token-minting script.

### `$hmac(message, key)`

Hex HMAC-SHA256, for signing webhook payloads and API requests.

```yaml
headers:
  X-Signature: "{{$hmac({{captured.payload}}, {{$env(WEBHOOK_SECRET)}})}}"
```

### `$base64url(value)`

Unpadded base64url encoding, the form JWT segments use.

## TOTP Function

### `$totp(secret)`

Generate a 6-digit TOTP code per RFC 6238 (HMAC-SHA1, 30-second period). Useful for testing 2FA/MFA login flows.

The `secret` argument must be a base32-encoded string (RFC 4648), which is the standard format provided by authenticator app setup flows (e.g., Google Authenticator, Authy).

```yaml
variables:
  otp_code: "{{$totp(JBSWY3DPEHPK3PXP)}}"
  # → "482931" (changes every 30 seconds)
```

**Example: 2FA Login Flow**

```yaml
name: TC-LOGIN-TOTP-001
description: Login with TOTP two-factor authentication

execute:
  # Step 1: Login with credentials
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/auth/login"
    body:
      email: "user@example.com"
      password: "{{$env(TEST_PASSWORD)}}"
    capture:
      mfa_token: "$.mfa_token"
    assert:
      status: 200

  # Step 2: Submit TOTP code
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/auth/verify-totp"
    body:
      mfa_token: "{{captured.mfa_token}}"
      totp_code: "{{$totp(JBSWY3DPEHPK3PXP)}}"
    assert:
      status: 200
      json:
        - path: "$.access_token"
          exists: true
```

**Using secrets from environment variables:**

```yaml
variables:
  totp_secret: "{{$env(TOTP_SECRET)}}"

execute:
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/auth/verify-totp"
    body:
      totp_code: "{{$totp({{$env(TOTP_SECRET)}})}}"
```

---

## Usage Examples

### Dynamic Test Data

```yaml
name: TC-USER-001
description: Create user with unique data

variables:
  unique_email: "test-{{$uuid()}}@example.com"
  random_age: "{{$random(18, 65)}}"
  created_at: "{{$isoDate()}}"

execute:
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/users"
    body:
      email: "{{unique_email}}"
      age: "{{random_age}}"
      createdAt: "{{created_at}}"
```

### Environment-Based Configuration

```yaml
execute:
  - adapter: http
    action: request
    headers:
      Authorization: "Bearer {{$env(API_TOKEN)}}"
      X-API-Key: "{{$env(API_KEY)}}"
```

### Time-Based Tests

```yaml
name: TC-EXPIRY-001
description: Test token expiration

variables:
  valid_token_expiry: "{{$dateAdd(1, hour)}}"
  expired_token_expiry: "{{$dateSub(1, hour)}}"

execute:
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/tokens"
    body:
      expiresAt: "{{valid_token_expiry}}"
```

### External Test Data

```yaml
name: TC-IMPORT-001
description: Import data from file

execute:
  - adapter: http
    action: request
    method: POST
    url: "{{baseUrl}}/import"
    headers:
      Content-Type: "application/json"
    body: "{{$file(./fixtures/large-import.json)}}"
```

### Authentication Headers

```yaml
execute:
  # Basic Auth
  - adapter: http
    action: request
    headers:
      Authorization: "Basic {{$base64(username:password)}}"

  # Bearer Token from environment
  - adapter: http
    action: request
    headers:
      Authorization: "Bearer {{$env(ACCESS_TOKEN)}}"
```

### Hashing Passwords

```yaml
variables:
  password: "secret123"
  password_hash: "{{$sha256(secret123)}}"

execute:
  - adapter: postgresql
    action: queryOne
    sql: "SELECT * FROM users WHERE password_hash = $1"
    params: ["{{password_hash}}"]
```

---

## Function Reference Table

| Function | Arguments | Description | Example Output |
|----------|-----------|-------------|----------------|
| `$uuid()` | none | UUID v4 | `550e8400-e29b-41d4-...` |
| `$timestamp()` | none | Unix ms | `1703145600000` |
| `$isoDate()` | none | ISO date | `2024-12-21T10:30:00.000Z` |
| `$random(min, max)` | 2 numbers | Random int | `4523` |
| `$randomString(len)` | number | Random string | `aB3dF7gH...` |
| `$env(name)` | string | Env variable | `value` |
| `$file(path)` | string | File contents | `{...}` |
| `$base64(value)` | string | Base64 encode | `SGVsbG8=` |
| `$base64Decode(value)` | string | Base64 decode | `Hello` |
| `$md5(value)` | string | MD5 hash | `482c811da5d5b4...` |
| `$sha256(value)` | string | SHA256 hash | `ef92b778bafe...` |
| `$now(format)` | string | Formatted date | varies |
| `$dateAdd(n, unit)` | number, string | Future date | ISO date |
| `$dateSub(n, unit)` | number, string | Past date | ISO date |
| `$lower(value)` | string | Lowercase | `hello` |
| `$upper(value)` | string | Uppercase | `HELLO` |
| `$trim(value)` | string | Trimmed | `hello` |
| `$jsonStringify(value)` | any | JSON string | `{"key":"value"}` |
| `$totp(secret)` | base32 string | TOTP 6-digit code (RFC 6238) | `482931` |
| `$json(value)` | any | Parse a JSON string | object/array |
| `$jsonPath(value, path)` | any, string | Read a path out of a value | varies |
| `$jsonFile(path[, jsonPath])` | string[, string] | Read a JSON file | varies |
| `$int(value)` | any | Coerce to an integer | `42` |
| `$number(value)` | any | Coerce to a number | `42.5` |
| `$bool(value)` | any | Coerce to a boolean | `true` |
| `$default(value, fallback)` | any, any | Value, or fallback when unset | varies |
| `$jwt(alg, key, claims[, ttl][, kid])` | strings | Signed JWT (HS256, RS256) | `eyJhbGci…` |
| `$hmac(message, key)` | string, string | Hex HMAC-SHA256 | `9f86d081…` |
| `$base64url(value)` | any | Unpadded base64url | `SGVsbG8` |
