---
name: autoflow-settings
description: "Interactive configuration of autoflow bootstrap settings (worktree config files, install command, verify command, services command). Writes to .autoflow/bootstrap.json. Supports 'You Decide' option where the agent inspects project source to pick sensible defaults. Triggers on: '/autoflow-settings', 'configure autoflow', 'setup worktree bootstrap'."
argument-hint: "[--reset | --show]"
metadata:
  context: fork
  model: sonnet
---

# Autoflow Settings

$ARGUMENTS

Configures how `autoflow` workflows (autoflow-deliver, worktree bootstrap, future skills) set up an isolated git worktree so the user can run build, test, and dev services without manual steps.

**Config namespace:** All autoflow state lives under `.autoflow/`. This skill writes `.autoflow/bootstrap.json`. It does NOT touch `.planning/` (which is owned by `autoflow-deliver`).

---

## Modes

| Trigger | Mode | Behavior |
|---------|------|----------|
| `/autoflow-settings` (no args) | `interactive` | Ask questions via `AskUserQuestion`, write config |
| `/autoflow-settings --show` | `show` | Print current `.autoflow/bootstrap.json` |
| `/autoflow-settings --reset` | `reset` | Delete `.autoflow/bootstrap.json` and re-run interactive |

---

## Config Location

Writes to `.autoflow/bootstrap.json`:

```json
{
  "language": "go",
  "config_files": [".env"],
  "install_cmd": "go mod download",
  "build_cmd": "make build",
  "verify_cmd": "make compile",
  "test_cmd": "make test",
  "base_branch": "uat",
  "services_cmd": "make up",
  "detected_at": "2026-04-07T12:00:00Z"
}
```

Sibling files under `.autoflow/`:
- `jira-config.json` — Jira connection (managed by `autoflow deliver jira config`)
- `bootstrap.json` — this skill
- Future: other autoflow state

---

## Process

### Step 1: Ensure `.autoflow/` exists

```bash
mkdir -p .autoflow
```

Read current config at `.autoflow/bootstrap.json`. If it already exists and mode is `interactive`, show current values and ask if the user wants to re-configure, keep, or reset.

### Step 2: Detect project (always run — used for "You Decide" defaults)

Inspect the repo root and record findings for later. Do NOT commit to choices yet — this is just recon:

| Signal | What to check | Example finding |
|--------|---------------|-----------------|
| `go.mod` exists | → language = Go | install = `go mod download` |
| `package.json` exists | → language = Node; check `packageManager` field | install = `yarn install --frozen-lockfile` / `npm ci` / `pnpm install --frozen-lockfile` |
| Lockfiles | `yarn.lock`, `pnpm-lock.yaml`, `package-lock.json` | Confirms package manager |
| `Cargo.toml` | → language = Rust | install = `cargo fetch` |
| `requirements.txt` or `pyproject.toml` | → language = Python | install = `pip install -r requirements.txt` or `uv sync` |
| `Makefile` | Scan for `compile:`, `build:`, `up:`, `test:` targets | verify = `make compile`, services = `make up` |
| `docker-compose.yml` / `docker-compose.yaml` | Present | services = `docker compose up -d` |
| `.gitignore` | Scan for lines matching `.env*`, `local.settings*`, `*.local.*`, `config/local.*`, `*.secret` | Candidate config files to copy |
| Common secret files | `.env`, `.env.local`, `local.settings.json`, `config/local.yaml`, `e2e.config.yaml` | Verify existence in main dir |

Store findings in memory — used to populate "You Decide" answers.

### Step 3: Ask questions via `AskUserQuestion`

**Question 1 — Language / Package Manager:**
Options (include "You Decide" as first option with label "You Decide (Recommended)" when detection is confident):
- Go (go mod)
- Node (npm)
- Node (yarn)
- Node (pnpm)
- Python (pip)
- Python (uv)
- Rust (cargo)
- Other / None
- You Decide — agent picks based on detection

**Question 2 — Config files to copy to worktree:**
Multi-select. Only show options that EXIST in the main directory:
- .env
- .env.local
- local.settings.json
- config/local.yaml
- e2e.config.yaml
- None (all committed)
- You Decide — agent picks based on .gitignore scan + file existence

If no candidate files exist, present just "None" and "You Decide".

**Question 3 — Build command (optional):**
- make build
- go build ./...
- yarn build
- npm run build
- cargo build
- Skip (no build)
- You Decide

**Question 4 — Verify command (optional):**
- make compile
- make build
- go vet ./...
- yarn lint
- npm run lint
- cargo clippy
- Skip (no verify)
- You Decide

**Question 5 — Test command (optional):**
- make test
- go test ./...
- yarn test
- npm test
- cargo test
- pytest
- Skip (no test command)
- You Decide

**Question 6 — Services command (optional):**
- make up
- docker compose up -d
- Skip (no services)
- You Decide

**Question 7 — Base branch (for PRs and diffs):**
- uat
- main
- master
- develop
- Custom (ask for branch name)
- You Decide — agent reads CLAUDE.md for "primary branch" hint, else defaults to `main`

**Question 8 — Save as global default?**
- Yes — write to `~/.autoflow/defaults.json`
- No — only this project

### Step 4: Resolve "You Decide" selections

For any answer that was "You Decide", apply detection results:

**Language fallback chain:**
1. `go.mod` → Go
2. `package.json` with `packageManager: "yarn@..."` → Node (yarn)
3. `yarn.lock` → Node (yarn)
4. `pnpm-lock.yaml` → Node (pnpm)
5. `package-lock.json` → Node (npm)
6. `package.json` without lockfile → Node (npm) — default
7. `Cargo.toml` → Rust
8. `pyproject.toml` with `[tool.uv]` → Python (uv)
9. `requirements.txt` → Python (pip)
10. Nothing detected → Other / None (install skipped)

**Install command from language:**
- Go → `go mod download`
- Node (npm) → `npm ci`
- Node (yarn) → `yarn install --frozen-lockfile`
- Node (pnpm) → `pnpm install --frozen-lockfile`
- Python (pip) → `pip install -r requirements.txt`
- Python (uv) → `uv sync`
- Rust → `cargo fetch`
- Other → `""` (empty = skip)

**Config files fallback:**
- Scan `.gitignore` for lines matching `.env*`, `local.settings*`, `*.local.*`, `config/local.*`, `*.secret*`
- Intersect with files that actually exist in main dir
- Return the intersection

**Test command fallback:**
- If `Makefile` has `test:` target → `make test`
- Else per language: Go → `go test ./...`, Node → check `package.json.scripts.test`, Rust → `cargo test`, Python → `pytest`
- Else → Skip

**Build command fallback:**
- If `Makefile` has `build:` target → `make build`
- Else per language: Go → `go build ./...`, Node → check `package.json.scripts.build`, Rust → `cargo build`
- Else → Skip

**Verify command fallback:**
- If `Makefile` has `compile:` target → `make compile`
- Else per language: Go → `go vet ./...`, Node → check `package.json.scripts.lint`, Rust → `cargo clippy`
- Else → Skip

**Services command fallback:**
- If `Makefile` has `up:` target → `make up`
- Else if `docker-compose.yml` or `docker-compose.yaml` exists → `docker compose up -d`
- Else → Skip

**Base branch fallback:**
- Read CLAUDE.md for "primary branch is **`<name>`**" pattern → use `<name>`
- Else if `origin/uat` exists → `uat`
- Else if `origin/develop` exists → `develop`
- Else → `main`

### Step 5: Write config

Build the bootstrap config object and write atomically to `.autoflow/bootstrap.json`:

```bash
mkdir -p .autoflow
cat > .autoflow/bootstrap.json.tmp <<EOF
{
  "language": "<resolved>",
  "config_files": [<resolved array>],
  "install_cmd": "<resolved>",
  "build_cmd": "<resolved or empty>",
  "verify_cmd": "<resolved or empty>",
  "test_cmd": "<resolved or empty>",
  "base_branch": "<resolved or main>",
  "services_cmd": "<resolved or empty>",
  "detected_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
mv .autoflow/bootstrap.json.tmp .autoflow/bootstrap.json
```

Prefer using the Write tool to emit valid JSON directly rather than bash heredocs.

### Step 6: Save as global default (if requested)

```bash
mkdir -p ~/.autoflow
cp .autoflow/bootstrap.json ~/.autoflow/defaults.json
```

### Step 7: Confirm

Display:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 AUTOFLOW ► BOOTSTRAP SETTINGS UPDATED
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

| Setting       | Value |
|---------------|-------|
| Language      | <value> |
| Config Files  | <comma-separated or "none"> |
| Install       | <cmd or "skip"> |
| Build         | <cmd or "skip"> |
| Verify        | <cmd or "skip"> |
| Test          | <cmd or "skip"> |
| Base Branch   | <branch name> |
| Services      | <cmd or "skip"> |
| Saved Global  | <Yes/No> |

Saved to: .autoflow/bootstrap.json

Used by:
- `autoflow deliver worktree bootstrap` (command)
- autoflow-deliver (Step 2: worktree creation)

To re-configure: /autoflow-settings
To view: /autoflow-settings --show
```

---

## Mode: show

Read `.autoflow/bootstrap.json`, pretty-print. If missing, print a message and suggest running `/autoflow-settings`.

---

## Mode: reset

1. Delete `.autoflow/bootstrap.json` (if exists)
2. Re-run interactive mode

---

## Global Defaults

When `.autoflow/bootstrap.json` does not exist, check `~/.autoflow/defaults.json` first and use those as pre-selected values in the questions. This lets users configure once per machine and skip the interview on new projects with similar setups.

---

## Success Criteria

- [ ] `.autoflow/bootstrap.json` exists and is valid JSON
- [ ] Contains all 9 fields (language, config_files, install_cmd, build_cmd, verify_cmd, test_cmd, base_branch, services_cmd, detected_at)
- [ ] `detected_at` is a valid ISO 8601 timestamp
- [ ] If user chose "You Decide", resolved values match detection heuristics
- [ ] If user saved as global default, `~/.autoflow/defaults.json` exists
- [ ] `.planning/config.json` was NOT touched
- [ ] Final summary displayed
