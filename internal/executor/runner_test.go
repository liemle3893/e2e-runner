package executor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/liemle3893/go-tryve/internal/executor"
	"github.com/liemle3893/go-tryve/internal/reporter"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// newNoopReporter returns a Multi reporter with no sinks, making it a no-op
// suitable for use in tests that do not need reporter output.
func newNoopReporter() *reporter.Multi {
	return reporter.NewMulti()
}

// newShellExecStep returns a shell/exec StepDefinition for the given command.
func newShellExecStep(id, command string) tryve.StepDefinition {
	return tryve.StepDefinition{
		ID:      id,
		Adapter: "shell",
		Action:  "exec",
		Params:  map[string]any{"command": command},
	}
}

// TestRunTest_SimplePass verifies that a single-step test with a shell echo
// command produces a passed result.
func TestRunTest_SimplePass(t *testing.T) {
	reg := newTestRegistry("")
	rep := newNoopReporter()

	step := newShellExecStep("step-1", "echo hello")
	td := &tryve.TestDefinition{
		Name:    "simple-pass",
		Execute: []tryve.StepDefinition{step},
	}

	result := executor.RunTest(context.Background(), td, reg, rep, 0, 0, "", nil, false, tryve.ModernCompat())

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Status != tryve.StatusPassed {
		t.Errorf("expected status passed, got %s; err: %v", result.Status, result.Error)
	}
	if len(result.Steps) != 1 {
		t.Errorf("expected 1 step outcome, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != tryve.StatusPassed {
		t.Errorf("expected step status passed, got %s", result.Steps[0].Status)
	}
}

// TestRunTest_SkippedTest verifies that a test with Skip=true is immediately
// returned as StatusSkipped without executing any steps.
func TestRunTest_SkippedTest(t *testing.T) {
	reg := newTestRegistry("")
	rep := newNoopReporter()

	td := &tryve.TestDefinition{
		Name:    "skipped-test",
		Skip:    true,
		Execute: []tryve.StepDefinition{newShellExecStep("step-1", "echo should-not-run")},
	}

	result := executor.RunTest(context.Background(), td, reg, rep, 0, 0, "", nil, false, tryve.ModernCompat())

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Status != tryve.StatusSkipped {
		t.Errorf("expected status skipped, got %s", result.Status)
	}
	if len(result.Steps) != 0 {
		t.Errorf("expected 0 step outcomes for skipped test, got %d", len(result.Steps))
	}
}

// TestRunTest_AllPhases verifies that steps in all four phases (setup, execute,
// verify, teardown) are executed and recorded when all pass.
func TestRunTest_AllPhases(t *testing.T) {
	reg := newTestRegistry("")
	rep := newNoopReporter()

	td := &tryve.TestDefinition{
		Name:     "all-phases",
		Setup:    []tryve.StepDefinition{newShellExecStep("setup-1", "echo setup")},
		Execute:  []tryve.StepDefinition{newShellExecStep("exec-1", "echo execute")},
		Verify:   []tryve.StepDefinition{newShellExecStep("verify-1", "echo verify")},
		Teardown: []tryve.StepDefinition{newShellExecStep("teardown-1", "echo teardown")},
	}

	result := executor.RunTest(context.Background(), td, reg, rep, 0, 0, "", nil, false, tryve.ModernCompat())

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Status != tryve.StatusPassed {
		t.Errorf("expected status passed, got %s; err: %v", result.Status, result.Error)
	}
	if len(result.Steps) != 4 {
		t.Errorf("expected 4 step outcomes (one per phase), got %d", len(result.Steps))
	}
	for i, s := range result.Steps {
		if s.Status != tryve.StatusPassed {
			t.Errorf("step %d: expected passed, got %s; err: %v", i, s.Status, s.Error)
		}
	}
}

// TestRunTest_WithTimeout verifies that a test with a short timeout fails when
// a step's pre-delay exceeds the deadline, triggering context cancellation.
// We use step.Delay (handled in ExecuteStep's select) rather than a shell sleep
// so the cancellation is detected reliably within the executor itself.
func TestRunTest_WithTimeout(t *testing.T) {
	reg := newTestRegistry("")
	rep := newNoopReporter()

	// Timeout of 100ms but the step has a 5-second pre-delay: the context will
	// expire and ExecuteStep will surface a failed outcome via ctx.Done().
	td := &tryve.TestDefinition{
		Name:    "timeout-test",
		Timeout: 100, // 100ms
		Execute: []tryve.StepDefinition{
			{
				ID:      "delayed-step",
				Adapter: "shell",
				Action:  "exec",
				Params:  map[string]any{"command": "echo ok"},
				Delay:   5000, // 5s pre-delay — will be cancelled by the 100ms timeout
			},
		},
	}

	result := executor.RunTest(context.Background(), td, reg, rep, 0, 0, "", nil, false, tryve.ModernCompat())

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Status != tryve.StatusFailed {
		t.Errorf("expected status failed due to timeout, got %s", result.Status)
	}
}

// TestRunHook verifies that RunHook successfully executes a valid shell script.
func TestRunHook(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "hook.sh")

	// Write a minimal shell script that exits 0.
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("failed to write hook script: %v", err)
	}

	err := executor.RunHook(context.Background(), scriptPath, dir, nil)
	if err != nil {
		t.Errorf("expected no error from RunHook, got: %v", err)
	}
}

// TestRunHook_Failure verifies that RunHook returns an error when the command
// exits with a non-zero exit code.
func TestRunHook_Failure(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fail.sh")

	// Write a script that always exits 1.
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatalf("failed to write hook script: %v", err)
	}

	err := executor.RunHook(context.Background(), scriptPath, dir, nil)
	if err == nil {
		t.Error("expected error from RunHook for non-zero exit, got nil")
	}
}

// TestRunHook_EmptyCommand verifies that RunHook is a no-op when given an empty
// command string.
func TestRunHook_EmptyCommand(t *testing.T) {
	err := executor.RunHook(context.Background(), "", "", nil)
	if err != nil {
		t.Errorf("expected no error for empty command, got: %v", err)
	}
}

// TestRunTest_TeardownAlwaysRuns verifies that the teardown phase still runs
// even when an earlier phase fails.
func TestRunTest_TeardownAlwaysRuns(t *testing.T) {
	reg := newTestRegistry("")
	rep := newNoopReporter()

	td := &tryve.TestDefinition{
		Name: "teardown-always",
		Execute: []tryve.StepDefinition{
			// This step will fail (exit 1) but does not use continueOnError.
			{
				ID:      "fail-step",
				Adapter: "shell",
				Action:  "exec",
				Params:  map[string]any{"command": "exit 1"},
				Assert: map[string]any{
					"path":   "$.exitCode",
					"equals": float64(0),
				},
			},
		},
		Teardown: []tryve.StepDefinition{newShellExecStep("teardown-1", "echo teardown")},
	}

	result := executor.RunTest(context.Background(), td, reg, rep, 0, 0, "", nil, false, tryve.ModernCompat())

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Status != tryve.StatusFailed {
		t.Errorf("expected status failed due to execute phase failure, got %s", result.Status)
	}

	// Teardown step must still appear in the results.
	var teardownFound bool
	for _, s := range result.Steps {
		if s.Phase == tryve.PhaseTeardown {
			teardownFound = true
		}
	}
	if !teardownFound {
		t.Error("expected teardown step to run even after execute phase failure")
	}
}

// TestRunTest_Duration verifies that the result Duration is positive after a run.
func TestRunTest_Duration(t *testing.T) {
	reg := newTestRegistry("")
	rep := newNoopReporter()

	td := &tryve.TestDefinition{
		Name:    "duration-test",
		Execute: []tryve.StepDefinition{newShellExecStep("step-1", "echo ok")},
	}

	start := time.Now()
	result := executor.RunTest(context.Background(), td, reg, rep, 0, 0, "", nil, false, tryve.ModernCompat())
	elapsed := time.Since(start)

	if result.Duration <= 0 {
		t.Error("expected positive Duration in test result")
	}
	if result.Duration > elapsed+50*time.Millisecond {
		t.Errorf("result.Duration (%v) exceeds wall-clock elapsed (%v)", result.Duration, elapsed)
	}
}
