package executor

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/liemle3893/go-tryve/internal/adapter"
	"github.com/liemle3893/go-tryve/internal/assertion"
	"github.com/liemle3893/go-tryve/internal/interpolate"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

const maxBackoffDelay = 30 * time.Second

// failedOutcome constructs a StatusFailed StepOutcome for the given step and error.
// duration is the elapsed time since step execution began (including any pre-delay).
func failedOutcome(step *tryve.StepDefinition, err error, duration time.Duration) *tryve.StepOutcome {
	return &tryve.StepOutcome{
		Step:     step,
		Status:   tryve.StatusFailed,
		Error:    err,
		Duration: duration,
	}
}

// warnedOutcome constructs a StatusWarned StepOutcome with assertion results attached.
// It is used when continueOnError is true and a failure would otherwise block the test.
func warnedOutcome(
	step *tryve.StepDefinition,
	result *tryve.StepResult,
	outcomes []tryve.AssertionOutcome,
	err error,
	duration time.Duration,
) *tryve.StepOutcome {
	return &tryve.StepOutcome{
		Step:       step,
		Status:     tryve.StatusWarned,
		Result:     result,
		Assertions: outcomes,
		Error:      err,
		Duration:   duration,
	}
}

// passedOutcome constructs a StatusPassed StepOutcome with assertion results attached.
func passedOutcome(
	step *tryve.StepDefinition,
	result *tryve.StepResult,
	outcomes []tryve.AssertionOutcome,
	duration time.Duration,
) *tryve.StepOutcome {
	return &tryve.StepOutcome{
		Step:       step,
		Status:     tryve.StatusPassed,
		Result:     result,
		Assertions: outcomes,
		Duration:   duration,
	}
}

// ExecuteStep runs a single step through the full pipeline:
// pre-delay → interpolation → adapter execution → capture → assertions.
//
// The returned outcome always includes the elapsed Duration from step start
// (including any pre-delay). An error is only returned for internal/unexpected
// failures; step-level failures (assertion, execution) are surfaced via the
// outcome status instead.
func ExecuteStep(
	ctx context.Context,
	step *tryve.StepDefinition,
	registry *adapter.Registry,
	interpCtx *tryve.InterpolationContext,
) (*tryve.StepOutcome, error) {
	start := time.Now()

	// 1. Pre-delay: honour step.Delay (milliseconds), respect context cancellation.
	if step.Delay > 0 {
		delay := time.Duration(step.Delay) * time.Millisecond
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			elapsed := time.Since(start)
			return failedOutcome(step, ctx.Err(), elapsed), nil
		}
	}

	// 2. Interpolate params.
	resolvedParams, err := interpolate.ResolveMap(step.Params, interpCtx)
	if err != nil {
		elapsed := time.Since(start)
		return failedOutcome(step, tryve.InterpolationError(step.Action, err.Error()), elapsed), nil
	}

	// Inject step.Name into resolvedParams so adapters that require it (e.g. process)
	// can read it from params after the parser extracts it as a first-class field.
	if step.Name != "" {
		resolvedParams["name"] = step.Name
	}

	// 3. Get adapter from registry (connects lazily on first access).
	adp, err := registry.Get(ctx, step.Adapter)
	if err != nil {
		elapsed := time.Since(start)
		return failedOutcome(step, err, elapsed), nil
	}

	// Store resolved params for debug display.
	storeResolved := func(o *tryve.StepOutcome) *tryve.StepOutcome {
		o.ResolvedParams = resolvedParams
		return o
	}

	// 4. Execute adapter action.
	result, execErr := adp.Execute(ctx, step.Action, resolvedParams)
	elapsed := time.Since(start)

	// 5. Capture: extract JSONPath values even if execution returned an error,
	//    as long as there is result data (e.g. shell stdout before non-zero exit).
	if step.Capture != nil && result != nil && result.Data != nil {
		// For HTTP adapter, capture paths evaluate against the response body
		// (matching TS e2e-runner behavior where $.id means response.body.id).
		captureData := result.Data
		if step.Adapter == "http" {
			if body, ok := result.Data["body"]; ok {
				if bodyMap, ok := body.(map[string]any); ok {
					captureData = bodyMap
				}
			}
		}
		for varName, path := range step.Capture {
			val, _ := assertion.EvalJSONPath(captureData, path)
			if step.Name != "" {
				nested, _ := interpCtx.Captured[step.Name].(map[string]any)
				if nested == nil {
					nested = make(map[string]any)
				}
				nested[varName] = val
				interpCtx.Captured[step.Name] = nested
			} else {
				interpCtx.Captured[varName] = val
			}
		}
	}

	// If adapter returned an error, fail the step (but keep result for debug output).
	if execErr != nil {
		if step.ContinueOnError {
			return warnedOutcome(step, result, nil, execErr, elapsed), nil
		}
		o := failedOutcome(step, execErr, elapsed)
		o.Result = result // preserve for --debug output
		return o, nil
	}

	// 6. Assert: evaluate assertion definitions against result data.
	//    Assertions may reference interpolated values (e.g. equals: "{{captured.id}}"),
	//    so they must be resolved first.
	var assertionOutcomes []tryve.AssertionOutcome
	if step.Assert != nil && result != nil && result.Data != nil {
		resolvedAssert, _ := resolveAssertDef(step.Assert, interpCtx)
		outcomes, err := assertion.RunAssertions(result.Data, resolvedAssert)
		if err != nil {
			elapsed = time.Since(start)
			if step.ContinueOnError {
				return warnedOutcome(step, result, nil, err, elapsed), nil
			}
			return failedOutcome(step, err, elapsed), nil
		}
		assertionOutcomes = outcomes

		// Check whether any assertion failed.
		for _, o := range outcomes {
			if !o.Passed {
				elapsed = time.Since(start)
				assertErr := tryve.AssertionError(o.Path, o.Operator, o.Expected, o.Actual)
				if step.ContinueOnError {
					return warnedOutcome(step, result, outcomes, assertErr, elapsed), nil
				}
				return &tryve.StepOutcome{
					Step:       step,
					Status:     tryve.StatusFailed,
					Result:     result,
					Assertions: outcomes,
					Error:      assertErr,
					Duration:   elapsed,
				}, nil
			}
		}
	}

	// 7. Shell exit code check: if a shell command exited non-zero and there was
	//    no explicit exitCode assertion, treat it as a step failure.
	if step.Adapter == "shell" && result != nil && result.Data != nil {
		if exitCode, ok := result.Data["exitCode"].(float64); ok && exitCode != 0 {
			if !hasExitCodeAssertion(step.Assert) {
				elapsed = time.Since(start)
				stderr, _ := result.Data["stderr"].(string)
				if len(stderr) > 200 {
					stderr = stderr[:200] + "..."
				}
				execErr := tryve.ExecutionError(step.ID,
					fmt.Sprintf("command exited with code %d: %s", int(exitCode), stderr), nil)
				if step.ContinueOnError {
					return warnedOutcome(step, result, assertionOutcomes, execErr, elapsed), nil
				}
				o := failedOutcome(step, execErr, elapsed)
				o.Result = result
				return o, nil
			}
		}
	}

	// 8. Success.
	elapsed = time.Since(start)
	return storeResolved(passedOutcome(step, result, assertionOutcomes, elapsed)), nil
}

// backoffDelay computes exponential backoff with ~15% jitter.
// Formula: base * 2^attempt, capped at maxBackoffDelay.
func backoffDelay(base time.Duration, attempt int) time.Duration {
	exp := math.Pow(2, float64(attempt))
	d := time.Duration(float64(base) * exp)
	if d > maxBackoffDelay {
		d = maxBackoffDelay
	}
	// Add up to 15% jitter.
	jitter := time.Duration(float64(d) * 0.15 * rand.Float64())
	return d + jitter
}

// ExecuteStepWithRetry wraps ExecuteStep with configurable retry logic.
//
// It attempts the step up to maxRetries+1 times. On a passing or warned outcome
// it returns immediately. On failure it waits with exponential backoff before
// retrying. If the context is cancelled during backoff the current (failed)
// outcome is returned along with the retry count so far.
//
// Returns the final StepOutcome and the number of retries actually performed
// (0 on first-attempt success).
func ExecuteStepWithRetry(
	ctx context.Context,
	step *tryve.StepDefinition,
	registry *adapter.Registry,
	interpCtx *tryve.InterpolationContext,
	maxRetries int,
	baseDelay time.Duration,
) (*tryve.StepOutcome, int) {
	var outcome *tryve.StepOutcome
	retries := 0

	for attempt := 0; attempt <= maxRetries; attempt++ {
		var err error
		outcome, err = ExecuteStep(ctx, step, registry, interpCtx)
		// ExecuteStep only returns a non-nil err for unexpected internal failures;
		// step-level failures are encoded in outcome.Status. Treat both as failure.
		if err != nil {
			outcome = failedOutcome(step, err, 0)
		}

		// Return immediately on success or warned (continueOnError) outcomes.
		if outcome.Status == tryve.StatusPassed || outcome.Status == tryve.StatusWarned {
			return outcome, retries
		}

		// No more retries remaining — return the last failed outcome.
		if attempt == maxRetries {
			break
		}

		// Wait with exponential backoff, honouring context cancellation.
		delay := backoffDelay(baseDelay, attempt)
		select {
		case <-time.After(delay):
			retries++
		case <-ctx.Done():
			return outcome, retries
		}
	}

	return outcome, retries
}

// resolveAssertDef interpolates values inside assertion definitions.
func resolveAssertDef(assertDef any, ctx *tryve.InterpolationContext) (any, error) {
	switch def := assertDef.(type) {
	case map[string]any:
		return interpolate.ResolveMap(def, ctx)
	case []any:
		return interpolate.ResolveSlice(def, ctx)
	default:
		return assertDef, nil
	}
}

// hasExitCodeAssertion checks whether the step's assert definition includes
// an exitCode check (meaning the test author explicitly handles exit codes).
func hasExitCodeAssertion(assertDef any) bool {
	switch def := assertDef.(type) {
	case map[string]any:
		if _, ok := def["exitCode"]; ok {
			return true
		}
	case []any:
		for _, item := range def {
			if m, ok := item.(map[string]any); ok {
				if path, ok := m["path"].(string); ok && path == "$.exitCode" {
					return true
				}
			}
		}
	}
	return false
}
