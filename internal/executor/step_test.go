package executor_test

import (
	"context"
	"testing"
	"time"

	"github.com/liemle3893/go-tryve/internal/adapter"
	"github.com/liemle3893/go-tryve/internal/executor"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// newTestRegistry builds a Registry with an HTTP adapter (pointed at baseURL)
// and a ShellAdapter for tests that do not require a live HTTP server.
func newTestRegistry(baseURL string) *adapter.Registry {
	r := adapter.NewRegistry()
	r.Register("http", adapter.NewHTTPAdapter(baseURL))
	r.Register("shell", adapter.NewShellAdapter(nil))
	return r
}

// newShellStep constructs a minimal StepDefinition for shell/exec with the
// given command.
func newShellStep(command string) *tryve.StepDefinition {
	return &tryve.StepDefinition{
		ID:      "test-step",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": command},
	}
}

// TestExecuteStep_BasicShell verifies that a simple echo command produces a
// passed outcome with the expected stdout value.
func TestExecuteStep_BasicShell(t *testing.T) {
	registry := newTestRegistry("")
	interpCtx := tryve.NewInterpolationContext()
	step := newShellStep("echo hello")

	outcome, err := executor.ExecuteStep(context.Background(), step, registry, interpCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome == nil {
		t.Fatal("expected non-nil outcome")
	}
	if outcome.Status != tryve.StatusPassed {
		t.Errorf("expected status passed, got %s; err: %v", outcome.Status, outcome.Error)
	}
	if outcome.Result == nil {
		t.Fatal("expected non-nil result")
	}
	stdout, ok := outcome.Result.Data["stdout"].(string)
	if !ok {
		t.Fatalf("stdout not a string: %v", outcome.Result.Data["stdout"])
	}
	if stdout != "hello" {
		t.Errorf("expected stdout %q, got %q", "hello", stdout)
	}
}

// TestExecuteStep_WithCapture verifies that a captured JSONPath value from the
// step result is stored in the interpolation context under the given variable name.
func TestExecuteStep_WithCapture(t *testing.T) {
	registry := newTestRegistry("")
	interpCtx := tryve.NewInterpolationContext()
	step := &tryve.StepDefinition{
		ID:      "capture-step",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": "echo captured_value"},
		Capture: map[string]string{
			"myVar": "$.stdout",
		},
	}

	outcome, err := executor.ExecuteStep(context.Background(), step, registry, interpCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Status != tryve.StatusPassed {
		t.Errorf("expected status passed, got %s; err: %v", outcome.Status, outcome.Error)
	}

	capturedVal, ok := interpCtx.Captured["myVar"]
	if !ok {
		t.Fatal("expected 'myVar' to be captured in interpCtx.Captured")
	}
	if capturedVal != "captured_value" {
		t.Errorf("expected captured value %q, got %q", "captured_value", capturedVal)
	}
}

// TestExecuteStep_WithDelay verifies that when step.Delay is set the step
// takes at least that many milliseconds to complete.
func TestExecuteStep_WithDelay(t *testing.T) {
	registry := newTestRegistry("")
	interpCtx := tryve.NewInterpolationContext()
	step := &tryve.StepDefinition{
		ID:      "delay-step",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": "echo ok"},
		Delay:   100, // 100ms
	}

	start := time.Now()
	outcome, err := executor.ExecuteStep(context.Background(), step, registry, interpCtx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Status != tryve.StatusPassed {
		t.Errorf("expected status passed, got %s", outcome.Status)
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected elapsed >= 100ms, got %v", elapsed)
	}
	if outcome.Duration < 100*time.Millisecond {
		t.Errorf("expected outcome.Duration >= 100ms, got %v", outcome.Duration)
	}
}

// TestExecuteStep_ContinueOnError verifies that when an assertion fails and
// continueOnError is true the outcome status is warned (not failed) and the
// step does not block further execution.
func TestExecuteStep_ContinueOnError(t *testing.T) {
	registry := newTestRegistry("")
	interpCtx := tryve.NewInterpolationContext()
	step := &tryve.StepDefinition{
		ID:      "warn-step",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": "echo hello"},
		Assert: map[string]any{
			// Assertion that will fail: stdout will be "hello", not "nope".
			"path":   "$.stdout",
			"equals": "nope",
		},
		ContinueOnError: true,
	}

	outcome, err := executor.ExecuteStep(context.Background(), step, registry, interpCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Status != tryve.StatusWarned {
		t.Errorf("expected status warned, got %s", outcome.Status)
	}
}

// TestExecuteStep_InterpolatesParams verifies that template variables in step
// params are resolved from the interpolation context before execution.
func TestExecuteStep_InterpolatesParams(t *testing.T) {
	registry := newTestRegistry("")
	interpCtx := tryve.NewInterpolationContext()
	interpCtx.Variables["greeting"] = "interpolated"

	step := &tryve.StepDefinition{
		ID:      "interp-step",
		Adapter: "shell",
		Action:  "exec",
		// The command uses a {{variable}} template that should be resolved.
		Params: map[string]any{"command": "echo {{greeting}}"},
	}

	outcome, err := executor.ExecuteStep(context.Background(), step, registry, interpCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Status != tryve.StatusPassed {
		t.Errorf("expected status passed, got %s; err: %v", outcome.Status, outcome.Error)
	}

	stdout, ok := outcome.Result.Data["stdout"].(string)
	if !ok {
		t.Fatalf("stdout not a string: %v", outcome.Result.Data["stdout"])
	}
	if stdout != "interpolated" {
		t.Errorf("expected stdout %q, got %q", "interpolated", stdout)
	}
}

// TestExecuteStep_UnknownAdapter verifies that requesting an unregistered
// adapter produces a failed outcome (not a panic or nil outcome).
func TestExecuteStep_UnknownAdapter(t *testing.T) {
	registry := newTestRegistry("")
	interpCtx := tryve.NewInterpolationContext()
	step := &tryve.StepDefinition{
		ID:      "bad-adapter",
		Adapter: "nonexistent",
		Action:  "exec",
		Params:  map[string]any{"command": "echo hi"},
	}

	outcome, err := executor.ExecuteStep(context.Background(), step, registry, interpCtx)
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if outcome.Status != tryve.StatusFailed {
		t.Errorf("expected status failed for unknown adapter, got %s", outcome.Status)
	}
}

// TestExecuteStepWithRetry_PassOnFirstAttempt verifies that a step passing on
// the first attempt returns retryCount 0 and a passed outcome.
func TestExecuteStepWithRetry_PassOnFirstAttempt(t *testing.T) {
	registry := newTestRegistry("")
	interpCtx := tryve.NewInterpolationContext()
	step := newShellStep("echo ok")

	outcome, retries := executor.ExecuteStepWithRetry(
		context.Background(), step, registry, interpCtx,
		3, 10*time.Millisecond,
	)
	if outcome.Status != tryve.StatusPassed {
		t.Errorf("expected status passed, got %s", outcome.Status)
	}
	if retries != 0 {
		t.Errorf("expected 0 retries, got %d", retries)
	}
}

// TestExecuteStepWithRetry_ContextCancelled verifies that cancelling the context
// during a retry wait causes the function to return early with a failed outcome.
func TestExecuteStepWithRetry_ContextCancelled(t *testing.T) {
	registry := newTestRegistry("")
	interpCtx := tryve.NewInterpolationContext()

	// This assertion will always fail so retries are triggered.
	step := &tryve.StepDefinition{
		ID:      "retry-fail",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": "echo hi"},
		Assert: map[string]any{
			"path":   "$.stdout",
			"equals": "impossible",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	outcome, _ := executor.ExecuteStepWithRetry(
		ctx, step, registry, interpCtx,
		10, 200*time.Millisecond, // long backoff so context cancels first
	)
	// The step must not be nil; status must be failed (assertion failure).
	if outcome == nil {
		t.Fatal("expected non-nil outcome")
	}
	if outcome.Status == tryve.StatusPassed {
		t.Error("expected non-passed status after context cancellation")
	}
}

// TestExecuteStep_NamespacedCapture verifies that steps with a Name store
// captured values in a nested map under the step name.
func TestExecuteStep_NamespacedCapture(t *testing.T) {
	registry := newTestRegistry("")
	interpCtx := tryve.NewInterpolationContext()

	step := &tryve.StepDefinition{
		ID:      "named-step",
		Name:    "my-server",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": "echo captured-value"},
		Capture: map[string]string{"output": "stdout"},
	}

	outcome, err := executor.ExecuteStep(context.Background(), step, registry, interpCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Status != tryve.StatusPassed {
		t.Fatalf("expected passed, got %s: %v", outcome.Status, outcome.Error)
	}

	nested, ok := interpCtx.Captured["my-server"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested map under 'my-server', got %T: %v",
			interpCtx.Captured["my-server"], interpCtx.Captured)
	}
	if nested["output"] != "captured-value" {
		t.Fatalf("expected captured output='captured-value', got %v", nested["output"])
	}
}

// TestExecuteStep_FlatCaptureBackwardCompat verifies that steps without a Name
// still use flat capture (backward compatibility).
func TestExecuteStep_FlatCaptureBackwardCompat(t *testing.T) {
	registry := newTestRegistry("")
	interpCtx := tryve.NewInterpolationContext()

	step := &tryve.StepDefinition{
		ID:      "flat-step",
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": "echo flat-value"},
		Capture: map[string]string{"out": "stdout"},
	}

	outcome, err := executor.ExecuteStep(context.Background(), step, registry, interpCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Status != tryve.StatusPassed {
		t.Fatalf("expected passed, got %s: %v", outcome.Status, outcome.Error)
	}

	if interpCtx.Captured["out"] != "flat-value" {
		t.Fatalf("expected captured out='flat-value', got %v", interpCtx.Captured["out"])
	}
}
