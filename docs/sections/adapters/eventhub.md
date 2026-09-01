# EventHub Adapter

For testing Azure EventHub messaging.

## Configuration

```yaml
environments:
  local:
    adapters:
      eventhub:
        connectionString: "Endpoint=sb://namespace.servicebus.windows.net/;SharedAccessKeyName=...;SharedAccessKey=...;EntityPath=hub-name"
        consumerGroup: "$Default"
```

For local development with EventHub emulator:

```yaml
eventhub:
  connectionString: "Endpoint=sb://localhost;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=SAS_KEY_VALUE;UseDevelopmentEmulator=true;EntityPath=events"
  consumerGroup: "$Default"
```

## Action: `publish`

Publish message(s) to EventHub.

```yaml
- adapter: eventhub
  action: publish
  message:
    type: "user.created"
    data:
      userId: "{{captured.user_id}}"
      email: "{{user_email}}"
  partitionKey: "user-partition"      # Optional: partition key for ordering
```

Publish multiple messages in a single batch:

```yaml
- adapter: eventhub
  action: publish
  messages:
    - type: "event.one"
      data: { id: 1 }
    - type: "event.two"
      data: { id: 2 }
```

## Action: `consume`

Consume N messages from EventHub. Resolves when `count` messages are received or `timeout` is reached (returns whatever was collected).

```yaml
- adapter: eventhub
  action: consume
  count: 5                           # Number of messages to consume (default: 1)
  timeout: 10000                     # Timeout in ms (default: 10000)
```

## Action: `waitFor`

Wait for a single message matching a filter. Fails with `TimeoutError` if no match is found within the timeout.

```yaml
- adapter: eventhub
  action: waitFor
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

Filter uses dot-notation for nested matching. All filter fields must match
exactly. `match:` is accepted as a synonym for `filter:`.

## Action: `clear`

Clear the internal received-events buffer.

```yaml
- adapter: eventhub
  action: clear
```

## EventHub Assertions

Assertions use dot-notation paths on the matched event body. Supported operators:

```yaml
assert:
  - path: "type"
    equals: "user.created"           # Exact equality
  - path: "data.userId"
    exists: true                     # Field must exist (not undefined)
  - path: "data.email"
    contains: "@example.com"         # Substring match
  - path: "data.status"
    matches: "^(active|pending)$"    # Regex match
```
