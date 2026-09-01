# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Tryve is a **Go** project: a single-binary, YAML-driven, multi-protocol end-to-end
test runner. The repository is entirely Go — there is no `src/`, no
`package.json`, and no Node toolchain anywhere, including for the test fixtures.

## Build & Run Commands

```bash
make build             # Build ./bin/tryve (embeds skills + docs)
make test              # go test ./...
make test-v            # go test -v ./...
make lint              # golangci-lint run ./...
make clean             # Remove bin/
make run ARGS="run -d tests/e2e"   # go run ./cmd/tryve with arguments
```

`make build` stamps the version from `git describe` into `main.version`.

## CLI Commands

The binary is `tryve` (`./bin/tryve` after a local build).

```bash
# Run tests
tryve run                                 # Run every test under the configured testDir
tryve run tests/e2e/users/TC-USER-001.test.yaml   # One file — fastest iteration
tryve run tests/e2e/users tests/e2e/auth  # Several directories
tryve run 'tests/e2e/**/TC-AUTH-*.test.yaml'      # Glob (quote it)
tryve run --tag smoke --bail              # Filter by tag, stop on failure
tryve run --strict                        # Fail on unresolvable {{expressions}}

# Other commands
tryve validate                            # Parse and validate test files
tryve list                                # List discovered tests
tryve health                              # Check adapter connectivity
tryve init                                # Create a starter e2e.config.yaml
tryve test create <name>                  # Create a test from a template
tryve test list-templates                 # List available templates

# Documentation
tryve doc                                 # List documentation sections
tryve doc assertions                      # Show the assertions reference
tryve doc adapters.http                   # Show HTTP adapter docs

# Skills
tryve install --skills                    # Install the Claude Code skill into .claude/
```

Global flags: `-c/--config` (config path, default `e2e.config.yaml`),
`-e/--env` (environment name, default `local`).

## Layout

| Path | Contents |
|------|----------|
| `cmd/tryve/` | `main` — thin entry point |
| `internal/cli/` | Cobra commands: `run`, `validate`, `list`, `health`, `init`, `test`, `doc`, `install`, `version` |
| `internal/config/` | `e2e.config.yaml` loading, `${VAR}` substitution, `.env` loading |
| `internal/loader/` | Test-file discovery (`DiscoverAll`), YAML parsing, validation |
| `internal/migrate/` | Static analysis of behaviour that differs between compatibility levels, and the per-file pins that make a large suite migratable |
| `internal/executor/` | Step execution, retries, per-test orchestration, lifecycle hooks |
| `internal/adapter/` | One file per protocol adapter, plus the `Adapter` interface and `Registry` |
| `internal/assertion/` | Operator matchers, assertion dispatch, JSONPath |
| `internal/interpolate/` | `{{expression}}` / `${expression}` resolution, built-in functions |
| `internal/reporter/` | console, junit, html, json reporters |
| `internal/watcher/` | `--watch` file watching |
| `internal/tryve/` | Shared types, error constructors, and the compatibility mode — imported by everything, imports nothing |
| `pkg/runner/` | Public library API (`RunTests`, `ValidateTests`, `ListTests`, `CheckHealth`) |
| `embed.go` | Embeds `skills/` and `docs/sections/` into the binary for `install --skills` |
| `cmd/demo-server/` | The system under test for `tests/e2e/` — a small REST API over PostgreSQL, Redis, MongoDB, and Event Hubs. Not shipped; built with `make demo-server` |
| `tests/integration/` | Go integration tests |
| `tests/e2e/adapters/` | Tryve's own YAML tests, run against demo-server and the docker-compose services |

Unit tests live beside the code they exercise (`internal/<pkg>/*_test.go`), not
under `tests/`.

## Shared Utilities (use these, don't reimplement)

New adapters and features **must** reuse these:

- **`internal/adapter/adapter.go`** — the `Adapter` interface; `MeasureDuration()`
  for timing; `SuccessResult()` for return values; `CheckUnresolvedEnvVars()` to
  turn a leftover `${VAR}` in a connection string into a clear error.
- **`internal/assertion/assertion.go`** — `RunAssertions(data, assertDef)` handles
  every assertion shape (HTTP keys, result fields, `{path, op}` lists, and
  `{row, column, op}` for SQL). Never hand-roll assertion dispatch in an adapter;
  return result data and let this evaluate it.
- **`internal/assertion/matchers.go`** — `Match(operator, actual, expected)` plus
  `CanonicalOperator()` for alias resolution. Add new operators here, not in an
  adapter.
- **`internal/interpolate/`** — `ResolveValue()` (type-preserving),
  `ResolveString()` (text), `Lookup()` (dotted paths into captured data).
  Adapters receive already-resolved params and must not interpolate themselves.
- **`internal/tryve/errors.go`** — `AdapterError`, `ConnectionError`,
  `ValidationError`, `ConfigError`, `ExecutionError`, `InterpolationError`.
  Every error surfaced to a user should come from one of these.

## Documentation Sync Rule

Every change to CLI commands, adapters, configuration, assertions, built-in
functions, or YAML test syntax **must** also be reflected in **all three** of
these locations:

1. **Docs** — `docs/sections/` markdown files
2. **CLI doc registry** — `docs/sections/index.json` (maps section names to files for `tryve doc <section>`)
3. **Skill template** — `skills/e2e-runner/SKILL.md` (the source skill file shipped in the binary)

Docs are not decoration here: they are compiled into the binary by `embed.go` and
installed into users' projects, where agents read them as the authority on what
Tryve can do. A feature documented but not implemented is worse than one that is
undocumented — test authors write against it and their assertions silently do
nothing.

### How Skills Are Installed

`tryve install --skills` (see `internal/cli/install.go`) copies the embedded
trees into the user's project:

- `skills/e2e-runner/SKILL.md` → `.claude/skills/e2e-runner/SKILL.md`
- `docs/sections/**` → `.claude/skills/e2e-runner/references/**`

**Always edit `skills/e2e-runner/SKILL.md`** — this is the source of truth. Never
edit `.claude/skills/` directly; those are generated output. The reference files
under `.claude/skills/e2e-runner/references/` come from `docs/sections/`
automatically at install time, so updating docs is sufficient for references.

Because both trees are `//go:embed`-ed, a docs change only reaches users after a
rebuild.

Relevant doc files:

- `docs/sections/cli.md` — CLI commands and flags
- `docs/sections/adapters/` — Per-adapter reference docs
- `docs/sections/index.json` — CLI `doc` command section registry (must list every adapter)
- `docs/sections/config.md` — Configuration (`e2e.config.yaml`) reference
- `docs/sections/assertions.md` — Assertion operators and JSONPath syntax
- `docs/sections/built-in-functions.md` — Built-in functions (`$uuid`, `$now`, `$jwt`, etc.)
- `docs/sections/yaml-test.md` — YAML test file syntax and structure
- `docs/sections/examples.md` — Usage examples

## Adding a New Adapter — Checklist

When adding a new adapter, **all** of these files must be created or updated:

| File | Action |
|------|--------|
| `internal/adapter/<name>.go` | Implement the `Adapter` interface; add a `New<Name>Adapter(cfg map[string]any)` constructor |
| `internal/cli/run.go` | Register the adapter in the `cfg.Environment.Adapters` switch |
| `internal/cli/health.go` | Register it in that command's copy of the same switch |
| `pkg/runner/runner.go` | Register it in `buildRegistry()` — the library API's copy of the switch |
| `internal/loader/validator.go` | Add to `validAdapters` (and the suggestion string beside it); add a case to `validateAdapterConstraints()` declaring the valid actions and required params |
| `docs/sections/adapters/<name>.md` | Create full adapter documentation |
| `docs/sections/adapters/index.md` | Add to the adapter table |
| `docs/sections/index.json` | Register the `adapters.<name>` section for `tryve doc` |
| `skills/e2e-runner/SKILL.md` | Add the adapter to the syntax reference |
| `internal/adapter/<name>_test.go` | Unit tests |
| `tests/e2e/adapters/TC-<NAME>-001.test.yaml` | E2E test exercising it end to end |
| `cmd/demo-server/` | An endpoint backed by the new service, when the adapter needs one to test against |

The registry switch is currently duplicated in three places (`internal/cli/run.go`,
`internal/cli/health.go`, `pkg/runner/runner.go`). Registering in only one of them
produces an adapter that works under `tryve run` but reports "not registered"
under `tryve health`, or vice versa. Consolidating them into one shared builder is
worth doing the next time this list is touched.

Adapter configuration is a plain `map[string]any` per environment
(`EnvironmentConfig.Adapters`), so no new config struct is needed — parse and
validate the keys inside the adapter's constructor.

## Running the E2E Suite

Tryve's own YAML tests run against `cmd/demo-server` and the services in
`docker-compose.yaml`:

```bash
docker compose up -d          # postgres, redis, mongodb, kafka, eventhub emulator
make demo-server && ./bin/demo-server &
tryve run --env demo -d tests/e2e/adapters
```

`tryve health --env demo` reports which backends are reachable, and
`GET localhost:3000/health` does the same from the server's side.

## API Versions

Behaviour that changes how an existing suite behaves goes behind `apiVersion` in
`e2e.config.yaml`, which defaults to `tryve/v1` — pointing a new binary at an
existing suite must never change a test's outcome. `internal/tryve/compat.go`
defines the versions and the areas they group (`assertions`, `interpolation`,
`execution`, `adapters`); a `compatibility` map refines the version per area. The
resolved level is threaded through `InterpolationContext.Compat`,
`assertion.RunAssertions`, the request `context.Context` for adapters, and each
adapter's constructor.

A test file may declare its own `apiVersion`, overriding the suite. That is what
`tryve migrate` writes when it pins a file, and why the level travels on the
context: adapters are built once per suite but must honour a per-file version.

When you change behaviour that an existing test could depend on:

1. Put the new behaviour behind the relevant area, defaulting to the old one.
   A new area, or a new version, means a new entry in `compat.go`.
2. Add a test pinning the **old** behaviour under `LegacyCompat()`, alongside the
   test for the new one under `ModernCompat()`. The legacy test is the contract.
3. Document both columns in the `compatibility` table in `docs/sections/config.md`
   and `skills/e2e-runner/SKILL.md`.
4. Add a detection rule to `internal/migrate/rules.go` so `tryve migrate` reports
   the change, marking it `WillChange` when it is decidable from the file and
   `MayChange` when it depends on runtime values. An ungated change nobody can
   find is as bad as an ungated change — a suite of a few hundred files cannot be
   audited by hand.

A purely additive change — a new operator, a new adapter action, a parameter that
did not previously exist, or fixing something that always errored — needs no gate,
because no passing test can depend on it.

## Conventions

- **Adapters return data; assertions evaluate it.** An adapter's job is to run an
  action and put its output in `StepResult.Data`. It must not decide pass/fail —
  the executor and `internal/assertion` do that, so every adapter behaves the same
  way under every operator.
- **Never drop an unrecognised key silently.** An unknown assertion operator, a
  misspelled field, an unresolved expression: surface it as a failure or a
  validation error. A silently ignored assertion turns a red test green, which is
  the worst outcome this tool can produce.
- **Normalise adapter values to JSON-friendly types.** Driver-specific types
  (`pgtype.Numeric`, raw UUID bytes, `time.Time`) must be converted before they
  reach assertions; otherwise comparisons fail against values a test author would
  reasonably write.
- **`internal/tryve` imports nothing from the rest of the tree.** Keep it that way;
  every other package depends on it.
