# Adapters Reference

Complete reference for all supported adapters and their actions.

## Overview

Adapters provide connectivity to different services and databases for E2E testing. Each adapter extends `BaseAdapter` and implements `connect()`, `disconnect()`, `execute()`, and `healthCheck()`.

## Available Adapters

| Adapter | Purpose | Peer Dependency |
|---------|---------|-----------------|
| [HTTP](http.md) | REST API testing | None (built-in) |
| [Shell](shell.md) | Shell/CLI command execution | None (built-in) |
| [Process](process.md) | Background process lifecycle | None (built-in) |
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

## Peer Dependencies

The HTTP and Shell adapters use built-in Node.js APIs and require no additional dependencies.

Database and messaging adapters are optional peer dependencies. Install only what you need:

```bash
# PostgreSQL
npm install pg

# MongoDB
npm install mongodb

# Redis
npm install ioredis

# Azure EventHub
npm install @azure/event-hubs

# Apache Kafka
npm install kafkajs
```

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
