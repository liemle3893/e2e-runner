# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run Commands

```bash
make build             # Build ./bin/autoflow
make test              # Run all tests
make test-v            # Tests with verbose output
make lint              # golangci-lint
make clean             # Remove build artifacts
```

## CLI Commands

Top-level commands live under `internal/cli/`; the binary is built from `cmd/autoflow/`.

```bash
# Core runner
autoflow e2e run                                 # Run all tests
autoflow e2e run --tag smoke --bail              # Filter + early-exit
autoflow e2e validate                            # Validate YAML test files
autoflow e2e list                                # List discovered tests
autoflow e2e health                              # Adapter connectivity check
autoflow e2e init                                # Create e2e.config.yaml
autoflow e2e test create <name>                  # Create test from template

# Docs
autoflow e2e doc                                 # List doc sections
autoflow e2e doc assertions                      # Assertions reference
autoflow e2e doc adapters.http                   # HTTP adapter docs

# Install
autoflow install --skills                    # Install autoflow-cli Claude skill
autoflow install --autoflow                  # Install autoflow skills + agents
                                          # (auto-cleans legacy .claude/scripts/autoflow/)

# Autoflow — ported from winx-autoflow, no bash scripts
autoflow deliver jira config {set,get,del,show}
autoflow deliver jira upload <issue-key> <file>...
autoflow deliver jira download <issue-key> <dest-dir>
autoflow deliver worktree bootstrap <worktree-path>
autoflow deliver {init,next,complete}
autoflow deliver loop-state {init,append,read,round-count} <state-file>
autoflow deliver config {set,get,del,show}
autoflow deliver doctor             # Preflight checklist
autoflow e2e scaffold --ticket KEY --area AREA --count N
```

## Shared Utilities (use these, don't reimplement)

### Test runner

New adapters and features **must** reuse these shared utilities:

- **`internal/adapter/base.go`** — `measureDuration()` for timing, `captureValues()` for capture loops, `successResult()`/`failResult()` for return values, `logAction()`/`logResult()` for logging.
- **`internal/assertion/runner.go`** — `Run(value, assertion, path)` handles `equals`, `contains`, `matches`, `exists`, `type`, `length`, `greaterThan`, `lessThan`, `isNull`, `isNotNull`. Use this instead of hand-rolling assertion logic.

### Autoflow packages

When extending the ported autoflow workflow, reuse these packages — do not recreate their logic in a CLI wrapper:

- **`internal/autoflow/state/`** — JSON state managers (workflow-progress, loop-state, review-state, verify).
- **`internal/autoflow/jira/`** — Jira config cache + REST v3 client (upload, download, myself).
- **`internal/autoflow/worktree/`** — bootstrap + safe command allowlist.
- **`internal/autoflow/deliver/`** — 13-step controller (Instruction type, step funcs, brief parser, gate helper).
- **`internal/autoflow/e2e/`** — env loader, file lock, git-merge-and-run, loop wrapper.
- **`internal/autoflow/report/`** — PR/Jira/execution report generators.
- **`internal/autoflow/scaffold/`** — E2E test stub generator.
- **`internal/autoflow/extract/`** — REVIEW-*.md parser for review-loop round data.
- **`internal/autoflow/doctor/`** — preflight check battery.

The CLI wrappers in `internal/cli/autoflow_*.go` are thin Cobra shells. Put real logic in the packages above.

## Documentation Sync Rule

Every change to CLI commands, adapters, configuration, assertions, built-in functions, or YAML test syntax **must** also be reflected in **all three** of these locations:

1. **Docs** — `docs/sections/` markdown files
2. **CLI doc registry** — `docs/sections/index.json` (maps section names to files for `autoflow e2e doc <section>`)
3. **Skill template** — `skills/autoflow-cli/SKILL.md` (the source skill file shipped with the binary)

### How Skills Are Installed

`autoflow install --skills` (see `internal/cli/install.go`) copies files to the user's project:
- `skills/autoflow-cli/SKILL.md` → `.claude/skills/autoflow-cli/SKILL.md`
- `docs/sections/**` → `.claude/skills/autoflow-cli/references/**`

`autoflow install --autoflow` copies:
- `skills/autoflow/**` → `.claude/skills/autoflow-*/`
- `agents/autoflow/**` → `.claude/agents/autoflow-*.md`
- Removes any legacy `.claude/scripts/autoflow/` directory left by the old bash installer.

**Always edit the sources** — `skills/autoflow-cli/SKILL.md`, `skills/autoflow/**`, `agents/autoflow/**`. Never edit `.claude/...` directly; those are generated output.

Relevant doc files:
- `docs/sections/cli.md` — CLI commands and flags
- `docs/sections/adapters/` — Per-adapter reference docs
- `docs/sections/index.json` — Section registry for `autoflow e2e doc`
- `docs/sections/config.md` — `e2e.config.yaml` reference
- `docs/sections/assertions.md` — Assertion operators and JSONPath
- `docs/sections/built-in-functions.md` — Built-in functions
- `docs/sections/yaml-test.md` — YAML test file syntax
- `docs/sections/examples.md` — Usage examples

## Adding a New Adapter — Checklist

When adding a new adapter, **all** of these files must be created or updated:

| File | Action |
|------|--------|
| `internal/adapter/<name>.go` | Create adapter implementing `adapter.Adapter` |
| `internal/adapter/registry.go` | Register adapter in `Default()` / `BuildFromConfig()` |
| `internal/config/config.go` | Add config struct, hook into `EnvironmentConfig.Adapters` |
| `internal/loader/validation.go` | Add to `validAdapters`, add validation case |
| `internal/cli/health.go` | Add display name |
| `docs/sections/adapters/<name>.md` | Create full adapter documentation |
| `docs/sections/adapters/index.md` | Add to adapter table and peer deps section |
| `docs/sections/index.json` | Register `adapters.<name>` |
| `skills/autoflow-cli/SKILL.md` | Add adapter to syntax reference and links |
| `internal/adapter/<name>_test.go` | Unit tests |
| `tests/e2e/adapters/TC-<NAME>-001.test.yaml` | E2E integration test |

## Autoflow Port Reference

The autoflow subcommand tree was ported from
[`winx-autoflow`](https://github.com/the-winx-corp/winx-ai-autoflow) — ~4 KLOC
of bash + Python collapsed into Go packages under `internal/autoflow/`.
The full design doc lives at `.planning/autoflow-port/DESIGN.md`.

Key contracts preserved during the port (all in `internal/autoflow/state/paths.go`):

- `.autoflow/ticket/<KEY>/workflow-progress.json` — shape matches the bash jq
  output (`ticket`, `started_at`, `worktree`, `branch`, `current_step`,
  `completed`, `pr_url`, `gsd_quick_id`, `impl_plan_dir`, optional `title`).
- `.autoflow/ticket/<KEY>/state/*.json` — loop / review state with
  `{loop, ticket, max_rounds, rounds[]}`.
- `.autoflow/jira-config.json` — `{cloudId, siteUrl, projectKey, email, cached_at}`.
- `.autoflow/bootstrap.json` — `{language, base_branch, config_files, install_cmd, verify_cmd, build_cmd, test_cmd, services_cmd}`.

Ticket keys are validated against `^[A-Z][A-Z0-9]+-\d+$` before use in any
path-building operation.
