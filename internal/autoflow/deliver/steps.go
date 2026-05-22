package deliver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/liemle3893/autoflow/internal/autoflow/state"
	"github.com/liemle3893/autoflow/internal/autoflow/worktree"
)

// autoflowCmd returns the best available invocation of the autoflow binary
// for use inside emitted bash commands. Prefers os.Executable (the
// in-process binary path) so agents do not need `autoflow` on PATH to run
// the command. Falls back to the bare name if the binary can't be
// located — e.g. under some CI wrappers.
var autoflowCmd = sync.OnceValue(func() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "autoflow"
	}
	// Resolve symlinks so a `/usr/local/bin/autoflow -> /opt/...` install
	// emits the canonical path.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
})

// stepFn is the signature shared by step_01..step_13.
type stepFn func(key string, progress *state.Progress) *Instruction

// stepRegistry returns the 1..13 → stepFn table bound to c. Rebuilt each
// call so step methods can close over the controller's Root.
func stepRegistry(c *Controller) map[int]stepFn {
	return map[int]stepFn{
		1:  func(k string, _ *state.Progress) *Instruction { return c.step01(k) },
		2:  c.step02,
		3:  c.step03,
		4:  c.step04,
		5:  c.step05,
		6:  c.step06,
		7:  c.step07,
		8:  c.step08,
		9:  c.step09,
		10: c.step10,
		11: c.step11,
		12: c.step12,
		13: c.step13,
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 1 — Fetch Jira + build task brief
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step01(key string) *Instruction {
	tdir := state.TicketDir(c.Root, key)
	briefPath := filepath.Join(tdir, "task-brief.md")

	// If the brief is already on disk, the agent already ran — auto-complete.
	if _, err := os.Stat(briefPath); err == nil {
		meta, _ := ParseBrief(briefPath)
		title := meta["title"]
		if title == "" {
			title = key
		}
		ac := autoComplete(1, fmt.Sprintf("Task brief already exists. TITLE=%s", title))
		ac.PassToComplete = fmt.Sprintf(`--title %q`, title)
		return ac
	}

	// Dispatch the fetcher. The prompt carries the cloud ID discovered
	// from the Jira cache so the agent can wire it into the REST calls
	// without a second lookup. Instead of spawning a bash pre-step we
	// embed a literal placeholder and tell the agent to run
	// `autoflow deliver jira config get --field cloudId` to fill it in — matches
	// the skill's instructions.
	prompt := strings.Join([]string{
		"TICKET_KEY: " + key,
		"REPO_ROOT: " + c.Root,
		"OUTPUT_PATH: " + filepath.Join(tdir, "task-brief.md"),
		"ATTACHMENTS_DIR: " + filepath.Join(tdir, "attachments") + "/",
		"",
		`CLOUD_ID: <run "autoflow deliver jira config get --field cloudId" in REPO_ROOT>`,
		"",
		"Follow your role definition. Produce a verbatim task brief — no rephrasing of AC/DoD. No worktree exists yet.",
	}, "\n")

	return &Instruction{
		Action:         ActionDispatch,
		SubagentType:   "autoflow-jira-fetcher",
		Description:    "Fetch Jira: " + key,
		Prompt:         prompt,
		ParseReturn:    "## BRIEF COMPLETE",
		Extract:        map[string]string{"title": "TITLE"},
		PassToComplete: "--title <extracted-title>",
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 2 — Worktree + bootstrap + seed progress
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step02(key string, progress *state.Progress) *Instruction {
	if progress != nil && progress.Worktree != "" {
		if _, err := os.Stat(progress.Worktree); err == nil {
			return autoComplete(2, "Worktree already exists at "+progress.Worktree)
		}
	}

	// Step 2 runs entirely inside the controller. We create the worktree,
	// bootstrap it, and seed workflow-progress.json in-process — no bash
	// round-trip. Progress output goes to a dedicated log file so the JSON
	// `deliver next` prints stays clean.
	title := key
	if progress != nil && progress.Title != nil && *progress.Title != "" {
		title = *progress.Title
	}

	cfg, _ := worktree.ReadConfig(c.Root)
	if cfg == nil {
		cfg = &worktree.Config{}
	}
	worktree.AutoDetect(cfg, c.Root)
	baseBranch := cfg.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	slug := makeSlug(title)
	branch := fmt.Sprintf("jira-iss/%s-%s", strings.ToLower(key), slug)
	worktreeDir := fmt.Sprintf("%s-%s",
		filepath.Join(filepath.Dir(c.Root), filepath.Base(c.Root)),
		strings.ToLower(key),
	)

	logPath := filepath.Join(state.TicketDir(c.Root, key), "step-02.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return escalate("step 2: mkdir ticket dir: " + err.Error())
	}
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return escalate("step 2: open log: " + err.Error())
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "step 2 for %s — branch %s worktree %s\n", key, branch, worktreeDir)

	// 1. Fetch the base branch.
	if err := runGit(logFile, c.Root, "fetch", "origin", baseBranch); err != nil {
		return escalate("step 2: git fetch origin " + baseBranch + ": " + err.Error() + " (see " + logPath + ")")
	}

	// 2. Create the linked worktree — unless a prior partial run already
	//    left one behind. In that case, if git confirms the dir is a
	//    registered linked worktree of this main repo we ADOPT it
	//    (seed progress pointing at it + mark step 1 done); only
	//    escalate when the dir exists but git doesn't know about it.
	if _, err := os.Stat(worktreeDir); err == nil {
		existingBranch, ok := findRegisteredWorktree(c.Root, worktreeDir)
		if !ok {
			return escalate(fmt.Sprintf(
				"step 2: %s exists but git does not register it as a linked worktree of %s — remove it manually or run `git worktree add` yourself",
				worktreeDir, c.Root))
		}
		if _, err := state.InitProgress(c.Root, key, worktreeDir, existingBranch, false); err != nil {
			return escalate("step 2: adopt existing worktree — init progress: " + err.Error())
		}
		if err := state.SetField(c.Root, key, "title", title); err != nil {
			return escalate("step 2: adopt existing worktree — set title: " + err.Error())
		}
		if err := state.CompleteStep(c.Root, key, 1); err != nil {
			return escalate("step 2: adopt existing worktree — mark step 1 complete: " + err.Error())
		}
		ac := autoComplete(2, fmt.Sprintf("Adopted existing worktree at %s on branch %s", worktreeDir, existingBranch))
		ac.PostActions = []PostAction{{
			Action:         "jira_transition",
			Ticket:         key,
			FromStatus:     "To Do",
			ToStatus:       "In Development",
			TransitionName: "start dev",
		}}
		return ac
	}
	if err := runGit(logFile, c.Root, "worktree", "add", worktreeDir, "-b", branch, "origin/"+baseBranch); err != nil {
		return escalate("step 2: git worktree add: " + err.Error() + " (see " + logPath + ")")
	}

	// 3. Bootstrap the worktree (copy .claude, config files, install+verify).
	if err := worktree.Bootstrap(worktree.BootstrapOptions{
		MainDir:     c.Root,
		WorktreeDir: worktreeDir,
		Config:      cfg,
		Prompter:    worktree.NonInteractivePrompter{},
		Stdout:      logFile,
		Stderr:      logFile,
	}); err != nil {
		return escalate("step 2: bootstrap: " + err.Error() + " (see " + logPath + ")")
	}

	// 4. Seed workflow-progress.json and record step 1 as complete.
	if _, err := state.InitProgress(c.Root, key, worktreeDir, branch, false); err != nil {
		return escalate("step 2: init progress: " + err.Error())
	}
	if err := state.SetField(c.Root, key, "title", title); err != nil {
		return escalate("step 2: set title: " + err.Error())
	}
	if err := state.CompleteStep(c.Root, key, 1); err != nil {
		return escalate("step 2: mark step 1 complete: " + err.Error())
	}

	ac := autoComplete(2, fmt.Sprintf("Worktree created at %s on branch %s", worktreeDir, branch))
	ac.PostActions = []PostAction{{
		Action:         "jira_transition",
		Ticket:         key,
		FromStatus:     "To Do",
		ToStatus:       "In Development",
		TransitionName: "start dev",
	}}
	return ac
}

// makeSlug turns a ticket title into a branch-safe slug: lowercase, all
// non-alphanumeric runs collapsed to '-', trimmed to 40 chars, trailing
// dash stripped.
func makeSlug(title string) string {
	lower := strings.ToLower(title)
	var b strings.Builder
	lastDash := true
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := b.String()
	if len(out) > 40 {
		out = out[:40]
	}
	return strings.TrimRight(out, "-")
}

// runGit executes a git subcommand with stdout+stderr teed to logOut. The
// git working directory is dir — callers should pass the main repo root
// for worktree operations.
func runGit(logOut *os.File, dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = logOut
	cmd.Stderr = logOut
	fmt.Fprintf(logOut, "+ git %s (cwd=%s)\n", strings.Join(args, " "), dir)
	return cmd.Run()
}

// ═══════════════════════════════════════════════════════════════════════
// Step 3 — Write E2E tests
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step03(key string, progress *state.Progress) *Instruction {
	wt := progress.Worktree
	brief := filepath.Join(state.TicketDir(c.Root, key), "task-brief.md")
	return &Instruction{
		Action:       ActionDispatch,
		SubagentType: "autoflow-test-writer",
		Description:  "Write E2E tests: " + key,
		Prompt: strings.Join([]string{
			"TICKET_KEY: " + key,
			"REPO_ROOT: " + c.Root,
			"WORKTREE_DIR: " + wt,
			"TASK_BRIEF_PATH: " + brief,
			"",
			`Follow your role definition. Run "autoflow e2e scaffold --ticket ` + key + ` --area <AREA> --count <N>" from WORKTREE_DIR.`,
			`Write tests under ${WORKTREE_DIR}/tests/e2e/. Tests must fail (no implementation yet).`,
			`Every file MUST include tags: [<area>, ` + key + `].`,
		}, "\n"),
		ParseReturn: "## TESTS WRITTEN",
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 4 — AC coverage review loop (max 3 rounds)
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step04(key string, progress *state.Progress) *Instruction {
	wt := progress.Worktree
	brief := filepath.Join(state.TicketDir(c.Root, key), "task-brief.md")
	stateFile := filepath.Join(state.TicketStateDir(c.Root, key), "coverage-review-state.json")

	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		// Initialise the loop state file via the Go CLI.
		return &Instruction{
			Action:      ActionBash,
			Description: "Initialize AC coverage loop for " + key,
			Commands: []string{
				fmt.Sprintf(`cd "%s"`, c.Root),
				fmt.Sprintf(
					`%s loop-state init ".autoflow/ticket/%s/state/coverage-review-state.json" --loop coverage-review --ticket %s --max-rounds 3`,
					autoflowCmd(), key, shellQuote(key),
				),
			},
			OnFailure: "escalate",
			Loop:      true,
		}
	}

	// Read existing rounds to decide next action.
	loopState, err := readLoopStateRaw(stateFile)
	if err != nil {
		return escalate(fmt.Sprintf("Corrupt state file: %s — %v", stateFile, err))
	}
	if len(loopState.Rounds) > 0 {
		last := loopState.Rounds[len(loopState.Rounds)-1]
		if statusOf(last) == "PASS" {
			return autoComplete(4, "AC coverage passed")
		}
		if len(loopState.Rounds) >= 3 {
			return escalate("AC coverage loop exhausted after 3 rounds. Remaining gaps need user decision.")
		}
	}
	currentRound := len(loopState.Rounds) + 1

	return &Instruction{
		Action:       ActionDispatch,
		SubagentType: "autoflow-ac-reviewer",
		Description:  fmt.Sprintf("AC review round %d: %s", currentRound, key),
		Prompt: strings.Join([]string{
			"TICKET_KEY: " + key,
			"REPO_ROOT: " + c.Root,
			"WORKTREE_DIR: " + wt,
			"TASK_BRIEF_PATH: " + brief,
			"STATE_FILE: " + stateFile,
			"TEST_GLOB: tests/e2e/**/TC-" + key + "-*.test.yaml",
			fmt.Sprintf("ROUND: %d", currentRound),
			"",
			"Follow your role definition.",
		}, "\n"),
		ParseReturn: "## COVERAGE",
		Loop:        true,
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 5 — Implement (Path A direct / Path B plan+execute)
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step05(key string, progress *state.Progress) *Instruction {
	wt := progress.Worktree
	br := progress.Branch
	tdir := state.TicketDir(c.Root, key)
	briefPath := filepath.Join(tdir, "task-brief.md")
	planPath := filepath.Join(tdir, "PLAN.md")
	summaryPath := filepath.Join(tdir, "SUMMARY.md")

	meta, _ := ParseBrief(briefPath)
	pathRec := strings.ToUpper(meta["path_recommendation"])
	if pathRec == "" {
		pathRec = "B"
	}
	hasFix := strings.EqualFold(meta["has_fix_strategy"], "true")
	estFiles := 99
	if s := meta["estimated_files"]; s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			estFiles = n
		}
	}

	if pathRec == "A" || (hasFix && estFiles <= 5) {
		return &Instruction{
			Action:       ActionDispatch,
			SubagentType: "autoflow-executor",
			Description:  "Execute direct fix: " + key,
			Prompt: strings.Join([]string{
				"TICKET_KEY: " + key,
				"WORKTREE_DIR: " + wt,
				"TASK_BRIEF_PATH: " + briefPath,
				"SUMMARY_OUTPUT_PATH: " + summaryPath,
				"BRANCH: " + br,
				"",
				"MODE: direct-fix",
				"The task brief contains a Fix Strategy section with file:line references.",
				"Treat the Fix Strategy as your plan — implement each fix, verify, and commit atomically.",
				"Write SUMMARY.md when done.",
				"",
				"Follow your role definition.",
			}, "\n"),
			ParseReturn: "## EXECUTION COMPLETE",
		}
	}

	if _, err := os.Stat(planPath); errors.Is(err, os.ErrNotExist) {
		sentinel := filepath.Join(state.TicketStateDir(c.Root, key), "planner-dispatched")
		if _, err := os.Stat(sentinel); err == nil {
			return escalate(fmt.Sprintf(
				"Planner was dispatched but PLAN.md was not created at %s. The planner may have failed or written to the wrong path.",
				planPath))
		}
		_ = os.MkdirAll(filepath.Dir(sentinel), 0o755)
		_ = os.WriteFile(sentinel, []byte("dispatched"), 0o644)

		return &Instruction{
			Action:       ActionDispatch,
			SubagentType: "autoflow-planner",
			Description:  "Plan implementation: " + key,
			Prompt: strings.Join([]string{
				"TICKET_KEY: " + key,
				"REPO_ROOT: " + c.Root,
				"WORKTREE_DIR: " + wt,
				"TASK_BRIEF_PATH: " + briefPath,
				"PLAN_OUTPUT_PATH: " + planPath,
				"DESIGN_DIR: " + filepath.Join(wt, "docs", "design"),
				"",
				"Follow your role definition.",
			}, "\n"),
			ParseReturn: "## PLAN COMPLETE",
			Loop:        true,
		}
	}

	// PLAN.md exists → parallel-batched execution via plan-tasks.json.
	return c.step05DispatchBatch(key, wt, br, planPath, summaryPath)
}

// step05DispatchBatch reads PLAN.md, recovers any stale "running" entries,
// and decides what to emit next: a per-task dispatch, a parallel fan-out
// of up to MaxParallelTasks dispatches, or SUMMARY.md + auto_complete
// once every task is done. Dependencies drive ordering.
func (c *Controller) step05DispatchBatch(key, wt, br, planPath, summaryPath string) *Instruction {
	_ = summaryPath // SUMMARY.md path is derived inside WriteSummary; kept for signature symmetry.

	plan, err := ParsePlan(planPath)
	if err != nil {
		return escalate("step 5: parse PLAN.md: " + err.Error())
	}

	// Clear any "running" entry that has no commit and no active lock
	// holder — evidence the executor for it died without finishing.
	if err := state.ResetStaleRunning(c.Root, key); err != nil {
		return escalate("step 5: reset stale running: " + err.Error())
	}

	ps, err := state.ReadPlanState(c.Root, key)
	if err != nil {
		return escalate("step 5: read plan-tasks: " + err.Error())
	}

	// Failed tasks block progress → surface immediately so the user can
	// either rewrite PLAN.md or reset the failed entry.
	var failed []string
	for id, r := range ps.Tasks {
		if r.Status == state.TaskFailed {
			failed = append(failed, id)
		}
	}
	if len(failed) > 0 {
		return escalate(fmt.Sprintf(
			"step 5: %d task(s) marked failed: %s — fix PLAN.md or reset them in %s",
			len(failed), strings.Join(failed, ", "), state.PlanTasksFile(c.Root, key)))
	}

	batch, status := NextBatch(plan, ps, MaxParallelTasks)
	if status.AllDone {
		if _, err := WriteSummary(c.Root, key); err != nil {
			return escalate("step 5: write SUMMARY.md: " + err.Error())
		}
		return autoComplete(5, fmt.Sprintf(
			"All %d plan tasks complete; SUMMARY.md written.", status.Total))
	}
	if len(batch) == 0 {
		// Nothing ready but not all done — some task is "running" (owned
		// by an executor from this round). Loop-true means the runtime
		// only re-enters here after that dispatch drained, so this is a
		// defensive branch.
		return escalate(fmt.Sprintf(
			"step 5: no tasks ready (done=%d/%d, running=%d) — check plan-tasks.json",
			status.Done, status.Total, status.Running))
	}

	if len(batch) == 1 {
		instr := c.buildExecutorDispatch(key, wt, br, planPath, batch[0])
		instr.Loop = true
		return instr
	}

	dispatches := make([]*Instruction, 0, len(batch))
	for _, t := range batch {
		dispatches = append(dispatches, c.buildExecutorDispatch(key, wt, br, planPath, t))
	}
	return &Instruction{
		Action:      ActionDispatchParallel,
		Step:        5,
		Description: fmt.Sprintf("Execute %d plan tasks in parallel", len(batch)),
		Dispatches:  dispatches,
		Loop:        true,
		Note:        "Spawn all executors in ONE message (multiple Agent tool uses). After they all finish, call next again to advance to the next batch.",
	}
}

// buildExecutorDispatch produces one single-task executor instruction
// with everything the agent needs to work on just that task: the plan
// path for context, the task id to scope to, and the list of files it
// is expected to modify (also used by the commit helper).
func (c *Controller) buildExecutorDispatch(key, wt, br, planPath string, t Task) *Instruction {
	files := strings.Join(t.Files, ",")
	commitMsg := fmt.Sprintf("%s: %s (%s)", t.ID, t.Name, key)
	commitCmd := fmt.Sprintf(
		"%s _commit-task --ticket %s --task-id %s --worktree %s --message %s --files %s",
		autoflowCmd(), key, t.ID, shellQuote(wt), shellQuote(commitMsg), shellQuote(files))

	prompt := strings.Join([]string{
		"TICKET_KEY: " + key,
		"WORKTREE_DIR: " + wt,
		"PLAN_PATH: " + planPath,
		"BRANCH: " + br,
		"TASK_ID: " + t.ID,
		"TASK_FILES: " + files,
		"",
		"MODE: single-task",
		"Scope: implement ONLY the task whose <id> is " + t.ID + ".",
		"Ignore all other <task> blocks in PLAN.md.",
		"",
		"When the task is verified, finalise with this exact command (do NOT run git commit yourself):",
		"  " + commitCmd,
		"The command serialises commits across parallel tasks and records the SHA for SUMMARY.md.",
		"",
		"Return marker on success: ## TASK COMPLETE: " + t.ID,
		"Return marker on failure: ## TASK FAILED: " + t.ID + " — <reason>",
		"",
		"Follow your role definition.",
	}, "\n")

	return &Instruction{
		Action:       ActionDispatch,
		SubagentType: "autoflow-executor",
		Description:  "Execute " + t.ID + ": " + t.Name,
		Prompt:       prompt,
		ParseReturn:  "## TASK COMPLETE: " + t.ID,
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 6 — Build + test gate (max 3 attempts)
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step06(key string, progress *state.Progress) *Instruction {
	wt := progress.Worktree
	br := progress.Branch

	cfg, _ := worktree.ReadConfig(c.Root)
	if cfg == nil {
		cfg = &worktree.Config{}
	}
	worktree.AutoDetect(cfg, c.Root)

	buildCmd := cfg.BuildCmd
	if buildCmd == "" {
		buildCmd = cfg.VerifyCmd
	}
	testCmd := cfg.TestCmd

	if buildCmd == "" && testCmd == "" {
		return autoComplete(6, "No build_cmd or test_cmd configured — skipping gate.")
	}
	if wt == "" {
		return escalate(fmt.Sprintf("Worktree missing or not set: %q", wt))
	}

	stateDir := state.TicketStateDir(c.Root, key)
	_ = os.MkdirAll(stateDir, 0o755)

	gate, err := readBuildGate(c.Root, key)
	if err != nil {
		return escalate(fmt.Sprintf("Corrupt build gate state: %v", err))
	}

	if gate.LastResult == "pass" {
		return autoComplete(6, "Build + test gate passed")
	}
	if gate.Attempt >= 3 {
		return escalate(fmt.Sprintf("Build/test gate failed after %d attempts. Last error in %s",
			gate.Attempt, gate.ErrorFile))
	}
	if gate.LastResult == "pending" && gate.Attempt > 0 {
		return escalate(fmt.Sprintf(
			"Build gate state is 'pending' after attempt %d — state write may have failed. Check %s",
			gate.Attempt, filepath.Join(stateDir, "build-gate-state.json")))
	}
	if gate.LastResult == "fail" && gate.FixDispatched {
		marker := filepath.Join(stateDir, fmt.Sprintf("build-fix-failed-%d.marker", gate.Attempt))
		if _, err := os.Stat(marker); err == nil {
			return escalate(fmt.Sprintf(
				"Code fixer explicitly failed on attempt %d. Error output: %s. Fix manually or skip.",
				gate.Attempt, gate.ErrorFile))
		}
	}

	// Dispatch the fixer when we had a failure and haven't asked yet.
	if gate.LastResult == "fail" && !gate.FixDispatched {
		gate.FixDispatched = true
		_ = writeBuildGate(c.Root, key, gate)
		marker := filepath.Join(stateDir, fmt.Sprintf("build-fix-failed-%d.marker", gate.Attempt))
		return &Instruction{
			Action:       ActionDispatch,
			SubagentType: "autoflow-code-fixer",
			Description:  "Fix build errors: " + key,
			Prompt: strings.Join([]string{
				"TICKET_KEY: " + key,
				"WORKTREE_DIR: " + wt,
				"BRANCH: " + br,
				"ERROR_FILE: " + gate.ErrorFile,
				"",
				"The build/test gate failed. The error output is at ERROR_FILE.",
				"Read it, diagnose the root cause, and fix the code in WORKTREE_DIR.",
				"Commit your fix but do NOT push.",
				"",
				"Return: ## FIX COMPLETE or ## FIX FAILED: <reason>",
			}, "\n"),
			Loop:              true,
			OnFixFailedMarker: marker,
		}
	}

	// Run the gate again. Capture exit code and log then hand over to
	// the `_gate-result` internal subcommand.
	nextAttempt := gate.Attempt + 1
	gateChain := []string{fmt.Sprintf(`cd "%s"`, wt)}
	if buildCmd != "" {
		gateChain = append(gateChain, buildCmd)
	}
	if testCmd != "" {
		gateChain = append(gateChain, testCmd)
	}
	logFile := filepath.Join(stateDir, fmt.Sprintf("build-gate-log-%d.log", nextAttempt))

	cmd := fmt.Sprintf(
		`GATE_RC_FILE=$(mktemp); ( set -o pipefail; (%s); echo $? > "$GATE_RC_FILE") 2>&1 | tee %s; GATE_RC=$(cat "$GATE_RC_FILE"); rm -f "$GATE_RC_FILE"; %s deliver _gate-result --ticket %s --attempt %d --exit-code $GATE_RC --log-file %s`,
		strings.Join(gateChain, " && "),
		shellQuote(logFile),
		autoflowCmd(),
		shellQuote(key),
		nextAttempt,
		shellQuote(logFile),
	)

	return &Instruction{
		Action:      ActionBash,
		Description: fmt.Sprintf("Build + test gate attempt %d: %s", nextAttempt, key),
		Commands:    []string{cmd},
		Loop:        true,
		OnFailure:   "escalate",
		Note:        "Call next again — controller reads gate state to decide: auto_complete, dispatch fixer, or escalate.",
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 7 — AC E2E test loop (max 5 rounds)
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step07(key string, progress *state.Progress) *Instruction {
	wt := progress.Worktree
	br := progress.Branch
	stateDir := state.TicketStateDir(c.Root, key)
	stateFile := filepath.Join(stateDir, "e2e-fix-state.json")

	if wt == "" {
		return escalate(fmt.Sprintf("Worktree missing or not set: %q", wt))
	}

	loopState, _ := readLoopStateRaw(stateFile)
	rounds := 0
	if loopState != nil {
		rounds = len(loopState.Rounds)
	}

	// Stale-state guard — mirrors the bash e2e-run-counter.txt pattern.
	counterFile := filepath.Join(stateDir, "e2e-run-counter.txt")
	runCount := 0
	if data, err := os.ReadFile(counterFile); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			runCount = n
		}
	}
	if rounds == 0 && runCount >= 3 {
		return escalate(fmt.Sprintf(
			"E2E tests dispatched %d times but no rounds recorded in state. Check %s",
			runCount, stateFile))
	}

	if loopState != nil && rounds > 0 {
		last := loopState.Rounds[rounds-1]
		lastStatus := statusOf(last)
		if lastStatus == "PASSED" {
			return autoComplete(7, "AC E2E tests passed")
		}
		maxRounds := 5
		if loopState.MaxRounds > 0 {
			maxRounds = loopState.MaxRounds
		}
		if rounds >= maxRounds {
			return escalate(fmt.Sprintf("E2E tests failed after %d rounds. Fix manually or skip.", rounds))
		}
		fixMarker := filepath.Join(stateDir, fmt.Sprintf("e2e-fix-dispatched-round-%d.marker", rounds))
		fixFailedMarker := filepath.Join(stateDir, fmt.Sprintf("e2e-fix-failed-round-%d.marker", rounds))
		if _, err := os.Stat(fixFailedMarker); err == nil {
			return escalate(fmt.Sprintf(
				"Code fixer failed on round %d. Test output: %s. Fix manually or skip.",
				rounds, stringOrNil(last, "output_file")))
		}
		if lastStatus == "FAILED" {
			if _, err := os.Stat(fixMarker); errors.Is(err, os.ErrNotExist) {
				_ = os.MkdirAll(filepath.Dir(fixMarker), 0o755)
				_ = os.WriteFile(fixMarker, []byte("dispatched"), 0o644)
				return &Instruction{
					Action:       ActionDispatch,
					SubagentType: "autoflow-code-fixer",
					Description:  fmt.Sprintf("Fix E2E failures round %d: %s", rounds, key),
					Prompt: strings.Join([]string{
						"TICKET_KEY: " + key,
						"WORKTREE_DIR: " + wt,
						"BRANCH: " + br,
						"TEST_OUTPUT: " + stringOrNil(last, "output_file"),
						"",
						"E2E tests failed. Read the test output at TEST_OUTPUT.",
						"Diagnose whether this is a test bug or an implementation bug.",
						"Fix the root cause in WORKTREE_DIR. Commit your fix.",
						"",
						"IMPORTANT: Return ## FIX COMPLETE if you fixed something.",
						"Return ## FIX FAILED: <reason> if you cannot fix it.",
					}, "\n"),
					Loop:              true,
					OnFixFailedMarker: fixFailedMarker,
				}
			}
		}
	}

	// Run one E2E round via the Go CLI. Tests execute directly in the
	// worktree (no merge onto base branch). The runner writes state at
	// the expected path so subsequent next() calls pick it up.
	qKey := shellQuote(key)
	cmd := fmt.Sprintf(
		`cd "%s" && %s deliver _e2e-round --ticket %s --max-rounds 5`,
		wt, autoflowCmd(), qKey,
	)
	// Increment the counter after the command runs.
	cmd += fmt.Sprintf(
		`; RC=$?; mkdir -p "$(dirname %s)"; echo $(($(cat %s 2>/dev/null || echo 0) + 1)) > %s; exit $RC`,
		shellQuote(counterFile), shellQuote(counterFile), shellQuote(counterFile),
	)

	return &Instruction{
		Action:      ActionBash,
		Description: "Run E2E tests for " + key,
		Commands:    []string{cmd},
		Loop:        true,
		OnFailure:   "escalate",
		Note:        "Bash only fails on setup errors. State file records PASSED/FAILED — call next to check.",
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 8 — Add coverage tests from implementation diff
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step08(key string, progress *state.Progress) *Instruction {
	wt := progress.Worktree
	br := progress.Branch
	cfg, _ := worktree.ReadConfig(c.Root)
	baseBranch := "main"
	if cfg != nil {
		worktree.AutoDetect(cfg, c.Root)
		baseBranch = cfg.BaseBranch
	}

	return &Instruction{
		Action:       ActionDispatch,
		SubagentType: "autoflow-e2e-enhancer",
		Description:  "Add coverage tests: " + key,
		Prompt: strings.Join([]string{
			"TICKET_KEY: " + key,
			"REPO_ROOT: " + c.Root,
			"WORKTREE_DIR: " + wt,
			"BRANCH: " + br,
			"BASE_BRANCH: " + baseBranch,
			"TEST_GLOB: tests/e2e/**/TC-" + key + "-*.test.yaml",
			"",
			fmt.Sprintf(`Run "git diff origin/%s...HEAD" from WORKTREE_DIR to see changes.`, baseBranch),
			"Write new tests. Commit and push when done.",
			"",
			"Follow your role definition.",
		}, "\n"),
		ParseReturn: "## ENHANCER",
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 9 — Review + fix (3 parallel reviewers → fixer)
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step09(key string, progress *state.Progress) *Instruction {
	wt := progress.Worktree
	br := progress.Branch
	cfg, _ := worktree.ReadConfig(c.Root)
	baseBranch := "main"
	if cfg != nil {
		worktree.AutoDetect(cfg, c.Root)
		baseBranch = cfg.BaseBranch
	}

	stateDir := state.TicketStateDir(c.Root, key)
	_ = os.MkdirAll(stateDir, 0o755)
	reviewCode := filepath.Join(stateDir, "REVIEW-code.md")
	reviewSimplify := filepath.Join(stateDir, "REVIEW-simplify.md")
	reviewRules := filepath.Join(stateDir, "REVIEW-rules.md")
	reviewFix := filepath.Join(stateDir, "REVIEW-FIX.md")
	all := []string{reviewCode, reviewSimplify, reviewRules}

	if _, err := os.Stat(reviewFix); err == nil {
		return autoComplete(9, "Review + fix complete")
	}

	allPresent := true
	for _, p := range all {
		if _, err := os.Stat(p); err != nil {
			allPresent = false
			break
		}
	}
	if allPresent {
		total := 0
		for _, p := range all {
			c, h := countReviewFindings(p)
			total += c + h
		}
		if total == 0 {
			return autoComplete(9, "Review clean — no Critical/High findings")
		}
		fixFailed := filepath.Join(stateDir, "review-fix-failed.marker")
		if _, err := os.Stat(fixFailed); err == nil {
			return escalate("Review fixer explicitly failed. Fix the review findings manually or skip this step.")
		}
		sentinel := filepath.Join(stateDir, "review-fixer-dispatched.marker")
		if _, err := os.Stat(sentinel); err == nil {
			return escalate(fmt.Sprintf(
				"Review fixer was dispatched but REVIEW-FIX.md was not created at %s.",
				reviewFix))
		}
		_ = os.WriteFile(sentinel, []byte("dispatched"), 0o644)

		return &Instruction{
			Action:       ActionDispatch,
			SubagentType: "autoflow-code-fixer",
			Description:  "Fix review findings: " + key,
			Prompt: strings.Join([]string{
				"TICKET_KEY: " + key,
				"WORKTREE_DIR: " + wt,
				"BRANCH: " + br,
				"",
				"<config>",
				"review_paths: [" + strings.Join(all, ", ") + "]",
				"output_path: " + reviewFix,
				"fix_scope: critical_warning",
				"</config>",
				"",
				"Read the review files. Fix Critical and High findings.",
				"Commit and push from WORKTREE_DIR.",
				"",
				"Follow your role definition.",
			}, "\n"),
			Loop:              true,
			OnFixFailedMarker: fixFailed,
		}
	}

	// Phase 1 — dispatch the three reviewers in parallel.
	diffCmd := fmt.Sprintf(
		`git diff --name-only origin/%s...HEAD -- . ":!.planning/" ":!.autoflow/"`,
		baseBranch,
	)

	var dispatches []*Instruction
	if _, err := os.Stat(reviewCode); errors.Is(err, os.ErrNotExist) {
		dispatches = append(dispatches, &Instruction{
			Action:       ActionDispatch,
			SubagentType: "autoflow-code-reviewer",
			Description:  "Code review: " + key,
			Prompt: strings.Join([]string{
				"TICKET_KEY: " + key,
				"WORKTREE_DIR: " + wt,
				"",
				"<config>",
				"depth: standard",
				"output_path: " + reviewCode,
				"diff_base: origin/" + baseBranch,
				"mode: standalone",
				"</config>",
				"",
				fmt.Sprintf("Review changed files from WORKTREE_DIR. Run `%s` to get the file list.", diffCmd),
				"Write REVIEW-code.md at the output_path.",
				"",
				"Follow your role definition.",
			}, "\n"),
		})
	}
	if _, err := os.Stat(reviewSimplify); errors.Is(err, os.ErrNotExist) {
		dispatches = append(dispatches, &Instruction{
			Action:       ActionDispatch,
			SubagentType: "autoflow-simplify-reviewer",
			Description:  "Simplify review: " + key,
			Prompt: strings.Join([]string{
				"TICKET_KEY: " + key,
				"WORKTREE_DIR: " + wt,
				"",
				"<config>",
				"output_path: " + reviewSimplify,
				"diff_base: origin/" + baseBranch,
				"mode: standalone",
				"</config>",
				"",
				fmt.Sprintf("Review changed files from WORKTREE_DIR. Run `%s` to get the file list.", diffCmd),
				"Write REVIEW-simplify.md at the output_path.",
				"",
				"Follow your role definition.",
			}, "\n"),
		})
	}
	if _, err := os.Stat(reviewRules); errors.Is(err, os.ErrNotExist) {
		dispatches = append(dispatches, &Instruction{
			Action:       ActionDispatch,
			SubagentType: "autoflow-rules-enforcer",
			Description:  "Rules review: " + key,
			Prompt: strings.Join([]string{
				"TICKET_KEY: " + key,
				"WORKTREE_DIR: " + wt,
				"",
				"<config>",
				"output_path: " + reviewRules,
				"diff_base: origin/" + baseBranch,
				"mode: standalone",
				"</config>",
				"",
				fmt.Sprintf("Review changed files from WORKTREE_DIR. Run `%s` to get the file list.", diffCmd),
				"Write REVIEW-rules.md at the output_path.",
				"",
				"Follow your role definition.",
			}, "\n"),
		})
	}

	if len(dispatches) == 0 {
		return autoComplete(9, "All reviews present, no action needed")
	}
	return &Instruction{
		Action:     ActionDispatchParallel,
		Dispatches: dispatches,
		Loop:       true,
		Note:       "Dispatch all agents in ONE Agent call with multiple tool uses. Call next again after all complete.",
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 10 — Implementation summary
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step10(key string, progress *state.Progress) *Instruction {
	wt := progress.Worktree
	br := progress.Branch
	cfg, _ := worktree.ReadConfig(c.Root)
	baseBranch := "main"
	if cfg != nil {
		worktree.AutoDetect(cfg, c.Root)
		baseBranch = cfg.BaseBranch
	}
	briefPath := filepath.Join(state.TicketDir(c.Root, key), "task-brief.md")
	meta, _ := ParseBrief(briefPath)
	title := meta["title"]
	if title == "" {
		title = key
	}

	return &Instruction{
		Action:       ActionDispatch,
		SubagentType: "autoflow-docs-writer",
		Description:  "Write summary: " + key,
		Prompt: strings.Join([]string{
			"TICKET_KEY: " + key,
			"REPO_ROOT: " + c.Root,
			"WORKTREE_DIR: " + wt,
			"TICKET_TITLE: " + title,
			"BRANCH: " + br,
			"BASE_BRANCH: " + baseBranch,
			"TASK_BRIEF_PATH: " + briefPath,
			"",
			fmt.Sprintf(`Run "git diff origin/%s...HEAD" from WORKTREE_DIR.`, baseBranch),
			"Write under ${WORKTREE_DIR}/docs/changes/.",
			"Also copy to " + state.TicketDir(c.Root, key) + "/IMPL-SUMMARY.md.",
			"Commit and push from WORKTREE_DIR.",
			"",
			"Follow your role definition.",
		}, "\n"),
		ParseReturn: "## DOCS",
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 11 — Create PR
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step11(key string, progress *state.Progress) *Instruction {
	wt := progress.Worktree
	br := progress.Branch
	cfg, _ := worktree.ReadConfig(c.Root)
	baseBranch := "main"
	if cfg != nil {
		worktree.AutoDetect(cfg, c.Root)
		baseBranch = cfg.BaseBranch
	}
	briefPath := filepath.Join(state.TicketDir(c.Root, key), "task-brief.md")
	meta, _ := ParseBrief(briefPath)
	title := meta["title"]
	if title == "" {
		title = key
	}

	prTitle := fmt.Sprintf("feat: %s [%s]", title, key)
	prBody := fmt.Sprintf("## Summary\n\nDelivered via autoflow-deliver.\n\nTicket: %s\n\n## Test plan\n- [ ] E2E tests passed locally", key)
	commands := []string{
		fmt.Sprintf(`cd "%s"`, wt),
		fmt.Sprintf(`git push -u origin %s`, shellQuote(br)),
		fmt.Sprintf(`gh pr create --base %s --title %s --body %s`,
			shellQuote(baseBranch), shellQuote(prTitle), shellQuote(prBody)),
		`PR_URL=$(gh pr view --json url -q .url)`,
		`echo "PR_URL=$PR_URL"`,
	}

	return &Instruction{
		Action:         ActionBash,
		Description:    "Create PR for " + key,
		Commands:       commands,
		OnFailure:      "escalate",
		Extract:        map[string]string{"pr_url": "PR_URL"},
		PassToComplete: "--pr-url <extracted-pr-url>",
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 12 — Generate delivery reports
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step12(key string, progress *state.Progress) *Instruction {
	br := progress.Branch
	pr := derefOr(progress.PRURL, "")
	qKey := shellQuote(key)
	qBranch := shellQuote(br)
	qPR := shellQuote(pr)

	commands := []string{
		fmt.Sprintf(`cd "%s"`, c.Root),
		fmt.Sprintf(`%s deliver _verify-gates --ticket %s`, autoflowCmd(), qKey),
		fmt.Sprintf(`%s deliver _report --ticket %s --branch %s --pr-url %s`,
			autoflowCmd(), qKey, qBranch, qPR),
		fmt.Sprintf(
			`PR_NUMBER=$(gh pr view "%s" --json number -q .number 2>/dev/null || echo "")`,
			br,
		),
		fmt.Sprintf(
			`[ -n "$PR_NUMBER" ] && [ -f ".autoflow/ticket/%s/PR-BODY.md" ] && gh pr edit "$PR_NUMBER" --body "$(cat ".autoflow/ticket/%s/PR-BODY.md")" || true`,
			key, key,
		),
	}

	return &Instruction{
		Action:      ActionBash,
		Description: "Generate delivery reports for " + key,
		Commands:    commands,
		OnFailure:   "escalate",
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Step 13 — Jira update + upload artifacts
// ═══════════════════════════════════════════════════════════════════════

func (c *Controller) step13(key string, progress *state.Progress) *Instruction {
	return &Instruction{
		Action:      ActionBash,
		Description: "Update Jira for " + key,
		Commands: []string{
			fmt.Sprintf(`cd "%s"`, c.Root),
			fmt.Sprintf(`%s jira upload %s ".autoflow/ticket/%s/EXECUTION-REPORT.md"`,
				autoflowCmd(), shellQuote(key), key),
			`echo "Jira updated. Transitioned via REST (see jira-transitions reference)."`,
		},
		OnFailure: "escalate",
		PostActions: []PostAction{{
			Action:         "jira_transition",
			Ticket:         key,
			FromStatus:     "In Development",
			ToStatus:       "In Code Review",
			TransitionName: "Dev Done",
		}},
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════

// shellQuote wraps s in single quotes, escaping any embedded single
// quotes. Suitable for POSIX shells. Internal to the step JSON output.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// derefOr safely dereferences a *string with a fallback.
func derefOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

// rawLoopState lets us read the shared loop-state shape without coupling
// to the state/loop.go types.
type rawLoopState struct {
	Loop      string            `json:"loop"`
	Ticket    string            `json:"ticket"`
	MaxRounds int               `json:"max_rounds"`
	Rounds    []json.RawMessage `json:"rounds"`
}

func readLoopStateRaw(path string) (*rawLoopState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &rawLoopState{}, nil
		}
		return nil, err
	}
	var s rawLoopState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// statusOf extracts the string status field from a raw round body.
func statusOf(round json.RawMessage) string {
	var probe struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(round, &probe)
	return probe.Status
}

// stringOrNil extracts an optional string field from a raw round body.
func stringOrNil(round json.RawMessage, field string) string {
	m := map[string]any{}
	_ = json.Unmarshal(round, &m)
	if v, ok := m[field]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// countReviewFindings reads a REVIEW-*.md frontmatter and returns
// (critical, warning) counts. Used by step_09 to decide whether findings
// warrant a fixer pass.
func countReviewFindings(path string) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	content := string(data)
	// Locate the frontmatter block.
	const delim = "---"
	start := strings.Index(content, delim)
	if start == -1 {
		return 0, 0
	}
	tail := content[start+len(delim):]
	end := strings.Index(tail, delim)
	if end == -1 {
		return 0, 0
	}
	fm := tail[:end]

	// Very small ad-hoc parser — we only need the two numeric keys.
	critical, warning := 0, 0
	inFindings := false
	for _, line := range strings.Split(fm, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "findings:") {
			inFindings = true
			continue
		}
		if inFindings && (trimmed == "" || !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t")) {
			if trimmed == "" {
				continue
			}
			inFindings = false
		}
		if !inFindings {
			continue
		}
		if strings.HasPrefix(trimmed, "critical:") {
			critical = parseInt(strings.TrimSpace(strings.TrimPrefix(trimmed, "critical:")))
		}
		if strings.HasPrefix(trimmed, "warning:") {
			warning = parseInt(strings.TrimSpace(strings.TrimPrefix(trimmed, "warning:")))
		}
	}
	return critical, warning
}

func parseInt(s string) int {
	s = strings.Trim(s, `"' `)
	n, _ := strconv.Atoi(s)
	return n
}
