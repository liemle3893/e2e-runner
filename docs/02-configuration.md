# Configuration Reference

The E2E Test Runner is configured via `e2e.config.yaml` in your project root.

## Complete Configuration Schema

```yaml
version: "1.0"                    # Required: config version

environments:                     # Required: at least one environment
  local:                          # Environment name
    baseUrl: "http://localhost:3000"  # Required: base URL for HTTP adapter
    adapters:                     # Optional: adapter configurations
      postgresql:
        connectionString: "postgresql://user:pass@localhost:5432/db"
        schema: "public"          # Default schema
        poolMin: 1                # Min pool connections
        poolMax: 10               # Max pool connections

      redis:
        connectionString: "redis://localhost:6379"
        db: 0                     # Redis database number
        keyPrefix: ""             # Key prefix for all operations

      mongodb:
        connectionString: "mongodb://user:pass@localhost:27017"
        database: "mydb"          # Database name

      eventhub:
        connectionString: "Endpoint=sb://...;EntityPath=events"
        consumerGroup: "$Default" # Consumer group name

  staging:
    baseUrl: "https://staging.example.com"
    adapters:
      # Staging adapter configs...

  production:
    baseUrl: "https://api.example.com"
    adapters:
      # Production adapter configs...

defaults:                         # Optional: default settings
  timeout: 30000                  # Default test timeout (ms)
  retries: 2                      # Default retry count
  retryDelay: 1000                # Delay between retries (ms)
  parallel: 4                     # Parallel test count

variables:                        # Optional: global variables
  testPrefix: "e2e_"
  defaultUserId: "test-user"
  apiVersion: "v1"

reporters:                        # Optional: report configuration
  - type: console
    verbose: true

  - type: junit
    output: "./reports/junit.xml"

  - type: html
    output: "./reports/report.html"

  - type: json
    output: "./reports/results.json"
```

## Environment Configuration

### Base URL

The `baseUrl` is required and used as the base for all HTTP requests:

```yaml
environments:
  local:
    baseUrl: "http://localhost:3000"
```

Access in tests via `{{baseUrl}}`:

```yaml
- adapter: http
  action: request
  url: "{{baseUrl}}/api/users"
```

### Adapter Configuration

Each adapter has specific configuration options:

#### PostgreSQL

```yaml
postgresql:
  connectionString: "postgresql://user:password@host:port/database"
  schema: "public"      # Default schema for queries
  poolMin: 1            # Minimum connection pool size
  poolMax: 10           # Maximum connection pool size
```

#### Redis

```yaml
redis:
  connectionString: "redis://user:password@host:port"
  db: 0                 # Database number (0-15)
  keyPrefix: "test:"    # Prefix added to all keys
```

#### MongoDB

```yaml
mongodb:
  connectionString: "mongodb://user:password@host:port"
  database: "mydb"      # Database name
```

#### EventHub

```yaml
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

## Environment Variables

Use `${env.VAR_NAME}` or `${VAR_NAME}` in configuration to reference environment variables:

```yaml
environments:
  staging:
    baseUrl: "${STAGING_URL}"
    adapters:
      postgresql:
        connectionString: "${STAGING_PG_CONNECTION_STRING}"
      redis:
        connectionString: "${STAGING_REDIS_URL}"
```

Set variables before running:

```bash
export STAGING_URL="https://staging.example.com"
export STAGING_PG_CONNECTION_STRING="postgresql://..."
npx e2e run --env staging
```

## Global Variables

Define variables accessible in all tests:

```yaml
variables:
  testPrefix: "e2e_test_"
  defaultUserId: "test-user-001"
  apiKey: "${API_KEY}"
```

Access in tests:

```yaml
execute:
  - adapter: http
    action: request
    url: "{{baseUrl}}/users/{{defaultUserId}}"
    headers:
      X-API-Key: "{{apiKey}}"
```

## Default Settings

Configure defaults applied to all tests:

```yaml
defaults:
  timeout: 30000    # 30 second timeout
  retries: 2        # Retry failed tests twice
  retryDelay: 1000  # 1 second between retries
  parallel: 4       # Run 4 tests concurrently
```

Override in individual tests:

```yaml
name: TC-SLOW-001
timeout: 120000     # Override: 2 minute timeout
retries: 5          # Override: 5 retries

execute:
  - adapter: http
    action: request
    url: "{{baseUrl}}/slow-endpoint"
```

## Reporters

Configure multiple reporters for different output formats:

### Console Reporter

Real-time terminal output:

```yaml
reporters:
  - type: console
    verbose: true     # Show detailed step output
```

### JUnit Reporter

XML format for CI/CD integration:

```yaml
reporters:
  - type: junit
    output: "./reports/junit.xml"
```

### HTML Reporter

Interactive HTML report:

```yaml
reporters:
  - type: html
    output: "./reports/report.html"
```

### JSON Reporter

Machine-readable JSON:

```yaml
reporters:
  - type: json
    output: "./reports/results.json"
```

## Multiple Environments

Define multiple environments for different stages:

```yaml
environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      postgresql:
        connectionString: "postgresql://postgres:postgres@localhost:5432/dev"

  staging:
    baseUrl: "https://staging-api.example.com"
    adapters:
      postgresql:
        connectionString: "${STAGING_DB_URL}"

  production:
    baseUrl: "https://api.example.com"
    adapters:
      postgresql:
        connectionString: "${PROD_DB_URL}"
```

Run against specific environment:

```bash
npx e2e run --env staging
npx e2e run --env production
```
