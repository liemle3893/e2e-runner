package executor

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/liemle3893/go-tryve/internal/adapter"
	"github.com/liemle3893/go-tryve/internal/config"
	"github.com/liemle3893/go-tryve/internal/reporter"
	"github.com/liemle3893/go-tryve/internal/tryve"
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
func (o *Orchestrator) Run(ctx context.Context, tests []*tryve.TestDefinition) *tryve.SuiteResult {
	start := time.Now()
	defaults := o.config.Defaults

	// Initialise the suite result passed to reporter events.
	suite := &tryve.SuiteResult{}
	_ = o.reporter.OnSuiteStart(ctx, suite)

	// Run beforeAll hook; a failure is logged but does not abort the suite so
	// that the orchestrator always produces a valid SuiteResult.
	_ = RunHook(ctx, o.config.Hooks.BeforeAll, "", nil)

	// Topological sort respects `depends` declarations.
	sorted := topoSortTests(tests)

	// Shared mutable state protected by mu.
	var mu sync.Mutex
	bailed := false
	results := make([]tryve.TestResult, 0, len(sorted))
	completedStatus := make(map[string]tryve.TestStatus, len(sorted))

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
				completedStatus[td.Name] = tryve.StatusSkipped
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
					completedStatus[td.Name] = tryve.StatusSkipped
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
					completedStatus[td.Name] = tryve.StatusSkipped
					close(doneCh[td.Name])
					mu.Unlock()
					return nil
				}
				mu.Lock()
				depStatus := completedStatus[dep]
				mu.Unlock()
				if depStatus == tryve.StatusFailed {
					// A required dependency failed; skip this test.
					res := skipResult(td)
					mu.Lock()
					results = append(results, *res)
					completedStatus[td.Name] = tryve.StatusSkipped
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
				o.config.Defaults.StrictResolve,
				o.config.Compat,
			)

			// Run afterEach hook regardless of test outcome.
			_ = RunHook(gCtx, o.config.Hooks.AfterEach, "", nil)

			mu.Lock()
			results = append(results, *res)
			completedStatus[td.Name] = res.Status
			close(doneCh[td.Name])
			if res.Status == tryve.StatusFailed && o.bail {
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
		case tryve.StatusPassed:
			suite.Passed++
		case tryve.StatusFailed:
			suite.Failed++
		case tryve.StatusSkipped:
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
func skipResult(td *tryve.TestDefinition) *tryve.TestResult {
	return &tryve.TestResult{
		Test:   td,
		Status: tryve.StatusSkipped,
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
	// Names, when non-empty, restricts tests to those whose name is in the set.
	Names map[string]struct{}
}

// FilterTests returns the subset of tests that satisfy all non-empty filter criteria.
// Multiple criteria are ANDed: a test must satisfy every specified filter.
func FilterTests(tests []*tryve.TestDefinition, opts FilterOptions) []*tryve.TestDefinition {
	out := make([]*tryve.TestDefinition, 0, len(tests))

	var grepRe *regexp.Regexp
	if opts.Grep != "" {
		re, err := regexp.Compile(opts.Grep)
		if err == nil {
			grepRe = re
		}
	}

	for _, td := range tests {
		if len(opts.Names) > 0 {
			if _, ok := opts.Names[td.Name]; !ok {
				continue
			}
		}
		if !matchesTags(td, opts.Tags) {
			continue
		}
		if opts.Grep != "" && !matchesGrep(td.Name, opts.Grep, grepRe) {
			continue
		}
		if opts.Priority != "" && string(td.Priority) != opts.Priority {
			continue
		}
		out = append(out, td)
	}
	return out
}

// matchesTags reports true when tags is empty or the test has at least one tag
// that appears in the filter list.
func matchesTags(td *tryve.TestDefinition, tags []string) bool {
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
func topoSortTests(tests []*tryve.TestDefinition) []*tryve.TestDefinition {
	// Build a name→definition index.
	index := make(map[string]*tryve.TestDefinition, len(tests))
	for _, td := range tests {
		index[td.Name] = td
	}

	visited := make(map[string]bool, len(tests))
	sorted := make([]*tryve.TestDefinition, 0, len(tests))

	var visit func(td *tryve.TestDefinition)
	visit = func(td *tryve.TestDefinition) {
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
