package reporter_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/liemle3893/go-tryve/internal/reporter"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// TestConsoleReporter_SuiteComplete verifies that the summary line contains
// the correct pass and fail counts.
func TestConsoleReporter_SuiteComplete(t *testing.T) {
	var buf bytes.Buffer
	c := reporter.NewConsole(&buf, false, false)

	result := &tryve.SuiteResult{
		Passed:   2,
		Failed:   1,
		Skipped:  0,
		Total:    3,
		Duration: 500 * time.Millisecond,
	}

	if err := c.OnSuiteComplete(context.Background(), &tryve.SuiteResult{}, result); err != nil {
		t.Fatalf("OnSuiteComplete returned unexpected error: %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, "2 passed") {
		t.Errorf("expected output to contain %q, got: %q", "2 passed", out)
	}
	if !strings.Contains(out, "1 failed") {
		t.Errorf("expected output to contain %q, got: %q", "1 failed", out)
	}
}

// TestConsoleReporter_TestComplete verifies that the per-test line contains
// the test name and a PASS label.
func TestConsoleReporter_TestComplete(t *testing.T) {
	var buf bytes.Buffer
	c := reporter.NewConsole(&buf, false, false)

	test := &tryve.TestDefinition{Name: "login-flow"}
	result := &tryve.TestResult{
		Test:     test,
		Status:   tryve.StatusPassed,
		Duration: 123 * time.Millisecond,
	}

	if err := c.OnTestComplete(context.Background(), test, result); err != nil {
		t.Fatalf("OnTestComplete returned unexpected error: %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, "login-flow") {
		t.Errorf("expected output to contain test name %q, got: %q", "login-flow", out)
	}
	if !strings.Contains(out, "PASS") {
		t.Errorf("expected output to contain %q, got: %q", "PASS", out)
	}
}

// TestMultiReporter verifies that all contained reporters receive events.
func TestMultiReporter(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	c1 := reporter.NewConsole(&buf1, false, false)
	c2 := reporter.NewConsole(&buf2, false, false)
	m := reporter.NewMulti(c1, c2)

	suite := &tryve.SuiteResult{}
	if err := m.OnSuiteStart(context.Background(), suite); err != nil {
		t.Fatalf("OnSuiteStart returned unexpected error: %v", err)
	}

	if buf1.Len() == 0 {
		t.Error("expected buf1 to be non-empty after OnSuiteStart")
	}
	if buf2.Len() == 0 {
		t.Error("expected buf2 to be non-empty after OnSuiteStart")
	}
}
