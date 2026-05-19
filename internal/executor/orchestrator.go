package executor

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/liemle3893/autoflow/internal/adapter"
	"github.com/liemle3893/autoflow/internal/config"
	"github.com/liemle3893/autoflow/internal/reporter"
	"github.com/liemle3893/autoflow/internal/core"
)

// Orchestrator coordinates parallel test execution with dependency ordering,
// bail-on-failure support, and lifecycle hook invocation.
type Orchestrator struct {
	registry *adapter.Registry
	reporter reporter.Reporter
	config   *config.LoadedConfig
	bail     bool
}

// NewOrchestrator constructs an Orchestrator using the given adapter registry,
// reporter, and loaded configuration.
func NewOrchestrator(
	registry *adapter.Registry,
	rep reporter.Reporter,
	cfg *config.LoadedConfig,
) *Orchestrator {
	return &Orchestrator{
		registry: registry,
		reporter: rep,
		config:   cfg,
	}
}

// SetBail configures whether the orchestrator stops scheduling new tests after
// the first failure.  Must be called before Run.
func (o *Orchestrator) SetBail(bail bool) {
	o.bail = bail
}

// Run executes all provided tests, honouring dependency ordering, parallelism
// limits, bail settings, and suite lifecycle hooks.
//
// The returned SuiteResult contains aggregated pass/fail/skip counts and the
// individual TestResult for each test.
func (o *Orchestrator) Run(ctx context.Context, tests []*core.TestDefinition) *core.SuiteResult {
	start := time.Now()
	defaults := o.config.Defaults

	// Initialise the suite result passed to reporter events.
	suite := &core.SuiteResult{}
	_ = o.reporter.OnSuiteStart(ctx, suite)

	// Run beforeAll hook; a failure is logged but does not abort the suite so
	// that the orchestrator always produces a valid SuiteResult.
	_ = RunHook(ctx, o.config.Hooks.BeforeAll, "", nil)

	// Topological sort respects `depends` declarations.
	sorted := topoSortTests(tests)

	// Shared mutable state protected by mu.
	var mu sync.Mutex
	bailed := false
	results := make([]core.TestResult, 0, len(sorted))
	completedStatus := make(map[string]core.TestStatus, len(sorted))

	// doneCh is closed when a particular test name finishes; waiters poll it.
	doneCh := make(map[string]chan struct{}, len(sorted))
	for _, td := range sorted {
		doneCh[td.Name] = make(chan struct{})
	}

	parallel := defaults.Parallel
	if parallel <= 0 {
		parallel = 1
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(parallel)

	for _, td := range sorted {
		td := td // capture loop variable

		g.Go(func() error {
			// Check bail flag before starting any work.
			mu.Lock()
			shouldBail := bailed
			mu.Unlock()
			if shouldBail {
				res := skipResult(td)
				mu.Lock()
				results = append(results, *res)
				completedStatus[td.Name] = core.StatusSkipped
				close(doneCh[td.Name])
				mu.Unlock()
				return nil
			}

			// Wait for every declared dependency to complete.
			for _, dep := range td.Depends {
				ch, exists := doneCh[dep]
				if !exists {
					// Unknown dependency — treat as missing (skip this test).
					res := skipResult(td)
					mu.Lock()
					results = append(results, *res)
					completedStatus[td.Name] = core.StatusSkipped
					close(doneCh[td.Name])
					mu.Unlock()
					return nil
				}
				select {
				case <-ch:
					// Dependency finished; check its outcome.
				case <-gCtx.Done():
					res := skipResult(td)
					mu.Lock()
					results = append(results, *res)
					completedStatus[td.Name] = core.StatusSkipped
					close(doneCh[td.Name])
					mu.Unlock()
					return nil
				}
				mu.Lock()
				depStatus := completedStatus[dep]
				mu.Unlock()
				if depStatus == core.StatusFailed {
					// A required dependency failed; skip this test.
					res := skipResult(td)
					mu.Lock()
					results = append(results, *res)
					completedStatus[td.Name] = core.StatusSkipped
					close(doneCh[td.Name])
					mu.Unlock()
					return nil
				}
			}

			// Run beforeEach hook (failure is non-fatal to keep results consistent).
			_ = RunHook(gCtx, o.config.Hooks.BeforeEach, "", nil)

			// Execute the test.
			res := RunTest(
				gCtx,
				td,
				o.registry,
				o.reporter,
				defaults.Retries,
				defaults.RetryDelay,
				o.config.Environment.BaseURL,
				o.config.Variables,
			)

			// Run afterEach hook regardless of test outcome.
			_ = RunHook(gCtx, o.config.Hooks.AfterEach, "", nil)

			mu.Lock()
			results = append(results, *res)
			completedStatus[td.Name] = res.Status
			close(doneCh[td.Name])
			if res.Status == core.StatusFailed && o.bail {
				bailed = true
			}
			mu.Unlock()

			return nil
		})
	}

	// Wait for all goroutines to finish; errors are not returned from the
	// goroutines themselves (test failures are captured in results).
	_ = g.Wait()

	// Run afterAll hook.
	_ = RunHook(ctx, o.config.Hooks.AfterAll, "", nil)

	// Compute suite totals.
	for _, r := range results {
		switch r.Status {
		case core.StatusPassed:
			suite.Passed++
		case core.StatusFailed:
			suite.Failed++
		case core.StatusSkipped:
			suite.Skipped++
		}
	}
	suite.Total = len(results)
	suite.Tests = results
	suite.Duration = time.Since(start)

	_ = o.reporter.OnSuiteComplete(ctx, suite, suite)
	_ = o.reporter.Flush()

	return suite
}

// skipResult creates a StatusSkipped TestResult for a test that was not run.
func skipResult(td *core.TestDefinition) *core.TestResult {
	return &core.TestResult{
		Test:   td,
		Status: core.StatusSkipped,
	}
}

// FilterOptions defines the criteria for selecting a subset of tests.
type FilterOptions struct {
	// Tags filters tests to those with at least one matching tag.
	Tags []string
	// Grep filters tests whose name matches the pattern (regex; fallback to substring).
	Grep string
	// Priority filters tests by exact priority string match.
	Priority string
	// Names restricts tests to those whose name appears in this set (used by --failed-only).
	Names map[string]struct{}
}

// FilterTests returns the subset of tests that satisfy all non-empty filter criteria.
// Multiple criteria are ANDed: a test must satisfy every specified filter.
func FilterTests(tests []*core.TestDefinition, opts FilterOptions) []*core.TestDefinition {
	out := make([]*core.TestDefinition, 0, len(tests))

	var grepRe *regexp.Regexp
	if opts.Grep != "" {
		re, err := regexp.Compile(opts.Grep)
		if err == nil {
			grepRe = re
		}
	}

	for _, td := range tests {
		if !matchesTags(td, opts.Tags) {
			continue
		}
		if opts.Grep != "" && !matchesGrep(td.Name, opts.Grep, grepRe) {
			continue
		}
		if opts.Priority != "" && string(td.Priority) != opts.Priority {
			continue
		}
		if len(opts.Names) > 0 {
			if _, ok := opts.Names[td.Name]; !ok {
				continue
			}
		}
		out = append(out, td)
	}
	return out
}

// matchesTags reports true when tags is empty or the test has at least one tag
// that appears in the filter list.
func matchesTags(td *core.TestDefinition, tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	for _, want := range tags {
		for _, have := range td.Tags {
			if have == want {
				return true
			}
		}
	}
	return false
}

// matchesGrep reports true when the test name matches the compiled regex or,
// if regex compilation failed, when the name contains the raw pattern as a substring.
func matchesGrep(name, raw string, re *regexp.Regexp) bool {
	if re != nil {
		return re.MatchString(name)
	}
	return strings.Contains(name, raw)
}

// topoSortTests performs a DFS-based topological sort so that tests with
// `depends` entries are always placed after the tests they depend on.
// Tests without dependencies retain their original relative order.
func topoSortTests(tests []*core.TestDefinition) []*core.TestDefinition {
	// Build a name→definition index.
	index := make(map[string]*core.TestDefinition, len(tests))
	for _, td := range tests {
		index[td.Name] = td
	}

	visited := make(map[string]bool, len(tests))
	sorted := make([]*core.TestDefinition, 0, len(tests))

	var visit func(td *core.TestDefinition)
	visit = func(td *core.TestDefinition) {
		if visited[td.Name] {
			return
		}
		visited[td.Name] = true
		for _, dep := range td.Depends {
			if depTD, ok := index[dep]; ok {
				visit(depTD)
			}
		}
		sorted = append(sorted, td)
	}

	for _, td := range tests {
		visit(td)
	}
	return sorted
}
