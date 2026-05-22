// Package doctor implements `autoflow deliver doctor` — a preflight
// checklist that surfaces the common misconfigurations the autoflow
// agents will otherwise hit mid-run. Every check is standalone; the
// aggregator prints a table and exits with a status that reflects the
// worst outcome.
package doctor

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Status is one of three outcomes per check.
type Status string

const (
	OK   Status = "OK"
	Fail Status = "FAIL"
	Warn Status = "WARN"
	Skip Status = "SKIP"
)

// Result is one row of the doctor report.
type Result struct {
	Name   string // short label printed in the leftmost column
	Status Status
	Detail string // one-line explanation; empty when OK
}

// Checker runs one check and returns one Result. Checkers should never
// return an error — any issue should be folded into Status=FAIL/WARN.
type Checker func(ctx context.Context) Result

// Opts control the doctor run. Root is required for checks that read
// repo-local config.
type Opts struct {
	Root    string
	Timeout time.Duration // per-check timeout; defaults to 5s when zero
}

// Run executes the standard nine-check battery in order. Returns the
// overall worst status (OK < WARN < FAIL) and the per-check results.
func Run(ctx context.Context, opts Opts) (Status, []Result) {
	return RunCheckers(ctx, opts, StandardChecks(opts))
}

// RunCheckers is exposed for tests so they can inject deterministic
// checkers instead of the StandardChecks battery.
func RunCheckers(ctx context.Context, opts Opts, checkers []Checker) (Status, []Result) {
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}
	results := make([]Result, 0, len(checkers))
	worst := OK
	for _, c := range checkers {
		cctx, cancel := context.WithTimeout(ctx, opts.Timeout)
		r := c(cctx)
		cancel()
		results = append(results, r)
		worst = worstOf(worst, r.Status)
	}
	return worst, results
}

// Format prints a human-readable table to w. Each row is
// `<STATUS>  <name>  <detail>`. Widths match the longest name so the
// columns align.
func Format(w io.Writer, results []Result) {
	nameWidth := 0
	for _, r := range results {
		if len(r.Name) > nameWidth {
			nameWidth = len(r.Name)
		}
	}
	for _, r := range results {
		fmt.Fprintf(w, "%-4s  %-*s  %s\n", r.Status, nameWidth, r.Name, r.Detail)
	}
}

func worstOf(a, b Status) Status {
	// Skip ranks below OK — it never promotes the worst-so-far.
	rank := map[Status]int{Skip: -1, OK: 0, Warn: 1, Fail: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// StandardChecks returns the default nine-check battery in the order
// they should run. Order matters: git/gh first, then env/config, then
// network, then install-layout checks.
func StandardChecks(opts Opts) []Checker {
	return []Checker{
		checkBinary("git", "git --version exits 0"),
		checkGH,
		checkJIRAToken,
		checkJIRAConfig(opts.Root),
		checkJIRAReachable(opts.Root),
		checkBootstrap(opts.Root),
		checkSkillsInstalled(opts.Root),
		checkAgentsInstalled(opts.Root),
		checkNoLegacyScripts(opts.Root),
		checkCodingAgent(opts.Root),
		checkSbxInstalled(opts.Root),
		checkSbxVersion(opts.Root),
		checkSbxDocker(opts.Root),
		checkSbxAuth(opts.Root),
		checkSbxAgentCreds(opts.Root),
		checkSbxAutoflow(opts.Root),
	}
}

// ExitCode maps a worst status to the process exit code. 0 for OK, 1 for
// FAIL, 2 for WARN-only.
func ExitCode(worst Status) int {
	switch worst {
	case Fail:
		return 1
	case Warn:
		return 2
	default:
		return 0
	}
}
