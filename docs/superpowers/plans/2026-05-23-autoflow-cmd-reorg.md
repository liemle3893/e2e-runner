# Autoflow CLI Command Reorganization — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Re-parent six workflow primitives under `deliver` and move `scaffold-e2e` under `e2e`, so `autoflow --help` shows two clean product trees with no behavior change.

**Architecture:** Pure cobra re-parenting plus mechanical text edits. Single binary, single Go module. No new commands, no flag changes, no logic changes. Hard cut — no aliases.

**Tech Stack:** Go 1.22+, cobra, golangci-lint, ripgrep for verification sweeps.

**Spec:** `docs/superpowers/specs/2026-05-23-autoflow-cmd-reorg-design.md`

---

## File Structure

Files modified or touched, grouped by responsibility:

**Cobra wiring (4 files):**
- `internal/cli/root.go` — stop registering 7 commands at root
- `internal/cli/autoflow_deliver.go` — adopt the 6 workflow-primitive commands as children
- `internal/cli/e2e.go` — adopt `scaffold` as a child
- `internal/cli/autoflow_scaffold.go` — rename `Use: "scaffold-e2e"` → `Use: "scaffold"`

**Layout tests (1 file):**
- `internal/cli/cmd_layout_test.go` — flip expected trees; add a `deliver` subtree test

**Go-internal text references (8 files):**
- `internal/autoflow/deliver/steps.go`
- `internal/autoflow/deliver/preconditions.go` (no change — references `_report` which stays)
- `internal/autoflow/doctor/check.go`
- `internal/autoflow/doctor/checks.go`
- `internal/autoflow/doctor/sandbox_checks.go`
- `internal/autoflow/jira/client.go`
- `internal/cli/autoflow_deliver.go` (error string update)
- `internal/cli/install.go`

**Agents (4 files):**
- `agents/autoflow/autoflow-ac-reviewer.md`
- `agents/autoflow/autoflow-e2e-enhancer.md`
- `agents/autoflow/autoflow-jira-fetcher.md`
- `agents/autoflow/autoflow-test-writer.md`

**Skills (5 files):**
- `skills/autoflow/autoflow-deliver/SKILL.md` (`deliver next/complete` unchanged — verify)
- `skills/autoflow/autoflow-deliver/RESUME.md` (verify)
- `skills/autoflow/autoflow-deliver/references/directory-contract.md`
- `skills/autoflow/autoflow-deliver/references/jira-transitions.md`
- `skills/autoflow/autoflow-settings/SKILL.md`
- `skills/autoflow/autoflow-ticket/SKILL.md`

**Top-level docs (2 files):**
- `README.md` — command catalog lines 244–251
- `CLAUDE.md` — command catalog lines 40–47

---

## Task 1: Update layout tests to pin the new tree

**Files:**
- Modify: `internal/cli/cmd_layout_test.go`

- [ ] **Step 1: Replace the `want` lists in the two existing tests and add a new test for the deliver subtree**

Open `internal/cli/cmd_layout_test.go` and replace lines 10–36 (the two existing test functions) with:

```go
// TestRootTopLevelCommands pins the top-level command list so accidental
// renames, removals, or additions surface immediately in review.
func TestRootTopLevelCommands(t *testing.T) {
	want := []string{
		"completion", "deliver", "e2e", "help", "install", "version",
	}
	sort.Strings(want)

	root := NewRoot("test")
	got := commandNames(root.Commands())
	assertEqualNames(t, "root", got, want)
}

// TestE2ESubtreeCommands pins the e2e subtree (includes the moved scaffold).
func TestE2ESubtreeCommands(t *testing.T) {
	want := []string{
		"doc", "health", "init", "list", "run", "scaffold", "test", "validate",
	}
	sort.Strings(want)

	root := NewRoot("test")
	e2e := findChild(root, "e2e")
	if e2e == nil {
		t.Fatal("e2e subcommand not found under root")
	}
	got := commandNames(e2e.Commands())
	assertEqualNames(t, "e2e", got, want)
}

// TestDeliverSubtreeCommands pins the deliver umbrella's children. Includes
// public verbs, the six workflow primitives moved under deliver, and the
// internal underscore-prefixed subcommands consumed by agents.
func TestDeliverSubtreeCommands(t *testing.T) {
	want := []string{
		// Public verbs
		"init", "next", "complete", "timings",
		// Workflow primitives moved under deliver
		"config", "doctor", "jira", "loop-state", "sandbox", "worktree",
		// Internal API for skill agents
		"_commit-task", "_complete-step", "_e2e-round", "_gate-result",
		"_report", "_set-field", "_verify-gates",
	}
	sort.Strings(want)

	root := NewRoot("test")
	deliver := findChild(root, "deliver")
	if deliver == nil {
		t.Fatal("deliver subcommand not found under root")
	}
	got := commandNames(deliver.Commands())
	assertEqualNames(t, "deliver", got, want)
}
```

Note: cobra auto-adds `completion` and `help` commands. Both appear in the root's `Commands()` slice and must be included in the `want` list (this matches what `TestRootTopLevelCommands` would already see if `completion` and `help` were enumerated — verify when running).

- [ ] **Step 2: Run the layout tests to confirm they now fail**

Run:
```bash
go test ./internal/cli/ -run TestRootTopLevelCommands -run TestE2ESubtreeCommands -run TestDeliverSubtreeCommands -v
```

Expected: All three fail because the current binary still registers the old tree (e.g., `TestRootTopLevelCommands` reports `jira`/`worktree`/etc. present at root).

If `completion`/`help` aren't in `root.Commands()` (cobra version dependent), remove them from `want` and re-run. The point is the new asserted shape must match what cobra surfaces today, plus our new arrangement.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/cmd_layout_test.go
git commit -m "test: pin target command tree for reorg"
```

---

## Task 2: Re-parent workflow primitives under `deliver`

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/autoflow_deliver.go`
- Modify: `internal/cli/e2e.go`
- Modify: `internal/cli/autoflow_scaffold.go`

- [ ] **Step 1: Update `root.go` to register only the new top-level commands**

Replace the `root.AddCommand(...)` block (lines 19–34) with:

```go
	root.AddCommand(
		// E2E test-runner subtree
		newE2ECmd(),
		// Delivery workflow umbrella (owns jira/worktree/sandbox/loop-state/config/doctor as children)
		newAutoflowDeliverCmd(),
		// Cross-cutting
		newInstallCmd(),
		newVersionCmd(version),
	)
```

Update the comment block at lines 6–8 to reflect the new shape:

```go
// NewRoot builds and returns the root cobra command for the autoflow binary.
// Two visible products: the YAML-driven E2E test runner (`e2e`) and the
// Jira-to-PR delivery workflow (`deliver`). Workflow primitives (jira,
// worktree, sandbox, loop-state, config, doctor) live as children of
// `deliver`. `install` and `version` are cross-cutting top-level peers.
```

- [ ] **Step 2: Update `autoflow_deliver.go` to adopt the six primitives as children**

In `newAutoflowDeliverCmd()` (lines 20–39), expand the `cmd.AddCommand(...)` call to include the moved primitives:

```go
func newAutoflowDeliverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deliver",
		Short: "Jira-to-PR delivery workflow + supporting primitives",
	}
	cmd.AddCommand(
		// Public verbs
		newDeliverNextCmd(),
		newDeliverCompleteCmd(),
		newDeliverInitCmd(),
		newDeliverTimingsCmd(),
		// Internal API for skill agents
		newDeliverGateResultCmd(),
		newDeliverSetFieldCmd(),
		newDeliverCompleteStepCmd(),
		newDeliverE2ERoundCmd(),
		newDeliverReportCmd(),
		newDeliverVerifyGatesCmd(),
		newDeliverCommitTaskCmd(),
		// Workflow primitives (moved from top level in the 2026-05-23 reorg)
		newAutoflowJiraCmd(),
		newAutoflowWorktreeCmd(),
		newAutoflowSandboxCmd(),
		newAutoflowLoopStateCmd(),
		newAutoflowConfigCmd(),
		newAutoflowDoctorCmd(),
	)
	return cmd
}
```

- [ ] **Step 3: Update `e2e.go` to adopt `scaffold` as a child**

Add `newAutoflowScaffoldCmd()` to the `cmd.AddCommand(...)` call (after `newDocCmd()`):

```go
	cmd.AddCommand(
		newRunCmd(),
		newListCmd(),
		newValidateCmd(),
		newInitCmd(),
		newHealthCmd(),
		newTestCmd(),
		newDocCmd(),
		newAutoflowScaffoldCmd(),
	)
```

Add `scaffold` to the `Long:` docstring listing in `newE2ECmd` so `--help` reflects it. After the `doc` line in the existing block (around line 21):

```go
		Long: `E2E test-runner commands.

Commands:
  run       Discover and run YAML test files
  list      List discovered test files and their metadata
  validate  Parse and validate YAML test files without running them
  init      Create a starter e2e.config.yaml in the current directory
  health    Check connectivity for all configured adapters
  test      Helpers for creating and managing test files
  doc       Show built-in documentation
  scaffold  Generate E2E test stubs for a ticket`,
```

- [ ] **Step 4: Rename the `scaffold-e2e` command to `scaffold`**

In `internal/cli/autoflow_scaffold.go`, change line 14:

```go
		Use:   "scaffold",
		Short: "Generate E2E test stubs for a ticket",
```

(Drop the legacy `(replaces scaffold-e2e.sh)` suffix from `Short` since the user-facing name is now `scaffold`.)

- [ ] **Step 5: Run layout tests to confirm they pass**

Run:
```bash
go test ./internal/cli/ -run TestRootTopLevelCommands -run TestE2ESubtreeCommands -run TestDeliverSubtreeCommands -v
```

Expected: All three PASS. If `completion`/`help` mismatches surfaced in Task 1, reconcile the `want` list in `cmd_layout_test.go` now and re-run.

- [ ] **Step 6: Smoke-test the binary**

```bash
make build && ./bin/autoflow --help
./bin/autoflow deliver --help
./bin/autoflow e2e --help
./bin/autoflow deliver jira --help
./bin/autoflow e2e scaffold --help
```

Expected: top-level help shows only `e2e`, `deliver`, `install`, `version`, plus cobra's auto `completion`/`help`. `deliver --help` lists the six moved subgroups. `e2e --help` lists `scaffold`. The two leaf `--help` commands work.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/root.go internal/cli/autoflow_deliver.go internal/cli/e2e.go internal/cli/autoflow_scaffold.go
git commit -m "refactor: re-parent workflow primitives under deliver, scaffold under e2e"
```

---

## Task 3: Update Go-internal text references

These are user-facing error strings, hint messages, and comments that name old command paths. None affect behavior; they all affect what users see when something goes wrong.

**Files:**
- Modify: `internal/autoflow/deliver/steps.go`
- Modify: `internal/autoflow/doctor/check.go`
- Modify: `internal/autoflow/doctor/checks.go`
- Modify: `internal/autoflow/doctor/sandbox_checks.go`
- Modify: `internal/autoflow/jira/client.go`
- Modify: `internal/cli/autoflow_deliver.go`
- Modify: `internal/cli/install.go`

- [ ] **Step 1: `internal/autoflow/deliver/steps.go`**

Two changes. Around line 83 (comment) and line 91 (instruction template string), replace `autoflow jira config get` with `autoflow deliver jira config get`. Around line 281 (instruction template), replace `autoflow scaffold-e2e --ticket` with `autoflow e2e scaffold --ticket`.

Verify with:
```bash
rg "autoflow (jira|scaffold-e2e)" internal/autoflow/deliver/steps.go
```
Expected: zero hits after edits.

- [ ] **Step 2: `internal/autoflow/doctor/check.go`**

Package comment on line 1 says `// Package doctor implements `autoflow doctor`...`. Replace `autoflow doctor` with `autoflow deliver doctor`.

- [ ] **Step 3: `internal/autoflow/doctor/checks.go`**

Two error-string hints. Replace `autoflow jira config set ...` with `autoflow deliver jira config set ...` (line ~73), and `autoflow worktree` with `autoflow deliver worktree` (line ~128 comment).

- [ ] **Step 4: `internal/autoflow/doctor/sandbox_checks.go`**

Replace `autoflow sandbox bootstrap` with `autoflow deliver sandbox bootstrap` (line ~249).

- [ ] **Step 5: `internal/autoflow/jira/client.go`**

Comment on line ~33: replace `autoflow doctor` with `autoflow deliver doctor`.

- [ ] **Step 6: `internal/cli/autoflow_deliver.go`**

Error string around line 238 references "run `autoflow deliver _report`" — that stays (internal subcommand under deliver, unchanged). No change to this file in Task 3 unless other references surfaced; re-verify with rg.

- [ ] **Step 7: `internal/cli/install.go`**

Line ~142 prints a suggestion: `... && autoflow sandbox bootstrap --name %s`. Replace `autoflow sandbox bootstrap` with `autoflow deliver sandbox bootstrap`.

- [ ] **Step 8: Verify the Go sweep is clean**

```bash
rg -t go "autoflow (jira|worktree|loop-state|sandbox|scaffold-e2e|doctor)\b" internal/
```
Expected: zero hits (boundary anchor `\b` skips the legitimate `autoflow deliver ...` paths).

- [ ] **Step 9: Run tests and build**

```bash
make build && make test
```
Expected: all green.

- [ ] **Step 10: Commit**

```bash
git add internal/
git commit -m "refactor: update Go error strings to reference new command paths"
```

---

## Task 4: Update agent prompt files

Agents emit shell commands based on these prompts. Every old-path reference becomes a runtime failure if missed.

**Files:**
- Modify: `agents/autoflow/autoflow-ac-reviewer.md`
- Modify: `agents/autoflow/autoflow-e2e-enhancer.md`
- Modify: `agents/autoflow/autoflow-jira-fetcher.md`
- Modify: `agents/autoflow/autoflow-test-writer.md`

- [ ] **Step 1: `autoflow-ac-reviewer.md`**

Three occurrences of `autoflow loop-state append`. Replace each with `autoflow deliver loop-state append`.

- [ ] **Step 2: `autoflow-e2e-enhancer.md`**

Three occurrences of `autoflow loop-state append`. Replace each with `autoflow deliver loop-state append`.

- [ ] **Step 3: `autoflow-jira-fetcher.md`**

Five occurrences. Replace:
- `autoflow jira config get --field cloudId` → `autoflow deliver jira config get --field cloudId`
- `autoflow jira fetch <TICKET-KEY>` → `autoflow deliver jira fetch <TICKET-KEY>`
- `autoflow jira search --jql ...` (two lines) → `autoflow deliver jira search --jql ...`
- `autoflow jira download <TICKET-KEY>` → `autoflow deliver jira download <TICKET-KEY>`

- [ ] **Step 4: `autoflow-test-writer.md`**

Three occurrences of `autoflow scaffold-e2e --ticket`. Replace each with `autoflow e2e scaffold --ticket`.

- [ ] **Step 5: Verify the agent sweep is clean**

```bash
rg "autoflow (jira|worktree|loop-state|sandbox|scaffold-e2e|doctor)\b" agents/
```
Expected: zero hits.

- [ ] **Step 6: Commit**

```bash
git add agents/
git commit -m "docs(agents): update command paths for reorg"
```

---

## Task 5: Update skill prompt files

**Files:**
- Modify: `skills/autoflow/autoflow-deliver/references/directory-contract.md`
- Modify: `skills/autoflow/autoflow-deliver/references/jira-transitions.md`
- Modify: `skills/autoflow/autoflow-settings/SKILL.md`
- Modify: `skills/autoflow/autoflow-ticket/SKILL.md`
- Verify (no edits expected): `skills/autoflow/autoflow-deliver/SKILL.md`, `skills/autoflow/autoflow-deliver/RESUME.md`, `skills/autoflow/autoflow-code-review/SKILL.md`, `skills/autoflow/autoflow-local-merge/SKILL.md`

- [ ] **Step 1: `references/directory-contract.md`**

Line ~24 mentions `autoflow loop-state` MUST run from `REPO_ROOT`. Replace `autoflow loop-state` with `autoflow deliver loop-state`. Line ~51 mentions `autoflow d...` (truncated, probably `autoflow deliver`) — verify and update if needed. Line ~60 says state is written by the `autoflow d...` (likely `autoflow deliver`) — verify.

- [ ] **Step 2: `references/jira-transitions.md`**

Seven occurrences of `autoflow jira` (transitions, transition, etc.). Replace each with `autoflow deliver jira`.

- [ ] **Step 3: `autoflow-settings/SKILL.md`**

Two occurrences:
- `autoflow jira config` (line ~49) → `autoflow deliver jira config`
- `autoflow worktree bootstrap` (line ~268) → `autoflow deliver worktree bootstrap`

- [ ] **Step 4: `autoflow-ticket/SKILL.md`**

Four occurrences:
- `autoflow jira config get --field cloudId` → `autoflow deliver jira config get --field cloudId`
- `autoflow jira config set --cloud-id ...` → `autoflow deliver jira config set --cloud-id ...`
- `autoflow jira up...` (truncated, `upload`) → `autoflow deliver jira upload`
- `autoflow jira upload <EPIC_KEY>` → `autoflow deliver jira upload <EPIC_KEY>`

- [ ] **Step 5: Verify the skills sweep is clean**

```bash
rg "autoflow (jira|worktree|loop-state|sandbox|scaffold-e2e|doctor)\b" skills/
```
Expected: zero hits. (`autoflow deliver ...` and `autoflow-deliver` skill-name mentions are not matched by the `\b`-anchored regex.)

- [ ] **Step 6: Commit**

```bash
git add skills/
git commit -m "docs(skills): update command paths for reorg"
```

---

## Task 6: Update top-level docs

**Files:**
- Modify: `README.md` (lines ~244–251)
- Modify: `CLAUDE.md` (lines ~40–47)

- [ ] **Step 1: `README.md`**

In the command catalog around lines 244–251, update each line:

| Old                                                                            | New                                                                                  |
|--------------------------------------------------------------------------------|--------------------------------------------------------------------------------------|
| `autoflow jira config {set,get,del,show}      Manage .autoflow/jira-config.json` | `autoflow deliver jira config {set,get,del,show}      Manage .autoflow/jira-config.json` |
| `autoflow jira upload <KEY> <file>...          Upload attachments to a Jira issue` | `autoflow deliver jira upload <KEY> <file>...          Upload attachments to a Jira issue` |
| `autoflow jira download <KEY> <dir>            Download attachments from a Jira issue` | `autoflow deliver jira download <KEY> <dir>            Download attachments from a Jira issue` |
| `autoflow worktree bootstrap <path>            Copy .claude + config files into worktree` | `autoflow deliver worktree bootstrap <path>            Copy .claude + config files into worktree` |
| `autoflow deliver {init,next,complete}         13-step delivery state machine`   | unchanged                                                                          |
| `autoflow loop-state {init,append,read,round-count}   Generic agentic-loop state manager` | `autoflow deliver loop-state {init,append,read,round-count}   Generic agentic-loop state manager` |
| `autoflow scaffold-e2e --ticket KEY --area A --count N   Generate E2E test stubs` | `autoflow e2e scaffold --ticket KEY --area A --count N   Generate E2E test stubs`   |
| `autoflow doctor                               Preflight checklist (git, gh, Jira)` | `autoflow deliver doctor                       Preflight checklist (git, gh, Jira)` |

- [ ] **Step 2: `CLAUDE.md`**

Update the corresponding lines (40–47) to the new paths, plus add `autoflow deliver config {set,get,del,show}` if it isn't already listed (the top-level `config` group moves under `deliver`).

- [ ] **Step 3: Verify the docs sweep is clean**

```bash
rg "autoflow (jira|worktree|loop-state|sandbox|scaffold-e2e|doctor)\b" README.md CLAUDE.md
```
Expected: zero hits.

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: update top-level command catalog for reorg"
```

---

## Task 7: Final verification sweep

- [ ] **Step 1: Whole-repo grep**

```bash
rg -t md -t go "autoflow (jira|worktree|loop-state|sandbox|scaffold-e2e|doctor)\b"
```
Expected: zero hits anywhere in the repo. If any survive, fix in place and amend the most relevant prior commit (or create a follow-up commit).

- [ ] **Step 2: Build + test + lint**

```bash
make build && make test && make lint
```
Expected: all green.

- [ ] **Step 3: Help-output sanity check**

```bash
./bin/autoflow --help
./bin/autoflow deliver --help
./bin/autoflow deliver jira --help
./bin/autoflow deliver worktree --help
./bin/autoflow deliver sandbox --help
./bin/autoflow deliver loop-state --help
./bin/autoflow deliver config --help
./bin/autoflow deliver doctor --help
./bin/autoflow e2e --help
./bin/autoflow e2e scaffold --help
```
Expected: every command resolves, no `unknown command` errors, every subcommand is reachable at its new path.

- [ ] **Step 4: Doctor smoke-test**

```bash
./bin/autoflow deliver doctor
```
Expected: same set of checks the old top-level `autoflow doctor` ran. Any error hints in the output reference new paths only (no `autoflow jira ...` / `autoflow worktree ...` strings).

- [ ] **Step 5: Commit (or skip if nothing changed)**

If any straggler edits were needed in Step 1:
```bash
git add -A
git commit -m "fix: clean up stray references missed in reorg"
```

Otherwise this task is verification-only — no commit.

---

## Self-Review Notes

Run through the spec one section at a time:

1. **Problem section** — covered by Tasks 1–6 (Tasks 4–6 fix the discoverability and consistency issues; Tasks 1–2 implement the shape).
2. **Goals 1 (visible products)** — Task 2 splits the help output.
3. **Goals 2 (predictable agent prompts)** — Tasks 4, 5 update every agent/skill.
4. **Goals 3 (config collision, scaffold misfiling)** — Task 2 moves `config` under `deliver` and `scaffold-e2e` → `e2e scaffold`.
5. **Non-goals** — verbs stay (`init/next/complete`), no aliases, binary unchanged. All preserved.
6. **Target shape** — Task 1 pins it in tests; Task 2 implements it.
7. **Migration map** — every row covered.
8. **Implementation outline (cobra)** — Task 2 step-by-step.
9. **Implementation outline (text)** — Tasks 3–6 split by file family.
10. **Implementation outline (tests)** — Task 1.
11. **Verification** — Task 7 step-by-step.
12. **Risks** — the grep sweeps in Tasks 3–6 plus the final whole-repo grep in Task 7 mitigate "missed reference" risk; layout test catches structural drift.
13. **Open question (install location)** — resolved to "stay at top level"; Task 2 keeps it there.

No placeholders, no "TBD". Method names and command paths reused consistently across tasks.
