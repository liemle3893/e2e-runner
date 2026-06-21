# Autoflow Port — Design Doc

Port `winx-autoflow` (~4,164 LOC of bash + Python) into `tryve` as native Go
subcommands, so a single binary delivers both the test runner and the
Jira-to-PR workflow. No external runtime deps (`bash`, `jq`, `curl`,
`python3`, `flock`/`lockf`) except for `git` and `gh`.

Source repo to port from: `/Users/liemlhd/Documents/git/masan/crownx/winx/winx-autoflow`.
Target repo: this one (`e2e-runner` → distributed as `tryve`).

DoD (from user): "fully functional and replaceable. single repo for all the
work." Validation: end-to-end run against `WINX-1..WINX-20` (real or fake).

---

## 1. Source Inventory

### 1.1 Scripts being ported

| Source file | LOC | Target Go package | User-facing CLI? |
|---|---:|---|---|
| `scripts/autoflow/jira-config.sh` | 92 | `internal/autoflow/jira` (config.go) | Yes — `tryve autoflow jira config ...` |
| `scripts/autoflow/jira-env.sh` | 29 | `internal/autoflow/jira` (env.go) | No (helper) |
| `scripts/autoflow/jira-upload.sh` | 115 | `internal/autoflow/jira` (upload.go) | Yes — `tryve autoflow jira upload` |
| `scripts/autoflow/jira-download.sh` | 134 | `internal/autoflow/jira` (download.go) | Yes — `tryve autoflow jira download` |
| `scripts/autoflow/worktree-bootstrap.sh` | 260 | `internal/autoflow/worktree` | Yes — `tryve autoflow worktree bootstrap` |
| `skills/autoflow-deliver/scripts/step-controller.py` | 1547 | `internal/autoflow/deliver` | Yes — `tryve autoflow deliver {next,complete,init}` |
| `.../progress-state.sh` | 290 | `internal/autoflow/state` (progress.go) | No (internal) |
| `.../loop-state.sh` | 174 | `internal/autoflow/state` (loop.go) | Yes — `tryve autoflow loop-state ...` (agents call it) |
| `.../review-loop.sh` | 367 | `internal/autoflow/state` (review.go) | No (internal — called by deliver) |
| `.../e2e-local.sh` | 349 | `internal/autoflow/e2e` (local.go) | No (internal) |
| `.../e2e-loop.sh` | 239 | `internal/autoflow/e2e` (loop.go) | No (internal — called by deliver step 7) |
| `.../generate-report.sh` | 769 | `internal/autoflow/report` | No (internal — called by deliver step 12) |
| `.../scaffold-e2e.sh` | 158 | `internal/autoflow/scaffold` | Yes — `tryve autoflow scaffold-e2e` (test-writer agent calls it) |
| `.../verify-gates.sh` | 64 | `internal/autoflow/state` (verify.go) | No (internal) |
| `.../extract-round-data.sh` | 247 | `internal/autoflow/extract` | No (internal) |
| **Total** | **4,834** | | |

### 1.2 Skills + agents being vendored (embedded only, not rewritten as code)

- **Skills** (9): `autoflow-code-review`, `autoflow-deep-review`, `autoflow-deliver`,
  `autoflow-design-sync`, `autoflow-docs-init`, `autoflow-local-merge`,
  `autoflow-settings`, `autoflow-simplify`, `autoflow-ticket`
- **Agents** (14): `autoflow-{ac-reviewer, code-fixer, code-reviewer, codebase-mapper,
  design-syncer, docs-writer, e2e-enhancer, executor, jira-fetcher, planner,
  review-composite, rules-enforcer, simplify-reviewer, test-writer}`

These are embedded via `embed.FS` and dropped into the target project's
`.claude/{skills,agents}/` by `tryve install --autoflow`.

### 1.3 Not being ported

- `harness/runtime/workflow-runtime.py` (42 KB) — excluded per earlier scope
- `install.sh` — replaced by `tryve install --autoflow`
- `scripts/autoflow/` standalone distribution — superseded

---

## 2. CLI Surface

Only commands the skills/agents call externally are exposed. Everything else
is internal Go packages called from `deliver`.

```
tryve autoflow
├── jira
│   ├── config
│   │   ├── set   --cloud-id X --site-url Y --project-key Z [--email E]
│   │   ├── get   --field {cloudId|siteUrl|projectKey|email}
│   │   ├── del   [--field <name>]        # no flag → delete whole config
│   │   └── show                           # prints cached JSON
│   ├── upload <issue-key> <file>...
│   └── download <issue-key> <output-dir>
├── worktree
│   └── bootstrap <worktree-path>
├── deliver
│   ├── init --ticket KEY --worktree PATH --branch BRANCH
│   ├── next --ticket KEY
│   └── complete --ticket KEY [--title ...] [--pr-url ...]
├── loop-state
│   ├── init <state-file> --loop NAME --ticket KEY --max-rounds N [--force]
│   ├── append <state-file> --round-json JSON
│   ├── read <state-file>
│   └── round-count <state-file>
├── scaffold-e2e --ticket KEY --area AREA --count N
└── doctor                                  # preflight health check
```

**`tryve autoflow doctor` — preflight check.** Prevents the common failure
mode where agents hunt for `JIRA_API_TOKEN` endlessly when the user forgot
to export it. Runs a fixed checklist and prints pass/fail per item:

| Check | Pass criteria |
|---|---|
| `git` on PATH | `git --version` exits 0 |
| `gh` on PATH + authed | `gh auth status` exits 0 |
| `JIRA_API_TOKEN` env var set | non-empty |
| `.autoflow/jira-config.json` present + valid | file exists, parses, has `cloudId`+`siteUrl`+`projectKey`+`email` |
| Jira reachable | `HEAD https://<site>/rest/api/3/myself` with cached creds → 200 |
| `.autoflow/bootstrap.json` present | warn if missing (suggest `/autoflow-settings`) |
| Skills installed | `.claude/skills/autoflow-deliver/SKILL.md` exists |
| Agents installed | `.claude/agents/autoflow-jira-fetcher.md` exists |
| No stale `.claude/scripts/autoflow/` dir | warn if present (old winx-autoflow install) |

Exit codes: 0 = all pass, 1 = one or more fail, 2 = warnings only.
Output is table-formatted and machine-parseable (one line per check, `OK`/`FAIL`/`WARN` prefix).

**Install command extension:**
```
tryve install --autoflow
```
Drops skills + agents into `.claude/{skills,agents}/`. Coexists with the
existing `--skills` flag.

---

## 3. Go Package Layout

Library-first: all logic lives in `internal/autoflow/*` as plain Go packages.
CLI under `internal/cli/autoflow_*.go` is a thin Cobra wrapper.

```
internal/autoflow/
├── jira/
│   ├── config.go        # .autoflow/jira-config.json read/write
│   ├── env.go           # JIRA_SITE/JIRA_EMAIL resolver (from cache + env)
│   ├── client.go        # HTTP client w/ Basic auth (Jira Cloud REST v3)
│   ├── upload.go        # multipart upload
│   └── download.go      # fetch attachments
├── worktree/
│   ├── bootstrap.go     # orchestration: copy .claude, copy configs, install, verify
│   ├── config.go        # .autoflow/bootstrap.json reader + auto-detect
│   └── safecmd.go       # allowlist + interactive prompt for unknown binaries
├── state/
│   ├── paths.go         # .planning/ticket/<KEY>/{state,attachments,...}
│   ├── progress.go      # workflow-progress.json (init/start/complete/set/read/get)
│   ├── loop.go          # generic loop-state.json (init/append/read/round-count)
│   ├── review.go        # code-review-state.json + feedback.json + sha256 integrity
│   ├── verify.go        # structural validation (verify-gates.sh equivalent)
│   └── atomic.go        # atomic JSON writer (tmp + rename)
├── deliver/
│   ├── controller.go    # next/complete/init entry points
│   ├── instruction.go   # 6 action types as Go structs + JSON marshalling
│   ├── steps.go         # step_01..step_13 as Step funcs
│   ├── brief.go         # task-brief.md metadata parser (frontmatter + bare)
│   ├── gate.go          # _gate-result helper (build gate state writer)
│   ├── resolve.go       # path/script resolvers (no longer needed — all Go)
│   └── ticket.go        # ticket-key validation (PROJ-123 regex)
├── e2e/
│   ├── local.go         # git merge + env loading + tryve.Run() + cleanup
│   ├── loop.go          # round tracking wrapper over local
│   ├── env.go           # .env + local.settings.json loader
│   ├── lock.go          # cross-platform file lock (flock syscall on unix)
│   └── parse.go         # NOT NEEDED — we call tryve with --reporter=json
├── report/
│   ├── generate.go      # PR-BODY.md, JIRA-COMMENT.md, EXECUTION-REPORT.md
│   ├── state.go         # loop-summary extraction
│   ├── e2ejson.go       # consume tryve JSON reporter output (no regex parsing)
│   ├── summary.go       # SUMMARY.md parser (created:/modified: sections)
│   └── templates.go     # embedded md templates
├── extract/
│   └── review.go        # REVIEW-*.md frontmatter + finding-heading parser
├── scaffold/
│   └── e2e.go           # YAML stub generator (embedded template)
└── doctor/
    ├── check.go         # Check interface + aggregate runner
    ├── checks_env.go    # git, gh, JIRA_API_TOKEN
    ├── checks_config.go # jira-config.json, bootstrap.json
    ├── checks_net.go    # Jira reachability ping
    └── checks_install.go # skills/agents present, stale scripts/ warning

internal/cli/
├── autoflow.go          # root cobra command
├── autoflow_jira.go     # jira subtree
├── autoflow_worktree.go
├── autoflow_deliver.go
├── autoflow_loopstate.go
├── autoflow_scaffold.go
└── autoflow_doctor.go

# At repo root
embed.go                  # existing — extended to embed skills/autoflow + agents
skills/autoflow/...       # vendored from winx-autoflow/skills/ (9 skills)
agents/autoflow/...       # vendored from winx-autoflow/agents/ (14 agents)
```

**Key design choices:**

- **CLI is a thin wrapper.** Every `autoflow_*.go` in `internal/cli/` ≤ 100
  LOC. Logic lives in the `internal/autoflow/*` packages so step-controller's
  internal calls share the exact same code path as the CLI. No duplication.
- **No more path resolution hacks.** step-controller.py has `skill_script()`
  / `autoflow_script()` to locate bash files at either `.claude/skills/...`
  or `skills/...`. Go port has no scripts to locate — all logic is in-binary.
- **tryve integration.** `internal/autoflow/e2e/local.go` imports `pkg/runner`
  (tryve's own programmatic API) directly instead of shelling out to `tryve
  run`. Faster, no subprocess, no output regex parsing.
- **Reports stop regex-parsing tryve output.** `generate-report.sh` does ~200
  LOC of `grep -oE 'Running [0-9]+ test\(s\)'` and friends on tryve's console
  output. The Go port uses tryve's JSON reporter, so we consume structured
  data. Delete ~200 LOC of fragile parsing.

---

## 4. Data Contracts (must preserve for interop)

State files written by the Go port must be byte-compatible with the existing
bash scripts so partial installs (old winx-autoflow + new tryve autoflow)
don't corrupt each other's state.

| File | Schema | Writer | Reader |
|---|---|---|---|
| `.autoflow/jira-config.json` | `{cloudId, siteUrl, projectKey, email, cached_at}` | `jira config write` | `jira config read/cloudid/...`, `jira-env.sh` equivalent |
| `.autoflow/bootstrap.json` | `{language, base_branch, config_files[], install_cmd, verify_cmd, build_cmd, test_cmd, services_cmd}` | `autoflow-settings` skill | `worktree bootstrap`, step-6 gate, step-12 reports |
| `.planning/ticket/<KEY>/task-brief.md` | markdown + YAML frontmatter | `autoflow-jira-fetcher` agent | step-controller steps 1, 5, 10, 11 |
| `.planning/ticket/<KEY>/title.txt` | plain text (step-1 → step-2 sidecar) | `deliver complete --title` pre-init | `deliver next` step_02 when no progress |
| `.planning/ticket/<KEY>/workflow-progress.json` | `{ticket, started_at, worktree, branch, current_step, completed[], pr_url, gsd_quick_id, impl_plan_dir, title}` | progress-state | all step fns + reports |
| `.planning/ticket/<KEY>/state/coverage-review-state.json` | `{loop, ticket, max_rounds, rounds: [{round, timestamp, status, problems, fixes}]}` | loop-state append | step_04, reports |
| `.planning/ticket/<KEY>/state/e2e-fix-state.json` | same schema + `output_file` per round | e2e-loop | step_07, reports |
| `.planning/ticket/<KEY>/state/build-gate-state.json` | `{attempt, last_result, error_file, fix_dispatched}` | `_gate-result` internal | step_06 |
| `.planning/ticket/<KEY>/state/code-review-state.json` + `review-feedback.json` + `.review-state-checksum` | append-only with sha256 integrity | review-loop | step_09 (via extract) |
| `.planning/ticket/<KEY>/state/REVIEW-{code,simplify,rules,FIX}.md` | markdown with YAML frontmatter `{status, findings: {critical, warning, info}}` | reviewer agents | step_09 + extract-round-data |
| `.planning/ticket/<KEY>/state/*.marker` | empty files as sentinels | step functions | step functions (idempotency) |
| `.planning/ticket/<KEY>/state/e2e-run-counter.txt` | decimal integer | step_07 bash | step_07 stale-guard |
| `.planning/ticket/<KEY>/{PR-BODY,JIRA-COMMENT,EXECUTION-REPORT}.md` | markdown | `generate-report` | user, `gh pr edit` |

**All JSON writers use atomic tmp+rename**, matching the bash pattern, so
reader never sees partial state.

**All file modes match**: state files `0644`, directories `0755`.

---

## 5. Migration of SKILL.md + agents

When vendored into this repo, the files get rewritten to call `tryve autoflow
...` instead of shelling into `.claude/scripts/autoflow/*.sh` or
`python3 step-controller.py`. The vendored copies are the new source of
truth.

### SKILL.md rewrites

| File | Old | New |
|---|---|---|
| `autoflow-ticket/SKILL.md:34` | `` `.claude/scripts/autoflow/jira-config.sh cloudid` `` | `` `tryve autoflow jira config cloudid` `` |
| `autoflow-ticket/SKILL.md:37` | `.claude/scripts/autoflow/jira-config.sh write ...` | `tryve autoflow jira config write ...` |
| `autoflow-ticket/SKILL.md:40` | `required by jira-upload.sh and jira-download.sh` | `required by \`tryve autoflow jira upload\` and \`... download\`` |
| `autoflow-ticket/SKILL.md:213` | `.claude/scripts/autoflow/jira-upload.sh` | `tryve autoflow jira upload` |
| `autoflow-settings/SKILL.md:49` | `managed by jira-config.sh` | `managed by \`tryve autoflow jira config\`` |
| `autoflow-settings/SKILL.md:268` | `- .claude/scripts/autoflow/worktree-bootstrap.sh` | `- (removed — worktree bootstrap is \`tryve autoflow worktree bootstrap\`)` |
| `autoflow-deliver/SKILL.md:59` | `python3 "$CTRL" next --ticket <KEY>` | `tryve autoflow deliver next --ticket <KEY>` |
| `autoflow-deliver/RESUME.md:14,32` | `step-controller.py next` | `tryve autoflow deliver next` |
| `autoflow-deliver/references/directory-contract.md` | references to `loop-state.sh`, `e2e-loop.sh`, `generate-report.sh`, `step-controller.py` | `tryve autoflow ...` equivalents |

### Agent rewrites

| File | Old | New |
|---|---|---|
| `autoflow-jira-fetcher.md:17` | `.claude/scripts/autoflow/jira-config.sh cloudid` | `tryve autoflow jira config cloudid` |
| `autoflow-jira-fetcher.md:34` | `.claude/scripts/autoflow/jira-download.sh <KEY> <DIR>` | `tryve autoflow jira download <KEY> <DIR>` |
| `autoflow-ac-reviewer.md:31,44` | `.claude/skills/autoflow-deliver/scripts/loop-state.sh append ...` | `tryve autoflow loop-state append ...` |
| `autoflow-e2e-enhancer.md:34,53` | same | same |
| `autoflow-test-writer.md:28,40` | `.claude/skills/autoflow-deliver/scripts/scaffold-e2e.sh` | `tryve autoflow scaffold-e2e` |
| `autoflow-{planner,executor,docs-writer,test-writer}.md` "DO NOT CALL progress-state.sh" | (drop the path — just say "workflow progress is the orchestrator's responsibility") | same |

No agent spawns the full `deliver` chain, so none need to know about
`tryve autoflow deliver`. That's the orchestrator's (skill's) job.

---

## 6. Install Command

Extend `internal/cli/install.go`:

```
tryve install --autoflow                  # new flag (in addition to --skills)
tryve install --skills --autoflow          # both
```

Behaviour with `--autoflow`:
1. Copy `skills/autoflow/*` → `<cwd>/.claude/skills/autoflow-*/`
2. Copy `agents/autoflow/*` → `<cwd>/.claude/agents/autoflow-*.md`
3. Ensure `<cwd>/.gitignore` has the `autoflow-*` pattern (idempotent append)
4. **Auto-clean legacy layout** — if `<cwd>/.claude/scripts/autoflow/` exists,
   remove it (old winx-autoflow `install.sh` wrote bash scripts there; they
   are dead in the Go port). Print what was removed.
5. Do NOT copy shell scripts — they no longer exist; all logic is in the
   `tryve` binary.

---

## 7. Parity Test Plan

Validation bar the user agreed to: **fully run** a real or fake ticket
through all 13 steps, end-to-end.

### 7.1 Tier 1 — unit tests (blocking)

One test file per package, covering the bash script's observable behaviour:

| Package | Critical tests |
|---|---|
| `jira/config` | write + read round-trip; missing file; email omitted; malformed JSON rejected |
| `jira/client` | basic-auth header; multipart upload body; error on non-200; path-traversal rejected |
| `worktree/bootstrap` | config parse; auto-detect fallbacks; allowlist acceptance; unknown binary prompt (non-interactive → skip); worktree == main-dir rejection |
| `state/progress` | init idempotency; complete advances past consecutive completed steps; field whitelist; atomic write integrity |
| `state/loop` | init refuses overwrite without --force; append increments round; max-rounds enforcement; required `status` field |
| `state/review` | append-only integrity (mutation detection via sha256); round numbering; file-path inputs |
| `deliver/brief` | YAML frontmatter; bare `key: value` after `# Title`; missing file → {} |
| `deliver/steps` | each of 13 steps with representative progress dicts; escalate paths; auto_complete paths |
| `e2e/env` | .env precedence; local.settings.json Values.* import; non-POSIX key rejection; NUL-delimited parsing |
| `e2e/local` | lock acquisition; merge rollback on exit; --no-lock path |
| `report/generate` | 3 reports from a fixture state tree; empty state dir; legacy vs new layout |
| `extract/review` | CR/RLW/SMC prefix→severity map; disposition extraction; missing files → empty |
| `scaffold/e2e` | start number inference; collision skip; padded numbering |

### 7.2 Tier 2 — golden-file parity (blocking for `deliver`)

For `deliver next`, record Python output on a matrix of progress states,
run Go against same inputs, assert JSON equality (after key sorting).

Fixture seed:
```
tests/autoflow/fixtures/
├── step-01-fresh/            # no progress, no brief
├── step-01-brief-exists/     # brief present, sidecar title.txt missing
├── step-02-worktree-missing/
├── step-02-worktree-exists/  # should auto_complete
├── step-04-no-state/         # → bash init
├── step-04-round-1-gaps/     # → dispatch ac-reviewer round 2
├── step-04-round-1-pass/     # → auto_complete
├── step-04-round-3-exhausted/
├── step-05-path-A/
├── step-05-path-B-no-plan/
├── step-05-path-B-plan-exists/
├── step-06-first-run/
├── step-06-failed-needs-fixer/
├── step-06-max-attempts/
├── step-07-round-1-fresh/
├── step-07-last-failed-dispatch-fixer/
├── step-07-last-failed-fix-failed-marker/
├── step-07-passed/
├── step-09-phase-1-nothing/    # dispatch 3 reviewers
├── step-09-phase-2-all-reviews-clean/
├── step-09-phase-2-findings/   # dispatch fixer
├── step-09-phase-3-fixed/
├── step-11-first-run/
├── step-12/
├── step-13/
└── done/
```

Each fixture is a tree (`{progress.json, task-brief.md, state/*}`), a matching
`expected-next.json`, and a matching `expected-complete.json`. Test runs:
```
python3 step-controller.py next --ticket WINX-1   # generates expected
tryve autoflow deliver next --ticket WINX-1       # must match
```

Driver is a Go test that calls both, normalises (strip timestamps, resolve
absolute paths to relative), and diffs.

### 7.3 Tier 3 — real-ticket smoke test (blocking DoD)

User runs `tryve autoflow deliver WINX-1` (or fake ticket equivalent) and
validates:
- [ ] Jira fetch succeeds (via jira-fetcher agent + `tryve autoflow jira config/download`)
- [ ] Worktree created + bootstrapped
- [ ] E2E tests written by test-writer + `tryve autoflow scaffold-e2e`
- [ ] AC coverage loop runs (`tryve autoflow loop-state` from agent)
- [ ] Implementation path chosen correctly from brief metadata
- [ ] Build gate passes (or dispatches fixer correctly)
- [ ] E2E tests run via `tryve` directly (no subprocess hop)
- [ ] Review triad (code/simplify/rules) dispatched in parallel
- [ ] PR created via `gh`
- [ ] Reports generated at ticket dir
- [ ] Jira updated with EXECUTION-REPORT attached via `tryve autoflow jira upload`

Test env: user supplies `JIRA_API_TOKEN`. Can target a throwaway sandbox
project or dry-run the Jira calls by pointing at a mock server.

### 7.4 What's explicitly NOT covered by automated tests

- Agent behaviour itself (the SKILL.md / agent prompts) — that's LLM turf,
  covered by the Tier 3 smoke run only
- Atlassian MCP interactions — external, covered by Tier 3
- `gh pr create` — external, covered by Tier 3
- Platform-specific lock (`flock` vs `lockf`) — port uses a single Go
  implementation, so no platform branch exists

---

## 8. Known Risks / Behavioural Deltas

Places where the Go port CAN'T be a byte-identical substitute. Called out
explicitly so they're not discovered mid-implementation.

1. **E2E output parsing.** `generate-report.sh` regex-greps tryve console
   output. Go port uses tryve's `--reporter=json`. Net behaviour identical,
   but log files under `/tmp/e2e-results-*.txt` now contain JSON instead of
   ansi-coloured text. Users who eyeball those logs will see a change.
2. **Error file format.** `_gate-result` writes tail-100 lines of the gate
   log. Go port preserves this. No delta expected.
3. **stdout/stderr formatting.** Status banners (`══════ ═══`) are
   preserved for familiarity.
4. **Concurrency.** bash scripts use `set -euo pipefail` + read-modify-write
   JSON. Go port uses the same pattern (single-writer, atomic rename). No
   file locking added — the existing scripts don't lock either.
5. **`run_safe_cmd` allowlist.** Preserved. User prompts on unknown binaries
   only in interactive TTY; non-interactive skips (matches bash behaviour).
6. **Path resolution.** `skill_script()` / `autoflow_script()` dual-lookup
   (`.claude/skills/...` or `skills/...`) is gone — all logic is in-binary.
   If an agent has hardcoded a path expecting one of those locations, it
   breaks. Covered by the agent rewrite list.
7. **Python return shape for `complete`.** Python currently prints:
   ```
   {"completed_step": N, "next_step": M}
   ```
   Go port prints the same JSON. Tested by Tier 2.
8. **`step_01` pre-init path.** If `task-brief.md` exists but
   `workflow-progress.json` does not, `complete --title` writes
   `title.txt` sidecar and `next` synthesises a step_02 instruction. Python
   handles this explicitly (cmd_next + cmd_complete); Go port must replicate.
   Fixture `step-01-brief-exists` covers it.

---

## 9. Implementation Plan

Breakdown once approved. Each sub-task a separate task in `TaskList`,
implemented behind a single feature branch. Target: commit per package +
CLI wrapper, merged to `main` only when Tier 1 + Tier 2 pass.

Approximate ordering (dependency-driven):

| # | Sub-task | Deps | Est LOC |
|---|---|---|---|
| 1 | `state/*` — paths, progress, loop, review, verify, atomic | — | ~800 |
| 2 | `jira/*` — config, env, client, upload, download | state/paths | ~500 |
| 3 | `worktree/*` — bootstrap, config, safecmd | state/paths | ~400 |
| 4 | `scaffold/*`, `extract/*`, `doctor/*` | state, jira | ~700 |
| 5 | `e2e/*` — env, lock, local, loop (+ reuse `pkg/runner`) | state | ~600 |
| 6 | `report/*` — templates, state, e2ejson, summary, generate | state, e2e | ~900 |
| 7 | `deliver/*` — instruction types, steps, brief, gate, controller | all above | ~1500 |
| 8 | CLI wrappers (`internal/cli/autoflow_*.go`) | all above | ~500 |
| 9 | Vendor skills + agents under `skills/autoflow/` + `agents/autoflow/` | — | 0 (copy) |
| 10 | Rewrite SKILL.md + agent path references | 9 | 0 (sed) |
| 11 | Extend `embed.go` + `install.go` for `--autoflow` | 9, 10 | ~100 |
| 12 | Tier 1 unit tests | per package | ~1500 |
| 13 | Tier 2 golden fixtures + diff driver | 7 | ~400 |
| 14 | Tier 3 real-ticket run (user-executed) | all | 0 |
| 15 | Update `CLAUDE.md` + `README.md` | all | ~100 |

**Expected total:** ~7,650 Go LOC (source) + ~1,900 test LOC.

Commits land continuously; no big-bang merge.

---

## 10. Resolved Decisions

All five open questions answered:

1. **Install flag** — `tryve install --autoflow` (flag form, next to `--skills`).
2. **`tryve autoflow` is a separate root subcommand tree** — confirmed.
3. **Old `.claude/scripts/autoflow/` dir** — `tryve install --autoflow` auto-deletes
   it as part of migration. Deletion is idempotent; prints what it removed.
4. **Skill sidecars** — keep `RESUME.md` and `SKILL-v5.md` as-is when vendoring.
5. **Tier 3 smoke test** — user provides `JIRA_API_TOKEN`; real Jira ticket.
   No mock harness needed. Plus: ship `tryve autoflow doctor` (see §2) so
   the "endless token search" failure surfaces immediately as a FAIL line
   instead of a looping agent.

**Jira config CLI revised** — user preferred verb-style with flags rather
than positional args. Updated §2 accordingly:
`set --cloud-id X --site-url Y --project-key Z [--email E]`,
`get --field F`, `del [--field F]`, `show`.
