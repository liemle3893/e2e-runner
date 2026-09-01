# Shell Adapter

Execute shell commands and scripts as test steps. Useful for CLI tool testing, database migrations, setup/teardown scripts, file operations, and system health checks.

## Configuration

```yaml
environments:
  local:
    baseUrl: "http://localhost:3000"
    adapters:
      shell:
        defaultTimeout: 60000        # Timeout in ms for steps that set none (default: 60000)
        cwd: "."                     # Working directory, relative to the config file
        allow:                       # Optional: only these commands may run
          - "^node scripts/e2e/"
          - "^yarn (build|db:migrate)$"
        deny:                        # Optional: refused before allow is consulted
          - "rm -rf /"
        env:                         # Optional: the only variables commands inherit
          - PATH
          - HOME
          - POSTGRESQL_CONNECTION_STRING
```

The Shell adapter runs commands through `sh -c` (`cmd /C` on Windows) with **no
peer dependencies**.

## Command Policy

A shell step runs a command on whatever machine the suite runs on, with whatever
that machine's environment holds. The policy keys narrow that:

- **`allow`** — a list of regular expressions. When present the adapter is
  deny-by-default: a command matching none of them is refused before it runs,
  with an error naming the policy. When absent, any command runs.
- **`deny`** — regular expressions refused outright. Evaluated before `allow`.
- **`env`** — the environment variables a command inherits. When present, nothing
  else is passed through, so a step cannot read a credential it was not given.
  When absent, the full process environment is inherited. A step's own `env:`
  block is always applied on top.
- **`cwd`** — the working directory, resolved relative to the config file.

Start by adding an `allow` list covering the scripts your suite genuinely needs;
anything else then fails loudly rather than executing.

**Every command is bounded by a timeout** — the step's `timeout`, else
`defaultTimeout`, else 60 seconds. On expiry the whole process group is killed,
so a server or container the command started in the background does not outlive
its step.

## Action: `exec`

Execute a shell command with full shell feature support (pipes, redirects, globbing, subshells).

```yaml
- adapter: shell
  action: exec
  command: string                    # Required: shell command to execute
  cwd?: string                      # Working directory override
  timeout?: number                  # Command timeout in ms (default: 30000)
  env?: Record<string, string>      # Environment variables (merged with process.env)
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `command` | string | Yes | - | The shell command to execute |
| `cwd` | string | No | process.cwd() | Working directory for the command |
| `timeout` | number | No | 30000 | Kill command after this many milliseconds |
| `env` | object | No | process.env | Environment variables merged with process.env |

### Response Structure

The `exec` action returns the following data structure:

```typescript
{
  exitCode: number;    // Process exit code (0 = success)
  stdout: string;      // Standard output content
  stderr: string;      // Standard error content
  duration: number;    // Execution time in milliseconds
}
```

Non-zero exit codes are returned as data, not as errors. This allows you to assert on specific exit codes.

## Assertions

Assert on exit code, stdout, and stderr content:

```yaml
assert:
  # Exit code (exact match)
  exitCode: 0

  # Standard output assertions
  stdout:
    contains: "expected substring"
    matches: "^pattern\\d+$"         # Regex pattern
    equals: "exact output"

  # Standard error assertions
  stderr:
    contains: "warning"
    matches: "ERROR.*timeout"
    equals: "exact error message"
```

### Assertion Operators

| Field | Operator | Description |
|-------|----------|-------------|
| `exitCode` | exact number | Exit code must match exactly |
| `stdout.contains` | substring | stdout must include the string |
| `stdout.matches` | regex | stdout must match the pattern |
| `stdout.equals` | exact (trimmed) | stdout must equal the value (whitespace-trimmed) |
| `stderr.contains` | substring | stderr must include the string |
| `stderr.matches` | regex | stderr must match the pattern |
| `stderr.equals` | exact (trimmed) | stderr must equal the value (whitespace-trimmed) |

## Value Capture

Capture stdout, stderr, or exit code for use in later steps:

```yaml
capture:
  output: "stdout"      # Capture full stdout as variable
  errors: "stderr"      # Capture full stderr as variable
  code: "exitCode"      # Capture exit code as number
```

Use captured values in subsequent steps with `{{captured.output}}`.

## Examples

**Basic: Echo command with exit code assertion**
```yaml
- adapter: shell
  action: exec
  command: "echo 'hello world'"
  assert:
    exitCode: 0
    stdout:
      contains: "hello"
```

**Script: Run a script file and capture output**
```yaml
- adapter: shell
  action: exec
  command: "./scripts/get-version.sh"
  capture:
    version: "stdout"
  assert:
    exitCode: 0
```

**Setup: Database migration in setup phase**
```yaml
setup:
  - adapter: shell
    action: exec
    command: "npx prisma migrate deploy"
    timeout: 60000
    env:
      DATABASE_URL: "postgresql://user:pass@localhost:5432/testdb"
    assert:
      exitCode: 0
```

**Environment: Pass env vars and set cwd**
```yaml
- adapter: shell
  action: exec
  command: "npm run seed"
  cwd: "/app/backend"
  timeout: 30000
  env:
    NODE_ENV: "test"
    DB_HOST: "localhost"
  assert:
    exitCode: 0
```

**Capture: Use command output in later steps**
```yaml
execute:
  - adapter: shell
    action: exec
    command: "curl -s http://localhost:3000/version"
    capture:
      api_version: "stdout"
    assert:
      exitCode: 0

  - adapter: http
    action: request
    method: GET
    url: "{{baseUrl}}/health"
    assert:
      status: 200
      json:
        - path: "$.version"
          equals: "{{captured.api_version}}"
```

## Timeout Behavior

Every command has a timeout: the step's `timeout`, else the adapter's
`defaultTimeout`, else 60 seconds. When it expires:

- The command's entire process group is killed, not just the shell — anything the
  command backgrounded (a server, a `docker run`) dies with it
- The step fails with an error naming the timeout that was exceeded

## Preferring built-ins over shell steps

Several common shell steps have a built-in equivalent that runs in-process,
needs no policy exception, and fails with a clearer message:

| Shell command | Use instead |
|---|---|
| `node -e "process.stdout.write(String(Date.now()))"` | `{{$now(unixMs)}}` |
| `node -e "…JSON.parse(argv[1]).someId"` on a captured value | `{{captured.result.someId}}` |
| `cat file.json \| jq -r .some.key` | `{{$jsonFile(file.json, some.key)}}` |
| `echo "$JSON" \| jq .field` | `{{$jsonPath(captured.json, field)}}` |
| a script that mints a test JWT | `{{$jwt(HS256, secret, {"sub":"…"}, 1h)}}` |
| `sleep 2` between steps | `delay: 2000` on the following step |

See `tryve doc built-in-functions`.
