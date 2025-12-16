# Built-in Functions Reference

Complete reference for all built-in functions available in variable interpolation.

## Overview

Built-in functions are invoked using the `{{$functionName(args)}}` syntax in YAML tests.

```yaml
variables:
  unique_id: "{{$uuid()}}"
  timestamp: "{{$timestamp()}}"
  random_num: "{{$random(1, 100)}}"
```

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
| `iso` | `2024-12-21T10:30:00.000Z` |
| `date` | `2024-12-21` |
| `time` | `10:30:00` |
| `datetime` | `2024-12-21 10:30:00` |
| `unix` | `1703145600` |
| Custom | Uses date-fns format |

```yaml
variables:
  iso: "{{$now(iso)}}"           # 2024-12-21T10:30:00.000Z
  date: "{{$now(date)}}"         # 2024-12-21
  time: "{{$now(time)}}"         # 10:30:00
  datetime: "{{$now(datetime)}}" # 2024-12-21 10:30:00
  unix: "{{$now(unix)}}"         # 1703145600
  custom: "{{$now(YYYY/MM/DD)}}" # 2024/12/21
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
