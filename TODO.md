# Tryve — Implementation TODO

Features not yet built, and known debt. Everything here is checked against the
current Go codebase; items completed by the TypeScript-to-Go port or since have
been removed rather than left as noise.

## Priority Levels

- **P0**: Critical — correctness gaps, or things that make a green suite untrustworthy
- **P1**: High — important for production use
- **P2**: Medium — nice to have
- **P3**: Low — future

---

## P0 — Critical

### 1. Consolidate the adapter registry

**Status**: Not done
**Files**: `internal/cli/run.go`, `internal/cli/health.go`, `pkg/runner/runner.go`

The `switch` that maps a config adapter name to its constructor is copy-pasted in
three places. Registering a new adapter in only one produces an adapter that works
under `tryve run` but reports "not registered" under `tryve health`, or that works
via the CLI but not via the `pkg/runner` library API.

Extract a single `adapter.BuildRegistry(cfg *config.LoadedConfig) *Registry` and
call it from all three. Then shorten the adapter checklist in `CLAUDE.md` and
`AGENTS.md` to one entry.

**Effort**: ~1 hour.

### 2. Validate unknown step fields

**Status**: Not done
**Files**: `internal/loader/parser.go`, `internal/loader/validator.go`

`parseStep` sweeps every key it does not recognise into `Params`, where an adapter
that does not read it ignores it in silence. A misspelled `commmand`, or a field
supported by one adapter and written on a step using another, produces no
diagnostic at all — the step just does something other than what was written.

Validation should compare each step's params against the keys its adapter actually
reads and report the leftovers. This needs adapters to declare their parameter
names; a `ParamSpec()` method on the `Adapter` interface is the obvious route, and
it would also let `validate` catch a required param that is present but of the
wrong type.

**Effort**: 4–6 hours.

---

## P1 — High Priority

### 3. `waitFor` / `retryUntil` on a step

**Status**: Not implemented
**Files**: `internal/executor/step.go`, `internal/tryve/types.go`, `internal/loader/parser.go`

Asserting on something a background worker produces currently means guessing a
`delay:` long enough, or shelling out to a bash polling loop. Both are flaky and
slow: the delay is either too short some of the time or wasted every time.

A step should be able to retry itself until its own assertions pass:

```yaml
- adapter: postgresql
  action: queryOne
  sql: "SELECT status FROM jobs WHERE id = $1"
  params: ["{{captured.job_id}}"]
  waitFor:
    timeout: 30000        # give up after 30s
    interval: 500         # re-run every 500ms
  assert:
    - column: status
      equals: "COMPLETED"
```

The retry machinery in `ExecuteStepWithRetry` is close to what is needed but
retries on *failure* with exponential backoff; this wants a fixed interval, a wall
-clock deadline, and a final outcome that reports how long it waited.

**Effort**: 4–6 hours.

### 4. Concurrent steps

**Status**: Not implemented
**Files**: `internal/executor/runner.go`, `internal/tryve/types.go`

Testing a race — two users claiming the last voucher, a double-submit against an
idempotency key — currently requires backgrounding `curl` from a shell step and
reassembling the results by hand from temp files. Tests are parallelised, but the
steps within a test are strictly sequential.

A phase should be able to declare a group of steps that run together and join
before the next step:

```yaml
execute:
  - parallel:
      - id: scan_a
        adapter: http
        ...
      - id: scan_b
        adapter: http
        ...
```

Captures from a parallel group must be deterministic — each step writes its own
keys, and the group joins before anything reads them.

**Effort**: 6–8 hours.

### 5. Multiple named instances of one adapter type

**Status**: Not implemented
**Files**: `internal/config/types.go`, the registry builder from item 1, `internal/loader/validator.go`

`EnvironmentConfig.Adapters` is keyed by adapter type, so a suite can reach exactly
one PostgreSQL database, one Redis, one Kafka. Testing anything that spans two
databases — a migration, a read replica, a second tenant's store — is impossible
without dropping to `psql` in a shell step.

Allow a qualified name, resolving `postgresql.reporting` to the adapter type before
the dot:

```yaml
adapters:
  postgresql:
    connectionString: "${PRIMARY_DSN}"
  postgresql.reporting:
    connectionString: "${REPORTING_DSN}"
```

```yaml
- adapter: postgresql.reporting
  action: query
  sql: "SELECT ..."
```

**Effort**: 4–5 hours.

### 6. Step-by-step interactive mode

**Status**: Not implemented; documented in `docs/sections/cli.md` as a flag that
does not exist
**Files**: `internal/cli/run.go`, new `internal/executor/interactive.go`

Pause after each step, print the result and the captured values, and let the user
continue, skip, or abort. Either implement `--step-by-step` or remove it from
`cli.md` — a documented flag that cobra rejects is worse than no flag.

**Effort**: 2–3 hours.

### 7. HTTP traffic capture

**Status**: Not implemented; `--capture-traffic` is likewise documented but absent
**Files**: `internal/adapter/http.go`, new `internal/executor/traffic.go`

Record the full request and response for every HTTP step to a file, keyed by test
and step id, for debugging a failure after the fact. HAR output would let the
result open in browser devtools.

Needs a hook the adapter can write to without knowing about the executor — a
`RoundTripper` wrapper installed on the client is the cleanest route.

**Effort**: 3–4 hours.

---

## P2 — Medium Priority

### 8. `matchesSchema` operator

**Status**: Not implemented
**Files**: `internal/assertion/matchers.go`

JSON Schema validation of a response body, for asserting a contract rather than
individual fields:

```yaml
assert:
  json:
    - path: "$.data"
      matchesSchema:
        type: object
        required: [id, name]
```

Adds a dependency (`santhosh-tekuri/jsonschema` is the usual choice); weigh that
against a single-binary distribution before taking it.

**Effort**: 2–3 hours.

### 9. Parallel test groups

**Status**: Not implemented
**Files**: `internal/executor/orchestrator.go`, `internal/tryve/types.go`

`defaults.parallel` is all-or-nothing. Tests that share mutable fixture state must
today force the whole suite to `parallel: 1`. Let a test declare a group whose
members run sequentially with respect to each other while different groups still
run concurrently.

Interacts with `depends`, which the orchestrator already topologically sorts —
build on `topoSortTests` rather than beside it.

**Effort**: 4–5 hours.

### 10. Report history and flaky-test detection

**Status**: Not implemented
**Files**: new `internal/reporter/history.go`

Persist results across runs (SQLite, or newline-delimited JSON to stay
dependency-free) and report pass/fail rates over time, so a test that fails one run
in ten is identified rather than re-run until it passes.

```yaml
reporters:
  - type: history
    output: "./reports/history.jsonl"
    retention: 30   # days
```

`--failed-only` already persists the previous run's failures; this generalises that
store.

**Effort**: 6–8 hours.

### 11. Custom matchers

**Status**: Not implemented
**Files**: `internal/assertion/matchers.go`

Project-specific operators (`toBeValidVietnamesePhone`, `toBeValidTenantSlug`).
Go's lack of runtime loading makes the TypeScript approach of importing a matcher
file impossible; the realistic options are a declarative regex/predicate table in
config, or leaving this to `matches`. Decide which before scheduling.

**Effort**: 3–4 hours for the config-table form.

---

## P3 — Low Priority

### 12. Read-only environments

**Status**: Not implemented
**Files**: `internal/config/types.go`, every adapter

Mark an environment read-only so write actions (`execute`, `insertOne`, `set`,
`del`, `produce`) are refused before they run. Aimed at `uat-verify`-style smoke
suites pointed at production, where a stray teardown step is expensive.

The shell command policy added in `adapters.shell` is the model: refuse with a
clear message naming the policy, rather than failing at the driver.

**Effort**: 2–3 hours.

### 13. Parameterised test templates

**Status**: `tryve test create --template http|shell` exists; templates take no
parameters
**Files**: `internal/cli/test_cmd.go`

Let a template be a file with placeholders filled from flags, so a CRUD suite for a
new resource is one command rather than a copy-paste.

**Effort**: 4–5 hours.

### 14. GraphQL adapter

**Status**: Not implemented
**Files**: new `internal/adapter/graphql.go`

Query/mutation support with GraphQL error handling, so `$.errors[0].extensions.code`
is assertable without treating the response as an opaque 200.

**Effort**: 4–5 hours. Follow the adapter checklist in `CLAUDE.md`.

### 15. gRPC adapter

**Status**: Not implemented
**Files**: new `internal/adapter/grpc.go`

Needs proto descriptors at runtime — server reflection, or a compiled descriptor
set named in config. Reflection is the better default for a test runner.

**Effort**: 6–8 hours. Follow the adapter checklist in `CLAUDE.md`.

### 16. Plugin system

**Status**: Not implemented, and probably should not be

Third-party adapters and reporters. Go has no practical runtime plugin story that
survives a single static binary across platforms (`plugin` is Linux-only and
requires matching toolchains). The realistic answer is that `pkg/runner` is the
extension point: consumers import Tryve as a library and register their own
adapters. Document that instead of building a plugin loader.

---

## Debt

- **`find` returns a different shape from `findOne`.** The mongodb `find` action
  returns `{documents: [...], count: N}`, while `docs/sections/adapters/mongodb.md`
  shows `path: "[0].status"` and `capture: {n: "length"}` — addressing the array
  directly. One of the two should change; `findOne` was aligned with the docs, and
  `find` has not been.
- **`kafka clear` takes 18 seconds.** It drains a topic by reading until the
  read times out, so every test using it pays the adapter's full timeout in
  setup. It should stop at the high-water mark instead.
- **`--priority` is a single value**, not repeatable, despite reading like a list
  filter. Either accept a comma-separated list or rename it.
- **`migrate` cannot rewrite, only pin.** Some differences are mechanically
  fixable — a `count` step whose SQL aggregates, a quoted builtin argument — and
  a `--fix` mode could apply those, shrinking the pin set. Everything else
  (whether a captured value is a number, whether a body is an array) needs the
  suite to actually run, so the pin-and-work-through loop stays the general case.
- **Retiring `tryve/v1`.** Every behaviour gated by `apiVersion` carries two code
  paths and two tests. Once suites have migrated, drop the v1 branches and the
  `legacyOperators`/`legacyStringify`/`legacyNormalizeValue` helpers with them.
  A `tryve/v3` would then be gated against v2, not v1.
- **Keep the docs honest.** `docs/sections/cli.md` once listed five flags that did
  not exist, and `multipart` and `{row, column}` assertions were documented for a
  long time before either was implemented. Test authors write against these pages;
  a documented-but-missing feature means their checks silently do nothing. Build
  the feature or leave it out of the docs.


---

## Contributing

When implementing a feature:

1. Create a branch: `feature/<feature-name>`.
2. Add unit tests beside the code (`internal/<pkg>/<file>_test.go`), and a YAML
   test under `tests/e2e/` when the change is user-visible.
3. Update documentation in all three places named by the Documentation Sync Rule in
   `CLAUDE.md`.
4. Run `make test` and `make lint`.
5. Open a PR describing the change and what it was verified against.

## Notes

- Effort estimates assume familiarity with the codebase.
- YAML syntax is backward compatible: a change that makes an existing test file
  behave differently needs a stated migration path, except where the old behaviour
  was to silently skip a check.
