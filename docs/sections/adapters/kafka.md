# Kafka Adapter

For testing Apache Kafka messaging using kafkajs.

## Configuration

```yaml
environments:
  local:
    adapters:
      kafka:
        brokers:
          - "localhost:9092"
        clientId: "e2e-runner"
        ssl: false
        connectionTimeout: 10000
        requestTimeout: 30000
```

Multiple brokers:

```yaml
kafka:
  brokers:
    - "broker1:9092"
    - "broker2:9092"
    - "broker3:9092"
  clientId: "e2e-runner"
```

With SASL authentication:

```yaml
kafka:
  brokers:
    - "kafka.example.com:9093"
  ssl: true
  sasl:
    mechanism: "plain"
    username: "${KAFKA_USERNAME}"
    password: "${KAFKA_PASSWORD}"
```

## Action: `produce`

Produce message(s) to a Kafka topic.

```yaml
- adapter: kafka
  action: produce
  topic: "user-events"
  message:
    key: "user-123"
    value:
      type: "user.created"
      userId: "{{captured.user_id}}"
      email: "{{user_email}}"
```

Produce multiple messages in a single call:

```yaml
- adapter: kafka
  action: produce
  topic: "events"
  messages:
    - key: "key-1"
      value: { type: "event.one", id: 1 }
    - key: "key-2"
      value: { type: "event.two", id: 2 }
```

Message fields:
- `key` (optional): Message key for partitioning
- `value` (required): Message payload (serialized as JSON)
- `partition` (optional): Target partition number
- `headers` (optional): Message headers as key-value pairs

A payload may also be given directly as `value:` on the step, without the
`message:` wrapper. Topics are created on first produce if the broker allows it.

## Action: `consume`

Consume N messages from a topic. Resolves when `count` messages are received or `timeout` is reached (returns whatever was collected).

```yaml
- adapter: kafka
  action: consume
  topic: "user-events"
  count: 5                           # Number of messages to consume (default: 1)
  timeout: 10000                     # Timeout in ms (default: 10000)
```

## Action: `waitFor`

Wait for a single message matching a filter. Fails with `TimeoutError` if no match is found within the timeout.

```yaml
- adapter: kafka
  action: waitFor
  topic: "user-events"
  timeout: 30000                     # Timeout in ms (default: 30000)
  filter:
    type: "user.created"
    data.userId: "{{captured.user_id}}"
  capture:
    event_data: "data"
  assert:
    - path: "type"
      equals: "user.created"
```

Filter keys address the decoded message payload using dot-notation, so `type`
and `data.userId` refer to fields of the value that was produced. Envelope
fields — `key`, `topic`, `partition`, `offset`, `headers` — are matchable by the
same names. All filter fields must match exactly. `match:` is accepted as a
synonym for `filter:`.

The matched message's payload fields are returned at the top level, so
`path: "data.message"` and `capture: {id: "data.id"}` address the payload. The
envelope fields (`key`, `value`, `headers`, `topic`, `partition`, `offset`) are
returned alongside them, so `path: "$.value.type"` also works; a payload field
of the same name takes precedence, and the untouched envelope is always
available under `message`.

## Action: `clear`

No-op action for test symmetry (Kafka topics cannot be cleared like in-memory buffers).

```yaml
- adapter: kafka
  action: clear
```

## Kafka Assertions

Assertions use the shared assertion engine with all 12 operators. Examples:

```yaml
assert:
  - path: "type"
    equals: "user.created"           # Exact equality
  - path: "data.userId"
    exists: true                     # Field must exist
  - path: "data.email"
    contains: "@example.com"         # Substring match
  - path: "data.status"
    matches: "^(active|pending)$"    # Regex match
  - path: "data.roles"
    type: "array"                    # Type check
    length: 2                        # Array length
```
