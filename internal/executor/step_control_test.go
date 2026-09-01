package executor_test

import (
	"context"
	"testing"
	"time"

	"github.com/liemle3893/go-tryve/internal/adapter"
	"github.com/liemle3893/go-tryve/internal/executor"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// shellRegistry returns a registry with only the shell adapter registered.
func shellRegistry() *adapter.Registry {
	reg := adapter.NewRegistry()
	reg.Register("shell", adapter.NewShellAdapter(&adapter.ShellConfig{}))
	return reg
}

// modernCtx returns an interpolation context with every compatibility area on
// its current behaviour.
func modernCtx() *tryve.InterpolationContext {
	ctx := tryve.NewInterpolationContext()
	ctx.Compat = tryve.ModernCompat()
	return ctx
}

// TestStepTimeoutFailsTheStep covers the per-step timeout. It was previously
// parsed into the params map and ignored, so a step declaring `timeout: 500`
// ran for as long as the command took.
func TestStepTimeoutFailsTheStep(t *testing.T) {
	step := &tryve.StepDefinition{
		ID:      "sleeper",
		Adapter: "shell",
		Action:  "exec",
		Timeout: 400,
		Params:  map[string]any{"command": "sleep 5"},
	}

	start := time.Now()
	outcome, err := executor.ExecuteStep(context.Background(), step, shellRegistry(),
		modernCtx())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected internal error: %v", err)
	}
	if outcome.Status != tryve.StatusFailed {
		t.Errorf("expected the step to fail, got %s", outcome.Status)
	}
	if elapsed > 3*time.Second {
		t.Errorf("the step should have been cut short, took %s", elapsed)
	}
}

// TestShellExitCodeAssertionIsEvaluated covers the combination that used to make
// a failing command pass: `assert: {exitCode: 0}` suppressed the automatic
// non-zero-exit failure, and was itself never evaluated.
func TestShellExitCodeAssertionIsEvaluated(t *testing.T) {
	step := &tryve.StepDefinition{
		ID:      "fails",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": "echo out; exit 3"},
		Assert:  map[string]any{"exitCode": 0},
	}

	outcome, err := executor.ExecuteStep(context.Background(), step, shellRegistry(),
		modernCtx())
	if err != nil {
		t.Fatalf("unexpected internal error: %v", err)
	}
	if outcome.Status != tryve.StatusFailed {
		t.Fatalf("a command exiting 3 must fail `exitCode: 0`, got %s", outcome.Status)
	}
	if len(outcome.Assertions) == 0 {
		t.Errorf("the exitCode assertion should have been evaluated and recorded")
	}
}

// TestShellExitCodeAssertionCanExpectFailure checks that a command expected to
// fail still passes when the assertion says so.
func TestShellExitCodeAssertionCanExpectFailure(t *testing.T) {
	step := &tryve.StepDefinition{
		ID:      "expected-failure",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": "exit 2"},
		Assert:  map[string]any{"exitCode": 2},
	}

	outcome, err := executor.ExecuteStep(context.Background(), step, shellRegistry(),
		modernCtx())
	if err != nil {
		t.Fatalf("unexpected internal error: %v", err)
	}
	if outcome.Status != tryve.StatusPassed {
		t.Errorf("expected the step to pass, got %s (%v)", outcome.Status, outcome.Error)
	}
}

// TestStdoutAssertionIsEvaluated covers `assert: {stdout: {contains: …}}`.
func TestStdoutAssertionIsEvaluated(t *testing.T) {
	step := &tryve.StepDefinition{
		ID:      "greet",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": "echo BOTH_OK"},
		Assert: map[string]any{
			"exitCode": 0,
			"stdout":   map[string]any{"contains": "ABSENT"},
		},
	}

	outcome, err := executor.ExecuteStep(context.Background(), step, shellRegistry(),
		modernCtx())
	if err != nil {
		t.Fatalf("unexpected internal error: %v", err)
	}
	if outcome.Status != tryve.StatusFailed {
		t.Errorf("stdout that does not contain the expected text must fail, got %s", outcome.Status)
	}
}

// TestTypedComparisonAgainstCapturedNumber covers an interpolated expected value
// keeping its type, so a captured number compares equal to a number.
func TestTypedComparisonAgainstCapturedNumber(t *testing.T) {
	interpCtx := modernCtx()
	interpCtx.Captured["expected"] = float64(0)

	step := &tryve.StepDefinition{
		ID:      "compare",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": "true"},
		Assert:  map[string]any{"exitCode": "{{captured.expected}}"},
	}

	outcome, err := executor.ExecuteStep(context.Background(), step, shellRegistry(), interpCtx)
	if err != nil {
		t.Fatalf("unexpected internal error: %v", err)
	}
	if outcome.Status != tryve.StatusPassed {
		t.Errorf("a captured number should compare equal to a numeric result, got %s (%v)",
			outcome.Status, outcome.Error)
	}
}

// TestLegacyExecutionIgnoresStepControls pins the pre-v2 behaviour: a step's own
// timeout was parsed and ignored, and an exitCode assertion suppressed the
// automatic non-zero-exit failure without being evaluated.
func TestLegacyExecutionIgnoresStepControls(t *testing.T) {
	legacy := tryve.NewInterpolationContext() // zero CompatMode is legacy

	t.Run("timeout ignored", func(t *testing.T) {
		step := &tryve.StepDefinition{
			ID: "sleeper", Adapter: "shell", Action: "exec", Timeout: 200,
			Params: map[string]any{"command": "sleep 1"},
		}
		outcome, err := executor.ExecuteStep(context.Background(), step, shellRegistry(), legacy)
		if err != nil {
			t.Fatalf("unexpected internal error: %v", err)
		}
		if outcome.Status != tryve.StatusPassed {
			t.Errorf("legacy should run the command to completion, got %s (%v)",
				outcome.Status, outcome.Error)
		}
	})

	t.Run("exitCode assertion suppresses the failure", func(t *testing.T) {
		step := &tryve.StepDefinition{
			ID: "fails", Adapter: "shell", Action: "exec",
			Params: map[string]any{"command": "exit 3"},
			Assert: map[string]any{"exitCode": 0},
		}
		outcome, err := executor.ExecuteStep(context.Background(), step, shellRegistry(), legacy)
		if err != nil {
			t.Fatalf("unexpected internal error: %v", err)
		}
		if outcome.Status != tryve.StatusPassed {
			t.Errorf("legacy passed this step; got %s", outcome.Status)
		}
		if len(outcome.Assertions) != 0 {
			t.Errorf("legacy evaluated no assertion here, got %+v", outcome.Assertions)
		}
	})

	t.Run("non-zero exit still fails without an exitCode assertion", func(t *testing.T) {
		step := &tryve.StepDefinition{
			ID: "bare", Adapter: "shell", Action: "exec",
			Params: map[string]any{"command": "exit 3"},
		}
		outcome, _ := executor.ExecuteStep(context.Background(), step, shellRegistry(), legacy)
		if outcome.Status != tryve.StatusFailed {
			t.Errorf("a bare non-zero exit failed in both modes, got %s", outcome.Status)
		}
	})
}
