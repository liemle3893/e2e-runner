# Process Adapter

Manage background process lifecycles within test suites. Start servers, workers, or any long-running command as part of test setup with automatic port allocation, readiness probes, and graceful teardown.

## Configuration

The Process adapter requires no configuration in `e2e.config.yaml`. It is always available (like the Shell adapter) and has **no peer dependencies**.

## Action: `start`

Launch a background process with optional free port allocation and readiness probes.

```yaml
- name: my-server                    # Required: identifies this process for capture/stop
  adapter: process
  action: start
  command: go run main.go start      # Required: shell command to execute
  env:                               # Optional: environment variables
    APP_HTTP_PORT: "{{free_port}}"
  cwd: /path/to/project              # Optional: working directory
  readiness:                         # Optional: gate tests until ready
    http: "http://localhost:{{free_port}}/health"  # HTTP probe (2xx = ready)
    # tcp: "localhost:{{free_port}}"              # TCP probe (connection = ready)
    # cmd: "curl -sf http://localhost:{{free_port}}/health"  # Command probe (exit 0 = ready)
    timeout: 15s                     # Max wait time (default: 15s)
    interval: 500ms                  # Poll interval (default: 500ms)
  auto_teardown: true                # Kill when suite ends (default: true)
  teardown_timeout: 5s               # SIGTERM → wait → SIGKILL timeout (default: 5s)
  capture:
    port: "port"                     # Capture allocated port
    pid: "pid"                       # Capture process ID
```

### Result Data

| Field | Type | Description |
|-------|------|-------------|
| `pid` | number | OS process ID of the started process |
| `port` | number | Allocated free port |

### Free Port Allocation

Every `process/start` step automatically allocates a free TCP port. The token `{{free_port}}` (or `${free_port}`) is replaced with the allocated port number in `command`, `env` values, and `readiness` configuration before the process starts.

This avoids port conflicts when running tests in parallel or across multiple test suites.

### Readiness Probes

Readiness probes gate test execution until the background process is healthy. Exactly one probe type must be configured:

| Probe | Success Condition | Use Case |
|-------|-------------------|----------|
| `http` | GET returns 2xx status | Web servers with health endpoints |
| `tcp` | TCP connection established | Services with TCP listeners |
| `cmd` | Command exits with code 0 | Custom health checks, CLI tools, pidfile checks |

If the probe times out, the process is killed and the step fails with diagnostic output (stdout/stderr from the process).

If the process crashes before becoming ready, the probe fails immediately without waiting for the full timeout.

### Namespaced Capture

When a step has a `name`, captured values are stored in a nested map accessible via dot notation:

```yaml
# Access via: {{captured.my-server.port}} and {{captured.my-server.pid}}
capture:
  port: "port"
  pid: "pid"
```

This prevents capture collisions when running multiple background processes.

## Action: `stop`

Terminate a background process by name or PID.

```yaml
# Stop by name (preferred)
- adapter: process
  action: stop
  target: my-server                  # Process name from the start step
  signal: SIGTERM                    # Optional: SIGTERM (default), SIGKILL, SIGINT
  timeout: 5s                       # Optional: wait before SIGKILL (default: 5s)

# Stop by PID (for cross-suite cleanup)
- adapter: process
  action: stop
  pid: "{{captured.my-server.pid}}"
  signal: SIGTERM
  timeout: 5s
```

### Graceful Shutdown

1. Send the configured signal (default: SIGTERM) to the process group
2. Wait up to `timeout` for the process to exit
3. If still running, send SIGKILL

### Result Data

| Field | Type | Description |
|-------|------|-------------|
| `stopped` | string/number | Name or PID of the stopped process |

## Auto-Teardown

When `auto_teardown: true` (the default), background processes are automatically killed when the test suite ends — via `SIGTERM → wait → SIGKILL` using each process's `teardown_timeout`.

For earlier cleanup, add an explicit `process/stop` step in the teardown phase.

## Signal Handling

When the test runner receives SIGINT or SIGTERM (e.g., Ctrl+C), all tracked background processes are terminated before the runner exits. This prevents orphaned processes.

## Examples

### Start a Go server with health check

```yaml
setup:
  - name: api-server
    adapter: process
    action: start
    command: go run ./cmd/server
    env:
      PORT: "{{free_port}}"
      DB_URL: "postgres://localhost:5432/testdb"
    readiness:
      http: "http://localhost:{{free_port}}/healthz"
      timeout: 30s
    capture:
      port: "port"
      pid: "pid"

execute:
  - adapter: http
    action: request
    url: "http://localhost:{{captured.api-server.port}}/api/users"
    method: GET
    assert:
      status: 200
```

### Multiple background services

```yaml
setup:
  - name: api
    adapter: process
    action: start
    command: go run ./cmd/api
    env:
      PORT: "{{free_port}}"
    readiness:
      http: "http://localhost:{{free_port}}/health"
    capture:
      port: "port"

  - name: worker
    adapter: process
    action: start
    command: go run ./cmd/worker
    env:
      API_URL: "http://localhost:{{captured.api.port}}"
    readiness:
      cmd: "curl -sf http://localhost:{{captured.api.port}}/worker-status"
      timeout: 20s
    capture:
      pid: "pid"
```

### Explicit teardown

```yaml
teardown:
  - adapter: process
    action: stop
    target: api-server
    signal: SIGTERM
    timeout: 10s
```
