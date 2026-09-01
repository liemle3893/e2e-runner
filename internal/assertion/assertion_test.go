package assertion_test

import (
	"testing"

	"github.com/liemle3893/go-tryve/internal/assertion"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// TestRunAssertions_HTTPStatus verifies that a single numeric status assertion passes
// when the actual status matches exactly.
func TestRunAssertions_HTTPStatus(t *testing.T) {
	data := map[string]any{
		"status": 200,
		"body":   map[string]any{},
	}
	assertDef := map[string]any{
		"status": 200,
	}

	outcomes, err := assertion.RunAssertions(data, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if !outcomes[0].Passed {
		t.Errorf("expected assertion to pass, got message: %s", outcomes[0].Message)
	}
}

// TestRunAssertions_HTTPStatusMismatch verifies that a status mismatch produces a failing assertion.
func TestRunAssertions_FailingAssertion(t *testing.T) {
	data := map[string]any{
		"status": 404,
	}
	assertDef := map[string]any{
		"status": 200,
	}

	outcomes, err := assertion.RunAssertions(data, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].Passed {
		t.Error("expected assertion to fail for status 404 vs expected 200")
	}
}

// TestRunAssertions_HTTPStatusArray verifies that a status assertion passes when the actual
// status is one of the values in an expected array.
func TestRunAssertions_HTTPStatusArray(t *testing.T) {
	data := map[string]any{"status": 201}
	assertDef := map[string]any{
		"status": []any{200, 201, 202},
	}

	outcomes, err := assertion.RunAssertions(data, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if !outcomes[0].Passed {
		t.Errorf("expected status 201 to be in [200,201,202], message: %s", outcomes[0].Message)
	}
}

// TestRunAssertions_HTTPStatusArrayFail verifies that the oneOf check fails when actual is not in the array.
func TestRunAssertions_HTTPStatusArrayFail(t *testing.T) {
	data := map[string]any{"status": 500}
	assertDef := map[string]any{
		"status": []any{200, 201, 202},
	}

	outcomes, err := assertion.RunAssertions(data, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if outcomes[0].Passed {
		t.Error("expected status 500 NOT to be in [200,201,202]")
	}
}

// TestRunAssertions_StatusRange verifies that a statusRange [min,max] check passes for
// a status within the range and fails outside.
func TestRunAssertions_StatusRange(t *testing.T) {
	inRange := map[string]any{"status": 201}
	outRange := map[string]any{"status": 404}

	assertDef := map[string]any{
		"statusRange": []any{200, 299},
	}

	outcomes, err := assertion.RunAssertions(inRange, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if !outcomes[0].Passed {
		t.Errorf("expected 201 within [200,299] to pass: %s", outcomes[0].Message)
	}

	outcomes2, err := assertion.RunAssertions(outRange, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if outcomes2[0].Passed {
		t.Error("expected 404 outside [200,299] to fail")
	}
}

// TestRunAssertions_JSONPathAssertions verifies the json array format with path+operator pairs.
func TestRunAssertions_JSONPathAssertions(t *testing.T) {
	data := map[string]any{
		"status": 200,
		"body": map[string]any{
			"user": map[string]any{
				"id":   42,
				"name": "Alice",
			},
			"count": 5,
		},
	}
	assertDef := map[string]any{
		"json": []any{
			map[string]any{"path": "$.user.name", "equals": "Alice"},
			map[string]any{"path": "$.user.id", "equals": 42},
			map[string]any{"path": "$.count", "greaterThan": 0},
		},
	}

	outcomes, err := assertion.RunAssertions(data, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if len(outcomes) != 3 {
		t.Fatalf("expected 3 outcomes, got %d", len(outcomes))
	}
	for i, o := range outcomes {
		if !o.Passed {
			t.Errorf("outcome[%d] failed: %s", i, o.Message)
		}
	}
}

// TestRunAssertions_Headers verifies that header assertions match case-insensitively.
func TestRunAssertions_Headers(t *testing.T) {
	data := map[string]any{
		"status": 200,
		"headers": map[string]any{
			"Content-Type": "application/json",
			"X-Request-Id": "abc123",
		},
	}
	assertDef := map[string]any{
		"headers": map[string]any{
			"content-type": "application/json",
		},
	}

	outcomes, err := assertion.RunAssertions(data, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if !outcomes[0].Passed {
		t.Errorf("expected header assertion to pass (case-insensitive): %s", outcomes[0].Message)
	}
}

// TestRunAssertions_Body verifies the body assertion map with contains/matches/equals sub-keys.
func TestRunAssertions_Body(t *testing.T) {
	data := map[string]any{
		"body": "Hello, World!",
	}
	assertDef := map[string]any{
		"body": map[string]any{
			"contains": "World",
		},
	}

	outcomes, err := assertion.RunAssertions(data, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if !outcomes[0].Passed {
		t.Errorf("expected body contains assertion to pass: %s", outcomes[0].Message)
	}
}

// TestRunAssertions_Duration verifies duration lessThan and greaterThan assertions.
func TestRunAssertions_Duration(t *testing.T) {
	data := map[string]any{
		"duration": float64(150),
	}
	assertDef := map[string]any{
		"duration": map[string]any{
			"lessThan":    float64(500),
			"greaterThan": float64(50),
		},
	}

	outcomes, err := assertion.RunAssertions(data, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 outcomes (lessThan+greaterThan), got %d", len(outcomes))
	}
	for i, o := range outcomes {
		if !o.Passed {
			t.Errorf("outcome[%d] failed: %s", i, o.Message)
		}
	}
}

// TestRunAssertions_SliceFormat verifies the generic []any assertion format used by non-HTTP adapters.
func TestRunAssertions_SliceFormat(t *testing.T) {
	data := map[string]any{
		"response": map[string]any{
			"id":     99,
			"active": true,
		},
	}
	assertDef := []any{
		map[string]any{"path": "$.response.id", "equals": 99},
		map[string]any{"path": "$.response.active", "equals": true},
	}

	outcomes, err := assertion.RunAssertions(data, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(outcomes))
	}
	for i, o := range outcomes {
		if !o.Passed {
			t.Errorf("outcome[%d] failed: %s", i, o.Message)
		}
	}
}

// TestRunAssertions_DirectOperator verifies that a top-level path+operator map (non-HTTP style)
// is handled for adapters that embed assertions directly.
func TestRunAssertions_DirectOperator(t *testing.T) {
	data := map[string]any{
		"value": "hello",
	}
	assertDef := map[string]any{
		"path":     "$.value",
		"contains": "ell",
	}

	outcomes, err := assertion.RunAssertions(data, assertDef, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions returned error: %v", err)
	}
	if len(outcomes) == 0 {
		t.Fatal("expected at least 1 outcome for direct operator format")
	}
	if !outcomes[0].Passed {
		t.Errorf("expected direct operator assertion to pass: %s", outcomes[0].Message)
	}
}

// TestRunAssertions_NilAssertDef verifies that nil assertDef returns empty outcomes without error.
func TestRunAssertions_NilAssertDef(t *testing.T) {
	data := map[string]any{"status": 200}
	outcomes, err := assertion.RunAssertions(data, nil, tryve.ModernCompat())
	if err != nil {
		t.Fatalf("RunAssertions with nil assertDef returned error: %v", err)
	}
	if len(outcomes) != 0 {
		t.Errorf("expected 0 outcomes for nil assertDef, got %d", len(outcomes))
	}
}
