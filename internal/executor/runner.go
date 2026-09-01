package executor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/liemle3893/go-tryve/internal/adapter"
	"github.com/liemle3893/go-tryve/internal/interpolate"
	"github.com/liemle3893/go-tryve/internal/reporter"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// phaseEntry groups a phase identifier with its ordered step list.
type phaseEntry struct {
	phase tryve.TestPhase
	steps []tryve.StepDefinition
}

// RunTest executes a single test through all lifecycle phases (setup, execute,
// verify, teardown) and returns an aggregated TestResult.
//
// Parameters:
//   - ctx            – parent context; a child deadline is created when td.Timeout > 0.
//   - td             – the parsed test definition to execute.
//   - registry       – adapter registry used to resolve step adapters.
//   - rep            – reporter that receives lifecycle events.
//   - defaultRetries – retry count used when td.Retries is not set (0 = no retries).
//   - defaultRetryDelay – base retry back-off delay in milliseconds.
//   - baseURL        – base URL that {{baseUrl}} resolves to.
//   - configVars     – suite-level variables, overridden by the test's own.
//   - strictResolve  – fail a step whose {{…}} expression cannot be resolved
//     instead of passing the raw token through to the system under test.
//   - compat         – which behaviours use their current semantics; the zero
//     value keeps every area on its pre-v2 behaviour.
//
// If td.Skip is true the function returns immediately with StatusSkipped without
// calling any reporter methods beyond OnTestStart/OnTestComplete.
func RunTest(
	ctx context.Context,
	td *tryve.TestDefinition,
	registry *adapter.Registry,
	rep reporter.Reporter,
	defaultRetries int,
	defaultRetryDelay int,
	baseURL string,
	configVars map[string]any,
	strictResolve bool,
	compat tryve.CompatMode,
) *tryve.TestResult {
	// 1. Early-return for skipped tests.
	if td.Skip {
		result := &tryve.TestResult{
			Test:   td,
			Status: tryve.StatusSkipped,
		}
		return result
	}

	// 2. Apply per-test timeout when configured.
	runCtx := ctx
	if td.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(td.Timeout)*time.Millisecond)
		defer cancel()
	}

	// 3. Notify reporter that the test is starting.
	_ = rep.OnTestStart(runCtx, td)

	start := time.Now()

	// result is set by the main body below; the deferred closure guarantees
	// OnTestComplete fires on every exit path (including early failures).
	var result *tryve.TestResult
	defer func() {
		if result != nil {
			_ = rep.OnTestComplete(runCtx, td, result)
		}
	}()

	// 4. Build the interpolation context seeded with config + test variables.
	interpCtx := tryve.NewInterpolationContext()
	interpCtx.BaseURL = baseURL
	interpCtx.Strict = strictResolve
	// A test file may declare its own apiVersion, so a suite can move to tryve/v2
	// while individual files stay on tryve/v1 until they are worked through.
	if td.APIVersion != nil || td.Compatibility != nil {
		fileCompat, compatErr := tryve.ResolveLevel(td.APIVersion, td.Compatibility)
		if compatErr != nil {
			result = &tryve.TestResult{
				Test:   td,
				Status: tryve.StatusFailed,
				Error:  fmt.Errorf("in %s: %w", td.SourceFile, compatErr),
			}
			return result
		}
		compat = fileCompat
	}
	interpCtx.Compat = compat

	// Populate environment variables from the process.
	for _, entry := range os.Environ() {
		if k, v, ok := strings.Cut(entry, "="); ok {
			interpCtx.Env[k] = v
		}
	}

	// Config-level variables (lower priority than test-level).
	for k, v := range configVars {
		interpCtx.Variables[k] = v
	}

	// Test-level variables override config variables.
	for k, v := range td.Variables {
		interpCtx.Variables[k] = v
	}

	// Resolve variable cross-references and built-in functions (e.g. $uuid()) once
	// so all phases see the same generated values.
	if len(interpCtx.Variables) > 0 {
		resolved, err := interpolate.ResolveVariables(interpCtx.Variables, interpCtx)
		if err != nil {
			result = &tryve.TestResult{
				Test:     td,
				Status:   tryve.StatusFailed,
				Duration: time.Since(start),
				Error:    fmt.Errorf("variable resolution failed: %w", err),
			}
			return result
		}
		interpCtx.Variables = resolved
	}

	// 5. Resolve retry settings.
	// td.Retries == -1 means "not set, use default".
	// td.Retries == 0 means "explicitly no retries".
	maxRetries := defaultRetries
	if td.Retries >= 0 {
		maxRetries = td.Retries
	}
	baseDelay := time.Duration(defaultRetryDelay) * time.Millisecond

	// 6. Execute phases in canonical order.
	phases := []phaseEntry{
		{tryve.PhaseSetup, td.Setup},
		{tryve.PhaseExecute, td.Execute},
		{tryve.PhaseVerify, td.Verify},
		{tryve.PhaseTeardown, td.Teardown},
	}

	var (
		steps  []tryve.StepOutcome
		failed bool
		runErr error
	)

	for _, pe := range phases {
		if len(pe.steps) == 0 {
			continue
		}

		// Skip non-teardown phases when a previous phase has already failed.
		if failed && pe.phase != tryve.PhaseTeardown {
			continue
		}

		for i := range pe.steps {
			step := &pe.steps[i]

			// A step-level skip was ignored before the execution area changed.
			if step.Skip && interpCtx.Compat.Modern(tryve.CompatExecution) {
				outcome := &tryve.StepOutcome{Step: step, Phase: pe.phase, Status: tryve.StatusSkipped}
				steps = append(steps, *outcome)
				_ = rep.OnStepComplete(runCtx, step, outcome)
				continue
			}

			outcome, _ := ExecuteStepWithRetry(runCtx, step, registry, interpCtx, maxRetries, baseDelay)

			// Stamp the phase on the outcome so callers can inspect it.
			outcome.Phase = pe.phase

			steps = append(steps, *outcome)
			_ = rep.OnStepComplete(runCtx, step, outcome)

			if outcome.Status == tryve.StatusFailed {
				// Record the first failure error.
				if runErr == nil {
					runErr = outcome.Error
				}
				failed = true

				// In teardown, continue executing remaining steps despite failure.
				if pe.phase == tryve.PhaseTeardown {
					continue
				}
				// In any other phase, stop processing further steps in this phase.
				break
			}
		}
	}

	// 7. Determine final status.
	status := tryve.StatusPassed
	if failed {
		status = tryve.StatusFailed
	}

	result = &tryve.TestResult{
		Test:     td,
		Status:   status,
		Duration: time.Since(start),
		Steps:    steps,
		Error:    runErr,
	}

	return result
}
