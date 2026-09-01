# Redis Adapter

For testing Redis cache operations.

## Configuration

```yaml
environments:
  local:
    adapters:
      redis:
        connectionString: "redis://user:password@host:port"
        keyPrefix: "test:"    # Prefix added to all keys (optional)
```

To select a specific Redis database, include it in the connection string: `redis://user:password@host:port/2`.

Connection settings (fixed, not configurable):
- `maxRetriesPerRequest`: 3
- `retryStrategy`: exponential backoff (200ms, 400ms, 600ms), stops after 3 attempts

## Action: `get`

Get string value.

```yaml
- adapter: redis
  action: get
  key: "user:123:name"
  capture: user_name
  assert:
    equals: "John Doe"
```

## Action: `set`

Set string value with optional TTL.

```yaml
- adapter: redis
  action: set
  key: "user:123:session"
  value: "session-token-xyz"
  ttl: 3600                          # Expires in 1 hour
```

## Action: `del`

Delete key.

```yaml
- adapter: redis
  action: del
  key: "user:123:cache"
```

## Action: `exists`

Check if key exists.

```yaml
- adapter: redis
  action: exists
  key: "user:123:session"
  assert:
    equals: 1                        # 1 = exists, 0 = doesn't exist
```

## Action: `incr`

Increment counter.

```yaml
- adapter: redis
  action: incr
  key: "stats:page_views"
  capture: view_count
  assert:
    greaterThan: 0
```

## Action: `hget`

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

## Action: `hset`

Set hash field.

```yaml
- adapter: redis
  action: hset
  key: "user:123"
  field: "status"
  value: "active"
```

## Action: `hgetall`

Get all hash fields.

```yaml
- adapter: redis
  action: hgetall
  key: "user:123"
  capture: user_data
  assert:
    exists: true
```

## Action: `keys`

Get keys matching pattern.

```yaml
- adapter: redis
  action: keys
  pattern: "user:*:session"
  capture: session_keys
```

## Action: `flushPattern`

Delete all keys matching pattern.

```yaml
- adapter: redis
  action: flushPattern
  pattern: "test:*"
```

## Redis Assertions

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
