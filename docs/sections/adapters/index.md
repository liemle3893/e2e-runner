# Adapters Reference

Complete reference for all supported adapters and their actions.

## Overview

Adapters provide connectivity to different services and databases for E2E testing. Each adapter extends `BaseAdapter` and implements `connect()`, `disconnect()`, `execute()`, and `healthCheck()`.

## Available Adapters

| Adapter | Purpose | Peer Dependency |
|---------|---------|-----------------|
| [HTTP](http.md) | REST API testing | None (built-in) |
| [Shell](shell.md) | Shell/CLI command execution | None (built-in) |
| [PostgreSQL](postgresql.md) | PostgreSQL database testing | `pg` |
| [MongoDB](mongodb.md) | MongoDB document testing | `mongodb` |
| [Redis](redis.md) | Redis cache testing | `ioredis` |
| [EventHub](eventhub.md) | Azure EventHub messaging | `@azure/event-hubs` |
| [Kafka](kafka.md) | Apache Kafka messaging | `kafkajs` |

## Adapter Configuration

Adapters are configured in `e2e.config.yaml` under each environment:

```yaml
environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      postgresql:
        connectionString: "postgresql://user:pass@localhost:5432/db"
      redis:
        connectionString: "redis://localhost:6379"
      mongodb:
        connectionString: "mongodb://user:pass@localhost:27017"
        database: "mydb"
      eventhub:
        connectionString: "Endpoint=sb://...;EntityPath=events"
        consumerGroup: "$Default"
      kafka:
        brokers:
          - "localhost:9092"
        clientId: "e2e-runner"
```

## Dependencies

None. Every adapter is compiled into the `tryve` binary — there is nothing to
install per adapter, and no runtime beyond the binary itself.

An adapter only needs the service it talks to be reachable and a corresponding
block under `environments.<env>.adapters` in `e2e.config.yaml`. Run
`tryve health` to check connectivity for everything the active environment
configures.

## Common Step Fields

All adapter steps share these common fields:

```yaml
- id: step_identifier            # Optional: step ID for logging
  adapter: http                  # Required: adapter name
  action: request                # Required: action name
  description: "Create user"     # Optional: step description
  continueOnError: false         # Optional: continue on failure
  retry: 3                       # Optional: step retry count
  delay: 1000                    # Optional: delay before execution (ms)

  capture:                       # Optional: capture values from result
    key: "$.path"

  assert:                        # Optional: assertions on result
    status: 200
```
