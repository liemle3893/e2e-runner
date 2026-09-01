package assertion_test

import (
	"testing"

	"github.com/liemle3893/go-tryve/internal/assertion"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// run evaluates an assertion definition and returns the outcomes.
func run(t *testing.T, data map[string]any, def any) []tryve.AssertionOutcome {
	t.Helper()
	return runMode(t, data, def, tryve.ModernCompat())
}

// runMode evaluates an assertion definition under a specific compatibility mode.
func runMode(t *testing.T, data map[string]any, def any, mode tryve.CompatMode) []tryve.AssertionOutcome {
	t.Helper()
	outcomes, err := assertion.RunAssertions(data, def, mode)
	if err != nil {
		t.Fatalf("RunAssertions returned an error: %v", err)
	}
	return outcomes
}

// assertOne requires exactly one outcome and returns it.
func assertOne(t *testing.T, outcomes []tryve.AssertionOutcome) tryve.AssertionOutcome {
	t.Helper()
	if len(outcomes) != 1 {
		t.Fatalf("expected exactly 1 outcome, got %d: %+v", len(outcomes), outcomes)
	}
	return outcomes[0]
}

// TestExitCodeIsEvaluated covers the shape used by shell steps. It previously
// matched no handler, so the assertion was dropped and the step passed no matter
// what the command returned.
func TestExitCodeIsEvaluated(t *testing.T) {
	data := map[string]any{"exitCode": float64(3), "stdout": "NOPE", "stderr": ""}

	got := assertOne(t, run(t, data, map[string]any{"exitCode": 0}))
	if got.Passed {
		t.Errorf("exitCode 3 must not satisfy `exitCode: 0`")
	}

	got = assertOne(t, run(t, map[string]any{"exitCode": float64(0)}, map[string]any{"exitCode": 0}))
	if !got.Passed {
		t.Errorf("exitCode 0 must satisfy `exitCode: 0`, got: %s", got.Message)
	}
}

// TestFieldOperatorMap covers `stdout: {contains: …}` and the other
// field-with-operators shapes.
func TestFieldOperatorMap(t *testing.T) {
	data := map[string]any{"stdout": "BOTH_OK", "exitCode": float64(0)}

	outcomes := run(t, data, map[string]any{
		"stdout": map[string]any{"contains": "BOTH"},
	})
	if got := assertOne(t, outcomes); !got.Passed {
		t.Errorf("expected stdout contains BOTH to pass, got: %s", got.Message)
	}

	outcomes = run(t, data, map[string]any{
		"stdout": map[string]any{"contains": "ABSENT"},
	})
	if got := assertOne(t, outcomes); got.Passed {
		t.Errorf("expected stdout contains ABSENT to fail")
	}
}

// TestRowColumnAssertions covers the SQL result shape `{row, column, operator}`,
// against both the multi-row "query" result and the flat "queryOne" result.
func TestRowColumnAssertions(t *testing.T) {
	queryData := map[string]any{
		"rows": []any{
			map[string]any{"reward_key": "only_reward", "total_grants": int64(2)},
			map[string]any{"reward_key": "second", "total_grants": int64(5)},
		},
		"rowCount": float64(2),
	}

	got := assertOne(t, run(t, queryData, []any{
		map[string]any{"row": 0, "column": "reward_key", "equals": "only_reward"},
	}))
	if !got.Passed {
		t.Errorf("row 0 reward_key should equal only_reward, got: %s", got.Message)
	}

	got = assertOne(t, run(t, queryData, []any{
		map[string]any{"row": 1, "column": "total_grants", "gte": 5},
	}))
	if !got.Passed {
		t.Errorf("row 1 total_grants should satisfy gte 5, got: %s", got.Message)
	}

	// queryOne places the columns at the top level.
	oneData := map[string]any{"reward_key": "only_reward", "found": true}
	got = assertOne(t, run(t, oneData, []any{
		map[string]any{"column": "reward_key", "exists": true},
	}))
	if !got.Passed {
		t.Errorf("column lookup against a single-row result should pass, got: %s", got.Message)
	}
}

// TestRowColumnMissingFails checks that addressing a column or row that is not
// there fails with a message naming what was available.
func TestRowColumnMissingFails(t *testing.T) {
	data := map[string]any{"rows": []any{map[string]any{"a": 1}}, "rowCount": float64(1)}

	got := assertOne(t, run(t, data, []any{
		map[string]any{"row": 0, "column": "nope", "equals": 1},
	}))
	if got.Passed {
		t.Fatalf("a missing column must fail")
	}
	if got.Message == "" {
		t.Errorf("a missing column should explain itself")
	}

	got = assertOne(t, run(t, data, []any{
		map[string]any{"row": 7, "column": "a", "equals": 1},
	}))
	if got.Passed {
		t.Errorf("an out-of-range row must fail")
	}
}

// TestUnknownOperatorFails ensures a misspelled operator produces a red outcome
// rather than being dropped.
func TestUnknownOperatorFails(t *testing.T) {
	got := assertOne(t, run(t, map[string]any{"a": 1}, []any{
		map[string]any{"path": "$.a", "equalz": 1},
	}))
	if got.Passed {
		t.Errorf("an unknown operator must not pass silently")
	}
}

// TestOperatorAliases covers the shorthand spellings that appear in real suites.
func TestOperatorAliases(t *testing.T) {
	data := map[string]any{"rowCount": float64(3), "label": "GOLD", "keys": []any{"a", "b", "c"}}

	cases := []struct {
		name string
		def  any
		want bool
	}{
		{"gte", map[string]any{"rowCount": map[string]any{"gte": 1}}, true},
		{"lte fails", map[string]any{"rowCount": map[string]any{"lte": 1}}, false},
		{"in", []any{map[string]any{"path": "$.label", "in": []any{"GOLD", "SILVER"}}}, true},
		{"in fails", []any{map[string]any{"path": "$.label", "in": []any{"BRONZE"}}}, false},
		{"minLength", []any{map[string]any{"path": "$.keys", "minLength": 2}}, true},
		{"minLength fails", []any{map[string]any{"path": "$.keys", "minLength": 9}}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := assertOne(t, run(t, data, tc.def))
			if got.Passed != tc.want {
				t.Errorf("expected pass=%v, got pass=%v (%s)", tc.want, got.Passed, got.Message)
			}
		})
	}
}

// TestHTTPShapeUnchanged guards the existing HTTP assertion handling against
// regression from the field-name fallback.
func TestHTTPShapeUnchanged(t *testing.T) {
	data := map[string]any{
		"status":  float64(200),
		"headers": map[string]any{"Content-Type": "application/json"},
		"body":    map[string]any{"data": map[string]any{"id": "abc"}},
	}

	outcomes := run(t, data, map[string]any{
		"status":  200,
		"headers": map[string]any{"content-type": "application/json"},
		"json":    []any{map[string]any{"path": "$.data.id", "equals": "abc"}},
	})
	if len(outcomes) != 3 {
		t.Fatalf("expected 3 outcomes, got %d: %+v", len(outcomes), outcomes)
	}
	for _, o := range outcomes {
		if !o.Passed {
			t.Errorf("%s %s failed: %s", o.Path, o.Operator, o.Message)
		}
	}
}

// TestLegacyAssertionsDropUnknownForms pins the pre-v2 behaviour: forms the old
// dispatcher did not recognise produced no outcomes at all. A suite that has not
// opted in must keep passing exactly as it did, however wrong that is.
func TestLegacyAssertionsDropUnknownForms(t *testing.T) {
	legacy := tryve.LegacyCompat()

	cases := []struct {
		name string
		data map[string]any
		def  any
	}{
		{
			name: "exitCode field",
			data: map[string]any{"exitCode": float64(3)},
			def:  map[string]any{"exitCode": 0},
		},
		{
			name: "stdout field",
			data: map[string]any{"stdout": "NOPE"},
			def:  map[string]any{"stdout": map[string]any{"contains": "ABSENT"}},
		},
		{
			name: "row/column",
			data: map[string]any{"rows": []any{map[string]any{"a": 1}}},
			def:  []any{map[string]any{"row": 0, "column": "a", "equals": 999}},
		},
		{
			name: "unknown operator",
			data: map[string]any{"a": 1},
			def:  []any{map[string]any{"path": "$.a", "equalz": 999}},
		},
		{
			name: "alias operator",
			data: map[string]any{"n": float64(1)},
			def:  []any{map[string]any{"path": "$.n", "gte": 999}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runMode(t, tc.data, tc.def, legacy); len(got) != 0 {
				t.Errorf("legacy mode should evaluate nothing here, got %+v", got)
			}
			// The same definition is evaluated once the area is opted in.
			if got := runMode(t, tc.data, tc.def, tryve.ModernCompat()); len(got) == 0 {
				t.Errorf("modern mode should evaluate this assertion")
			}
		})
	}
}

// TestLegacyAssertionsKeepWorkingForms pins the forms that were always
// evaluated, so opting out of the area does not disable real checks.
func TestLegacyAssertionsKeepWorkingForms(t *testing.T) {
	legacy := tryve.LegacyCompat()

	data := map[string]any{
		"status":  float64(200),
		"headers": map[string]any{"Content-Type": "application/json"},
		"body":    map[string]any{"data": map[string]any{"id": "abc"}},
	}
	def := map[string]any{
		"status":  200,
		"headers": map[string]any{"content-type": "application/json"},
		"json":    []any{map[string]any{"path": "$.data.id", "equals": "abc"}},
	}

	outcomes := runMode(t, data, def, legacy)
	if len(outcomes) != 3 {
		t.Fatalf("expected 3 outcomes, got %d: %+v", len(outcomes), outcomes)
	}
	for _, o := range outcomes {
		if !o.Passed {
			t.Errorf("%s %s failed: %s", o.Path, o.Operator, o.Message)
		}
	}
}

// TestLegacyArrayBodyRoot pins that "$" addressed the whole result, not the
// array, when a response body was a JSON array.
func TestLegacyArrayBodyRoot(t *testing.T) {
	data := map[string]any{
		"status": float64(200),
		"body":   []any{map[string]any{"id": "one"}},
	}
	def := map[string]any{"json": []any{map[string]any{"path": "$.status", "equals": 200}}}

	if got := assertOne(t, runMode(t, data, def, tryve.LegacyCompat())); !got.Passed {
		t.Errorf("legacy: $ should address the result, so $.status resolves: %s", got.Message)
	}
	if got := assertOne(t, runMode(t, data, def, tryve.ModernCompat())); got.Passed {
		t.Errorf("modern: $ should address the array, so $.status must not resolve")
	}
}
