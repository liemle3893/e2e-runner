# Directory Contract & State Layout

Reference loaded on-demand by `autoflow-deliver/SKILL.md`.

## Two-Root Contract

Subagents inherit no cwd assumptions. The orchestrator resolves and passes these two absolute paths to every `autoflow-*` dispatch after Step 2:

```bash
REPO_ROOT=$(git rev-parse --show-toplevel)                # main repo
WORKTREE_DIR=$(cd ../<repo>-<ticket-key> && pwd)          # feature worktree
```

### Why the split matters

| Location | Contains | Why |
|----------|----------|-----|
| `REPO_ROOT/.autoflow/ticket/<KEY>/` | Task brief, state files, feedback ledger, progress file | Must survive worktree create/remove; readable by orchestrator between dispatches |
| `WORKTREE_DIR/src/`, `WORKTREE_DIR/tests/e2e/` | Source code and test files on the feature branch | REPO_ROOT is on the base branch — writing source there corrupts the wrong tree |

### Command routing

- `git diff origin/${BASE_BRANCH}...HEAD` — MUST run from `WORKTREE_DIR` (diffs the wrong branch from REPO_ROOT)
- `autoflow deliver loop-state` — MUST run from `REPO_ROOT` (finds state files in `.autoflow/ticket/<KEY>/state/`)
- `BASE_BRANCH` — read from `.autoflow/bootstrap.json`

### Step 1 exception

`autoflow-jira-fetcher` runs BEFORE the worktree exists. It receives only `REPO_ROOT`.

## State Directory Layout

Per-ticket artifacts live under `.autoflow/ticket/<TICKET-KEY>/`:

```
.autoflow/ticket/PROJ-42/
|-- attachments/                   # Step 1: downloaded from Jira
|-- task-brief.md                  # Step 1: filled template
|-- title.txt                      # Step 1: extracted title (sidecar, pre-init)
|-- workflow-progress.json         # All steps: resume tracking
|-- PLAN.md                        # Step 5: implementation plan (Path B)
|-- SUMMARY.md                     # Step 5: execution summary
|-- IMPL-SUMMARY.md               # Step 10: copy of implementation summary
|-- PR-BODY.md                     # Step 12: concise PR description
|-- JIRA-COMMENT.md                # Step 12: detailed Jira comment
|-- EXECUTION-REPORT.md            # Step 12: full execution report artifact
+-- state/                              # All loop/review state files
    |-- coverage-review-state.json      # Step 4: AC coverage loop state
    |-- build-gate-state.json           # Step 6: build gate attempt/result
    |-- build-gate-log-N.log            # Step 6: build error output per attempt
    |-- e2e-fix-state.json              # Step 7: E2E fix loop (written by `autoflow deliver _e2e-round`, run from worktree)
    |-- e2e-run-counter.txt             # Step 7: stale-state guard counter
    |-- e2e-fix-dispatched-round-N.marker  # Step 7: fixer dispatch tracking
    |-- REVIEW-code.md                  # Step 9: code reviewer findings
    |-- REVIEW-simplify.md              # Step 9: simplify reviewer findings
    |-- REVIEW-rules.md                 # Step 9: rules enforcer findings
    +-- REVIEW-FIX.md                   # Step 9: code fixer report
```

All state is written under `.autoflow/ticket/<KEY>/state/` by the `autoflow deliver` subcommands. Paths are the same whether the workflow runs from REPO_ROOT or the worktree.
