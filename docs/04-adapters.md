# Adapters Reference

Complete reference for all supported adapters and their actions.

## HTTP Adapter

For testing REST APIs and HTTP endpoints.

### Action: `request`

Execute HTTP request with full REST support.

```yaml
- adapter: http
  action: request
  method: GET | POST | PUT | PATCH | DELETE | HEAD | OPTIONS
  url: string                        # Full URL or relative to baseUrl
  headers?: Record<string, string>   # Request headers
  body?: any                         # Request body (auto-JSON stringified)
  query?: Record<string, string>     # Query parameters
  timeout?: number                   # Request timeout (ms)
  followRedirects?: boolean          # Follow redirects (default: true)
```

#### Examples

**GET Request:**
```yaml
- adapter: http
  action: request
  method: GET
  url: "{{baseUrl}}/users/123"
  headers:
    Authorization: "Bearer {{access_token}}"
  assert:
    status: 200
```

**POST Request:**
```yaml
- adapter: http
  action: request
  method: POST
  url: "{{baseUrl}}/users"
  headers:
    Content-Type: "application/json"
  body:
    email: "user@example.com"
    name: "John Doe"
  capture:
    user_id: "$.id"
  assert:
    status: 201
    json:
      - path: "$.id"
        exists: true
```

**With Query Parameters:**
```yaml
- adapter: http
  action: request
  method: GET
  url: "{{baseUrl}}/search"
  query:
    q: "test"
    limit: "10"
  assert:
    status: 200
```

#### HTTP Assertions

```yaml
assert:
  # Status code
  status: 200                        # Exact status
  status: [200, 201]                 # One of multiple
  statusRange: [200, 299]            # Range check

  # Headers
  headers:
    Content-Type: "application/json"
    X-Custom: "/pattern/"            # Regex pattern

  # JSON body assertions
  json:
    - path: "$.id"
      exists: true
    - path: "$.name"
      equals: "John"
    - path: "$.email"
      contains: "@example.com"
    - path: "$.age"
      greaterThan: 18
      lessThan: 100
    - path: "$.tags"
      length: 3
    - path: "$.status"
      matches: "^(active|pending)$"
    - path: "$.type"
      type: "string"

  # Raw body assertions
  body:
    contains: "success"
    matches: "\\d{4}-\\d{2}-\\d{2}"
    equals: "OK"

  # Response time
  duration:
    lessThan: 1000                   # Response under 1 second
    greaterThan: 100
```

---

## PostgreSQL Adapter

For testing PostgreSQL database operations.

### Action: `execute`

Execute SQL without returning results.

```yaml
- adapter: postgresql
  action: execute
  sql: "DELETE FROM users WHERE email LIKE $1"
  params: ["test-%@example.com"]
```

### Action: `query`

Execute SQL and return all rows.

```yaml
- adapter: postgresql
  action: query
  sql: "SELECT * FROM users WHERE status = $1"
  params: ["active"]
  capture:
    first_user_id: "[0].id"
  assert:
    - row: 0
      column: status
      equals: "active"
```

### Action: `queryOne`

Execute SQL and return exactly one row.

```yaml
- adapter: postgresql
  action: queryOne
  sql: "SELECT * FROM users WHERE id = $1"
  params: ["{{captured.user_id}}"]
  capture:
    db_email: "email"
    db_name: "name"
  assert:
    - column: email
      equals: "{{user_email}}"
```

### Action: `count`

Count rows matching query.

```yaml
- adapter: postgresql
  action: count
  sql: "SELECT COUNT(*) FROM users WHERE status = $1"
  params: ["active"]
  assert:
    - column: count
      greaterThan: 0
```

#### PostgreSQL Assertions

```yaml
assert:
  - row: 0                           # Row index (default: 0)
    column: "email"                  # Column name
    equals: "test@example.com"
  - column: "age"
    greaterThan: 18
    lessThan: 100
  - column: "name"
    contains: "John"
    matches: "^[A-Z]"
  - column: "deleted_at"
    isNull: true
  - column: "id"
    isNotNull: true
```

---

## Redis Adapter

For testing Redis cache operations.

### Action: `get`

Get string value.

```yaml
- adapter: redis
  action: get
  key: "user:123:name"
  capture: user_name
  assert:
    equals: "John Doe"
```

### Action: `set`

Set string value with optional TTL.

```yaml
- adapter: redis
  action: set
  key: "user:123:session"
  value: "session-token-xyz"
  ttl: 3600                          # Expires in 1 hour
```

### Action: `del`

Delete key.

```yaml
- adapter: redis
  action: del
  key: "user:123:cache"
```

### Action: `exists`

Check if key exists.

```yaml
- adapter: redis
  action: exists
  key: "user:123:session"
  assert:
    equals: 1                        # 1 = exists, 0 = doesn't exist
```

### Action: `incr`

Increment counter.

```yaml
- adapter: redis
  action: incr
  key: "stats:page_views"
  capture: view_count
  assert:
    greaterThan: 0
```

### Action: `hget`

Get hash field.

```yaml
- adapter: redis
  action: hget
  key: "user:123"
  field: "email"
  capture: user_email
  assert:
    contains: "@example.com"
```

### Action: `hset`

Set hash field.

```yaml
- adapter: redis
  action: hset
  key: "user:123"
  field: "status"
  value: "active"
```

### Action: `hgetall`

Get all hash fields.

```yaml
- adapter: redis
  action: hgetall
  key: "user:123"
  capture: user_data
  assert:
    exists: true
```

### Action: `keys`

Get keys matching pattern.

```yaml
- adapter: redis
  action: keys
  pattern: "user:*:session"
  capture: session_keys
```

### Action: `flushPattern`

Delete all keys matching pattern.

```yaml
- adapter: redis
  action: flushPattern
  pattern: "test:*"
```

#### Redis Assertions

```yaml
assert:
  equals: "expected value"
  isNull: true                       # Key doesn't exist
  isNotNull: true                    # Key exists
  greaterThan: 10
  lessThan: 100
  contains: "substring"
  length: 5
```

---

## MongoDB Adapter

For testing MongoDB document operations.

### Action: `insertOne`

Insert single document.

```yaml
- adapter: mongodb
  action: insertOne
  collection: "users"
  document:
    email: "test@example.com"
    name: "Test User"
    createdAt: "{{$isoDate()}}"
  capture:
    inserted_id: "insertedId"
```

### Action: `insertMany`

Insert multiple documents.

```yaml
- adapter: mongodb
  action: insertMany
  collection: "items"
  documents:
    - name: "Item 1"
      price: 10
    - name: "Item 2"
      price: 20
```

### Action: `findOne`

Find single document.

```yaml
- adapter: mongodb
  action: findOne
  collection: "users"
  filter:
    email: "test@example.com"
  capture:
    user_id: "_id"
    user_name: "name"
  assert:
    - path: "name"
      equals: "Test User"
```

### Action: `find`

Find all matching documents.

```yaml
- adapter: mongodb
  action: find
  collection: "orders"
  filter:
    status: "pending"
  capture:
    pending_count: "length"
  assert:
    - path: "[0].status"
      equals: "pending"
```

### Action: `updateOne`

Update single document.

```yaml
- adapter: mongodb
  action: updateOne
  collection: "users"
  filter:
    _id: "{{captured.user_id}}"
  update:
    $set:
      name: "Updated Name"
      updatedAt: "{{$isoDate()}}"
```

### Action: `updateMany`

Update multiple documents.

```yaml
- adapter: mongodb
  action: updateMany
  collection: "orders"
  filter:
    status: "pending"
  update:
    $set:
      status: "processed"
```

### Action: `deleteOne`

Delete single document.

```yaml
- adapter: mongodb
  action: deleteOne
  collection: "users"
  filter:
    email: "test@example.com"
```

### Action: `deleteMany`

Delete multiple documents.

```yaml
- adapter: mongodb
  action: deleteMany
  collection: "test_data"
  filter:
    testRun: "{{test_run_id}}"
```

### Action: `count`

Count documents.

```yaml
- adapter: mongodb
  action: count
  collection: "users"
  filter:
    status: "active"
  assert:
    - path: "count"
      greaterThan: 0
```

### Action: `aggregate`

Run aggregation pipeline.

```yaml
- adapter: mongodb
  action: aggregate
  collection: "orders"
  pipeline:
    - $match:
        status: "completed"
    - $group:
        _id: "$customerId"
        total: { $sum: "$amount" }
  capture:
    totals: "result"
```

#### MongoDB Assertions

```yaml
assert:
  - path: "name"                     # Dot notation path
    equals: "expected"
  - path: "tags"
    length: 3
  - path: "nested.field"
    exists: true
  - path: "status"
    matches: "^(active|pending)$"
  - path: "deletedAt"
    isNull: true
```

---

## EventHub Adapter

For testing Azure EventHub messaging.

### Action: `publish`

Publish message(s) to EventHub.

```yaml
- adapter: eventhub
  action: publish
  topic: "events"
  message:
    type: "user.created"
    data:
      userId: "{{captured.user_id}}"
      email: "{{user_email}}"
  partitionKey: "user-partition"
```

Publish multiple messages:

```yaml
- adapter: eventhub
  action: publish
  topic: "events"
  messages:
    - type: "event.one"
      data: { id: 1 }
    - type: "event.two"
      data: { id: 2 }
```

### Action: `consume`

Consume N messages from EventHub.

```yaml
- adapter: eventhub
  action: consume
  topic: "events"
  count: 5                           # Number of messages to consume
  timeout: 10000                     # Timeout in ms
  capture:
    messages: "result"
```

### Action: `waitFor`

Wait for message matching filter.

```yaml
- adapter: eventhub
  action: waitFor
  topic: "events"
  timeout: 30000
  filter:
    type: "user.created"
    data.userId: "{{captured.user_id}}"
  capture:
    event_data: "data"
  assert:
    - path: "type"
      equals: "user.created"
```

### Action: `clear`

Clear received messages buffer.

```yaml
- adapter: eventhub
  action: clear
  topic: "events"
```

#### EventHub Assertions

```yaml
assert:
  - path: "type"
    equals: "user.created"
  - path: "data.userId"
    exists: true
  - path: "data.email"
    contains: "@example.com"
```
