# PostgreSQL Adapter

For testing PostgreSQL database operations.

## Configuration

```yaml
environments:
  local:
    adapters:
      postgresql:
        connectionString: "postgresql://user:password@host:port/database"
        poolMin: 2            # Minimum connection pool size (default: 2)
        poolMax: 5            # Maximum connection pool size (default: 5)
```

Connection pool settings:
- `idleTimeoutMillis`: 30000 (fixed, not configurable)
- `connectionTimeoutMillis`: 10000 (fixed, not configurable)

## Action: `execute`

Execute SQL without returning results.

```yaml
- adapter: postgresql
  action: execute
  sql: "DELETE FROM users WHERE email LIKE $1"
  params: ["test-%@example.com"]
```

## Action: `query`

Execute SQL and return all rows.

```yaml
- adapter: postgresql
  action: query
  sql: "SELECT * FROM users WHERE status = $1"
  params: ["active"]
  capture:
    first_user_id: "$.rows[0].id"
  assert:
    - path: "$.rowCount"
      greaterThan: 0
    - path: "$.rows[0].status"
      equals: "active"
```

## Action: `queryOne`

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
    - path: "$.email"
      equals: "{{user_email}}"
```

## Action: `count`

Return a count as `{"count": N, "rowCount": N, "rows": [...]}`.

`count` works with either style of query:

- **`SELECT COUNT(*) …`** — the query returns one row holding one numeric column, and `count` is that value.
- **A plain `SELECT`** — `count` is the number of rows returned.

`rowCount` is always the number of rows the query produced, so you can tell the
two apart when it matters.

```yaml
# Aggregate: count is 37 when the table holds 37 active users.
- adapter: postgresql
  action: count
  sql: "SELECT COUNT(*) FROM users WHERE status = $1"
  params: ["active"]
  assert:
    count:
      gte: 1

# Plain select: count is the number of rows returned.
- adapter: postgresql
  action: count
  sql: "SELECT * FROM users WHERE status = $1"
  params: ["active"]
  assert:
    - path: "$.count"
      greaterThan: 0
```

## Column Types

Values are converted to JSON-friendly types so that assertions compare against
what you would naturally write:

| PostgreSQL type | Becomes | Note |
|---|---|---|
| `int2` / `int4` / `int8` | number | |
| `numeric` / `decimal` | number | Exact decimal text when the value is too large for a float64, so precision is never silently lost |
| `float4` / `float8` | number | |
| `boolean` | boolean | |
| `text` / `varchar` | string | |
| `uuid` | string | Canonical `xxxxxxxx-xxxx-…` form |
| `date` | string | `2026-08-31` — no time component |
| `timestamp` / `timestamptz` | string | RFC 3339 |
| `interval` | string | ISO 8601 duration, e.g. `P2DT3600S` |
| `json` / `jsonb` | object or array | Parsed, so `$.col.field` works |
| arrays | array | Elements converted the same way |
| `NULL` | null | Use `isNull` / `isNotNull` |

Because a `numeric` column is a plain number, `equals: 100` and
`greaterThan: 50` work on it directly — no `::int` or `::text` cast needed.
Likewise a `date` column equals `"2026-08-31"` without a `::text` cast.

## PostgreSQL Assertions

Assertions use JSONPath (`path:`) evaluated against the action's result data.

- **`query`** returns `{ rows: [...], rowCount: N }` — use `$.rows[0].col`, `$.rowCount`, etc.
- **`queryOne`** returns the first row's columns at the top level — use `$.col_name`, plus `found: true`.

### Row/column form

For SQL results you can address a cell by row index and column name instead of
writing a JSONPath. `row` defaults to `0`:

```yaml
- adapter: postgresql
  action: query
  sql: "SELECT reward_key, total_grants FROM grants ORDER BY id"
  assert:
    - row: 0
      column: reward_key
      equals: "only_reward"
    - row: 1
      column: total_grants
      gte: 5
```

This works against `queryOne` too, where the single row's columns sit at the top
level:

```yaml
- adapter: postgresql
  action: queryOne
  sql: "SELECT status FROM orders WHERE id = $1"
  params: ["{{captured.order_id}}"]
  assert:
    - column: status
      equals: "PAID"
```

Naming a column that is not in the result, or a row index beyond the end of the
result, fails the step with a message listing the columns that *are* available.

### Field form

A result field can also be asserted on by name at the top level of the `assert`
block:

```yaml
assert:
  rowCount:
    gte: 1
  rowsAffected: 1
```

```yaml
# query assertions
assert:
  - path: "$.rowCount"
    greaterThan: 0
  - path: "$.rows[0].email"
    equals: "test@example.com"
  - path: "$.rows[0].age"
    greaterThan: 18
  - path: "$.rows[0].deleted_at"
    isNull: true

# queryOne assertions (row fields at top level)
assert:
  - path: "$.email"
    equals: "test@example.com"
  - path: "$.id"
    isNotNull: true
```

## Asserting that no row exists

`queryOne` fails the step when the query matches nothing. To make an empty
result something to assert on instead, set `allowEmpty: true`; the result then
carries `found: false`:

```yaml
- adapter: postgresql
  action: queryOne
  allowEmpty: true
  sql: "SELECT id FROM sessions WHERE user_id = $1"
  params: ["{{captured.user_id}}"]
  assert:
    found: false
```

## Value Capture

Capture values from query results using JSONPath:

```yaml
# From queryOne — row fields are at top level
- adapter: postgresql
  action: queryOne
  sql: "SELECT id, email FROM users WHERE id = $1"
  capture:
    db_id: "id"           # short form, equivalent to $.id
    db_email: "email"

# From query — access via $.rows[N].col
- adapter: postgresql
  action: query
  sql: "SELECT id, email FROM users LIMIT 5"
  capture:
    first_id: "$.rows[0].id"
    first_email: "$.rows[0].email"
```

Use captured values with `{{captured.db_id}}` in subsequent steps.
