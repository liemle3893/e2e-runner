# Autoflow CLI Command Reorganization

**Date:** 2026-05-23
**Status:** Draft for review
**Owner:** @liemlhd

## Problem

The current `autoflow` top-level command tree is a flat dump that mixes two
distinct products: the YAML-driven E2E test runner (`e2e`) and the Jira-to-PR
delivery workflow (everything else). Concrete pain points:

- `autoflow --help` reads as a single product surface, not two. New users (and
  authors of skill/agent prompts) can't tell where each command belongs.
- `scaffold-e2e` is misfiled — it's an `e2e` operation but sits at the top
  level next to unrelated workflow commands.
- Top-level `config` (manages `.autoflow/config.json`) collides conceptually
  with the global `--config` flag (which points at `e2e.config.yaml`). Two
  different files, same word.
- Workflow primitives (`jira`, `worktree`, `sandbox`, `loop-state`, `doctor`,
  `config`) all support the delivery workflow but sit flat next to `e2e`.
- Skill/agent prompts are inconsistent: some reach into top-level groups
  (`autoflow jira fetch`), others go through `deliver` (`autoflow deliver
  next`), with no predictable rule.

## Goals

1. Split the help output into two visible products: `e2e` and `deliver`.
2. Make agent skill prompts predictable — workflow stuff always lives under
   `deliver`; tests always live under `e2e`.
3. Eliminate the `config` collision and the misfiled `scaffold-e2e`.

## Non-goals

- Renaming the binary. It stays `autoflow`.
- Renaming the workflow verbs (`init` / `next` / `complete` keep their
  spelling — minimum churn for agents and muscle memory).
- Adding backwards-compatible aliases. This is a hard cut in one PR.
- Reorganizing `e2e`'s internal subtree (already coherent).
- Promoting `e2e` itself to a separate binary. Single binary is fine.

## Target shape

```
autoflow
├── e2e
│   ├── run | validate | list | health | init | doc
│   ├── test {create, list-templates}
│   └── scaffold                       # MOVED from top-level scaffold-e2e
├── deliver                             # promoted from single command to umbrella
│   ├── init | next | complete | timings
│   ├── doctor                         # MOVED from top-level
│   ├── config {set,get,show,del}      # MOVED from top-level (.autoflow/config.json)
│   ├── jira {fetch,search,transitions,transition,upload,download,config}
│   ├── worktree bootstrap
│   ├── sandbox {bootstrap,status}
│   └── loop-state {init,append,read,round-count}
├── install
├── version
└── completion
```

### Why `deliver` as the umbrella

- It's already canonical vocabulary: the agents are named
  `autoflow-deliver*`, the skill is `autoflow-deliver`, and the existing
  subcommand is `deliver`. Zero new words.
- Promoting it from "single command with `init/next/complete`" to "umbrella"
  is a low-cost change — those three subcommands keep their paths
  (`deliver init`, `deliver next`, `deliver complete`).
- Verb-as-group is normal in CLI tools (`docker compose up`, `git submodule
  add`). The mild oddness of `autoflow deliver jira upload` is offset by the
  consistency win.
- Alternatives considered:
  - `flow` — `autoflow flow` reads redundant with the binary name.
  - `work`, `ticket`, `pipeline` — generic, opinionated, or misleading.
  - Workflow-first binary (root verbs `start/next/complete` + `e2e` nested
    deeper) — asymmetric; punishes e2e-only users to save one word for
    workflow agents.

### Decisions locked

- **Umbrella name:** `deliver`.
- **Verbs:** keep `init` / `next` / `complete` / `timings` as-is. No new
  subcommands added by this reorg.
- **Aliases:** none. Hard cut in one PR. Anything calling the old paths
  breaks loudly, which is fine — the call sites are inside this repo.
- **Scope:** binary command tree + all in-repo references. No external
  scripts to worry about.

## Migration map

Every command move is one-to-one. No renames inside groups, no behavior
changes — only re-parenting.

| Old path                         | New path                                 |
|----------------------------------|------------------------------------------|
| `autoflow scaffold-e2e ...`      | `autoflow e2e scaffold ...`              |
| `autoflow doctor`                | `autoflow deliver doctor`                |
| `autoflow config {set,get,show,del}` | `autoflow deliver config {set,get,show,del}` |
| `autoflow jira ...`              | `autoflow deliver jira ...`              |
| `autoflow worktree ...`          | `autoflow deliver worktree ...`          |
| `autoflow sandbox ...`           | `autoflow deliver sandbox ...`           |
| `autoflow loop-state ...`        | `autoflow deliver loop-state ...`        |
| `autoflow deliver init|next|complete|timings` | unchanged                  |
| `autoflow e2e ...`               | unchanged                                |
| `autoflow install`               | unchanged                                |
| `autoflow version`               | unchanged                                |

Internal `_underscore` subcommands on `deliver` (`_gate-result`, `_set-field`,
`_complete-step`, `_e2e-round`, `_report`, `_commit-task`, `_verify-gates`)
stay where they are. They're already correctly nested.

## Implementation outline

The work is mostly cobra re-parenting plus text edits. Two layers:

### 1. Cobra wiring (small)

In `internal/cli/`:

- `root.go` — stop registering `jira`, `worktree`, `sandbox`, `loop-state`,
  `config`, `doctor`, `scaffold-e2e` on the root command.
- `autoflow_deliver.go` — make `deliver` accept these as child commands. The
  command object already exists; just call `deliverCmd.AddCommand(...)` on
  each instead of the root.
- `e2e.go` — register `scaffold` under `e2e`.
- Each `autoflow_<name>.go` file keeps its construction function; only the
  parent attachment changes.
- `cmd_layout_test.go` — update expected layout assertions.

### 2. Text references (larger but mechanical)

Update every string that names an old path. From the earlier audit, the
touch list is:

- `CLAUDE.md` — lines 40–47 (command catalog at the top).
- `README.md` — lines 244–251 (same catalog).
- `agents/autoflow/autoflow-ac-reviewer.md` — `loop-state append` lines.
- `agents/autoflow/autoflow-e2e-enhancer.md` — `loop-state append` lines.
- `agents/autoflow/autoflow-jira-fetcher.md` — multiple `jira fetch/search/download/config` lines.
- `agents/autoflow/autoflow-test-writer.md` — `scaffold-e2e` lines.
- `skills/autoflow/autoflow-deliver/SKILL.md` — multiple `deliver next/complete` lines (unchanged but verify).
- `skills/autoflow/autoflow-deliver/RESUME.md` — `deliver next` lines (unchanged).
- `skills/autoflow/autoflow-deliver/references/directory-contract.md` — `loop-state` and `deliver` mentions.
- `skills/autoflow/autoflow-deliver/references/jira-transitions.md` — every `jira` mention.
- `skills/autoflow/autoflow-settings/SKILL.md` — `worktree bootstrap` and `jira config`.
- `skills/autoflow/autoflow-ticket/SKILL.md` — `jira config`, `jira upload`.
- `internal/autoflow/deliver/preconditions.go` — error string `autoflow deliver _report` (internal subcommand, unchanged).
- `internal/autoflow/deliver/steps.go` — error strings + instruction templates referencing `jira config get` and `scaffold-e2e`.
- `internal/autoflow/doctor/check.go` — package comment.
- `internal/autoflow/doctor/checks.go` — error strings (`jira config set`, `worktree bootstrap`).
- `internal/autoflow/doctor/sandbox_checks.go` — error string (`sandbox bootstrap`).
- `internal/autoflow/jira/client.go` — comment reference to `autoflow doctor`.
- `internal/cli/autoflow_deliver.go` — error string mentioning a previous run.
- `internal/cli/install.go` — suggestion line printing `autoflow sandbox bootstrap`.

`docs/sections/cli.md` does not reference the workflow commands today (it's
the e2e runner reference), so no edits there beyond fixing the existing
stale `e2e install --skills` line if the user wants it — that's outside the
scope of this reorg.

### 3. Tests

- `internal/cli/cmd_layout_test.go` already asserts the command layout —
  update its expected tree.
- `internal/cli/autoflow_*_test.go` — any test that builds the root command
  and asserts a path will need to use the new path.

## Verification

The reorg is verified when:

1. `autoflow --help` shows exactly: `e2e`, `deliver`, `install`, `version`,
   `completion` (plus help). No `jira` / `worktree` / `sandbox` /
   `loop-state` / `config` / `doctor` / `scaffold-e2e` at the top level.
2. `autoflow deliver --help` shows the seven moved subgroups in addition to
   `init`, `next`, `complete`, `timings`.
3. `autoflow e2e --help` lists `scaffold` alongside `run`, `validate`,
   `list`, etc.
4. `make build` passes.
5. `make test` passes (after layout-test fixture is updated).
6. `make lint` passes.
7. `rg -t md -t go "autoflow (jira|worktree|loop-state|sandbox|scaffold-e2e|doctor)\b"` returns zero hits across the repo (boundary-anchored to skip `autoflow deliver ...`).
8. A fresh `autoflow deliver doctor` run on the repo reports the same set of
   checks the old top-level `autoflow doctor` did.
9. A spot-check delivery run (or dry-run) of one agent — e.g. the
   `autoflow-jira-fetcher` reading the updated skill — invokes
   `autoflow deliver jira fetch` without error.

## Risks and mitigations

- **Skill prompt drift.** Agents read SKILL.md / agent.md text and emit
  shell commands. If we miss a reference, the agent emits a now-invalid
  path. Mitigation: the audit list above is exhaustive (from `rg` over the
  repo); CI's `make test` includes a layout test that will fail loudly.
- **Internal Go string references.** Several Go files print suggestions
  like ``run `autoflow jira config set ...```. These don't break the build
  if missed, only the user-facing suggestion. Mitigation: included in the
  text-reference list above; sweep them in the same PR.
- **`autoflow deliver _report` and other underscore subcommands.** These
  are internal API for skill agents and already correctly scoped. No
  change.

## Open questions

1. Should `install` move under `deliver` too? It currently installs both
   `autoflow-cli` (e2e-focused) and `autoflow-*` (workflow-focused) skills,
   so it spans both products. Recommendation: leave at top level — it's a
   global setup verb.

## Out of scope

- Renaming `init` to `start`.
- Backwards-compat aliases.
- Improving the `e2e` subtree internals.
- Fixing the stale `e2e install --skills` line in `docs/sections/cli.md`
  (separate, pre-existing bug).
- Adding new commands.
