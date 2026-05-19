# AGENTS.md

Project instructions for AI coding agents working with this repository.

## Build & Run Commands

```bash
npm run build          # Compile TypeScript to dist/
npm run clean          # Remove dist/
npm test               # Run unit tests (vitest)
npm run prepublishOnly # Clean + build (for publishing)
npm run local:install  # Local installation
```

## CLI Commands

```bash
# Run tests
./bin/e2e.js run                          # Run all tests
./bin/e2e.js run --tag smoke --bail       # Filter by tag, stop on failure
./bin/e2e.js run --watch                  # Watch mode, auto-rerun on changes

# Other commands
./bin/e2e.js validate                     # Validate test file syntax
./bin/e2e.js list                         # List discovered tests
./bin/e2e.js health                       # Check adapter connectivity
./bin/e2e.js init                         # Initialize e2e.config.yaml
./bin/e2e.js test create <name>           # Create test from template

# Documentation
./bin/e2e.js doc                          # List documentation sections
./bin/e2e.js doc assertions               # Show assertions reference
./bin/e2e.js doc adapters.http            # Show HTTP adapter docs

# Skills
./bin/e2e.js install --skills             # Install Claude Code skills to project
```

## Shared Utilities (use these, don't reimplement)

New adapters and features **must** reuse these shared utilities:

- **`base.adapter.ts`** — `measureDuration()` for timing, `captureValues()` for capture loops, `successResult()`/`failResult()` for return values, `logAction()`/`logResult()` for logging
- **`assertion-runner.ts`** — `runAssertion(value, assertion, path)` handles `equals`, `contains`, `matches`, `exists`, `type`, `length`, `greaterThan`, `lessThan`, `isNull`, `isNotNull`. Use this instead of hand-rolling assertion logic.

## Documentation Sync Rule

Every change to CLI commands, adapters, configuration, assertions, built-in functions, or YAML test syntax **must** also be reflected in **all three** of these locations:

1. **Docs** — `docs/sections/` markdown files
2. **CLI doc registry** — `docs/sections/index.json` (maps section names to files for `e2e doc <section>`)
3. **Skill template** — `skills/autoflow-cli/SKILL.md` (the source skill file shipped with the package)

### How Skills Are Installed

`e2e install --skills` (see `src/cli/install.command.ts`) copies files to the user's project:
- `skills/autoflow-cli/SKILL.md` → `.claude/skills/autoflow-cli/SKILL.md`
- `docs/sections/**` → `.claude/skills/autoflow-cli/references/**`

**Always edit `skills/autoflow-cli/SKILL.md`** — this is the source of truth. Never edit `.claude/skills/` directly; those are generated output. The reference files under `.claude/skills/autoflow-cli/references/` come from `docs/sections/` automatically at install time, so updating docs is sufficient for references.

### Relevant doc files

- `docs/sections/cli.md` — CLI commands and flags
- `docs/sections/adapters/` — Per-adapter reference docs
- `docs/sections/index.json` — CLI `doc` command section registry (must list every adapter)
- `docs/sections/config.md` — Configuration (`e2e.config.yaml`) reference
- `docs/sections/assertions.md` — Assertion operators and JSONPath syntax
- `docs/sections/built-in-functions.md` — Built-in functions (`$uuid`, `$now`, `$totp`, etc.)
- `docs/sections/yaml-test.md` — YAML test file syntax and structure
- `docs/sections/examples.md` — Usage examples

## Adding a New Adapter — Checklist

When adding a new adapter, **all** of these files must be created or updated:

| File | Action |
|------|--------|
| `src/adapters/<name>.adapter.ts` | Create adapter extending `BaseAdapter` |
| `src/adapters/index.ts` | Export the adapter class and types |
| `src/adapters/adapter-registry.ts` | Register adapter in `initializeAdapters()`, add `get<Name>()`, update `parseAdapterType()` |
| `src/types.ts` | Add config interface, add to `AdapterType` union, add to `EnvironmentConfig.adapters` |
| `src/core/yaml-loader.ts` | Add to `VALID_ADAPTERS`, add validation case in `validateAdapterStep()` |
| `src/cli/health.command.ts` | Add display name to `formatAdapterName()` |
| `docs/sections/adapters/<name>.md` | Create full adapter documentation |
| `docs/sections/adapters/index.md` | Add to adapter table and peer deps section |
| `docs/sections/index.json` | Register `adapters.<name>` section for CLI `doc` command |
| `skills/autoflow-cli/SKILL.md` | Add adapter to syntax reference and links (source template) |
| `tests/unit/<name>-adapter.test.ts` | Unit tests |
| `tests/e2e/adapters/TC-<NAME>-001.test.yaml` | E2E integration test |

## Code Style & Conventions

- TypeScript strict mode
- Use vitest for unit tests
- Adapters must extend `BaseAdapter` and use shared utilities from `base.adapter.ts`
- Assertions must use `runAssertion()` from `assertion-runner.ts` — never hand-roll assertion logic
- YAML test files are the primary test definition format; TypeScript DSL is secondary

## Testing

- Run `npm test` before committing
- New adapters require both unit tests (`tests/unit/`) and E2E tests (`tests/e2e/`)
- Test files follow the pattern `TC-<ADAPTER>-001.test.yaml` for E2E tests

## PR Guidelines

- Ensure all checklist items are complete before submitting
- Documentation must be in sync with code changes
- No partial adapter implementations — all checklist items are mandatory
